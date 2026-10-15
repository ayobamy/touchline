"""The Verifier rejects wrong numbers and the Narrator rewrites, at most twice."""

from __future__ import annotations

import asyncio
import copy
from typing import Any

from support import TOOLS_FIXTURE, build, fake, mid, recorded_model_fixtures

from touchline_agents.clock import ManualClock
from touchline_agents.contracts import Metric, Moment, Overlay
from touchline_agents.evidence import Evidence, gather, supported
from touchline_agents.schema import OverlayValidator
from touchline_agents.touchline import FakeTools
from touchline_agents.verifier import check


def evidence_for(moment: Moment) -> Evidence:
    return asyncio.run(gather(FakeTools.from_file(TOOLS_FIXTURE), moment))


def overlay_with(
    moment: Moment,
    *,
    headline: str = "Rovers press high",
    body: str | None = None,
    metrics: list[Metric] | None = None,
    evidence_event_ids: list[str] | None = None,
) -> Overlay:
    overlay: Overlay = {
        "id": "ov-test",
        "match_id": moment.match_id,
        "period": moment.period,
        "show_at_ms": moment.clock_ms,
        "ttl_ms": 8000,
        "priority": 2,
        "slot": "side_panel",
        "audience": "analyst",
        "locale": "en",
        "headline": headline,
        "evidence_event_ids": list(moment.event_ids)
        if evidence_event_ids is None
        else evidence_event_ids,
        "producer": "narrator",
    }
    if body is not None:
        overlay["body"] = body
    if metrics is not None:
        overlay["metrics"] = metrics
    return overlay


def test_verifier_rejects_a_planted_wrong_number_and_the_narrator_fixes_it(
    moments: dict[str, Moment],
) -> None:
    moment = moments[mid("mo-018")]
    facts = evidence_for(moment).facts()
    assert not supported("7.4", facts), "the planted number must not be in the tool results"
    assert supported("6.0", facts)

    async def scenario() -> None:
        model = fake()
        orchestrator, publisher = build(model, ManualClock(moment.period, moment.clock_ms))
        published = await orchestrator.process(moment)

        first, second = orchestrator.trace.find(moment.id, "verifier")
        assert first.status == "rejected"
        (rejected,) = first.detail["rejected"]
        assert rejected["overlay_id"] == "ov-m-0001-mo-018-analyst-en"
        assert rejected["checked_by"] == "code"
        claims = {p["claim"] for p in rejected["problems"]}
        assert claims == {"7.4"}
        assert {"correct_value": "6.0"}.items() <= next(
            p for p in rejected["problems"] if "correct_value" in p
        ).items()

        (rewrite,) = orchestrator.trace.find(moment.id, "rewrite")
        assert (rewrite.status, rewrite.detail["attempt"]) == ("rewritten", 1)
        assert second.status == "approved"
        assert model.calls("narrator", moment.id) == 2
        assert model.calls("verifier", moment.id) == 2

        assert published == publisher.overlays
        analyst = next(o for o in published if o["audience"] == "analyst")
        assert "6.0" in analyst["body"]
        assert "7.4" not in analyst["body"]
        assert analyst["metrics"][0]["value"] == "6.0"
        assert not any(o.get("fallback") for o in published)
        assert {o["locale"] for o in published} == {"en", "pcm"}

    asyncio.run(scenario())


def _always_wrong(moment_id: str, drafts: int) -> dict[str, Any]:
    """Fixtures where every narrator reply keeps a wrong number."""
    fixtures = recorded_model_fixtures()
    wrong = copy.deepcopy(fixtures["narrator"]["m-0001-mo-018"][0])
    for draft in wrong["reply"]["drafts"]:
        draft["body"] = "Pressure index 7.4."
        draft["metrics"] = [{"label": "Pressure index", "value": "7.4"}]
    fixtures["narrator"][moment_id] = [wrong] * drafts
    fixtures["moment_detector"][moment_id] = fixtures["moment_detector"]["m-0001-mo-018"]
    return fixtures


