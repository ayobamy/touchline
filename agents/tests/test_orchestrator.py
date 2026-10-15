"""Recovery paths in the orchestrator beyond the headline tests."""

from __future__ import annotations

import asyncio
import json
from collections.abc import Sequence
from typing import Any

import pytest
from support import TOOLS_FIXTURE, build, fake, mid, recorded_model_fixtures

from touchline_agents import orchestrator as orchestrator_module
from touchline_agents.clock import ManualClock, ServerClock
from touchline_agents.contracts import Channel, Moment, Overlay
from touchline_agents.fallback import fallback_overlays
from touchline_agents.touchline import FakeTools
from touchline_agents.trace import TraceRecord


def slow(agent: str, moment_id: str, fixtures: dict[str, Any], *, call: int = 0) -> dict[str, Any]:
    """Make one recorded reply arrive after every deadline."""
    entries = fixtures[agent][moment_id]
    entries[call] = {**entries[call], "delay_s": 10}
    return fixtures


def test_a_verifier_timeout_gives_the_pending_channels_templates(
    moments: dict[str, Moment],
) -> None:
    moment = moments[mid("mo-001")]
    fixtures = slow("verifier", moment.id, recorded_model_fixtures())

    async def scenario() -> None:
        orchestrator, publisher = build(fake(fixtures), ManualClock(moment.period, moment.clock_ms))
        await orchestrator.process(moment)
        assert orchestrator.trace.find(moment.id, "verifier")[0].status == "timeout"
        assert [o.get("fallback") for o in publisher.overlays] == [True, True]

    asyncio.run(scenario())


def test_a_rewrite_timeout_gives_the_rejected_channel_a_template(
    moments: dict[str, Moment],
) -> None:
    moment = moments[mid("mo-018")]
    fixtures = slow("narrator", moment.id, recorded_model_fixtures(), call=1)

    async def scenario() -> None:
        orchestrator, publisher = build(fake(fixtures), ManualClock(moment.period, moment.clock_ms))
        await orchestrator.process(moment)
        (rewrite,) = orchestrator.trace.find(moment.id, "rewrite")
        assert rewrite.status == "timeout"
        by_channel = {(o["audience"], o.get("fallback", False)) for o in publisher.overlays}
        assert by_channel == {("analyst", True), ("casual", False)}

    asyncio.run(scenario())


def test_a_channel_the_narrator_left_out_gets_a_template(moments: dict[str, Moment]) -> None:
    moment = moments[mid("mo-001")]
    fixtures = recorded_model_fixtures()
    drafts = fixtures["narrator"][moment.id][0]["reply"]["drafts"]
    fixtures["narrator"][moment.id][0]["reply"]["drafts"] = drafts[:1]  # analyst only
    fixtures["verifier"][moment.id][0]["reply"]["verdicts"] = [
        {"overlay_id": "ov-m-0001-mo-001-analyst-en", "approved": True}
    ]

    async def scenario() -> None:
        orchestrator, publisher = build(fake(fixtures), ManualClock(moment.period, moment.clock_ms))
        await orchestrator.process(moment)
        assert orchestrator.trace.find(moment.id, "narrator")[0].detail["missing"] == [
            {"audience": "casual", "locale": "pcm"}
        ]
        fallback = {o["locale"]: o.get("fallback", False) for o in publisher.overlays}
        assert fallback == {"en": False, "pcm": True}

    asyncio.run(scenario())


def test_an_invalid_overlay_is_never_published_and_frees_its_slot(
    moments: dict[str, Moment], monkeypatch: pytest.MonkeyPatch
) -> None:
    moment = moments[mid("mo-003")]

    def broken(
        m: Moment, channels: Sequence[Channel], match: Any, *, priority: int | None = None
    ) -> list[Overlay]:
        overlays = fallback_overlays(m, channels, match, priority=priority)
        for overlay in overlays:
            overlay["ttl_ms"] = 60_000  # the schema allows at most 30 s
        return overlays

    monkeypatch.setattr(orchestrator_module, "fallback_overlays", broken)

    async def scenario() -> None:
        orchestrator, publisher = build(fake({}), ManualClock(moment.period, moment.clock_ms))
        await orchestrator.process(moment)
        assert publisher.overlays == []
        invalid = orchestrator.trace.find(moment.id, "publish")
        assert [r.status for r in invalid] == ["invalid", "invalid"]
        assert any("ttl_ms" in e for e in invalid[0].detail["errors"])
        assert orchestrator.director.on_screen == []

    asyncio.run(scenario())


def test_templates_use_ids_when_the_match_cannot_be_fetched(moments: dict[str, Moment]) -> None:
    moment = moments[mid("mo-003")]
    entries = json.loads(TOOLS_FIXTURE.read_text(encoding="utf-8"))
    tools = FakeTools([e for e in entries if e["tool"] != "get_match"])

    async def scenario() -> None:
        orchestrator, publisher = build(
            fake({}), ManualClock(moment.period, moment.clock_ms), tools=tools
        )
        await orchestrator.process(moment)
        assert "riv" in publisher.overlays[0]["headline"]

    asyncio.run(scenario())


def test_against_a_finished_match_every_moment_is_too_late() -> None:
    """With the server's clock at full time, nothing from the match is still fresh."""

    async def scenario() -> None:
        tools = FakeTools.from_file(TOOLS_FIXTURE)
        orchestrator, publisher = build(fake(), ServerClock(tools), tools=tools)
        summary = await orchestrator.run()
        assert (summary.moments, summary.skipped, summary.published) == (42, 42, 0)
        assert publisher.overlays == []

    asyncio.run(scenario())


def test_a_bug_in_one_moment_is_traced_not_raised(
    moments: dict[str, Moment], monkeypatch: pytest.MonkeyPatch
) -> None:
    moment = moments[mid("mo-001")]

    async def explode(*_args: object, **_kwargs: object) -> None:
        raise RuntimeError("boom")

    monkeypatch.setattr(orchestrator_module, "gather", explode)

    async def scenario() -> None:
        orchestrator, publisher = build(fake(), ManualClock(moment.period, moment.clock_ms))
        assert await orchestrator.process_safely(moment) == []
        (record,) = orchestrator.trace.find(moment.id, "orchestrator")
        assert record.status == "error"
        assert publisher.overlays == []

    asyncio.run(scenario())


def test_trace_records_read_as_one_line() -> None:
    record = TraceRecord(
        "m-0001-mo-018",
        "verifier",
        "rejected",
        3,
        {
            "reason": "x",
            "rejected": [
                {"overlay_id": "ov-1", "checked_by": "code", "problems": [{"claim": "7.4"}]}
            ],
        },
    )
    line = record.describe()
    assert "m-0001-mo-018 verifier" in line
    assert "rejected ov-1 (code): 7.4" in line
