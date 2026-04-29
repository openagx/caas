"""
ak_caas_adapter.py
==================
Agent Kernel (AK) reinforcement adapter for CAAS v2 CGL.

Drop this into any AK agent project. Wire it via AK's hook system.
It provides everything AK does not:
  - Pre-tool intercept (AK only has post-tool hooks)
  - Semantic guardrail layer (AK guardrails are keyword-based)
  - Scoped session key enumeration + purge (AK session API is get/set only)
  - A2A message interception
  - Constraint injection into system prompts

Usage
-----
from ak_caas_adapter import CAASAdapter, CAASConfig

config = CAASConfig(
    ak_reinforcement_endpoint="localhost:50066",
    agent_entity_id="agent:my-agent-001",
)
adapter = CAASAdapter(config)

# In your AK agent definition:
agent = Agent(
    ...
    pre_tool_hook=adapter.pre_tool_hook,
    post_tool_hook=adapter.post_tool_hook,
    on_completion_hook=adapter.on_completion_hook,
    on_task_start_hook=adapter.on_task_start_hook,
    on_task_end_hook=adapter.on_task_end_hook,
)
"""

from __future__ import annotations

import asyncio
import hashlib
import json
import logging
import time
from dataclasses import dataclass, field
from enum import Enum
from typing import Any, Callable, Coroutine, Optional

import grpc

# Generated stubs — run `make proto` from caas repo root
# from gen.go.caas.v1 import ak_reinforcement_pb2 as pb
# from gen.go.caas.v1 import ak_reinforcement_pb2_grpc as pb_grpc
# Shown as typed stubs here for clarity:

logger = logging.getLogger("caas.ak_adapter")


# ─── Config ──────────────────────────────────────────────────────────────────

@dataclass
class CAASConfig:
    ak_reinforcement_endpoint: str        # e.g. "localhost:50066"
    agent_entity_id: str                  # Maps to CAAS entity-service AI_AGENT entity
    framework: str = "LANGGRAPH"          # AK_FRAMEWORK_* enum name
    session_store: str = "IN_MEMORY"      # AK_SESSION_STORE_* enum name
    tls: bool = False
    # Drift thresholds (mirrors behavioral-gate defaults; override per-agent)
    warn_threshold: int = 400
    strong_warn_threshold: int = 600
    block_threshold: int = 800
    # Injection interval (turns); 0 = only drift-triggered
    injection_interval_turns: int = 10
    # Whether to run semantic guardrail on every completion
    semantic_guardrail_enabled: bool = True


# ─── Verdict enums (mirrors proto) ───────────────────────────────────────────

class InterceptVerdict(str, Enum):
    ALLOW       = "ALLOW"
    WARN        = "WARN"
    STRONG_WARN = "STRONG_WARN"
    BLOCK       = "BLOCK"

class AKRemediation(str, Enum):
    NONE             = "NONE"
    INJECT_NEXT_TURN = "INJECT_NEXT_TURN"
    RESET_SESSION    = "RESET_SESSION"
    ESCALATE_HUMAN   = "ESCALATE_HUMAN"
    SUSPEND          = "SUSPEND"


# ─── Exceptions ───────────────────────────────────────────────────────────────

class CAASBlockError(Exception):
    """Raised by pre_tool_hook when behavioral-gate returns BLOCK."""
    def __init__(self, reason: str, drift_score: int, review_token: str = ""):
        self.reason = reason
        self.drift_score = drift_score
        self.review_token = review_token
        super().__init__(f"CAAS BLOCK: {reason} (drift={drift_score})")

class CAASA2ABlockError(Exception):
    """Raised by a2a_hook when A2A intercept returns BLOCK."""
    def __init__(self, reason: str):
        super().__init__(f"CAAS A2A BLOCK: {reason}")


# ─── Adapter ─────────────────────────────────────────────────────────────────

