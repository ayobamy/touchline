# ADR 0003: Build our own synthetic match simulator

- **Status:** Accepted, implemented
- **Date:** 2026-10-15

## Context

The rules require synthetic, football-realistic data, and none is provided: the official repository contains only rules and learning links. The demo needs specific stories (a late press, a red card) to happen on cue, and judges should be able to replay exactly what the video shows.

## Decision

Write a seeded simulator in Go (`internal/sim`) that generates complete matches. The same seed and scenario always produce the same match, whatever speed it is replayed at. Scenarios (`scenarios/*.json`) steer a match by scaling a team's pressing and attacking between two minutes, and by scripting cards. They shift probabilities but never fix the result.

## Consequences

- Fully reproducible demos and tests: `seed 7` with `late-press.json` is the demo match.
- Counts directly towards the rules' "data innovation" area and the "optimized synthetic data creation" judging point.
- Realism has to be calibrated by hand. Tests guard volumes: 600 to 1,500 passes a match, at most 12 goals.
- The simulator does not track player positions between events, so there are no distance-covered or sprint metrics (see [ADR 0004](0004-simulator-possession-model.md)).

## Alternatives considered

- **Public open event datasets.** Rejected: they are real matches, which the rules exclude, and they come with licences to check.
- **Generating events with a language model.** Rejected: not reproducible, slow, and hard to make internally consistent (scores, possession, clock).

## In the code

`internal/sim`, `cmd/simulator`, `scenarios/`.
