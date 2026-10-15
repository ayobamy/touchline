# 09 Decisions

Design decisions are recorded as Architecture Decision Records in [docs/adr](adr/README.md), one file per decision, with context, consequences and the alternatives considered.

The earlier short list (D1–D11) maps to these records:

| Earlier | ADR |
| --- | --- |
| D1 The code computes; the AI explains | [0002](adr/0002-code-computes-ai-explains.md) |
| D2 Build our own simulator | [0003](adr/0003-build-our-own-match-simulator.md) |
| D3 Home team always attacks towards x = 105 | [0005](adr/0005-pitch-frame-and-clock-ordering.md) |
| D4 Order by (period, clock) | [0005](adr/0005-pitch-frame-and-clock-ordering.md) |
| D5 No evidence, no overlay | [0008](adr/0008-no-evidence-no-overlay.md) |
| D6 Four agents for the solo build | [0017](adr/0017-four-agents-for-solo-build.md) |
| D7 Redis for shared state | [0018](adr/0018-redis-for-shared-state.md) |
| D8 Go for the pipeline, Python for the agents | [0016](adr/0016-go-pipeline-python-agents.md) |
| D9 Newline-delimited JSON | [0006](adr/0006-ndjson-stream-with-match-header.md) |
| D10 Target Best Multi-Agent System | [0019](adr/0019-target-best-multi-agent-system.md) |
| D11 Our own small MCP server | [0014](adr/0014-own-mcp-server-implementation.md) |
