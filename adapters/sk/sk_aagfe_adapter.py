"""
sk_caas_adapter.py
==================
AAGFE v2 — Semantic Kernel Python adapter.
Wires SK's three filter interfaces to SKReinforcementService (port 50067).

SK Python filter interfaces:
  - FunctionInvocationFilter  (pre + post, single method)
  - AutoFunctionInvocationFilter (per planner iteration — NEW CGL concept)
  - PromptRenderFilter (pre + post prompt assembly)

Usage:
    from sk_caas_adapter import AAGFESKAdapter, AAGFESKConfig

    config = AAGFESKConfig(
        endpoint="localhost:50067",
        agent_entity_id="agent:my-agent-001",
    )
    adapter = AAGFESKAdapter(config)

    kernel = Kernel()
    kernel.add_service(AzureChatCompletion(...))
    kernel.add_filter(FilterTypes.FUNCTION_INVOCATION, adapter.function_filter)
    kernel.add_filter(FilterTypes.AUTO_FUNCTION_INVOCATION, adapter.auto_function_filter)
    kernel.add_filter(FilterTypes.PROMPT_RENDER, adapter.prompt_render_filter)

    task_id = await adapter.register_task(
        kernel=kernel,
        declared_goal="Analyse Q3 revenue data",
        permitted_plugins=["SqlPlugin", "FilePlugin"],
    )
"""

from __future__ import annotations

import asyncio
import hashlib
import json
import logging
import time
from dataclasses import dataclass, field
from typing import Any, Callable, Optional

# SK Python imports
# from semantic_kernel.filters.functions.function_invocation_context import FunctionInvocationContext
# from semantic_kernel.filters.auto_function_invocation.auto_function_invocation_context import (
#     AutoFunctionInvocationContext,
# )
# from semantic_kernel.filters.prompts.prompt_render_context import PromptRenderContext

import grpc

logger = logging.getLogger("aagfe.sk_adapter")


# ─── Config ──────────────────────────────────────────────────────────────────

@dataclass
class AAGFESKConfig:
    endpoint: str                       # e.g. "localhost:50067"
    agent_entity_id: str
    runtime: str = "PYTHON"             # SK_RUNTIME_PYTHON
    tls: bool = False
    warn_threshold: int = 400
    strong_warn_threshold: int = 600
    block_threshold: int = 800
    semantic_guardrail_enabled: bool = True
    injection_interval_turns: int = 10


# ─── Exceptions ───────────────────────────────────────────────────────────────

class AAGFEFunctionBlockedError(Exception):
    def __init__(self, reason: str, drift_score: int, review_token: str = ""):
        self.reason = reason
        self.drift_score = drift_score
        self.review_token = review_token
        super().__init__(f"AAGFE BLOCK: {reason} (drift={drift_score})")

class AAGFEPlannerTerminatedError(Exception):
    def __init__(self, reason: str):
        super().__init__(f"AAGFE PLANNER TERMINATE: {reason}")

class AAGFEAgentMessageBlockedError(Exception):
    def __init__(self, reason: str):
        super().__init__(f"AAGFE A2A BLOCK: {reason}")


# ─── Adapter ─────────────────────────────────────────────────────────────────

