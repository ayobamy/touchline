# ADR 0005: Fixed attacking direction and (period, clock) ordering

- **Status:** Accepted, implemented
- **Date:** 2026-10-15

## Context

Every direction-aware metric (forward progress, final third, xG) needs to know which way each team attacks. Real teams switch ends at half time. Football clocks also overlap: first-half stoppage time can run to 47:00 while the second half restarts at 45:00.

## Decision

- Coordinates are metres on a 105 × 68 pitch, and the **home team always attacks towards x = 105** in both halves.
- `clock_ms` is the football match clock, so the second half starts at 2,700,000 ms. Events are ordered by **(period, clock_ms)**, with `seq` as the tie-breaker.

## Consequences

- No half-time special cases in any metric. One helper (`attackingFrame`) converts a position into a team's own frame.
- Overlays and agents can say "since the 60th minute" exactly as viewers expect.
- Anything that sorts or windows by time must also respect the period. The stats engine and store reset or filter windows per period.
- The pitch view can still flip the drawing at half time if realism is wanted.

## In the code

`internal/events` (package comment), `internal/stats/metrics.go` (`attackingFrame`), `internal/store` (`periodEvents`).
