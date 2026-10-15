"""Shared test helpers. Model calls use FakeModel with recorded fixtures; tool
calls use FakeTools with tool results recorded from the Go server (seed 7,
scenarios/late-press.json), so these tests need neither Azure nor Go."""

from __future__ import annotations

import json
from collections.abc import Mapping
from pathlib import Path
from typing import Any

from touchline_agents.clock import MatchClock
from touchline_agents.config import Budget
from touchline_agents.contracts import Moment
from touchline_agents.model import FakeModel, Fixtures, Model
from touchline_agents.orchestrator import Orchestrator, Settings
from touchline_agents.publish import MemoryPublisher
from touchline_agents.touchline import FakeTools

AGENTS = Path(__file__).resolve().parent.parent
FIXTURES = AGENTS / "fixtures"
TOOLS_FIXTURE = FIXTURES / "late_press_tools.json"
MODEL_FIXTURE = FIXTURES / "late_press_model.json"

FAST = Budget().scaled(0.05)
"""The real latency budget at 1/20 scale: 75 ms detector, 150 ms narrator."""

GOALS = ("mo-001", "mo-003", "mo-006", "mo-009", "mo-017", "mo-032", "mo-037")
"""The seven goals of seed 7 late-press (Ironbridge win 5-2)."""


def mid(short: str) -> str:
    """Full moment ID from the short form used in tests, e.g. "mo-018"."""
    return f"m-0001-{short}"


def recorded_model_fixtures() -> dict[str, Any]:
    data: dict[str, Any] = json.loads(MODEL_FIXTURE.read_text(encoding="utf-8"))
    data.pop("_comment", None)
    return data


def recorded_moments() -> dict[str, Moment]:
    entries = json.loads(TOOLS_FIXTURE.read_text(encoding="utf-8"))
    for entry in entries:
        if entry["tool"] == "list_moments" and entry["arguments"] == {"after": 0}:
            return {m["id"]: Moment.from_json(m) for m in entry["result"]["moments"]}
    raise AssertionError("late_press_tools.json has no list_moments(after=0) recording")


def build(
    model: Model,
    clock: MatchClock,
    *,
    tools: FakeTools | None = None,
    budget: Budget = FAST,
) -> tuple[Orchestrator, MemoryPublisher]:
    """An orchestrator over the recorded tools, publishing to memory.
    Call it inside the event loop: the orchestrator owns an asyncio.Lock."""
    publisher = MemoryPublisher()
    orchestrator = Orchestrator(
        tools=tools or FakeTools.from_file(TOOLS_FIXTURE),
        model=model,
        publisher=publisher,
        clock=clock,
        settings=Settings(budget=budget),
    )
    return orchestrator, publisher


def fake(fixtures: Fixtures | Mapping[str, Any] | None = None) -> FakeModel:
    return FakeModel(fixtures if fixtures is not None else recorded_model_fixtures())
