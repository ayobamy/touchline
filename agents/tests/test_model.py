"""The model interface: FakeModel replays fixtures; FoundryModel reads only the environment."""

from __future__ import annotations

import asyncio
from pathlib import Path

import pytest

from touchline_agents.model import (
    DEPLOYMENT_ENV,
    ENDPOINT_ENV,
    ConfigError,
    FakeModel,
    FoundryConfig,
    FoundryModel,
    MissingFixtureError,
    ModelError,
    ModelNotWiredError,
    ModelRequest,
    RecordingModel,
)

FOUNDRY_ENV = {
    ENDPOINT_ENV: "https://example.invalid/api/projects/touchline",
    DEPLOYMENT_ENV["moment_detector"]: "small",
    DEPLOYMENT_ENV["narrator"]: "large",
    DEPLOYMENT_ENV["verifier"]: "small",
}


def request(moment_id: str = "m-0001-mo-001") -> ModelRequest:
    return ModelRequest("narrator", moment_id, "instructions", {"x": 1})


def test_fake_model_replays_replies_in_order_and_copies_them() -> None:
    model = FakeModel({"narrator": {"m-0001-mo-001": [{"reply": {"n": 1}}, {"reply": {"n": 2}}]}})

    async def scenario() -> None:
        first = await model.respond(request())
        first["n"] = 99
        assert await model.respond(request()) == {"n": 2}
        with pytest.raises(MissingFixtureError, match="#3"):
            await model.respond(request())

    asyncio.run(scenario())
    assert model.calls("narrator", "m-0001-mo-001") == 3


def test_fake_model_can_simulate_a_failure() -> None:
    model = FakeModel({"narrator": {"m-0001-mo-001": [{"error": "rate limited"}]}})
    with pytest.raises(ModelError, match="rate limited"):
        asyncio.run(model.respond(request()))


def test_recorded_replies_replay_through_fake_model(tmp_path: Path) -> None:
    recorder = RecordingModel(FakeModel({"narrator": {"m-0001-mo-001": [{"reply": {"ok": True}}]}}))
    asyncio.run(recorder.respond(request()))
    path = tmp_path / "fixtures.json"
    recorder.save(path)
    assert asyncio.run(FakeModel.from_file(path).respond(request())) == {"ok": True}


def test_foundry_config_names_every_missing_variable() -> None:
    with pytest.raises(ConfigError) as err:
        FoundryConfig.from_env({ENDPOINT_ENV: "https://example.invalid"})
    for variable in DEPLOYMENT_ENV.values():
        assert variable in str(err.value)


def test_foundry_model_reads_its_settings_and_is_not_wired_yet() -> None:
    model = FoundryModel.from_env(FOUNDRY_ENV)
    assert model.config.deployments["narrator"] == "large"
    with pytest.raises(ModelNotWiredError, match="'large'"):
        asyncio.run(model.respond(request()))


def test_no_secret_settings_are_read() -> None:
    names = {ENDPOINT_ENV, *DEPLOYMENT_ENV.values()}
    assert not any("KEY" in n or "SECRET" in n or "TOKEN" in n for n in names)