def test_rewrites_stop_after_two_and_the_moment_is_dropped(moments: dict[str, Moment]) -> None:
    moment = moments[mid("mo-018")]

    async def scenario() -> None:
        model = fake(_always_wrong(moment.id, drafts=3))
        orchestrator, publisher = build(model, ManualClock(moment.period, moment.clock_ms))
        published = await orchestrator.process(moment)

        assert published == []
        assert publisher.overlays == []
        assert model.calls("narrator", moment.id) == 3  # one draft, two rewrites
        assert [r.status for r in orchestrator.trace.find(moment.id, "verifier")] == [
            "rejected"
        ] * 3
        (exhausted,) = [
            r for r in orchestrator.trace.find(moment.id, "rewrite") if r.status == "exhausted"
        ]
        assert exhausted.detail["rewrites"] == 2

    asyncio.run(scenario())


def test_a_goal_still_rejected_after_two_rewrites_gets_a_template(
    moments: dict[str, Moment],
) -> None:
    moment = moments[mid("mo-003")]

    async def scenario() -> None:
        model = fake(_always_wrong(moment.id, drafts=3))
        orchestrator, publisher = build(model, ManualClock(moment.period, moment.clock_ms))
        await orchestrator.process(moment)
        assert publisher.overlays, "every goal reaches the screen"
        assert all(o.get("fallback") is True for o in publisher.overlays)
        assert all(o["producer"] == "template" for o in publisher.overlays)

    asyncio.run(scenario())


def test_a_model_rejection_is_rewritten_too(moments: dict[str, Moment]) -> None:
    moment = moments[mid("mo-001")]
    fixtures = recorded_model_fixtures()
    fixtures["verifier"][moment.id] = [
        {
            "reply": {
                "verdicts": [
                    {"overlay_id": "ov-m-0001-mo-001-analyst-en", "approved": True},
                    {
                        "overlay_id": "ov-m-0001-mo-001-casual-pcm",
                        "approved": False,
                        "problems": [{"claim": "Rovers dey lead", "reason": "say who scored"}],
                    },
                ]
            }
        },
        {"reply": {"verdicts": [{"overlay_id": "ov-m-0001-mo-001-casual-pcm", "approved": True}]}},
    ]
    pcm = fixtures["narrator"][moment.id][0]["reply"]["drafts"][1]
    fixtures["narrator"][moment.id].append({"reply": {"drafts": [pcm]}})

    async def scenario() -> None:
        model = fake(fixtures)
        orchestrator, publisher = build(model, ManualClock(moment.period, moment.clock_ms))
        await orchestrator.process(moment)
        first = orchestrator.trace.find(moment.id, "verifier")[0]
        assert first.detail["rejected"][0]["checked_by"] == "model"
        assert len(publisher.overlays) == 2

    asyncio.run(scenario())


def test_rounding_as_written_is_accepted_but_other_numbers_are_not(
    moments: dict[str, Moment], validator: OverlayValidator
) -> None:
    moment = moments[mid("mo-018")]
    evidence = evidence_for(moment)
    assert (
        check(overlay_with(moment, body="Pressure index 6 now, from 2.1."), evidence, validator)
        == []
    )
    problems = check(overlay_with(moment, body="Pressure index 6.5 now."), evidence, validator)
    assert [p.claim for p in problems] == ["6.5"]


def test_a_metric_must_carry_the_statistic_its_label_names(
    moments: dict[str, Moment], validator: OverlayValidator
) -> None:
    moment = moments[mid("mo-018")]
    evidence = evidence_for(moment)
    # 2.1 is a real number in the evidence, but it is the earlier pressure index...
    ok = overlay_with(moment, metrics=[{"label": "Pressure index", "value": "2.1"}])
    assert check(ok, evidence, validator) == []
    # ...while 63 (the match minute) is not a pressure index at all.
    wrong = overlay_with(moment, metrics=[{"label": "Pressure index", "value": "63"}])
    (problem,) = check(wrong, evidence, validator)
    assert (problem.claim, problem.correct_value) == ("63", "6")


def test_schema_breaks_and_unknown_evidence_are_problems(
    moments: dict[str, Moment], validator: OverlayValidator
) -> None:
    moment = moments[mid("mo-018")]
    evidence = evidence_for(moment)
    too_long = overlay_with(moment, headline="x" * 61)
    assert any("headline" in p.reason for p in check(too_long, evidence, validator))
    foreign = overlay_with(moment, evidence_event_ids=["m-0001-00001"])
    assert [p.claim for p in check(foreign, evidence, validator)] == ["m-0001-00001"]
