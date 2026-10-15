# ADR 0022: The orchestrator is a plain asyncio pipeline; models sit behind one interface

- **Status:** Accepted, implemented (Foundry models not wired yet)
- **Date:** 2026-10-15

## Context

The Best Multi-Agent System category judges distinct roles, handoffs, and recovery from
failure ([ADR 0019](0019-target-best-multi-agent-system.md)). The recovery rules are precise:
a deadline per step from the latency budget, at most two Verifier rewrites, templates on
timeout, stale overlays dropped. They must be testable without Azure, since the Foundry
project does not exist yet, and the risk table in [02 Plan](../02-plan.md#risks) asks for a
thin orchestrator so a fallback to direct model calls stays a small change.

Microsoft Agent Framework offers graph and handoff orchestration, but putting control flow
inside a framework would make deadlines and retry limits harder to test exactly and tie
every test to the framework.

## Decision

- **Control flow is our code.** `touchline_agents/orchestrator.py` polls `list_moments` and
  runs each moment through Moment Detector, Narrator, Verifier (with up to two rewrites) and
  Director, using `asyncio.timeout` for each step's deadline. A deadline is the step's budget
  from [03 Architecture](../03-architecture.md#latency-budget) or what remains of the moment's
  7.8 s, whichever is smaller. Moments run concurrently (four at a time by default).
- **One model interface.** Each agent sends a `ModelRequest` (agent, moment ID, instructions,
  JSON input) and gets a JSON object back. `FakeModel` replays recorded replies, including
  delays and errors; `RecordingModel` saves real replies as fixtures; `FoundryModel` will call
  Foundry and is a stub that reads `FOUNDRY_PROJECT_ENDPOINT` and one deployment name per agent
  from the environment. Agent Framework can live inside `FoundryModel` without touching
  anything else.
- **Instructions are files.** `agents/instructions/<agent>.md`, loaded at start-up and
  reviewed like code; a test checks that the Narrator's limits match the overlay schema.
- **Every step is traced** with the moment ID, ready for Redis and the trace panel.
- **Two clocks.** `ServerClock` reads the live match clock; `MomentClock` treats each moment's
  own time as now, which replays a finished match in seconds and is what the tests and
  `make agents` use.
- **Tooling.** Python 3.12+, dependencies pinned exactly in `pyproject.toml` and compiled with
  hashes into `requirements.txt` and `requirements-dev.txt`; ruff, mypy `--strict`, pytest
  with warnings as errors, and coverage of at least 90%, all in CI.

## Consequences

- The behaviours the demo depends on (a rejected number rewritten, a forced timeout falling
  back, a stale overlay dropped) are deterministic tests that run in seconds.
- The whole pipeline runs on a laptop with recorded replies (`make agents`).
- Agent Framework is not yet in the code. If its orchestration features become worth
  showing, they can wrap the same agents; the README must not claim it until then.
- Recorded fixtures are hand-written until Foundry is wired; `--record-model` replaces them
  with real replies.

## Alternatives considered

- **Agent Framework workflows for control flow.** Rejected for now: harder to test deadlines
  exactly, and it needs Foundry before anything runs.
- **One process per agent over Redis lists.** The target cloud shape
  ([ADR 0018](0018-redis-for-shared-state.md)); premature before the agents work in one
  process. Each handoff is already a typed message, so splitting later is mechanical.

## In the code

`agents/touchline_agents/orchestrator.py`, `model.py`, `config.py`, `clock.py`, `trace.py`,
`agents/instructions/`, `agents/tests/`.
