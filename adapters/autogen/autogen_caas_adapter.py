"""
autogen_caas_adapter.py
=======================
CAAS v2 — AutoGen v0.4 reinforcement adapter.
Wires AutoGen's actor model to AutoGenReinforcementService (port 50069).

AutoGen v0.4 integration points:
  MessageInterceptor  — fires on ALL inter-agent messages (pre+post)
  ToolCallInterceptor — fires specifically on tool_call messages
  CAASTerminationCondition — pluggable termination backed by CGL drift score
  StateBoundaryHook   — wraps save_state / load_state for memory isolation
  CompletionHook      — post-completion structured history submission

Key advantages over SK and AK adapters:
  1. MessageInterceptor covers A2A natively — no workaround needed
  2. HandoffMessage interception closes scope-transfer lateral movement
  3. Full state snapshot via save_state — no key registry
  4. Termination condition is first-class — BLOCK maps directly

Usage:
    from autogen_caas_adapter import CAASAutoGenAdapter, CAASAutoGenConfig
    from autogen_agentchat.agents import AssistantAgent
    from autogen_agentchat.teams import RoundRobinGroupChat

    config = CAASAutoGenConfig(
        endpoint="localhost:50069",
        task_declared_goal="Analyse Q3 revenue and produce report",
        permitted_tools=["sql_query", "file_write"],
        permitted_handoff_targets=["chart_agent"],
        max_rounds=20,
    )
    adapter = CAASAutoGenAdapter(config)

    # Register actors (maps AutoGen agents to CAAS entities)
    assistant_entity = await adapter.register_actor(
        autogen_agent_id="assistant",
        autogen_agent_type="AssistantAgent",
    )
    chart_entity = await adapter.register_actor(
        autogen_agent_id="chart_agent",
        autogen_agent_type="AssistantAgent",
    )

    # Register task
    task_id = await adapter.register_task(
        participant_entity_ids=[assistant_entity, chart_entity],
        team_name="revenue_team",
    )

    # Wrap agents with CAAS interceptors
    assistant = CAASAssistantAgent(
        name="assistant",
        model_client=...,
        caas_adapter=adapter,
    )

    # Use CAAS termination condition
    team = RoundRobinGroupChat(
        [assistant, chart_agent],
        termination_condition=adapter.termination_condition,
    )

    await team.run(task="Analyse Q3 revenue data")
    await adapter.close_task()
"""

from __future__ import annotations

import asyncio
import hashlib
import json
import logging
import time
import uuid
from dataclasses import dataclass, field
from typing import Any, Callable, Optional, Sequence

import grpc

logger = logging.getLogger("caas.autogen_adapter")


# ─── Config ──────────────────────────────────────────────────────────────────

@dataclass
class CAASAutoGenConfig:
    endpoint: str                               # e.g. "localhost:50069"
    task_declared_goal: str
    permitted_tools: list[str]                  = field(default_factory=list)
    permitted_resources: list[str]              = field(default_factory=list)
    permitted_handoff_targets: list[str]        = field(default_factory=list)  # Agent IDs
    max_rounds: int                             = 50
    max_turns: int                              = 0
    tls: bool                                   = False
    warn_threshold: int                         = 400
    strong_warn_threshold: int                  = 600
    block_threshold: int                        = 800
    semantic_guardrail_enabled: bool            = True


# ─── Exceptions ───────────────────────────────────────────────────────────────

class CAASMessageBlockedError(Exception):
    def __init__(self, reason: str, drift_score: int, review_token: str = ""):
        self.reason = reason
        self.drift_score = drift_score
        self.review_token = review_token
        super().__init__(f"CAAS BLOCK: {reason} (drift={drift_score})")

class CAASHandoffBlockedError(Exception):
    def __init__(self, reason: str, lateral_risk: float = 0.0):
        self.lateral_risk = lateral_risk
        super().__init__(f"CAAS HANDOFF BLOCK: {reason} (lateral_risk={lateral_risk:.2f})")

class CAASTerminateSignal(Exception):
    """Raised by termination condition to stop AutoGen loop."""
    def __init__(self, reason: str, cause: str, drift_score: int):
        self.reason = reason
        self.cause = cause
        self.drift_score = drift_score
        super().__init__(f"CAAS TERMINATE: {reason} (cause={cause} drift={drift_score})")


# ─── Main adapter ─────────────────────────────────────────────────────────────

