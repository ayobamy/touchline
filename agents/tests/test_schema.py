"""Every published overlay passes schema/overlay.schema.json; nothing else is published."""

from __future__ import annotations

import asyncio
from typing import Any

import pytest
from support import GOALS, build, fake, mid

from touchline_agents.clock import MomentClock
from touchline_agents.contracts import MODEL_AGENTS, Overlay
from touchline_agents.model import INSTRUCTIONS_DIR, load_instructions
from touchline_agents.orchestrator import RunSummary
from touchline_agents.publish import InvalidOverlayError, MemoryPublisher, ValidatingPublisher
from touchline_agents.schema import OverlayValidator


@pytest.mark.parametrize(
    "fixtures",
    [None, {}],
    ids=["recorded-replies", "no-replies-all-templates"],
)
def test_every_published_overlay_passes_the_schema(
    fixtures: dict[str, Any] | None, validator: OverlayValidator
) -> None:
    async def scenario() -> tuple[list[Overlay], RunSummary]:
        orchestrator, publisher = build(fake(fixtures), MomentClock())
        summary = await orchestrator.run()
        return publisher.overlays, summary

    published, summary = asyncio.run(scenario())
    assert summary.moments == 42
    assert summary.errors == 0
    assert published, "a full match must publish something"
    for overlay in published:
        assert validator.errors(overlay) == [], overlay["id"]
        assert overlay["evidence_event_ids"], "no evidence, no overlay"
    on_screen = {"-".join(o["id"].split("-")[1:5]) for o in published}
    assert {mid(g) for g in GOALS} <= on_screen, "every goal reaches the screen"


def test_the_publisher_refuses_an_invalid_overlay(validator: OverlayValidator) -> None:
    inner = MemoryPublisher()
    gate = ValidatingPublisher(inner, validator)
    overlay: Overlay = {
        "id": "ov-bad",
        "match_id": "m-0001",
        "period": 1,
        "show_at_ms": 0,
        "ttl_ms": 8000,
        "priority": 2,
        "slot": "lower_third",
        "audience": "casual",
        "locale": "pcm",
        "headline": "No evidence here",
        "evidence_event_ids": [],
        "producer": "narrator",
    }
    with pytest.raises(InvalidOverlayError, match="evidence_event_ids"):
        asyncio.run(gate.publish(overlay))
    assert inner.overlays == []


def test_the_orchestrator_always_publishes_through_the_schema_gate() -> None:
    async def scenario() -> None:
        orchestrator = build(fake(), MomentClock())[0]
        assert isinstance(orchestrator.publisher, ValidatingPublisher)

    asyncio.run(scenario())


def test_every_model_agent_has_instructions() -> None:
    for agent in MODEL_AGENTS:
        assert load_instructions(agent).startswith("# ")
    assert (INSTRUCTIONS_DIR / "director.md").is_file()


def test_narrator_instructions_quote_the_schema_limits(validator: OverlayValidator) -> None:
    text = load_instructions("narrator")
    assert f"Headline at most {validator.max_length('headline')} characters" in text
    assert f"Body at most {validator.max_length('body')}" in text
    assert f"At most {validator.max_items('metrics')} metrics" in text
    assert "## Nigerian Pidgin (`pcm`)" in text


def test_moment_ids_in_fixtures_exist(moments: dict[str, Any]) -> None:
    assert mid("mo-018") in moments
