# ADR 0018: Azure Managed Redis for shared state, not Cosmos DB

- **Status:** Proposed, to be implemented in milestone 2
- **Date:** 2026-10-15

## Context

Agents coordinate through shared state: candidate moments, briefs, drafts, verified overlays, the screen schedule and the trace. In the cloud the server, orchestrator and viewer run as separate apps.

## Decision

Use Azure Managed Redis (smallest tier) with lists and keys per match, as listed in [06 Agents](../06-agents.md#shared-state). Locally, the in-memory store ([ADR 0013](0013-in-memory-match-store.md)) and the server's JSON API stand in for it.

## Consequences

- Low latency, simple data structures, one service to provision.
- State is short-lived by design; nothing needs to outlive a match.
- No query language. Anything analytical stays in the Go store.

## Alternatives considered

- **Cosmos DB.** Rejected for now: more setup and more than this short-lived state needs.
- **Service Bus queues.** Considered for handoffs; Redis lists are enough and also hold the trace.
