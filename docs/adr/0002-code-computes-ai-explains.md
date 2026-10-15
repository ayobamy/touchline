# ADR 0002: The code computes; the AI explains

- **Status:** Accepted, implemented in the stats engine and MCP tools; agents to follow
- **Date:** 2026-10-15

## Context

Touchline puts statistics on screen during a live broadcast. Language models are unreliable at counting and arithmetic, and a wrong number on air is worse than no number. The rules also ask for *explainable* match intelligence.

## Decision

Every number Touchline shows comes from deterministic Go code: the stats engine (`internal/stats`) and the moment rules (`internal/moments`). Agents read numbers only through the read-only MCP tools and may only explain them. The MCP server's instructions tell the model this on connect, and the Verifier agent checks it.

## Consequences

- Numbers are reproducible and testable: the same events always give the same statistics.
- A separate Verifier becomes possible, because there is a source of truth to check claims against.
- Agents need tool calls to say anything numeric, which adds latency. This is accepted and budgeted in [03 Architecture](../03-architecture.md#latency-budget).
- Every new statistic needs code and tests before an agent can mention it.

## Alternatives considered

- **Let the model compute statistics from raw events.** Rejected: unreliable, untestable, and expensive in tokens.
- **Give the model a code-execution tool.** Rejected: the results are still not reproducible, and it adds a sandbox to secure.

## In the code

`internal/stats`, `internal/moments`, `internal/mcptools` (see `Instructions`).
