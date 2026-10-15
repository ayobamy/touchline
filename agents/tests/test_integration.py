"""End to end against the real Go server over MCP, using the official Python SDK.

Starts cmd/server (seed 7, late-press, full speed), runs the orchestrator with
FakeModel and checks the published overlays. It also checks that the server
still returns exactly what fixtures/late_press_tools.json recorded, so the
unit tests cannot drift from the Go stats engine. Skipped when Go is not on
PATH, unless TOUCHLINE_REQUIRE_INTEGRATION=1 (as in CI).
"""

from __future__ import annotations

import asyncio
import json
import os
import shutil

import pytest
from support import GOALS, TOOLS_FIXTURE, fake, mid

from touchline_agents.clock import MomentClock
from touchline_agents.devserver import replay_server
from touchline_agents.orchestrator import Orchestrator, RunSummary, Settings
from touchline_agents.publish import MemoryPublisher
from touchline_agents.schema import OverlayValidator
from touchline_agents.touchline import McpTools, RecordingTools

pytestmark = pytest.mark.integration

REGENERATE = (
    "cd agents && .venv/bin/python -m touchline_agents --replay --quiet "
    "--record-tools fixtures/late_press_tools.json --out /dev/null"
)


@pytest.fixture(autouse=True)
def _needs_go() -> None:
    if shutil.which("go") is None:
        if os.environ.get("TOUCHLINE_REQUIRE_INTEGRATION") == "1":
            pytest.fail("Go is required for the integration tests")
        pytest.skip("Go is not on PATH")


def test_a_full_replay_over_mcp(validator: OverlayValidator) -> None:
    async def scenario() -> tuple[MemoryPublisher, RunSummary, RecordingTools]:
        async with replay_server() as url, McpTools(url) as mcp:
            tools = RecordingTools(mcp)
            publisher = MemoryPublisher()
            orchestrator = Orchestrator(
                tools=tools,
                model=fake(),
                publisher=publisher,
                clock=MomentClock(),
                settings=Settings(),
            )
            summary = await orchestrator.run()
            return publisher, summary, tools

    publisher, summary, recording = asyncio.run(scenario())

    assert (summary.moments, summary.errors) == (42, 0)
    for overlay in publisher.overlays:
        assert validator.errors(overlay) == []
    on_screen = {"-".join(o["id"].split("-")[1:5]) for o in publisher.overlays}
    assert {mid(g) for g in GOALS} <= on_screen
    analyst = next(o for o in publisher.overlays if o["id"] == "ov-m-0001-mo-018-analyst-en")
    assert "6.0" in analyst["body"]

    recorded = {
        json.dumps([e["tool"], e["arguments"]], sort_keys=True): e["result"]
        for e in json.loads(TOOLS_FIXTURE.read_text(encoding="utf-8"))
    }
    for key, entry in recording.entries.items():
        if entry["tool"] == "get_status":
            continue  # the clock moves while the replay loads
        live = json.dumps([entry["tool"], entry["arguments"]], sort_keys=True)
        assert recorded.get(live) == entry["result"], f"fixture drift on {key}; run: {REGENERATE}"