class CAASAutoGenAdapter:
    """
    AutoGen v0.4 CAAS reinforcement adapter.
    One instance per team / group chat session.
    """

    def __init__(self, config: CAASAutoGenConfig):
        self.config = config
        self._channel: Optional[grpc.aio.Channel] = None
        self._stub = None
        self._task_id: Optional[str] = None
        self._session_id: Optional[str] = None
        self._round: int = 0
        self._pending_system_message: Optional[str] = None
        self._entity_registry: dict[str, str] = {}  # autogen_agent_id → entity_id
        self._termination_condition = CAASTerminationCondition(self)

    # ── Properties ────────────────────────────────────────────────────────────

    @property
    def termination_condition(self) -> "CAASTerminationCondition":
        """Pass to AutoGen RoundRobinGroupChat or SelectorGroupChat."""
        return self._termination_condition

    def get_message_interceptor(self) -> "CAASMessageInterceptor":
        """Pass to AutoGen agent as message_interceptor."""
        return CAASMessageInterceptor(self)

    def get_state_hooks(self) -> "CAASStateBoundaryHook":
        """Wraps save_state / load_state calls."""
        return CAASStateBoundaryHook(self)

    # ── Connection ────────────────────────────────────────────────────────────

    async def connect(self) -> None:
        if self.config.tls:
            creds = grpc.ssl_channel_credentials()
            self._channel = grpc.aio.secure_channel(self.config.endpoint, creds)
        else:
            self._channel = grpc.aio.insecure_channel(self.config.endpoint)
        logger.info("CAAS AutoGen adapter connected to %s", self.config.endpoint)

    async def close_connection(self) -> None:
        if self._channel:
            await self._channel.close()

    # ── Actor registration ────────────────────────────────────────────────────

    async def register_actor(
        self,
        autogen_agent_id: str,
        autogen_agent_type: str = "AssistantAgent",
        entity_type: str = "AI_AGENT",
        initial_trust_score: int = 0,
    ) -> str:
        """
        Register an AutoGen agent actor as a CAAS entity.
        Returns CAAS entity_id — use in all subsequent calls.
        """
        req = {
            "autogen_agent_id":    autogen_agent_id,
            "autogen_agent_type":  autogen_agent_type,
            "task_id":             self._task_id or "",
            "entity_type":         entity_type,
            "initial_trust_score": initial_trust_score,
        }
        resp = _stub_register_actor(req)
        entity_id = resp["entity_id"]
        self._entity_registry[autogen_agent_id] = entity_id
        logger.info("CAAS actor registered: %s → entity_id=%s", autogen_agent_id, entity_id)
        return entity_id

    async def deregister_actor(self, autogen_agent_id: str) -> None:
        entity_id = self._entity_registry.get(autogen_agent_id)
        if entity_id:
            _stub_deregister_actor({"entity_id": entity_id, "task_id": self._task_id})
            del self._entity_registry[autogen_agent_id]

    def entity_id(self, autogen_agent_id: str) -> str:
        return self._entity_registry.get(autogen_agent_id, autogen_agent_id)

    # ── Task lifecycle ────────────────────────────────────────────────────────

    async def register_task(
        self,
        participant_entity_ids: list[str],
        team_name: str = "",
        session_id: str | None = None,
        parent_task_id: str = "",
    ) -> str:
        self._session_id = session_id or str(uuid.uuid4())
        self._round = 0

        req = {
            "agent_entity_id":           participant_entity_ids[0] if participant_entity_ids else "",
            "session_id":                self._session_id,
            "team_name":                 team_name,
            "participant_entity_ids":    participant_entity_ids,
            "declared_goal":             self.config.task_declared_goal,
            "permitted_tools":           self.config.permitted_tools,
            "permitted_resources":       self.config.permitted_resources,
            "permitted_handoff_targets": self.config.permitted_handoff_targets,
            "max_rounds":                self.config.max_rounds,
            "max_turns":                 self.config.max_turns,
            "parent_task_id":            parent_task_id,
        }
        resp = _stub_register_task(req)
        self._task_id = resp["task_id"]

        if resp.get("initial_system_message"):
            self._pending_system_message = resp["initial_system_message"]

        if resp.get("termination_condition_id"):
            self._termination_condition.condition_id = resp["termination_condition_id"]

        logger.info("CAAS AutoGen task registered: %s", self._task_id)
        return self._task_id

    async def close_task(
        self,
        reason: str = "COMPLETED",
        summary: str = "",
        total_rounds: int = 0,
    ) -> dict:
        if not self._task_id:
            return {}

        resp = _stub_close_task({
            "task_id":      self._task_id,
            "session_id":   self._session_id,
            "reason":       reason,
            "summary":      summary,
            "total_rounds": total_rounds or self._round,
        })
        logger.info(
            "CAAS AutoGen task closed: %s | drift=%d | blocked_msgs=%d | handoffs=%d",
            self._task_id,
            resp.get("final_drift_score", 0),
            resp.get("blocked_messages", 0),
            resp.get("handoffs_intercepted", 0),
        )
        self._task_id = None
        return resp

    # ── Message interception ──────────────────────────────────────────────────

    async def intercept_message(
        self,
        sender_autogen_id: str,
        receiver_autogen_id: str,
        message_type: str,
        message_content: str,
        message_json: str = "",
        phase: str = "PRE",
    ) -> dict:
        """
        Central intercept for all AutoGen inter-agent messages.
        Raises CAASMessageBlockedError on BLOCK (PRE phase only).
        Raises CAASHandoffBlockedError for blocked HandoffMessages.
        """
        self._round += 1 if phase == "PRE" else 0

        is_handoff = message_type == "AUTOGEN_MESSAGE_TYPE_HANDOFF"
        cross_task  = is_handoff

        req = {
            "sender_entity_id":    self.entity_id(sender_autogen_id),
            "receiver_entity_id":  self.entity_id(receiver_autogen_id),
            "task_id":             self._task_id,
            "round_number":        self._round,
            "message_type":        message_type,
            "message_content":     message_content,
            "message_json":        message_json,
            "phase":               phase,
            "cross_agent_boundary":True,
            "cross_task_boundary": cross_task,
            "session_id":          self._session_id,
        }

        t0 = time.monotonic()
        resp = _stub_message_intercept(req)
        latency_ms = int((time.monotonic() - t0) * 1000)

        verdict = resp.get("verdict", "ALLOW")
        drift   = resp.get("drift_score", 0)
        reason  = resp.get("reason", "")

        logger.debug(
            "CAAS MessageIntercept %s: sender=%s type=%s verdict=%s drift=%d latency=%dms",
            phase, sender_autogen_id, message_type, verdict, drift, latency_ms,
        )

        if verdict == "BLOCK" and phase == "PRE":
            if is_handoff:
                raise CAASHandoffBlockedError(reason, resp.get("lateral_risk_score", 0.0))
            raise CAASMessageBlockedError(reason, drift, resp.get("review_token", ""))

        if verdict in ("WARN", "STRONG_WARN") and phase == "PRE":
            logger.warning("CAAS %s: sender=%s drift=%d signals=%s",
                           verdict, sender_autogen_id, drift,
                           [s.get("type") for s in resp.get("signals", [])])
            remediation = resp.get("remediation", "NONE")
            if remediation == "INJECT_SYSTEM":
                await self._fetch_system_injection()
            elif remediation == "TERMINATE":
                raise CAASTerminateSignal(reason, "DRIFT_BLOCK", drift)

        return resp

    async def intercept_tool_call(
        self,
        sender_autogen_id: str,
        tool_name: str,
        tool_args: dict,
        resource: str = "",
        phase: str = "PRE",
        tool_result: Any = None,
        duration_ms: int = 0,
    ) -> dict:
        """
        Tool-call specific intercept.
        PRE phase is synchronous and blocking on dispatch path.
        POST phase is fire-and-forget.
        """
        req = {
            "sender_entity_id": self.entity_id(sender_autogen_id),
            "task_id":          self._task_id,
            "round_number":     self._round,
            "tool_name":        tool_name,
            "tool_args_json":   json.dumps(tool_args),
            "resource":         resource,
            "phase":            phase,
            "tool_result_json": json.dumps(tool_result) if tool_result else "",
            "tool_succeeded":   tool_result is not None,
            "duration_ms":      duration_ms,
            "session_id":       self._session_id,
        }

        if phase == "POST":
            asyncio.create_task(self._tool_call_post_async(req))
            return {}

        resp = _stub_tool_call_intercept(req)
        verdict = resp.get("verdict", "ALLOW")

        if verdict == "BLOCK":
            raise CAASMessageBlockedError(
                resp.get("reason", ""), resp.get("drift_score", 0), resp.get("review_token", "")
            )
        return resp

    async def _tool_call_post_async(self, req: dict) -> None:
        try:
            _stub_tool_call_intercept(req)
        except Exception as e:
            logger.warning("CAAS ToolCallIntercept POST failed: %s", e)

    # ── Completion hook ───────────────────────────────────────────────────────

    async def completion_hook(
        self,
        agent_autogen_id: str,
        message_history: list[dict],
        completion_content: str,
        completion_role: str = "assistant",
        model: str = "",
        prompt_tokens: int = 0,
        completion_tokens: int = 0,
    ) -> dict:
        """
        Call after every LLM completion.
        AutoGen provides ChatCompletionContext — full structured history.
        Async — never blocks agent execution.
        """
        req = {
            "agent_entity_id":    self.entity_id(agent_autogen_id),
            "task_id":            self._task_id,
            "round_number":       self._round,
            "message_history":    message_history,
            "completion_content": completion_content,
            "completion_role":    completion_role,
            "prompt_tokens":      prompt_tokens,
            "completion_tokens":  completion_tokens,
            "model":              model,
            "session_id":         self._session_id,
        }
        asyncio.create_task(self._completion_async(req))
        return {}

    async def _completion_async(self, req: dict) -> None:
        try:
            resp = _stub_completion_hook(req)
            if resp.get("inject_system_message") and resp.get("system_message_fragment"):
                self._pending_system_message = resp["system_message_fragment"]
        except Exception as e:
            logger.warning("CAAS CompletionHook failed (non-blocking): %s", e)

    # ── Termination check ─────────────────────────────────────────────────────

    async def should_terminate(
        self,
        round_number: int,
        message_count: int,
        last_message_type: str = "",
        last_message_content: str = "",
        entity_id: str = "",
    ) -> tuple[bool, str]:
        """Returns (terminate, reason). Called by CAASTerminationCondition."""
        req = {
            "agent_entity_id":       entity_id or list(self._entity_registry.values())[0] if self._entity_registry else "",
            "task_id":               self._task_id,
            "round_number":          round_number,
            "message_count":         message_count,
            "last_message_type":     last_message_type,
            "last_message_content":  last_message_content,
            "session_id":            self._session_id,
        }
        resp = _stub_should_terminate(req)
        return resp.get("terminate", False), resp.get("reason", "")

    # ── State boundary ────────────────────────────────────────────────────────

    async def pre_state_save(
        self,
        agent_autogen_id: str,
        state_json: str,
        task_ending: bool = False,
    ) -> dict:
        """
        Call BEFORE AutoGen save_state().
        Returns redact_keys — remove these from state before saving.
        memory-isolator gets full state snapshot — no key registry needed.
        """
        req = {
            "agent_entity_id": self.entity_id(agent_autogen_id),
            "task_id":         self._task_id,
            "session_id":      self._session_id,
            "state_json":      state_json,
            "state_hash":      _hash8(state_json),
            "task_ending":     task_ending,
        }
        resp = _stub_pre_state_save(req)
        if resp.get("redact_keys"):
            logger.info("CAAS state save: redacting %d keys", len(resp["redact_keys"]))
        return resp

    async def post_state_load(
        self,
        agent_autogen_id: str,
        state_json: str,
        origin_task_id: str = "",
    ) -> str:
        """
        Call AFTER AutoGen load_state(), BEFORE injecting state into agent.
        Returns sanitised state JSON with bleed keys removed.
        """
        req = {
            "agent_entity_id": self.entity_id(agent_autogen_id),
            "task_id":         self._task_id,
            "session_id":      self._session_id,
            "state_json":      state_json,
            "state_hash":      _hash8(state_json),
            "origin_task_id":  origin_task_id,
        }
        resp = _stub_post_state_load(req)

        if resp.get("bleed_detected"):
            logger.warning(
                "CAAS state load: memory bleed detected — %s | removed=%s",
                resp.get("bleed_detail"), resp.get("removed_keys"),
            )

        if not resp.get("proceed", True):
            logger.error("CAAS state load BLOCKED: %s", resp.get("bleed_detail"))
            return "{}"

        return resp.get("sanitised_state_json", state_json)

    # ── System message injection ──────────────────────────────────────────────

    def consume_pending_system_message(self) -> str | None:
        """
        Returns pending constraint fragment and clears it.
        Call before constructing each round's system prompt.
        """
        msg = self._pending_system_message
        self._pending_system_message = None
        return msg

    async def _fetch_system_injection(self) -> None:
        # In real implementation: call constraint-injector via sk-reinforcement
        # For now just log — system message will arrive via CompletionHook response
        logger.info("CAAS: constraint injection requested for next round")

    def _hash8(self, s: str) -> str:
        return hashlib.sha256(s.encode()).hexdigest()[:16]