class CAASAdapter:
    """
    AK reinforcement adapter. Instantiate once per agent process.
    Thread-safe; uses asyncio-native gRPC channel.
    """

    def __init__(self, config: CAASConfig):
        self.config = config
        self._channel: Optional[grpc.aio.Channel] = None
        self._stub = None
        self._task_id: Optional[str] = None
        self._session_id: Optional[str] = None
        self._turn: int = 0
        self._pending_injection: Optional[str] = None  # Fragment to prepend next turn

    # ── Connection ────────────────────────────────────────────────────────────

    async def connect(self) -> None:
        """Open gRPC channel to AKReinforcementService."""
        if self.config.tls:
            creds = grpc.ssl_channel_credentials()
            self._channel = grpc.aio.secure_channel(self.config.ak_reinforcement_endpoint, creds)
        else:
            self._channel = grpc.aio.insecure_channel(self.config.ak_reinforcement_endpoint)
        # self._stub = pb_grpc.AKReinforcementServiceStub(self._channel)
        logger.info("CAAS adapter connected to %s", self.config.ak_reinforcement_endpoint)

    async def close(self) -> None:
        if self._channel:
            await self._channel.close()

    # ── Task lifecycle ─────────────────────────────────────────────────────────

    async def on_task_start_hook(
        self,
        *,
        session_id: str,
        declared_goal: str,
        permitted_tools: list[str],
        permitted_resources: list[str] | None = None,
        max_turns: int = 0,
        parent_task_id: str = "",
        ak_agent_config: dict | None = None,
    ) -> str:
        """
        Call at AK task start BEFORE any tool dispatch.
        Returns task_id — store and pass to subsequent hooks.
        Also returns initial constraint fragment for first system prompt.
        """
        req = {
            "agent_entity_id": self.config.agent_entity_id,
            "session_id": session_id,
            "declared_goal": declared_goal,
            "permitted_tools": permitted_tools,
            "permitted_resources": permitted_resources or [],
            "max_turns": max_turns,
            "parent_task_id": parent_task_id,
            "framework": self.config.framework,
            "ak_agent_config": ak_agent_config or {},
        }
        logger.debug("RegisterTask: %s", declared_goal[:80])

        # resp = await self._stub.RegisterTask(pb.AKRegisterTaskRequest(**req))
        # Stub response for type clarity:
        resp = _stub_register_task(req)

        self._task_id = resp["task_id"]
        self._session_id = session_id
        self._turn = 0
        # Stash initial fragment to inject into first prompt
        if resp.get("initial_constraint_fragment"):
            self._pending_injection = resp["initial_constraint_fragment"]
        logger.info("CAAS task registered: %s", self._task_id)
        return self._task_id

    async def on_task_end_hook(
        self,
        *,
        reason: str = "COMPLETED",
        summary: str = "",
    ) -> dict:
        """
        Call at AK task end. Closes task + purges scoped memory.
        Returns final stats dict.
        """
        if not self._task_id:
            return {}

        close_req = {
            "task_id": self._task_id,
            "session_id": self._session_id,
            "reason": reason,
            "summary": summary,
        }
        # close_resp = await self._stub.CloseTask(pb.AKCloseTaskRequest(**close_req))
        close_resp = _stub_close_task(close_req)

        # Purge session memory — adapter owns enumeration; AK session API cannot
        purge_req = {
            "task_id": self._task_id,
            "session_id": self._session_id,
            "dry_run": False,
            "session_store": self.config.session_store,
        }
        # purge_resp = await self._stub.PurgeTaskMemory(pb.PurgeTaskMemoryRequest(**purge_req))
        purge_resp = _stub_purge_memory(purge_req)

        logger.info(
            "CAAS task closed: %s | drift=%d | purged=%d keys",
            self._task_id,
            close_resp.get("final_drift_score", 0),
            purge_resp.get("keys_purged", 0),
        )
        self._task_id = None
        return {**close_resp, "keys_purged": purge_resp.get("keys_purged", 0)}

    # ── Pre-tool intercept (fills AK's missing pre-hook) ─────────────────────

    async def pre_tool_hook(
        self,
        *,
        tool_name: str,
        resource: str = "",
        parameters: dict | None = None,
    ) -> None:
        """
        AK must call this synchronously before every tool dispatch.
        Raises CAASBlockError if verdict is BLOCK — AK must not dispatch.
        Logs WARN/STRONG_WARN and continues.

        This is the hook AK does not provide natively.
        The adapter provides the pre-dispatch intercept.
        """
        self._turn += 1
        req = {
            "agent_entity_id": self.config.agent_entity_id,
            "task_id": self._task_id,
            "turn_number": self._turn,
            "tool_name": tool_name,
            "resource": resource,
            "parameters_json": json.dumps(parameters or {}),
            "session_id": self._session_id,
            "framework": self.config.framework,
        }

        t0 = time.monotonic()
        # resp = await self._stub.PreToolIntercept(pb.PreToolInterceptRequest(**req))
        resp = _stub_pre_tool(req)
        latency_ms = int((time.monotonic() - t0) * 1000)

        verdict = resp.get("verdict", "ALLOW")
        drift = resp.get("drift_score", 0)
        reason = resp.get("reason", "")

        logger.debug(
            "PreToolIntercept: tool=%s verdict=%s drift=%d latency=%dms",
            tool_name, verdict, drift, latency_ms,
        )

        if verdict == "BLOCK":
            logger.error("CAAS BLOCK: tool=%s reason=%s drift=%d", tool_name, reason, drift)
            raise CAASBlockError(
                reason=reason,
                drift_score=drift,
                review_token=resp.get("review_token", ""),
            )

        if verdict in ("WARN", "STRONG_WARN"):
            logger.warning(
                "CAAS %s: tool=%s reason=%s drift=%d signals=%s",
                verdict, tool_name, reason, drift, resp.get("active_signals", []),
            )
            remediation = resp.get("remediation", "NONE")
            if remediation == "INJECT_NEXT_TURN":
                await self._fetch_and_stash_injection()
            elif remediation == "RESET_SESSION":
                await self._purge_memory_immediate()
            elif remediation == "ESCALATE_HUMAN":
                logger.warning("CAAS: routing to M-of-N review, token=%s", resp.get("review_token"))

    # ── Post-tool record (supplements AK's existing post-hook) ───────────────

    async def post_tool_hook(
        self,
        *,
        tool_name: str,
        resource: str = "",
        outcome: str = "SUCCESS",
        result_summary: str = "",
        duration_ms: int = 0,
    ) -> None:
        """Async — never blocks AK execution."""
        req = {
            "agent_entity_id": self.config.agent_entity_id,
            "task_id": self._task_id,
            "turn_number": self._turn,
            "tool_name": tool_name,
            "resource": resource,
            "outcome": outcome,
            "result_summary": result_summary,
            "duration_ms": duration_ms,
            "session_id": self._session_id,
        }
        asyncio.create_task(self._post_tool_async(req))

    async def _post_tool_async(self, req: dict) -> None:
        try:
            # await self._stub.PostToolRecord(pb.PostToolRecordRequest(**req))
            _stub_post_tool(req)
        except Exception as e:
            logger.warning("CAAS PostToolRecord failed (non-blocking): %s", e)

    # ── LLM completion submission ─────────────────────────────────────────────

    async def on_completion_hook(
        self,
        *,
        cot_text: str,
        action_taken: str = "",
        ak_guardrail_verdict: str = "PASS",
        ak_flagged_keywords: list[str] | None = None,
    ) -> dict:
        """
        AK calls this after every LLM completion.
        Async — does not block. Returns updated drift score.

        Also runs semantic guardrail if enabled — complements AK's
        keyword-based guardrail with embedding-based detection.
        """
        req = {
            "agent_entity_id": self.config.agent_entity_id,
            "task_id": self._task_id,
            "turn_number": self._turn,
            "cot_text": cot_text,
            "action_taken": action_taken,
            "ak_guardrail_verdict": ak_guardrail_verdict,
            "ak_flagged_keywords": ak_flagged_keywords or [],
            "session_id": self._session_id,
        }

        # Submit CoT async — never blocks
        asyncio.create_task(self._submit_completion_async(req))

        # Semantic guardrail — also async but we want the result
        # for injection decision, so we await it with a tight timeout
        if self.config.semantic_guardrail_enabled:
            try:
                sem_result = await asyncio.wait_for(
                    self._semantic_guardrail(cot_text, "AGENT_RESPONSE", ak_guardrail_verdict, ak_flagged_keywords or []),
                    timeout=0.1,  # 100ms max — never stall AK
                )
                if sem_result.get("verdict") == "AUGMENT":
                    self._pending_injection = sem_result.get("constraint_fragment", "")
                elif sem_result.get("verdict") == "BLOCK":
                    logger.error("CAAS semantic guardrail BLOCK on completion: %s", sem_result.get("reason"))
                    # Emit as drift signal — cannot block post-completion but flags for next turn
            except asyncio.TimeoutError:
                logger.debug("CAAS semantic guardrail timeout — skipped")

        # Check injection schedule
        if self._turn % self.config.injection_interval_turns == 0:
            await self._fetch_and_stash_injection()

        return {}

    async def _submit_completion_async(self, req: dict) -> None:
        try:
            # resp = await self._stub.SubmitCompletion(pb.SubmitCompletionRequest(**req))
            resp = _stub_submit_completion(req)
            if resp.get("inject_next_turn"):
                await self._fetch_and_stash_injection()
        except Exception as e:
            logger.warning("CAAS SubmitCompletion failed (non-blocking): %s", e)

    async def _semantic_guardrail(
        self,
        content: str,
        content_type: str,
        ak_verdict: str,
        ak_keywords: list[str],
    ) -> dict:
        req = {
            "agent_entity_id": self.config.agent_entity_id,
            "task_id": self._task_id,
            "content": content,
            "content_type": content_type,
            "ak_verdict": ak_verdict,
            "ak_keywords": ak_keywords,
        }
        # return await self._stub.SemanticGuardrailCheck(pb.SemanticGuardrailRequest(**req))
        return _stub_semantic_guardrail(req)

    # ── A2A intercept ──────────────────────────────────────────────────────────

    async def a2a_hook(
        self,
        *,
        receiver_agent_entity_id: str,
        message_type: str,
        payload: dict,
        cross_sovereign: bool = False,
    ) -> None:
        """
        AK calls this before every A2A message dispatch.
        Raises CAASA2ABlockError if verdict is BLOCK.
        This closes the lateral movement surface in multi-agent topologies.
        """
        req = {
            "sender_agent_entity_id": self.config.agent_entity_id,
            "receiver_agent_entity_id": receiver_agent_entity_id,
            "sender_task_id": self._task_id,
            "message_type": message_type,
            "payload_json": json.dumps(payload),
            "cross_sovereign": cross_sovereign,
            "session_id": self._session_id,
        }
        # resp = await self._stub.CheckA2A(pb.A2ACheckRequest(**req))
        resp = _stub_a2a_check(req)

        if resp.get("verdict") == "BLOCK":
            raise CAASA2ABlockError(resp.get("reason", "A2A blocked by CAAS"))
        if resp.get("lateral_risk_score", 0) > 0.7:
            logger.warning(
                "CAAS A2A high lateral risk: receiver=%s score=%.2f",
                receiver_agent_entity_id, resp["lateral_risk_score"],
            )

    # ── Session key registration (fills AK enumeration gap) ───────────────────

    async def register_session_key(
        self,
        key: str,
        sensitivity: str = "NORMAL",
    ) -> dict:
        """
        AK calls this whenever it writes to its session store.
        Adapter maintains task-scoped key registry since AK session
        API has no enumeration capability — this is what enables
        memory-isolator's PurgeTaskMemory to work.
        """
        req = {
            "task_id": self._task_id,
            "session_id": self._session_id,
            "key": key,
            "sensitivity": sensitivity,
        }
        # resp = await self._stub.RegisterSessionKey(pb.RegisterSessionKeyRequest(**req))
        resp = _stub_register_key(req)
        if resp.get("bleed_risk"):
            logger.warning(
                "CAAS memory bleed risk: key=%s origin_task=%s",
                key, resp.get("origin_task_id"),
            )
        return resp

    # ── Constraint injection ───────────────────────────────────────────────────

    async def get_system_prompt_fragment(self, current_system_prompt: str) -> str | None:
        """
        AK calls this before constructing the next system prompt.
        Returns a fragment to prepend, or None if no injection needed.
        Idempotent — adapter checks if fragment already present.
        """
        # Return stashed fragment if pending
        if self._pending_injection:
            fragment = self._pending_injection
            self._pending_injection = None
            return fragment

        req = {
            "agent_entity_id": self.config.agent_entity_id,
            "task_id": self._task_id,
            "turn_number": self._turn,
            "system_prompt_hash": hashlib.sha256(current_system_prompt.encode()).hexdigest()[:16],
        }
        # resp = await self._stub.InjectConstraints(pb.InjectConstraintsRequest(**req))
        resp = _stub_inject_constraints(req)

        if resp.get("injection_required"):
            return resp.get("fragment_text")
        return None

    # ── Internal helpers ──────────────────────────────────────────────────────

    async def _fetch_and_stash_injection(self) -> None:
        try:
            req = {
                "agent_entity_id": self.config.agent_entity_id,
                "task_id": self._task_id,
                "turn_number": self._turn,
                "system_prompt_hash": "",
            }
            # resp = await self._stub.InjectConstraints(pb.InjectConstraintsRequest(**req))
            resp = _stub_inject_constraints(req)
            if resp.get("injection_required"):
                self._pending_injection = resp.get("fragment_text")
        except Exception as e:
            logger.warning("CAAS injection fetch failed: %s", e)

    async def _purge_memory_immediate(self) -> None:
        try:
            req = {
                "task_id": self._task_id,
                "session_id": self._session_id,
                "dry_run": False,
                "session_store": self.config.session_store,
            }
            # await self._stub.PurgeTaskMemory(pb.PurgeTaskMemoryRequest(**req))
            _stub_purge_memory(req)
        except Exception as e:
            logger.warning("CAAS immediate purge failed: %s", e)


