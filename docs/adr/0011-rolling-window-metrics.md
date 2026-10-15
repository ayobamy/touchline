# ADR 0011: Rolling five-minute window for momentum, pressure, chaos and rhythm

- **Status:** Accepted, implemented
- **Date:** 2026-10-15

## Context

The rules ask Touchline to explain "control vs chaos, tactical pressure changes, and game rhythm patterns". These are properties of a spell of play, not of the whole match, so they need a recent window.

## Decision

Summarise the last 5 minutes of the current half, resetting at half time:

- **Momentum**, from −1 to +1: each event credits its team (final-third completed pass 1, shot 3 or 4 if on target, goal +2, tackle won or interception 1.5, pressure 0.5). Momentum is (home − away) ÷ (home + away).
- **Pressure index:** pressures per 10 opponent passes.
- **Chaos:** possession changes per minute.
- **Rhythm:** events per minute.

Rates divide by the time actually played when a half is younger than the window. The same function (`stats.SummarizeWindow`) serves the engine's rolling window and the `get_window` and `compare_periods` tools, so every view of a spell agrees.

## Consequences

- Swings, presses and scrappy spells are visible as numbers that moment rules can watch ([ADR 0012](0012-rule-based-candidate-moments.md)).
- Early in each half the window is thin and noisy, which is why moment rules wait four minutes.
- A flat window weights a goal 4 minutes ago the same as one 10 seconds ago. Decay weighting is a possible later improvement.

## In the code

`internal/stats/engine.go` (`MomentumCredit`, `SummarizeWindow`, `Engine.Window`).