class AAGFESKAdapter:
    """
    Semantic Kernel AAGFE reinforcement adapter.
    One instance per kernel / agent session.
    """

    def __init__(self, config: AAGFESKConfig):
        self.config = config
        self._channel: Optional[grpc.aio.Channel] = None
        self._stub = None
        self._task_id: Optional[str] = None
        self._session_id: Optional[str] = None
        self._kernel_id: Optional[str] = None
        self._turn: int = 0
        self._iteration: int = 0
        self._pending_fragment: Optional[str] = None

    # ── Filter properties (pass these to kernel.add_filter) ──────────────────

    @property
    def function_filter(self) -> "_FunctionInvocationFilter":
        return _FunctionInvocationFilter(self)

    @property
    def auto_function_filter(self) -> "_AutoFunctionInvocationFilter":
        return _AutoFunctionInvocationFilter(self)

    @property
    def prompt_render_filter(self) -> "_PromptRenderFilter":
        return _PromptRenderFilter(self)

    # ── Connection ────────────────────────────────────────────────────────────

    async def connect(self) -> None:
        if self.config.tls:
            creds = grpc.ssl_channel_credentials()
            self._channel = grpc.aio.secure_channel(self.config.endpoint, creds)
        else:
            self._channel = grpc.aio.insecure_channel(self.config.endpoint)
        logger.info("AAGFE SK adapter connected to %s", self.config.endpoint)

    async def close(self) -> None:
        if self._channel:
            await self._channel.close()

    # ── Task lifecycle ────────────────────────────────────────────────────────

    async def register_task(
        self,
        kernel: Any,
        declared_goal: str,
        permitted_plugins: list[str] | None = None,
        permitted_functions: list[str] | None = None,
        permitted_resources: list[str] | None = None,
        max_turns: int = 0,
        max_iterations: int = 100,
        parent_task_id: str = "",
        session_id: str | None = None,
    ) -> str:
        import uuid
        self._session_id = session_id or str(uuid.uuid4())
        self._kernel_id  = str(id(kernel))
        self._turn       = 0
        self._iteration  = 0

        req = {
            "agent_entity_id":      self.config.agent_entity_id,
            "session_id":           self._session_id,
            "kernel_id":            self._kernel_id,
            "declared_goal":        declared_goal,
            "permitted_plugins":    permitted_plugins or [],
            "permitted_functions":  permitted_functions or [],
            "permitted_resources":  permitted_resources or [],
            "max_turns":            max_turns,
            "max_iterations":       max_iterations,
            "parent_task_id":       parent_task_id,
            "runtime":              self.config.runtime,
        }

        resp = _stub_register_task(req)
        self._task_id = resp["task_id"]
        if resp.get("initial_constraint_fragment"):
            self._pending_fragment = resp["initial_constraint_fragment"]

        logger.info("AAGFE SK task registered: %s", self._task_id)
        return self._task_id

    async def close_task(self, reason: str = "COMPLETED", summary: str = "") -> dict:
        if not self._task_id:
            return {}

        close_resp = _stub_close_task({
            "task_id": self._task_id, "session_id": self._session_id,
            "kernel_id": self._kernel_id, "reason": reason, "summary": summary,
        })
        purge_resp = _stub_purge_memory({
            "task_id": self._task_id, "session_id": self._session_id,
            "kernel_id": self._kernel_id, "dry_run": False,
        })

        logger.info(
            "AAGFE SK task closed: %s | drift=%d | purged=%d keys",
            self._task_id, close_resp.get("final_drift_score", 0), purge_resp.get("keys_purged", 0),
        )
        self._task_id = None
        return {**close_resp, "keys_purged": purge_resp.get("keys_purged", 0)}

    # ── Memory ────────────────────────────────────────────────────────────────

    async def register_memory_key(
        self,
        key: str,
        collection: str = "",
        store: str = "IN_MEMORY",
        high_sensitivity: bool = False,
    ) -> None:
        """Fire-and-forget — never blocks SK execution."""
        asyncio.create_task(self._register_memory_key_async(key, collection, store, high_sensitivity))

    async def _register_memory_key_async(self, key, collection, store, high_sensitivity) -> None:
        try:
            resp = _stub_register_memory_key({
                "task_id": self._task_id, "session_id": self._session_id,
                "kernel_id": self._kernel_id, "collection": collection,
                "key": key, "store": store,
                "sensitivity": "HIGH" if high_sensitivity else "NORMAL",
            })
            if resp.get("bleed_risk"):
                logger.warning("AAGFE memory bleed risk: key=%s origin=%s", key, resp.get("origin_task_id"))
        except Exception as e:
            logger.warning("AAGFE RegisterMemoryKey failed (non-blocking): %s", e)

    # ── A2A ───────────────────────────────────────────────────────────────────

    async def check_agent_message(
        self,
        receiver_agent_entity_id: str,
        channel_name: str,
        message_content: str,
        message_role: str = "assistant",
        cross_sovereign: bool = False,
    ) -> None:
        resp = _stub_agent_message_check({
            "sender_agent_entity_id":   self.config.agent_entity_id,
            "receiver_agent_entity_id": receiver_agent_entity_id,
            "sender_task_id":           self._task_id,
            "channel_name":             channel_name,
            "message_content":          message_content,
            "message_role":             message_role,
            "cross_sovereign":          cross_sovereign,
            "session_id":               self._session_id,
        })
        if resp.get("verdict") == "BLOCK":
            raise AAGFEAgentMessageBlockedError(resp.get("reason", ""))
        if resp.get("lateral_risk_score", 0) > 0.7:
            logger.warning("AAGFE A2A high lateral risk: receiver=%s score=%.2f",
                           receiver_agent_entity_id, resp["lateral_risk_score"])

    # ── Internal ──────────────────────────────────────────────────────────────

    async def _handle_function_pre(self, function_name: str, plugin_name: str, arguments: dict) -> dict:
        self._turn += 1
        t0 = time.monotonic()
        resp = _stub_function_filter({
            "agent_entity_id": self.config.agent_entity_id,
            "task_id": self._task_id,
            "turn_number": self._turn,
            "iteration_number": self._iteration,
            "phase": "PRE",
            "function_name": function_name,
            "plugin_name": plugin_name,
            "arguments_json": json.dumps(arguments),
            "session_id": self._session_id,
            "runtime": self.config.runtime,
        })
        latency_ms = int((time.monotonic() - t0) * 1000)
        logger.debug("AAGFE FunctionFilter PRE: fn=%s verdict=%s drift=%d latency=%dms",
                     function_name, resp.get("verdict"), resp.get("drift_score", 0), latency_ms)

        verdict = resp.get("verdict", "ALLOW")
        if verdict == "BLOCK":
            logger.error("AAGFE BLOCK: fn=%s reason=%s drift=%d",
                         function_name, resp.get("reason"), resp.get("drift_score", 0))
            raise AAGFEFunctionBlockedError(
                reason=resp.get("reason", ""),
                drift_score=resp.get("drift_score", 0),
                review_token=resp.get("review_token", ""),
            )
        if verdict in ("WARN", "STRONG_WARN"):
            logger.warning("AAGFE %s: fn=%s drift=%d", verdict, function_name, resp.get("drift_score", 0))
            if resp.get("remediation") == "INJECT_NEXT_TURN":
                await self._fetch_and_stash_injection()
        return resp

    async def _handle_function_post(self, function_name: str, result: Any, duration_ms: int) -> None:
        asyncio.create_task(self._function_post_async(function_name, result, duration_ms))

    async def _function_post_async(self, function_name: str, result: Any, duration_ms: int) -> None:
        try:
            _stub_function_filter({
                "agent_entity_id": self.config.agent_entity_id,
                "task_id": self._task_id, "turn_number": self._turn,
                "iteration_number": self._iteration, "phase": "POST",
                "function_name": function_name, "plugin_name": "",
                "result_json": str(result), "was_cancelled": False,
                "duration_ms": duration_ms, "session_id": self._session_id,
            })
        except Exception as e:
            logger.warning("AAGFE FunctionFilter POST failed (non-blocking): %s", e)

    async def _handle_auto_function_iteration(self, chat_history: list[dict], function_name: str,
                                               plugin_name: str, function_count: int) -> dict:
        """
        Called on each planner iteration BEFORE the LLM selects a tool.
        This is the IAutoFunctionInvocationFilter equivalent —
        the novel CGL reasoning-visibility hook.
        """
        self._iteration += 1
        resp = _stub_auto_function_filter({
            "agent_entity_id":    self.config.agent_entity_id,
            "task_id":            self._task_id,
            "turn_number":        self._turn,
            "iteration_number":   self._iteration,
            "chat_history":       chat_history,
            "requested_function": function_name,
            "requested_plugin":   plugin_name,
            "function_count":     function_count,
            "session_id":         self._session_id,
            "runtime":            self.config.runtime,
        })

        signals = resp.get("signals", [])
        logger.debug("AAGFE AutoFunctionFilter iter=%d verdict=%s drift=%d signals=%s",
                     self._iteration, resp.get("verdict"), resp.get("drift_score", 0),
                     [s.get("type") for s in signals])

        if resp.get("terminate_planner"):
            logger.error("AAGFE TERMINATE PLANNER iter=%d reason=%s",
                         self._iteration, resp.get("reason"))
            raise AAGFEPlannerTerminatedError(resp.get("reason", ""))

        if resp.get("inject_before_next") and resp.get("constraint_fragment"):
            self._pending_fragment = resp["constraint_fragment"]

        return resp

    async def _handle_prompt_render(self, prompt: str, phase: str) -> dict:
        resp = _stub_prompt_render_filter({
            "agent_entity_id": self.config.agent_entity_id,
            "task_id": self._task_id, "turn_number": self._turn,
            "phase": phase, "prompt_template": prompt if phase == "PRE" else "",
            "prompt_hash": _hash8(prompt), "rendered_prompt": prompt if phase == "POST" else "",
            "session_id": self._session_id,
        })
        if resp.get("injection_required") and resp.get("constraint_fragment"):
            self._pending_fragment = resp["constraint_fragment"]
        return resp

    async def _fetch_and_stash_injection(self) -> None:
        try:
            resp = _stub_prompt_render_filter({
                "agent_entity_id": self.config.agent_entity_id,
                "task_id": self._task_id, "turn_number": self._turn,
                "phase": "PRE", "prompt_template": "", "prompt_hash": "", "session_id": self._session_id,
            })
            if resp.get("injection_required"):
                self._pending_fragment = resp.get("constraint_fragment")
        except Exception as e:
            logger.warning("AAGFE injection fetch failed: %s", e)


