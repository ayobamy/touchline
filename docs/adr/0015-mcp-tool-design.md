# ADR 0015: MCP tool design — read-only, strict arguments, errors the model can fix

- **Status:** Accepted, implemented
- **Date:** 2026-10-15

## Context

The tools are the agents' only window onto the match. Tool design decides how many calls an agent needs, how much context each result costs, and whether a mistake is recoverable.

## Decision

- **Nine read-only tools:** `get_match`, `get_status`, `get_snapshot`, `get_player_stats`, `get_window`, `get_events`, `get_event`, `compare_periods`, `list_moments`. None can change state.
- **Strict arguments:** schemas set `additionalProperties: false` and the server rejects unknown fields, so a typo such as `include_player` is reported, not ignored.
- **Two kinds of error:** bad arguments, unknown IDs and unknown metrics are *tool errors* (`isError: true`) with a message saying what to do ("call get_match for player IDs"). Protocol mistakes (unknown tool, malformed JSON-RPC) are JSON-RPC errors.
- **Bounded results:** `get_snapshot` omits per-player stats unless asked; `get_events` returns at most 200 of the most recent matches plus the total; `list_moments` pages with `after` and `next`.
- **Server instructions** sent on initialize state the rules: every number from a tool, cite event IDs, clock and pitch conventions.
- The same store also backs a small JSON API (`/api/status`, `/api/match`, `/api/snapshot`, `/api/moments`) for the orchestrator and viewer.

## Consequences

- A model can recover from its own mistakes within the conversation.
- Results stay small enough for fast models.
- `compare_periods` cannot compare possession, because possession over an arbitrary range needs the owner at the range start. It says so in its description.

## In the code

`internal/mcptools/tools.go`, `internal/mcptools/tools_test.go`, `cmd/server/main.go`.