# ─── AutoGen integration classes ──────────────────────────────────────────────

class CAASMessageInterceptor:
    """
    AutoGen message interceptor backed by CAAS.
    Register with AutoGen agent's message_interceptor parameter.
    Intercepts ALL inter-agent messages pre and post.
    This is the primary CAAS enforcement surface for AutoGen.
    """

    def __init__(self, adapter: CAASAutoGenAdapter):
        self._adapter = adapter

    async def process(self, messages: list[Any], sender: Any, recipient: Any) -> list[Any] | None:
        """
        AutoGen message interceptor interface.
        Returns None to allow messages through.
        Returns empty list to block.
        Raises CAASMessageBlockedError to hard-stop.
        """
        for msg in messages:
            msg_type   = type(msg).__name__
            msg_content = getattr(msg, "content", str(msg))
            sender_id   = getattr(sender, "name", str(sender))
            receiver_id = getattr(recipient, "name", str(recipient))

            caas_type = _map_autogen_message_type(msg_type)

            try:
                resp = await self._adapter.intercept_message(
                    sender_autogen_id   = sender_id,
                    receiver_autogen_id = receiver_id,
                    message_type        = caas_type,
                    message_content     = msg_content if isinstance(msg_content, str) else json.dumps(msg_content),
                    phase               = "PRE",
                )
            except (CAASMessageBlockedError, CAASHandoffBlockedError) as e:
                logger.error("CAAS interceptor blocked message: %s", e)
                return []  # Block all messages in this batch
            except CAASTerminateSignal:
                return []  # Signal termination

        return None  # Allow through