# ─── Filter implementations ───────────────────────────────────────────────────

class _FunctionInvocationFilter:
    def __init__(self, adapter: AAGFESKAdapter):
        self._adapter = adapter

    async def on_function_invocation(self, context: Any, next: Callable) -> None:
        sw = time.monotonic()
        fn   = getattr(context.function, "name", "unknown")
        plug = getattr(context.function, "plugin_name", "") or ""
        args = dict(context.arguments) if hasattr(context, "arguments") else {}

        # PRE — synchronous gate on dispatch path
        try:
            await self._adapter._handle_function_pre(fn, plug, args)
        except AAGFEFunctionBlockedError:
            # Abort — do not call next()
            return

        # Apply pending fragment — PromptRenderFilter will pick it up
        await next(context)

        duration_ms = int((time.monotonic() - sw) * 1000)

        # POST — async, never blocks
        await self._adapter._handle_function_post(fn, context.result, duration_ms)


class _AutoFunctionInvocationFilter:
    """
    IAutoFunctionInvocationFilter equivalent.
    Fires on each iteration of SK's auto-function selection loop.
    Gives AAGFE CGL visibility into LLM reasoning BEFORE tool selection.
    This is the novel first-class CGL concept introduced in AAGFE v2.
    """
    def __init__(self, adapter: AAGFESKAdapter):
        self._adapter = adapter

    async def on_auto_function_invocation(self, context: Any, next: Callable) -> None:
        # Extract chat history for CoT analysis
        chat_history = []
        if hasattr(context, "chat_history"):
            for msg in context.chat_history:
                chat_history.append({
                    "role":    str(getattr(msg, "role", "user")).lower(),
                    "content": str(getattr(msg, "content", "")),
                })

        fn   = getattr(context.function, "name", "") if hasattr(context, "function") else ""
        plug = getattr(context.function, "plugin_name", "") if hasattr(context, "function") else ""
        fn_count = len(list(context.kernel.plugins)) if hasattr(context, "kernel") else 0

        try:
            await self._adapter._handle_auto_function_iteration(
                chat_history=chat_history,
                function_name=fn,
                plugin_name=plug,
                function_count=fn_count,
            )
        except AAGFEPlannerTerminatedError:
            # Terminate the planner loop — SK checks context.terminate
            if hasattr(context, "terminate"):
                context.terminate = True
            return

        await next(context)


