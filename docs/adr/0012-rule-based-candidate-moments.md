# ADR 0012: Rule-based candidate moments before any agent

- **Status:** Accepted, implemented
- **Date:** 2026-10-15

## Context

A match has about 1,800 events. Asking a model about each one would be slow and costly, and would bury the few moments that matter. The agents still need to decide what deserves screen time, so the filter should cast a wide but bounded net.

## Decision

Deterministic rules (`internal/moments`) watch the stats engine after every event and propose candidate moments:

| Kind | Trigger |
| --- | --- |
| goal, card | every one |
| big_chance | missed, saved or blocked shot with xG ≥ 0.25 |
| key_pass | completed pass with difficulty ≥ 8 |
| momentum_swing | a team now beyond ±0.5, having just crossed that level or moved by ≥ 0.6 in five minutes |
| press_switch | a team's pressure index up by ≥ 3 in five minutes, to at least 6 |
| chaos_spell | ≥ 4 possession changes a minute, held for two minutes |
| player_milestone | 40th completed pass, first saved shot on target, fourth tackle won |

Window rules wait four minutes into each half. The same rule for the same team then stays quiet for six minutes. Each moment carries up to six evidence event IDs, never from the future or the other half, and a plain-English summary using names, not IDs. All thresholds are in one struct, `moments.Thresholds`.

## Consequences

- About 35–55 candidates a match, so agents are woken roughly once every two minutes.
- Every candidate arrives with evidence, so [ADR 0008](0008-no-evidence-no-overlay.md) is easy to satisfy downstream.
- The first calibration produced about 60 candidates, mostly milestones and weak swings. Tightening those two rules brought it to about 45. Thresholds may need retuning once the agents are judging real output.

## Alternatives considered

- **Let an agent watch every event.** Rejected: cost, latency and noise.
- **Learned anomaly detection.** Rejected: no training data, and harder to explain.

## In the code

`internal/moments/moments.go`, tests in `internal/moments/moments_test.go`.