class CAASTerminationCondition:
    """
    AutoGen pluggable termination condition backed by CGL drift score.
    Pass to RoundRobinGroupChat or SelectorGroupChat as termination_condition.

    AutoGen calls __call__ after each message round.
    Returns True when CGL says TERMINATE (drift >= BLOCK threshold).
    """

    def __init__(self, adapter: CAASAutoGenAdapter):
        self._adapter = adapter
        self.condition_id: str = ""
        self._terminated = False

    @property
    def terminated(self) -> bool:
        return self._terminated

    async def __call__(self, messages: Sequence[Any]) -> Any | None:
        """
        AutoGen termination condition interface.
        Returns StopMessage to terminate, None to continue.
        """
        if not messages:
            return None

        last_msg = messages[-1]
        last_type    = type(last_msg).__name__
        last_content = getattr(last_msg, "content", "")
        if not isinstance(last_content, str):
            last_content = json.dumps(last_content)

        terminate, reason = await self._adapter.should_terminate(
            round_number        = self._adapter._round,
            message_count       = len(messages),
            last_message_type   = last_type,
            last_message_content= last_content,
        )

        if terminate:
            self._terminated = True
            logger.warning("CAAS termination condition fired: %s", reason)
            # Return a StopMessage — AutoGen will halt the loop
            # from autogen_agentchat.messages import StopMessage
            # return StopMessage(content=f"CAAS: {reason}", source="caas_termination")
            return _stub_stop_message(reason)

        return None

    async def reset(self) -> None:
        self._terminated = False


