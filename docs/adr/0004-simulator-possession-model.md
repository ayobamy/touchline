# ADR 0004: Simulator model — a possession loop over formation zones

- **Status:** Accepted, implemented
- **Date:** 2026-10-15

## Context

The simulator must produce believable volumes and outcomes (passes, accuracy, possession, shots, goals) and react visibly to scenarios. It must also stay small enough to build and calibrate in a day.

## Decision

Model a match as a loop over the team in possession. Each step:

1. **Pressure.** With a probability that scales with the defending team's press multiplier, a nearby defender presses. A pressed carrier may be tackled. A won tackle turns the ball over; a lost one may become a foul, sometimes a yellow card.
2. **Shot.** Inside about 27 m of goal and central enough, the carrier may shoot, more often the closer they are. Whether it scores is decided by the xG model ([ADR 0009](0009-expected-goals-model.md)) times a finishing factor of 0.55.
3. **Pass.** Otherwise the carrier passes: mostly 6–22 m, sometimes 25–50 m, biased forward. Success falls with length, pressure, the defending team's press and passes into the final third. A failed pass is intercepted or runs loose.

Players have base zones in a 4-3-3 that shift up and down the pitch with the ball, and the nearest player to the ball is chosen to act. A red card removes a player and multiplies the team's press and attack by 0.85. Stoppage time is 1–4 minutes in the first half and 2–6 in the second.

## Consequences

- Over 60 seeded matches the simulator averages about 2.9 goals, roughly 500 passes per team, about 80% pass accuracy and close to even possession.
- Scenarios show up clearly in the statistics. In the late-press demo, Ironbridge's pressure index climbs from about 4 to above 9.
- Positions are approximate, so the data is unsuitable for tracking-style metrics.
- Constants are hand-tuned. Changing one can move several statistics, so the sim tests must be rerun.

## Alternatives considered

- **Markov chain over pitch zones fitted to real data.** Rejected: it needs real data, and it is harder to steer with scenarios.
- **Full agent-based player movement.** Rejected: far more work, for metrics Touchline doesn't need yet.

## In the code

`internal/sim/sim.go` (`step`, `tackle`, `pass`, `shoot`), `internal/sim/roster.go` (formation and fictional squads).
