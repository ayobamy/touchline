"""A missed deadline or a failed agent produces a template overlay, marked fallback: true."""

from __future__ import annotations

import asyncio
from typing import Any

from support import TOOLS_FIXTURE, build, fake, mid, recorded_model_fixtures

from touchline_agents.clock import ManualClock
from touchline_agents.contracts import DEFAULT_PROFILES, Channel, Moment, channels_for
from touchline_agents.evidence import gather
from touchline_agents.fallback import MAX_BODY, MAX_HEADLINE, MAX_METRICS, fallback_overlays
from touchline_agents.schema import OverlayValidator
from touchline_agents.touchline import FakeTools
from touchline_agents.verifier import check


def test_a_forced_narrator_timeout_produces_a_fallback_overlay(
    moments: dict[str, Moment], validator: OverlayValidator
) -> None:
    moment = moments[mid("mo-017")]  # its recorded Narrator reply takes 10 s

    async def scenario() -> None:
        orchestrator, publisher = build(fake(), ManualClock(moment.period, moment.clock_ms))
        await orchestrator.process(moment)

        (narrator,) = orchestrator.trace.find(moment.id, "narrator")
        assert narrator.status == "timeout"
        assert narrator.duration_ms < 1000, "the deadline, not the 10 s reply, ends the step"
        assert len(publisher.overlays) == 2
        for overlay in publisher.overlays:
            assert overlay.get("fallback") is True
            assert overlay["producer"] == "template"
            assert overlay["id"].endswith("-fallback")
            assert overlay["evidence_event_ids"] == list(moment.event_ids)
            assert validator.errors(overlay) == []

    asyncio.run(scenario())


def test_a_failed_detector_on_a_goal_falls_back(moments: dict[str, Moment]) -> None:
    moment = moments[mid("mo-003")]  # no recorded replies at all

    async def scenario() -> None:
        orchestrator, publisher = build(fake({}), ManualClock(moment.period, moment.clock_ms))
        await orchestrator.process(moment)
        assert orchestrator.trace.find(moment.id, "detector")[0].status == "error"
        assert [o["locale"] for o in publisher.overlays] == ["en", "pcm"]
        assert all(o.get("fallback") for o in publisher.overlays)

    asyncio.run(scenario())


def test_a_timed_out_detector_on_a_quiet_moment_publishes_nothing(
    moments: dict[str, Moment],
) -> None:
    moment = moments[mid("mo-026")]  # a passing milestone
    slow: dict[str, Any] = {"moment_detector": {moment.id: [{"delay_s": 10, "reply": {}}]}}

    async def scenario() -> None:
        orchestrator, publisher = build(fake(slow), ManualClock(moment.period, moment.clock_ms))
        await orchestrator.process(moment)
        assert orchestrator.trace.find(moment.id, "detector")[0].status == "timeout"
        assert publisher.overlays == []

    asyncio.run(scenario())


def test_templates_only_state_numbers_from_the_tools(
    moments: dict[str, Moment], validator: OverlayValidator
) -> None:
    """Every template, for every moment of the match, passes the Verifier's code check."""
    tools = FakeTools.from_file(TOOLS_FIXTURE)
    match = asyncio.run(tools.get_match())
    channels = channels_for(list(DEFAULT_PROFILES))
    for moment in moments.values():
        evidence = asyncio.run(gather(tools, moment))
        for overlay in fallback_overlays(moment, channels, match):
            assert check(overlay, evidence, validator) == [], overlay


def test_template_momentum_reads_from_the_team_side(moments: dict[str, Moment]) -> None:
    moment = moments[mid("mo-002")]  # momentum -0.53: away team Ironbridge on top
    match = asyncio.run(FakeTools.from_file(TOOLS_FIXTURE).get_match())
    (overlay,) = fallback_overlays(moment, channels_for(list(DEFAULT_PROFILES))[:1], match)
    assert overlay["metrics"][0] == {"label": "Momentum", "value": "+0.53"}


def test_unknown_locales_get_english_templates(moments: dict[str, Moment]) -> None:
    moment = moments[mid("mo-001")]
    overlays = fallback_overlays(moment, [Channel("casual", "yo"), Channel("casual", "en")], {})
    assert [o["locale"] for o in overlays] == ["en"]


def test_template_limits_match_the_schema(validator: OverlayValidator) -> None:
    assert validator.max_length("headline") == MAX_HEADLINE
    assert validator.max_length("body") == MAX_BODY
    assert validator.max_items("metrics") == MAX_METRICS


def test_recorded_fixtures_do_not_cover_every_moment() -> None:
    """Moments without replies exercise the template path in the offline demo."""
    covered = set(recorded_model_fixtures()["moment_detector"])
    assert mid("mo-003") not in covered
