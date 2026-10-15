# ADR 0023: Evidence anchored to the moment, and a Verifier that checks numbers in code first

- **Status:** Accepted, implemented
- **Date:** 2026-10-15

## Context

[ADR 0002](0002-code-computes-ai-explains.md) says agents state only numbers from the tools.
Two gaps appeared while building the agents:

1. Several tools describe the match **now**: `get_window`, `get_status`, `get_snapshot` and
   `get_player_stats`. An agent that answers 8 seconds late, or runs against a replayed match,
   would quote the right tool but the wrong minute: a pressure index from 90 minutes on an
   overlay about 63 minutes.
2. A model Verifier asked "is every number right?" can itself be wrong, and is slow.

## Decision

- **Evidence packs are time-anchored.** For each moment the orchestrator fetches, in code: the
  moment (its metrics were computed by the rules engine at that instant), `get_event` for every
  evidence ID, and `compare_periods` for pressure index and momentum over the five minutes up
  to the moment and the five before that, within the same half. Agents see only this pack.
- **The Verifier checks in code first.** Before any model call, each draft must pass the overlay
  schema; every number in its headline, body and metric values must equal a number in the pack
  when rounded as written (9.41 may be written 9.4 or 9); a metric labelled with a statistic
  (pressure index, xG, ...) must carry a value of that statistic; every cited event must be in
  the moment's evidence. Failures go back to the Narrator with the claim, the reason and, where
  known, the correct value, without calling the Verifier model.
- **The Verifier model judges words.** Only drafts that pass in code reach it, and it checks
  what code cannot: wrong player or team, an event described as something else, causes or
  comparisons the evidence does not support, tone for the audience.

## Consequences

- An overlay can no longer state a number that is not in a tool result for that moment, and a
  late or replayed run gives the same evidence as a live one, so fixtures are deterministic.
- The common failure, a wrong number, is caught in under a millisecond and the rewrite starts
  sooner.
- The number check proves origin, not meaning: "6" in a sentence about shots would pass if the
  pack holds a 6 for something else. The label check and the model Verifier cover part of
  this; claim accuracy in the evaluation set measures the rest.
- A few constants count as facts: the match minute, the five-minute window, and the top of the
  pass difficulty scale (10).

## Alternatives considered

- **Let agents call any tool.** Simpler, but late answers would quote current values.
- **Model-only verification.** Slower and itself fallible; it stays, but as the second check.

## In the code

`agents/touchline_agents/evidence.py`, `agents/touchline_agents/verifier.py`,
`agents/tests/test_verifier.py`, `agents/tests/test_evidence.py`.