class CAASStateBoundaryHook:
    """
    Wraps AutoGen save_state / load_state with CAAS memory isolation.
    AutoGen v0.4 provides full state snapshots — memory-isolator
    gets the complete picture without needing a key registry.
    """

    def __init__(self, adapter: CAASAutoGenAdapter):
        self._adapter = adapter

    async def wrap_save_state(
        self,
        agent: Any,
        task_ending: bool = False,
    ) -> dict:
        """
        Call agent.save_state() through this wrapper.
        Redacts bleed-risk keys before state is persisted.
        """
        # raw_state = await agent.save_state()  # AutoGen API
        raw_state = _stub_agent_state()

        state_json = json.dumps(raw_state)
        resp = await self._adapter.pre_state_save(
            agent_autogen_id = getattr(agent, "name", str(agent)),
            state_json       = state_json,
            task_ending      = task_ending,
        )

        # Remove redacted keys from state
        redact_keys = resp.get("redact_keys", [])
        if redact_keys:
            state = raw_state.copy() if isinstance(raw_state, dict) else {}
            for k in redact_keys:
                state.pop(k, None)
            return state

        return raw_state

    async def wrap_load_state(
        self,
        agent: Any,
        state: dict,
        origin_task_id: str = "",
    ) -> None:
        """
        Sanitise loaded state before injecting into agent.
        Removes cross-task bleed keys.
        """
        state_json = json.dumps(state)
        sanitised_json = await self._adapter.post_state_load(
            agent_autogen_id = getattr(agent, "name", str(agent)),
            state_json       = state_json,
            origin_task_id   = origin_task_id,
        )
        sanitised = json.loads(sanitised_json) if sanitised_json else {}
        # await agent.load_state(sanitised)  # AutoGen API
        logger.debug("CAAS state loaded for agent %s (%d keys)", getattr(agent, "name", ""), len(sanitised))


