"""The tool client: recordings round-trip, and unknown calls fail loudly."""

from __future__ import annotations

import asyncio
import json
from pathlib import Path
from typing import Any

import pytest
from support import TOOLS_FIXTURE

from touchline_agents.touchline import FakeTools, RecordingTools, ToolError


def test_fake_tools_list_moments_in_pages() -> None:
    tools = FakeTools.from_file(TOOLS_FIXTURE)
    moments, after = asyncio.run(tools.list_moments(0))
    assert (len(moments), after) == (42, 42)
    assert asyncio.run(tools.list_moments(42)) == ([], 42)


def test_an_unrecorded_call_is_a_tool_error() -> None:
    with pytest.raises(ToolError, match="no recorded result"):
        asyncio.run(FakeTools.from_file(TOOLS_FIXTURE).get_event("m-0001-99999"))


def test_results_are_copies() -> None:
    tools = FakeTools.from_file(TOOLS_FIXTURE)
    first = asyncio.run(tools.get_match())
    first["venue"] = "changed"
    assert asyncio.run(tools.get_match())["venue"] == "Millbrook Stadium"


def test_a_recording_replays_through_fake_tools(tmp_path: Path) -> None:
    async def scenario() -> dict[str, Any]:
        recorder = RecordingTools(FakeTools.from_file(TOOLS_FIXTURE))
        status = await recorder.get_status()
        path = tmp_path / "tools.json"
        recorder.save(path)
        assert json.loads(path.read_text())[0]["tool"] == "get_status"
        return status

    status = asyncio.run(scenario())
    replay = FakeTools.from_file(tmp_path / "tools.json")
    assert asyncio.run(replay.get_status()) == status


def test_a_recording_must_be_a_list(tmp_path: Path) -> None:
    path = tmp_path / "bad.json"
    path.write_text("{}")
    with pytest.raises(TypeError):
        FakeTools.from_file(path)