class _PromptRenderFilter:
    def __init__(self, adapter: AAGFESKAdapter):
        self._adapter = adapter

    async def on_prompt_render(self, context: Any, next: Callable) -> None:
        prompt = getattr(context, "rendered_prompt", "") or ""

        # PRE — check for pending injection
        await self._adapter._handle_prompt_render(prompt, "PRE")

        # Apply pending fragment BEFORE render
        if self._adapter._pending_fragment:
            fragment = self._adapter._pending_fragment
            self._adapter._pending_fragment = None
            existing = getattr(context, "rendered_prompt", "") or ""
            context.rendered_prompt = f"{fragment}\n\n{existing}" if existing else fragment

        await next(context)

        # POST — record final hash
        final_prompt = getattr(context, "rendered_prompt", "") or ""
        await self._adapter._handle_prompt_render(final_prompt, "POST")


# ─── Stubs ────────────────────────────────────────────────────────────────────
# Replace with generated gRPC stubs after `make proto`

def _hash8(s: str) -> str:
    return hashlib.sha256(s.encode()).hexdigest()[:16]

def _stub_register_task(req: dict) -> dict:
    import uuid
    return {"task_id": str(uuid.uuid4()), "intent_hash": "stub", "initial_constraint_fragment": ""}

def _stub_close_task(req: dict) -> dict:
    return {"closed": True, "final_drift_score": 0, "total_turns": 0,
            "total_iterations": 0, "blocked_functions": 0, "injections_sent": 0}

def _stub_purge_memory(req: dict) -> dict:
    return {"keys_purged": 0, "purged_keys": [], "bleed_keys_found": 0}

def _stub_function_filter(req: dict) -> dict:
    return {"verdict": "ALLOW", "reason": "", "drift_score": 0,
            "review_token": "", "remediation": "NONE", "active_signals": []}

def _stub_auto_function_filter(req: dict) -> dict:
    return {"verdict": "CONTINUE", "reason": "", "drift_score": 0, "signals": [],
            "inject_before_next": False, "constraint_fragment": "",
            "terminate_planner": False, "iteration_event_id": ""}

def _stub_prompt_render_filter(req: dict) -> dict:
    return {"proceed": True, "injection_required": False, "constraint_fragment": "", "fragment_hash": ""}

def _stub_register_memory_key(req: dict) -> dict:
    return {"registered": True, "bleed_risk": False, "origin_task_id": ""}

def _stub_agent_message_check(req: dict) -> dict:
    return {"verdict": "ALLOW", "reason": "", "relay_token": "", "lateral_risk_score": 0.0}
