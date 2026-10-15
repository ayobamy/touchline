# ADR 0008: No evidence, no overlay

- **Status:** Accepted; contract implemented, enforcement in the publisher to follow
- **Date:** 2026-10-15

## Context

Even with [ADR 0002](0002-code-computes-ai-explains.md), an agent could still write an unsupported claim. Broadcasters also need to trace any graphic back to what happened on the pitch.

## Decision

An overlay must cite at least one event ID in `evidence_event_ids`, or `overlay.Validate` rejects it and it is never published. The contract also caps on-screen size so overlays stay readable during play: headline at most 60 characters, body at most 160, at most 4 metrics, shown for 2–30 seconds, with priority 1–5 and one of four slots.

## Consequences

- An unsupported claim cannot reach a screen, whatever the agents do.
- Every overlay has an audit trail.
- Template overlays (`fallback: true`) must also cite events, so templates are built from moments, which always carry evidence.

## In the code

`internal/overlay/overlay.go`, `schema/overlay.schema.json`.
