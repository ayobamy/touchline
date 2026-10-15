"""The command line: clear failures without a server or settings, and a full local replay."""

from __future__ import annotations

import json
import shutil
from pathlib import Path

import pytest

from touchline_agents.__main__ import load_profiles, main
from touchline_agents.model import DEPLOYMENT_ENV, ENDPOINT_ENV
from touchline_agents.schema import OverlayValidator


def test_foundry_without_settings_exits_with_a_message(
    monkeypatch: pytest.MonkeyPatch, caplog: pytest.LogCaptureFixture
) -> None:
    for name in (ENDPOINT_ENV, *DEPLOYMENT_ENV.values()):
        monkeypatch.delenv(name, raising=False)
    assert main(["--model", "foundry", "--quiet"]) == 2
    assert ENDPOINT_ENV in caplog.text


def test_an_unreachable_server_exits_with_a_message(caplog: pytest.LogCaptureFixture) -> None:
    assert main(["--mcp-url", "http://127.0.0.1:9/mcp", "--quiet"]) == 2
    assert "make serve" in caplog.text


def test_profiles_load_from_a_file(tmp_path: Path) -> None:
    path = tmp_path / "profiles.json"
    path.write_text(json.dumps([{"profile_id": "p", "audience": "casual", "locale": "en"}]))
    (profile,) = load_profiles(path)
    assert profile.channel.locale == "en"
    path.write_text("[]")
    with pytest.raises(ValueError, match="non-empty"):
        load_profiles(path)


@pytest.mark.integration
def test_a_local_replay_runs_end_to_end(tmp_path: Path, validator: OverlayValidator) -> None:
    if shutil.which("go") is None:
        pytest.skip("Go is not on PATH")
    out, trace, recorded = tmp_path / "o.ndjson", tmp_path / "t.ndjson", tmp_path / "m.json"
    argv = ["--replay", "--quiet", "--out", str(out), "--trace", str(trace)]
    assert main([*argv, "--record-model", str(recorded)]) == 0

    overlays = [json.loads(line) for line in out.read_text().splitlines()]
    assert overlays
    assert all(validator.errors(o) == [] for o in overlays)
    steps = {json.loads(line)["step"] for line in trace.read_text().splitlines()}
    assert {"detector", "narrator", "verifier", "rewrite", "publish"} <= steps
    assert "m-0001-mo-018" in json.loads(recorded.read_text())["narrator"]
