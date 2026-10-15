# ADR 0009: Expected goals from distance and goal-mouth angle

- **Status:** Accepted, implemented
- **Date:** 2026-10-15

## Context

xG is the metric viewers most associate with chance quality. Touchline needs one model that both the simulator (to decide goals) and the stats engine (to report chance quality) can use, without training data.

## Decision

A logistic model of shot distance and the angle the 7.32 m goal mouth subtends from the shot position:

```
angle = atan2(7.32 × dx, dx² + dy² − 3.66²)
xG    = 1 / (1 + e^−(−1.2 + 2.2 × angle − 0.09 × distance)),  clamped to 0.01–0.95
```

It gives about 0.70 from 6 m central, 0.31 from the penalty spot and 0.06 from 25 m, in line with published public models. The simulator scores a shot with probability `xG × 0.55` (`× 0.8` again when under pressure). The stats engine reports the unscaled xG.

## Consequences

- One documented formula, tested at calibration points and for symmetry between home and away.
- Teams typically score a little below their reported xG. This is expected and noted in [05 Stats engine](../05-stats-engine.md).
- The model ignores headers, body part, assist type and defenders. It is good enough for explaining chance quality, not for analysis.

## Alternatives considered

- **Distance only.** Rejected: it overrates tight-angle shots.
- **A trained model.** Rejected: there is no training data and no time.

## In the code

`internal/stats/metrics.go` (`ExpectedGoals`), `internal/sim/sim.go` (`shoot`), `internal/stats/stats_test.go`.
