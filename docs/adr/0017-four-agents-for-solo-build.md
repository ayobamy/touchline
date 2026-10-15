# ADR 0017: Four agents for the solo build

- **Status:** Accepted, implemented in the orchestrator with recorded model replies; Foundry models still to wire ([ADR 0022](0022-orchestrator-asyncio-pipeline.md))
- **Date:** 2026-10-15

## Context

The full design has six agents: Moment Detector, Tactical Analyst, Narrator, Localizer, Verifier and Director. One person has 13 days. The Best Multi-Agent System category judges distinct roles, shared state, handoffs, failure recovery, and an outcome one agent could not achieve.

## Decision

Build four agents: Moment Detector, Narrator, Verifier and Director. Fold the Tactical Analyst's "why it matters" and the Localizer's per-language writing into the Narrator's instructions. Ship two languages, English and Nigerian Pidgin.

## Consequences

- Still shows every behaviour the category judges, including a Verifier that rejects and a Director that falls back.
- Narrator prompts carry more responsibility and are the most likely to need tuning.
- The contracts allow splitting the folded roles back out later without changing other parts.
