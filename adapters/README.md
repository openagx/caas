# AAGFE v2 — Runtime Adapters

Reinforcement adapters that connect agent runtimes to the AAGFE v2
Cognitive Guard Layer (CGL). Each adapter exposes the CGL integration
contract to a specific runtime without requiring changes to the
underlying AAGFE authZ core.

## Adapters

| Adapter | Runtime | Port | Key advantage |
|---|---|---|---|
| `ak/` | Agent Kernel (AK) | 50066 | Multi-cloud, multi-framework (LangGraph, CrewAI, ADK) |
| `sk/` | Semantic Kernel | 50067 | `IAutoFunctionInvocationFilter` — best per-iteration reasoning hook |
| `autogen/` | AutoGen v0.4 | 50069 | Best A2A interception, full state snapshot, native termination condition |

## Integration contract

All runtimes must implement:

1. **Pre-action intercept** — call adapter before every tool dispatch (sync, blocking)
2. **Task registration** — register declared goal + permitted scope at task start
3. **Completion submission** — submit chain-of-thought after each LLM completion (async)
4. **Task close** — close task + purge scoped memory on completion

See each adapter's source file for runtime-specific wiring instructions.

## After `make proto`

Replace stub implementations in each adapter with generated gRPC stubs from
`gen/go/caas/v1/` and `gen/ts/`. Stubs are clearly marked in each file.

## Proto files

All service contracts: `proto/caas/v1/`
- `behavioral_gate.proto` — `BehavioralGateService` (port 50059)
- `cognitive_guard.proto` — 7 CGL services (ports 50060–50065, 50068)
- `sk_reinforcement.proto` — `SKReinforcementService` (port 50067)
- `ak_reinforcement.proto` — `AKReinforcementService` (port 50066)
- `autogen_reinforcement.proto` — `AutoGenReinforcementService` (port 50069)
- `cognitive-drift-event.schema.json` — Redpanda topic schema v2.0.0
