# Director

The Director decides what goes on screen. It runs as **code**, not as a model
(`touchline_agents/director.py`, ADR 0024): scheduling is arithmetic on the match clock, and it
must be fast (300 ms budget) and predictable. This file records the policy the code implements,
so it is reviewed like the other agents' instructions.

## Input

Verified overlays for one moment, from the Narrator and Verifier or from templates, and the
current match clock as `(period, clock_ms)`.

## Policy

1. **Drop stale overlays.** If `show_at_ms + ttl_ms` has already passed on the match clock, the
   overlay is dropped. A late overlay is worse than none. Time is compared as
   `(period, clock_ms)`, so a first-half overlay is stale once the second half starts.
2. **One overlay per slot per channel.** Two overlays conflict when they use the same slot and
   locale and their audiences match, or either audience is `all`.
3. **Priority wins.** 1 is most urgent. A more urgent overlay replaces a less urgent one in its
   slot.
4. **Otherwise wait or drop.** A less or equally urgent overlay waits until the slot is free,
   if at least 2 seconds of its time on screen would remain; its `show_at_ms` moves to that
   time and its `ttl_ms` shrinks so it still ends when it would have. Otherwise it is dropped.
5. **Ties keep what is on screen**, so nothing flickers.
6. **Publish only valid overlays.** Each one passes `schema/overlay.schema.json` immediately
   before publishing; one that fails frees its slot and is logged to the trace.

## Slots, time on screen and default priority

Code chooses these from the moment's kind; no model decides when or where a graphic appears.

| Kind | Slot | Time on screen | Default priority |
| --- | --- | --- | --- |
| goal | top_bar | 12 s | 1 |
| card | lower_third | 8 s | 2 |
| big_chance | lower_third | 8 s | 2 |
| press_switch | side_panel | 15 s | 2 |
| key_pass | lower_third | 8 s | 3 |
| momentum_swing | side_panel | 15 s | 3 |
| chaos_spell | side_panel | 10 s | 4 |
| player_milestone | lower_third | 8 s | 4 |

The Moment Detector's `suggested_priority` replaces the default when it is between 1 and 5.

## Not yet

- Per-profile adjustments from [06 Agents](../../docs/06-agents.md#personalization): club focus
  raising priority, a player-focus lower third, the favourite metric pinned to the side panel.
  These belong with the viewer in milestone 3.
- A model to break ties between equal-priority overlays. Ties are rare and keeping what is on
  screen works; add a model only if the demo shows a need.
