# ADR 0010: A transparent 0–10 pass difficulty rating

- **Status:** Accepted, implemented
- **Date:** 2026-10-15

## Context

The rules list "pass difficulty rating" as a feature. A score that a commentator or a casual fan can understand is more useful on screen than an opaque model.

## Decision

Sum four components, capped at 10 and rounded to 0.1:

| Component | Points |
| --- | --- |
| Length | up to 4, linear to 50 m |
| Under pressure | 2.5 |
| Forward progress | up to 2, linear to 30 m gained |
| Target in the final third (> 70 m from own goal) | 1.5 |

## Consequences

- Every rating can be explained in one sentence ("long, under pressure, into the final third").
- Completed passes rated 8 or more become `key_pass` moments ([ADR 0012](0012-rule-based-candidate-moments.md)).
- The rating ignores passing lanes, defender positions and the receiver's situation, which the data does not have.

## In the code

`internal/stats/metrics.go` (`PassDifficulty`), with ordering tests in `internal/stats/stats_test.go`.
