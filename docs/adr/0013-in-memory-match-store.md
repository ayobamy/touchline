# ADR 0013: In-memory match store with one writer and idempotent apply

- **Status:** Accepted, implemented
- **Date:** 2026-10-15

## Context

The server replays a match while agents query it concurrently over MCP and the viewer polls a JSON API. Event Hubs and other at-least-once transports can redeliver events.

## Decision

`internal/store` holds one match in memory behind a read-write mutex: one writer (the replay) and many readers. `Apply` validates every event, ignores any event whose `seq` is not greater than the last applied one, and records derived numbers per event and moments in order. Readers get copies, never shared slices.

## Consequences

- Safe concurrent reads without any external service, so the whole pipeline runs on a laptop with only Go.
- Redelivered events cannot double-count. This is tested.
- State is lost on restart and lives in one process. For separate cloud apps, moments and the agent trace will also be published to Redis ([ADR 0018](0018-redis-for-shared-state.md)); the store remains the source for tool calls.
- Out-of-order events are dropped rather than reordered. The simulator and Event Hubs (with one partition per match) both deliver in order.

## In the code

`internal/store/store.go`, `internal/store/store_test.go`.