# ─── Helpers ─────────────────────────────────────────────────────────────────

def _map_autogen_message_type(class_name: str) -> str:
    return {
        "TextMessage":          "AUTOGEN_MESSAGE_TYPE_TEXT",
        "ToolCallMessage":      "AUTOGEN_MESSAGE_TYPE_TOOL_CALL",
        "ToolCallResultMessage":"AUTOGEN_MESSAGE_TYPE_TOOL_CALL_RESULT",
        "HandoffMessage":       "AUTOGEN_MESSAGE_TYPE_HANDOFF",
        "StopMessage":          "AUTOGEN_MESSAGE_TYPE_STOP",
        "MultiModalMessage":    "AUTOGEN_MESSAGE_TYPE_MULTIMODAL",
        "SystemMessage":        "AUTOGEN_MESSAGE_TYPE_SYSTEM",
    }.get(class_name, "AUTOGEN_MESSAGE_TYPE_TEXT")

def _hash8(s: str) -> str:
    return hashlib.sha256(s.encode()).hexdigest()[:16]


# ─── Stubs (replace with generated gRPC stubs after `make proto`) ────────────

def _stub_register_actor(req: dict) -> dict:
    return {"entity_id": f"entity:{req['autogen_agent_id']}:{uuid.uuid4().hex[:8]}", "trust_token": "stub"}

def _stub_deregister_actor(req: dict) -> dict:
    return {"deregistered": True}

def _stub_register_task(req: dict) -> dict:
    return {"task_id": str(uuid.uuid4()), "intent_hash": "stub",
            "initial_system_message": "", "termination_condition_id": str(uuid.uuid4())}

def _stub_close_task(req: dict) -> dict:
    return {"closed": True, "final_drift_score": 0, "total_rounds": 0,
            "blocked_messages": 0, "blocked_tool_calls": 0,
            "handoffs_intercepted": 0, "injections_sent": 0,
            "state_boundaries": 0, "keys_purged": 0}

def _stub_message_intercept(req: dict) -> dict:
    return {"verdict": "ALLOW", "reason": "", "drift_score": 0, "signals": [],
            "review_token": "", "remediation": "NONE",
            "relay_required": False, "relay_token": "", "lateral_risk_score": 0.0}

def _stub_tool_call_intercept(req: dict) -> dict:
    return {"verdict": "ALLOW", "reason": "", "drift_score": 0, "review_token": "", "remediation": "NONE"}

def _stub_completion_hook(req: dict) -> dict:
    return {"anomaly_score": 0.0, "updated_drift_score": 0,
            "inject_system_message": False, "system_message_fragment": "", "anomaly_tags": []}

def _stub_should_terminate(req: dict) -> dict:
    return {"terminate": False, "reason": "", "cause": "UNSPECIFIED", "drift_score": 0}

def _stub_pre_state_save(req: dict) -> dict:
    return {"proceed": True, "boundary_created": False, "boundary_id": "", "redact_keys": []}

def _stub_post_state_load(req: dict) -> dict:
    return {"proceed": True, "bleed_detected": False, "bleed_detail": "",
            "sanitised_state_json": req.get("state_json", "{}"), "removed_keys": []}

def _stub_agent_state() -> dict:
    return {"messages": [], "memory": {}}

def _stub_stop_message(reason: str) -> dict:
    return {"type": "StopMessage", "content": f"CAAS: {reason}", "source": "caas_termination"}
