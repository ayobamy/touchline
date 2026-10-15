# ADR 0024: The Director and the fallback templates are code

- **Status:** Accepted, implemented
- **Date:** 2026-10-15

## Context

[06 Agents](../06-agents.md#roles) describes the Director as "code first; model only to break
ties". Its job is arithmetic on the match clock with a 300 ms budget: drop what is stale, keep
one overlay per slot, let urgent overlays win. Template overlays are needed exactly when a model
has failed, so they cannot depend on one.

## Decision

- **Director rules, in code:** drop an overlay once `show_at_ms + ttl_ms` has passed, comparing
  `(period, clock_ms)`; one overlay per slot per channel (same slot and locale, and the same
  audience or `all`); a more urgent overlay replaces a less urgent one; otherwise the newcomer
  waits until the slot frees if at least 2 s would remain, or is dropped; a tie keeps what is
  on screen. No model, even for ties.
- **Code owns timing and placement.** Slot, time on screen and default priority come from the
  moment's kind; `show_at_ms` is the moment's clock. Models write words only.
- **Templates** are built from the moment alone (names, kind, metrics), in English and Nigerian
  Pidgin, cite the moment's evidence, and are marked `fallback: true` with producer
  `template`. Momentum is shown from the moment's team's side.
- **When templates are used:** a timeout or failure after the Moment Detector has surfaced a
  moment; a Moment Detector failure on a goal, card, big chance, press switch or momentum swing;
  and a goal whose drafts are still rejected after two rewrites. Other drafts that fail twice
  are dropped.
- **The publisher validates every overlay** against `schema/overlay.schema.json` immediately
  before publishing, templates included; an invalid one frees its slot and is traced.

## Consequences

- Scheduling is predictable and fully unit-tested; the demo can show a drop or a fallback on
  purpose.
- Personalization (club focus raising priority, player-focus lower thirds) is not in the
  Director yet; it belongs with the viewer in milestone 3.
- Templates are plain. They are a safety net, not the experience.

## Alternatives considered

- **A model Director.** Slower than the budget allows and no better at arithmetic.
- **Dropping instead of templating on timeout.** Loses goals; the docs and the demo promise a
  visible recovery.

## In the code

`agents/touchline_agents/director.py`, `fallback.py`, `publish.py`, `config.py`,
`agents/instructions/director.md`, `agents/tests/test_director.py`,
`agents/tests/test_fallback.py`.
