"""The Director: stale overlays are dropped; priority decides who gets a slot."""

from __future__ import annotations

import asyncio
from dataclasses import dataclass

from support import build, fake, mid

from touchline_agents.clock import ManualClock
from touchline_agents.contracts import Moment, Overlay
from touchline_agents.director import Director, MatchTime, is_stale


@dataclass(frozen=True, slots=True)
class Spec:
    """An overlay with only the fields the Director looks at varied."""

    overlay_id: str
    show_at_ms: int = 60_000
    ttl_ms: int = 8_000
    priority: int = 3
    slot: str = "lower_third"
    audience: str = "casual"
    locale: str = "pcm"
    period: int = 1


def overlay(spec: Spec) -> Overlay:
    return {
        "id": spec.overlay_id,
        "match_id": "m-0001",
        "period": spec.period,
        "show_at_ms": spec.show_at_ms,
        "ttl_ms": spec.ttl_ms,
        "priority": spec.priority,
        "slot": spec.slot,
        "audience": spec.audience,
        "locale": spec.locale,
        "headline": spec.overlay_id,
        "evidence_event_ids": ["m-0001-00001"],
        "producer": "narrator",
    }


def test_an_overlay_is_stale_once_show_at_plus_ttl_has_passed() -> None:
    o = overlay(Spec("a", show_at_ms=60_000, ttl_ms=8_000))
    assert not is_stale(o, MatchTime(1, 67_999))
    assert is_stale(o, MatchTime(1, 68_000))
    assert is_stale(o, MatchTime(2, 2_700_000)), "a first-half overlay is stale in the second"


def test_the_director_drops_a_stale_overlay() -> None:
    (decision,) = Director().schedule([overlay(Spec("late"))], MatchTime(1, 90_000))
    assert decision.action == "drop"
    assert decision.reason.startswith("stale")


class LateClock:
    """Fresh when the moment arrives, 20 match seconds later when the Director looks."""

    sequential = True

    def __init__(self, moment: Moment) -> None:
        self._times = [
            MatchTime(moment.period, moment.clock_ms),
            MatchTime(moment.period, moment.clock_ms + 20_000),
        ]

    def observe(self, moment: Moment) -> None:
        """The times are fixed up front."""

    async def now(self) -> MatchTime:
        return self._times.pop(0) if len(self._times) > 1 else self._times[0]


def test_an_overlay_that_arrives_after_its_moment_is_dropped(moments: dict[str, Moment]) -> None:
    moment = moments[mid("mo-001")]  # goal: 12 s on screen

    async def scenario() -> None:
        orchestrator, publisher = build(fake(), LateClock(moment))
        published = await orchestrator.process(moment)
        assert published == []
        assert publisher.overlays == []
        drops = orchestrator.trace.find(moment.id, "director")
        assert len(drops) == 2
        assert all(r.status == "dropped" and r.detail["reason"].startswith("stale") for r in drops)

    asyncio.run(scenario())


def test_a_moment_already_over_never_reaches_the_agents(moments: dict[str, Moment]) -> None:
    moment = moments[mid("mo-001")]

    async def scenario() -> None:
        model = fake()
        orchestrator, publisher = build(model, ManualClock(moment.period, moment.clock_ms + 60_000))
        await orchestrator.process(moment)
        assert orchestrator.trace.find(moment.id, "detector")[0].status == "stale"
        assert model.requests == []
        assert publisher.overlays == []

    asyncio.run(scenario())


def test_a_more_urgent_overlay_replaces_a_less_urgent_one() -> None:
    director = Director()
    now = MatchTime(1, 60_000)
    director.schedule([overlay(Spec("milestone", priority=4))], now)
    (decision,) = director.schedule([overlay(Spec("goal", priority=1, show_at_ms=61_000))], now)
    assert decision.action == "publish"
    assert "replaces milestone" in decision.reason
    assert [o["id"] for o in director.on_screen] == ["goal"]


def test_a_less_urgent_overlay_waits_for_the_slot() -> None:
    director = Director()
    now = MatchTime(1, 60_000)
    director.schedule([overlay(Spec("goal", priority=1, ttl_ms=6_000))], now)
    (decision,) = director.schedule([overlay(Spec("pass", priority=3, ttl_ms=12_000))], now)
    assert decision.action == "publish"
    assert decision.reason.startswith("waits for goal")
    assert decision.overlay["show_at_ms"] == 66_000
    assert decision.overlay["ttl_ms"] == 6_000, "it still ends when it would have"


def test_a_less_urgent_overlay_is_dropped_when_no_time_would_be_left() -> None:
    director = Director()
    now = MatchTime(1, 60_000)
    director.schedule([overlay(Spec("goal", priority=1, ttl_ms=12_000))], now)
    (decision,) = director.schedule([overlay(Spec("pass", priority=3, ttl_ms=8_000))], now)
    assert decision.action == "drop"
    assert "held by goal" in decision.reason


def test_on_a_tie_the_overlay_on_screen_keeps_its_slot() -> None:
    director = Director()
    now = MatchTime(1, 60_000)
    director.schedule([overlay(Spec("first", priority=2, ttl_ms=10_000))], now)
    (decision,) = director.schedule([overlay(Spec("second", priority=2, ttl_ms=10_000))], now)
    assert decision.action == "drop"
    assert [o["id"] for o in director.on_screen] == ["first"]


def test_channels_and_slots_do_not_collide_with_each_other() -> None:
    director = Director()
    now = MatchTime(1, 60_000)
    decisions = director.schedule(
        [
            overlay(Spec("pcm", audience="casual", locale="pcm")),
            overlay(Spec("en", audience="analyst", locale="en")),
            overlay(Spec("side", slot="side_panel")),
        ],
        now,
    )
    assert [d.action for d in decisions] == ["publish"] * 3


def test_audience_all_collides_with_every_audience_in_its_locale() -> None:
    director = Director()
    now = MatchTime(1, 60_000)
    director.schedule([overlay(Spec("everyone", audience="all", priority=1))], now)
    (decision,) = director.schedule([overlay(Spec("casual", audience="casual", priority=3))], now)
    assert decision.action == "drop"


def test_released_overlays_free_their_slot() -> None:
    director = Director()
    now = MatchTime(1, 60_000)
    director.schedule([overlay(Spec("a", priority=1))], now)
    director.release("a")
    (decision,) = director.schedule([overlay(Spec("b", priority=3))], now)
    assert decision.action == "publish"
