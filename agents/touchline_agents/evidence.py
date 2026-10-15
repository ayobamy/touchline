"""Evidence packs: the tool results an agent may quote, anchored to the moment's time.

Agents get numbers only from the MCP tools (ADR 0002). Tools such as
get_window and get_status describe the match *now*, which drifts from the
moment when an agent runs late or a finished match is replayed, so the pack
uses only time-anchored sources (ADR 0023):

- the moment itself, whose metrics the rules engine computed at that instant;
- get_event for every evidence event ID;
- compare_periods for pressure index and momentum over the five minutes up to
  the moment and the five minutes before that.

`facts()` flattens every number in the pack. The Verifier rejects any number
in a draft that is not one of them.
"""

from __future__ import annotations

import asyncio
from collections.abc import Iterator, Mapping
from dataclasses import dataclass
from decimal import Decimal, InvalidOperation
from typing import Any

from touchline_agents.contracts import Moment
from touchline_agents.touchline import TimeRange, Tools

PERIOD_START_MS = {1: 0, 2: 2_700_000}
WINDOW_MINUTES = 5
"""Length of the stats engine's rolling window (stats.DefaultWindowMinutes)."""
WINDOW_MS = WINDOW_MINUTES * 60_000
PASS_DIFFICULTY_MAX = 10
"""Top of the pass difficulty scale (ADR 0010), so "9.4/10" can be stated."""
MIN_SPELL_MS = 60_000
SPELL_METRICS = ("pressure_index", "momentum")

# Keys whose numbers are positions, ordering or raw clock values. They are not
# claims a viewer would read, so they are not facts an overlay may state.
_NOT_FACTS = frozenset({"x", "y", "seq", "period", "clock_ms", "from_ms", "to_ms", "start", "end"})


@dataclass(frozen=True, slots=True)
class Fact:
    """One number from a tool result, with where it came from."""

    name: str
    value: float


@dataclass(frozen=True, slots=True)
class Evidence:
    moment: Moment
    events: Mapping[str, Mapping[str, Any]]
    spells: Mapping[str, Mapping[str, Any]]

    @property
    def match_minute(self) -> int:
        """Whole minutes on the match clock, as the moment summaries count them."""
        return self.moment.clock_ms // 60_000

    def for_model(self) -> dict[str, Any]:
        """What the models see: the moment, its events and the two spells."""
        return {
            "moment": self.moment.to_json(),
            "match_minute": self.match_minute,
            "window_minutes": WINDOW_MINUTES,
            "events": [dict(self.events[i]) for i in self.moment.event_ids if i in self.events],
            "spells": {k: dict(v) for k, v in self.spells.items()},
        }

    def facts(self) -> list[Fact]:
        out = [
            Fact("match_minute", self.match_minute),
            Fact("window_minutes", WINDOW_MINUTES),
        ]
        out.extend(_walk("moment.metrics", dict(self.moment.metrics)))
        for event_id, event in self.events.items():
            out.extend(_walk(f"event.{event_id}", event))
        for metric, spell in self.spells.items():
            out.extend(_walk(f"spell.{metric}", spell))
            for side in ("a", "b"):
                rng = spell.get(side, {}).get("range", {})
                for bound in ("from_ms", "to_ms"):
                    if isinstance(rng.get(bound), int):
                        minute = rng[bound] // 60_000
                        out.append(Fact(f"spell.{metric}.{side}.{bound[:-3]}_minute", minute))
        if any("pass_difficulty" in f.name for f in out):
            out.append(Fact("pass_difficulty_max", PASS_DIFFICULTY_MAX))
        return out


def _walk(prefix: str, value: Any) -> Iterator[Fact]:
    if isinstance(value, bool):
        return
    if isinstance(value, int | float):
        yield Fact(prefix, float(value))
    elif isinstance(value, Mapping):
        for key, item in value.items():
            if key not in _NOT_FACTS:
                yield from _walk(f"{prefix}.{key}", item)
    elif isinstance(value, list):
        for i, item in enumerate(value):
            yield from _walk(f"{prefix}[{i}]", item)


def spell_ranges(moment: Moment) -> tuple[TimeRange, TimeRange] | None:
    """The five minutes up to the moment (b) and the stretch before that (a).

    Both stay inside the moment's half. None when the half is too young for
    two spells of at least a minute each.
    """
    start = PERIOD_START_MS.get(moment.period, 0)
    b_from = max(start, moment.clock_ms - WINDOW_MS)
    a_from = max(start, b_from - WINDOW_MS)
    if moment.clock_ms - b_from < MIN_SPELL_MS or b_from - a_from < MIN_SPELL_MS:
        return None
    return (
        TimeRange(moment.period, a_from, b_from),
        TimeRange(moment.period, b_from, moment.clock_ms),
    )


async def gather(tools: Tools, moment: Moment) -> Evidence:
    """Fetch the evidence pack for a moment. Tool calls run concurrently."""
    event_calls = [tools.get_event(i) for i in moment.event_ids]
    ranges = spell_ranges(moment)
    spell_calls = [tools.compare_periods(m, *ranges) for m in SPELL_METRICS] if ranges else []
    results = await asyncio.gather(*event_calls, *spell_calls)
    events = dict(zip(moment.event_ids, results[: len(event_calls)], strict=True))
    spells = dict(zip(SPELL_METRICS, results[len(event_calls) :], strict=False))
    return Evidence(moment=moment, events=events, spells=spells)


def supported(claim: str, facts: list[Fact]) -> bool:
    """True when the number written in a draft is some fact, rounded as written.

    "9" and "9.4" both match a fact of 9.41; "9.5" does not. Signs are ignored,
    since text writes momentum as "-0.53" or "0.53 towards Rovers".
    """
    return matching(claim, facts) is not None


def matching(claim: str, facts: list[Fact]) -> Fact | None:
    try:
        written = abs(Decimal(claim))
    except InvalidOperation:
        return None
    exponent = written.as_tuple().exponent
    places = -exponent if isinstance(exponent, int) and exponent < 0 else 0
    tolerance = Decimal(5) / Decimal(10) ** (places + 1)
    for fact in facts:
        if abs(abs(Decimal(repr(fact.value))) - written) <= tolerance:
            return fact
    return None
