# Architecture Decision Records

Short records of the decisions that shape Touchline: the context, what we chose, and what follows from it. See [ADR 0001](0001-record-architecture-decisions.md) for the format and rules.

| ADR | Decision | Status |
| --- | --- | --- |
| [0001](0001-record-architecture-decisions.md) | Record architecture decisions | Accepted |
| [0002](0002-code-computes-ai-explains.md) | The code computes; the AI explains | Accepted, implemented |
| [0003](0003-build-our-own-match-simulator.md) | Build our own synthetic match simulator | Accepted, implemented |
| [0004](0004-simulator-possession-model.md) | Simulator model: possession loop over formation zones | Accepted, implemented |
| [0005](0005-pitch-frame-and-clock-ordering.md) | Fixed attacking direction and (period, clock) ordering | Accepted, implemented |
| [0006](0006-ndjson-stream-with-match-header.md) | Newline-delimited JSON stream with a match header | Accepted, implemented |
| [0007](0007-contracts-go-types-and-json-schemas.md) | Contracts are Go types plus JSON Schemas, guarded by drift tests | Accepted, implemented |
| [0008](0008-no-evidence-no-overlay.md) | No evidence, no overlay | Accepted, implemented |
| [0009](0009-expected-goals-model.md) | Expected goals from distance and goal-mouth angle | Accepted, implemented |
| [0010](0010-pass-difficulty-rating.md) | A transparent 0–10 pass difficulty rating | Accepted, implemented |
| [0011](0011-rolling-window-metrics.md) | Rolling five-minute window for momentum, pressure, chaos and rhythm | Accepted, implemented |
| [0012](0012-rule-based-candidate-moments.md) | Rule-based candidate moments before any agent | Accepted, implemented |
| [0013](0013-in-memory-match-store.md) | In-memory match store with one writer and idempotent apply | Accepted, implemented |
| [0014](0014-own-mcp-server-implementation.md) | Our own small MCP server, standard library only | Accepted, implemented |
| [0015](0015-mcp-tool-design.md) | MCP tool design: read-only, strict arguments, errors the model can fix | Accepted, implemented |
| [0016](0016-go-pipeline-python-agents.md) | Go for the data pipeline, Python for the agents | Accepted, implemented |
| [0017](0017-four-agents-for-solo-build.md) | Four agents for the solo build | Accepted, implemented (Foundry models to wire) |
| [0018](0018-redis-for-shared-state.md) | Azure Managed Redis for shared state | Proposed |
| [0019](0019-target-best-multi-agent-system.md) | Target the Best Multi-Agent System category | Accepted |
| [0020](0020-zero-go-dependencies.md) | No third-party Go dependencies, enforced by CI | Accepted, implemented |
| [0021](0021-official-mcp-python-sdk-client.md) | The official MCP Python SDK for the orchestrator's tool client | Accepted, implemented |
| [0022](0022-orchestrator-asyncio-pipeline.md) | The orchestrator is a plain asyncio pipeline; models sit behind one interface | Accepted, implemented (Foundry models to wire) |
| [0023](0023-time-anchored-evidence-code-first-verifier.md) | Evidence anchored to the moment, and a Verifier that checks numbers in code first | Accepted, implemented |
| [0024](0024-director-and-fallbacks-in-code.md) | The Director and the fallback templates are code | Accepted, implemented |
| [0025](0025-openapi-and-scalar-api-reference.md) | An OpenAPI document and a Scalar API reference, served by the server | Accepted, implemented |

To add one, copy the template in ADR 0001, take the next number, and add a row here in the same commit.
