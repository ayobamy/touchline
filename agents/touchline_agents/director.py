"""Director: decides what goes on screen, in code (ADR 0024).

Rules, in order:

1. **Stale overlays are dropped.** An overlay whose show_at_ms + ttl_ms has
   passed on the match clock never reaches a screen; a late overlay is worse
   than none. Time is compared as (period, clock_ms), so a first-half overlay
   is stale once the second half starts.
2. **One overlay per slot per channel.** Two overlays conflict when they share
   a slot and a locale, and their audiences match (or either is "all").
3. **Priority wins.** A more urgent overlay (lower number) replaces a less
   urgent one already in the slot. Otherwise the newcomer waits until the slot
   frees, if at least 2 s of its time-to-live would remain, or is dropped.
   On a tie the overlay already on screen keeps the slot, so nothing flickers.
"""

from __future__ import annotations

from collections.abc import Mapping, Sequence
from dataclasses import dataclass
from typing import Any, Literal

from touchline_agents.contracts import Overlay

MIN_TTL_MS = 2_000
"""The shortest time an overlay may stay on screen (schema minimum for ttl_ms)."""


@dataclass(frozen=True, slots=True, order=True)
class MatchTime:
    period: int
    clock_ms: int

    @classmethod
    def from_status(cls, status: Mapping[str, Any]) -> MatchTime:
        return cls(int(status["period"]), int(status["clock_ms"]))

    def __str__(self) -> str:
        minutes, seconds = divmod(self.clock_ms // 1000, 60)
        return f"P{self.period} {minutes:02d}:{seconds:02d}"


def starts_at(overlay: Overlay) -> MatchTime:
    return MatchTime(overlay["period"], overlay["show_at_ms"])


def ends_at(overlay: Overlay) -> MatchTime:
    return MatchTime(overlay["period"], overlay["show_at_ms"] + overlay["ttl_ms"])


def is_stale(overlay: Overlay, now: MatchTime) -> bool:
    return ends_at(overlay) <= now


def conflicts(a: Overlay, b: Overlay) -> bool:
    if a["slot"] != b["slot"] or a["locale"] != b["locale"]:
        return False
    return a["audience"] == b["audience"] or "all" in (a["audience"], b["audience"])


@dataclass(frozen=True, slots=True)
class Decision:
    overlay: Overlay
    action: Literal["publish", "drop"]
    reason: str


class Director:
    """Holds what is on screen and places new overlays. Not thread-safe; the
    orchestrator calls it from one asyncio task at a time."""

    def __init__(self) -> None:
        self._on_screen: list[Overlay] = []

    @property
    def on_screen(self) -> list[Overlay]:
        return list(self._on_screen)

    def schedule(self, overlays: Sequence[Overlay], now: MatchTime) -> list[Decision]:
        """Place overlays, most urgent first, and say what happened to each."""
        self._on_screen = [o for o in self._on_screen if not is_stale(o, now)]
        ordered = sorted(overlays, key=lambda o: (o["priority"], o["show_at_ms"], o["id"]))
        return [self._place(o, now) for o in ordered]

    def release(self, overlay_id: str) -> None:
        """Free the slot of an overlay that was scheduled but not published."""
        self._on_screen = [o for o in self._on_screen if o["id"] != overlay_id]

    def _place(self, overlay: Overlay, now: MatchTime) -> Decision:
        if is_stale(overlay, now):
            return Decision(overlay, "drop", f"stale: ended {ends_at(overlay)}, clock is {now}")

        start = max(starts_at(overlay), now)
        rivals = [o for o in self._on_screen if conflicts(o, overlay) and ends_at(o) > start]
        stronger = [o for o in rivals if o["priority"] <= overlay["priority"]]
        reason = "on screen"
        if stronger:
            free_at = max(ends_at(o) for o in stronger)
            end = ends_at(overlay)
            if free_at.period != end.period or end.clock_ms - free_at.clock_ms < MIN_TTL_MS:
                held = ", ".join(o["id"] for o in stronger)
                return Decision(overlay, "drop", f"{overlay['slot']} is held by {held}")
            overlay = overlay.copy()
            overlay["show_at_ms"] = free_at.clock_ms
            overlay["ttl_ms"] = end.clock_ms - free_at.clock_ms
            reason = f"waits for {', '.join(o['id'] for o in stronger)}"

        weaker = [o for o in rivals if o["priority"] > overlay["priority"]]
        if weaker:
            replaced = {o["id"] for o in weaker}
            self._on_screen = [o for o in self._on_screen if o["id"] not in replaced]
            reason += f"; replaces {', '.join(sorted(replaced))}"
        self._on_screen.append(overlay)
        return Decision(overlay, "publish", reason)
