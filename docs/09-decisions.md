# 09 Decisions

Short records of the choices that shape Touchline. Add a new entry rather than editing an old one when a decision changes.

## D1 The code computes; the AI explains

**Decision:** Every number on screen comes from the deterministic stats engine. Agents fetch numbers through MCP tools and may only explain them.

**Why:** Language models get arithmetic and counting wrong, and a wrong statistic on a broadcast is worse than none. Separating the two also makes the Verifier possible: it checks claims against a source of truth.

## D2 Build our own simulator

**Decision:** Generate synthetic matches with a seeded simulator rather than using a static dataset.

**Why:** The rules require synthetic data and no data is provided. A steerable simulator gives replayable demos (same seed, same match), scenarios that create the moments we want to explain, and scores on "creative and optimized synthetic data creation".

## D3 Home team always attacks towards x = 105

**Decision:** Teams do not switch ends at half time in the data.

**Why:** Every metric that depends on direction — progression, final third, xG — stays free of half-time special cases. The pitch view can still flip the drawing in the second half if it wants to look realistic.

## D4 Order by (period, clock), not clock

**Decision:** The second half's clock starts at 45:00 regardless of first-half stoppage time.

**Why:** It matches how football clocks work, which is what viewers and overlays expect ("since the 60th minute").

## D5 No evidence, no overlay

**Decision:** An overlay without at least one cited event ID fails validation and is never published.

**Why:** It enforces D1 at the last possible point, independent of agent behaviour, and gives broadcasters an audit trail.

## D6 Four agents for the solo build

**Decision:** Fold the Tactical Analyst and Localizer into the Narrator.

**Why:** One person has 13 days. Four agents still show distinct roles, shared state, handoffs and failure recovery; the contracts allow splitting them out later.

## D7 Redis for shared state, not Cosmos DB

**Decision:** Use Azure Managed Redis for match state, queues and the trace.

**Why:** The state is small, short-lived and latency-sensitive. One store is less to build and deploy alone.

## D8 Go for the data pipeline, Python for the agents

**Decision:** Simulator, stats engine and MCP server in Go; orchestrator and agents in Python.

**Why:** Go is the author's strongest language and suits a fast, dependency-free streaming core. Python has the most complete Agent Framework and Foundry support and examples. The contracts in [04](04-data-contracts.md) are the boundary.

## D9 Newline-delimited JSON on the wire

**Decision:** One JSON message per line locally and one per Event Hubs message in the cloud.

**Why:** Easy to pipe, inspect, record and replay; the same format in both modes.

## D10 Target Best Multi-Agent System

**Decision:** Of the category prizes, aim at Best Multi-Agent System.

**Why:** Its criteria (distinct roles, shared state, handoffs, failure recovery, an outcome one agent could not achieve) match Touchline's design point for point.

## D11 Our own small MCP server

**Decision:** Implement the MCP server side ourselves (`internal/mcp`, about 300 lines, standard library only) instead of using the official Go SDK.

**Why:** The build environment could not download Go modules, and Touchline needs only a small part of the protocol: initialize, ping, tools/list and tools/call over stdio and streamable HTTP with JSON responses. It keeps the Go code dependency-free. Tools are registered through one small interface, so switching to the official SDK later touches only `internal/mcp` and `internal/mcptools`.

**Trade-off:** No server-initiated messages, resources, prompts or SSE streaming. None are needed for read-only tools.
