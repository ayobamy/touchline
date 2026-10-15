"""Deadlines and screen layout, owned by code rather than by any model."""

from __future__ import annotations

import dataclasses
from dataclasses import dataclass

MAX_REWRITES = 2
"""The Verifier can send a draft back this many times (docs/03-architecture.md)."""

MIN_PRIORITY = 1
MAX_PRIORITY = 5
"""Overlay priority bounds from schema/overlay.schema.json; 1 is most urgent."""


@dataclass(frozen=True, slots=True)
class Budget:
    """Per-step deadlines in seconds, from the latency budget in docs/03-architecture.md.

    The target is an overlay on screen within 8 s of its event; the first 200 ms
    belong to the stats service, so one moment gets `total` = 7.8 s here. Each
    step's deadline is its own budget or what is left of `total`, whichever is
    smaller.
    """

    detector: float = 1.5
    narrator: float = 3.0
    verifier: float = 1.5
    rewrite: float = 1.5
    director: float = 0.3
    total: float = 7.8

    def scaled(self, factor: float) -> Budget:
        """Every deadline multiplied by factor; tests use this to stay fast."""
        if factor <= 0:
            raise ValueError("factor must be positive")
        values = {f.name: getattr(self, f.name) * factor for f in dataclasses.fields(self)}
        return Budget(**values)


@dataclass(frozen=True, slots=True)
class Layout:
    """Where and for how long a kind of moment is shown, and how urgent it is."""

    slot: str
    ttl_ms: int
    priority: int


LAYOUT: dict[str, Layout] = {
    "goal": Layout("top_bar", 12_000, 1),
    "card": Layout("lower_third", 8_000, 2),
    "big_chance": Layout("lower_third", 8_000, 2),
    "press_switch": Layout("side_panel", 15_000, 2),
    "key_pass": Layout("lower_third", 8_000, 3),
    "momentum_swing": Layout("side_panel", 15_000, 3),
    "chaos_spell": Layout("side_panel", 10_000, 4),
    "player_milestone": Layout("lower_third", 8_000, 4),
}
DEFAULT_LAYOUT = Layout("lower_third", 8_000, 4)


def layout_for(kind: str) -> Layout:
    return LAYOUT.get(kind, DEFAULT_LAYOUT)


ALWAYS_SURFACE = frozenset({"goal"})
"""Kinds the Moment Detector cannot skip: every goal reaches the screen."""

FALLBACK_KINDS = frozenset({"goal", "card", "big_chance", "press_switch", "momentum_swing"})
"""Kinds that get a template overlay when the Moment Detector itself fails.

Once the Moment Detector has chosen to surface a moment, any later failure
gets a template whatever the kind; this list only covers the case where no
agent has looked at the moment yet, so the quieter kinds are left out rather
than flooding the screen with templates.
"""