# ─── Stub implementations (replace with generated gRPC stubs after `make proto`)
# These return typed dicts matching proto response shapes for local testing.

def _stub_register_task(req: dict) -> dict:
    import uuid
    task_id = str(uuid.uuid4())
    return {"task_id": task_id, "intent_hash": "stub-hash", "initial_constraint_fragment": ""}

def _stub_close_task(req: dict) -> dict:
    return {"closed": True, "final_drift_score": 0, "total_turns": 0, "blocked_actions": 0, "injections_sent": 0}

def _stub_purge_memory(req: dict) -> dict:
    return {"keys_purged": 0, "purged_keys": [], "bleed_keys_found": 0, "dry_run": req.get("dry_run", False)}

def _stub_pre_tool(req: dict) -> dict:
    return {"verdict": "ALLOW", "reason": "", "drift_score": 0, "active_signals": [], "review_token": "", "remediation": "NONE"}

def _stub_post_tool(req: dict) -> dict:
    return {"recorded": True, "record_id": "stub"}

def _stub_submit_completion(req: dict) -> dict:
    return {"anomaly_score": 0.0, "updated_drift_score": 0, "inject_next_turn": False, "anomaly_tags": []}

def _stub_semantic_guardrail(req: dict) -> dict:
    return {"verdict": "PASS", "risk_score": 0.0, "triggered_policies": [], "constraint_fragment": "", "reason": ""}

def _stub_a2a_check(req: dict) -> dict:
    return {"verdict": "ALLOW", "reason": "", "relay_token": "", "lateral_risk_score": 0.0}

def _stub_register_key(req: dict) -> dict:
    return {"registered": True, "bleed_risk": False, "origin_task_id": ""}

def _stub_inject_constraints(req: dict) -> dict:
    return {"injection_required": False, "fragment_text": "", "fragment_hash": "", "position": "PREPEND"}
