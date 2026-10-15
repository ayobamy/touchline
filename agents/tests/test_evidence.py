"""Evidence packs come only from time-anchored tools, and number matching follows rounding."""

from __future__ import annotations

import asyncio

from support import TOOLS_FIXTURE, mid

from touchline_agents.contracts import Moment
from touchline_agents.evidence import Fact, gather, matching, spell_ranges, supported
from touchline_agents.touchline import FakeTools, TimeRange


def at(period: int, clock_ms: int) -> Moment:
    return Moment("m", "m-0001", "goal", period, clock_ms, "", ("e1",))


def test_spells_are_the_five_minutes_before_the_moment_and_the_five_before_that() -> None:
    assert spell_ranges(at(2, 3_818_578)) == (
        TimeRange(2, 3_218_578, 3_518_578),
        TimeRange(2, 3_518_578, 3_818_578),
    )


def test_spells_stay_inside_the_half() -> None:
    a, b = spell_ranges(at(2, 2_700_000 + 7 * 60_000)) or ((), ())
    assert a == TimeRange(2, 2_700_000, 2_820_000)
    assert b == TimeRange(2, 2_820_000, 3_120_000)
    assert spell_ranges(at(1, 90_000)) is None, "too early in the half for two spells"


def test_evidence_never_asks_for_values_that_describe_now(moments: dict[str, Moment]) -> None:
    tools = FakeTools.from_file(TOOLS_FIXTURE)
    evidence = asyncio.run(gather(tools, moments[mid("mo-018")]))
    assert {name for name, _ in tools.calls} == {"get_event", "compare_periods"}
    assert set(evidence.events) == set(moments[mid("mo-018")].event_ids)
    assert set(evidence.spells) == {"pressure_index", "momentum"}


def test_facts_skip_positions_and_raw_clock_values(moments: dict[str, Moment]) -> None:
    evidence = asyncio.run(gather(FakeTools.from_file(TOOLS_FIXTURE), moments[mid("mo-001")]))
    names = [f.name for f in evidence.facts()]
    assert "event.m-0001-00234.derived.xg" in names
    assert not any(n.endswith((".x", ".y", ".seq", ".clock_ms")) for n in names)
    assert "match_minute" in names


def test_numbers_match_when_rounded_as_written() -> None:
    facts = [Fact("pressure_index", 9.41), Fact("momentum", -0.53)]
    assert supported("9.4", facts)
    assert supported("9", facts)
    assert supported("0.53", facts), "signs are ignored"
    assert not supported("9.5", facts)
    assert not supported("9.40", [Fact("x", 9.46)])
    assert matching("not a number", facts) is None
