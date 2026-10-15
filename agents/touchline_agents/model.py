"""One interface for every model call, so the orchestrator never depends on a vendor SDK.

- `FakeModel` replays recorded replies; tests and offline runs use it.
- `RecordingModel` wraps another model and saves its replies as fixtures.
- `FoundryModel` is the Microsoft Foundry implementation, still a stub: it
  reads its settings from the environment and refuses to run until wired.

Each agent sends a `ModelRequest` whose input is plain JSON and expects a JSON
object back. Shapes are described in agents/instructions/<agent>.md.
"""

from __future__ import annotations

import asyncio
import copy
import json
import os
import time
from collections import Counter
from collections.abc import Mapping, Sequence
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Protocol

from touchline_agents.contracts import MODEL_AGENTS, AgentName

INSTRUCTIONS_DIR = Path(__file__).resolve().parent.parent / "instructions"


def load_instructions(agent: str, directory: Path | None = None) -> str:
    """The system instructions for an agent, from agents/instructions/<agent>.md."""
    return ((directory or INSTRUCTIONS_DIR) / f"{agent}.md").read_text(encoding="utf-8")


@dataclass(frozen=True, slots=True)
class ModelRequest:
    agent: AgentName
    moment_id: str
    instructions: str
    input: Mapping[str, Any]


class ModelError(Exception):
    """The model failed or answered in a shape the agent cannot use."""


class MissingFixtureError(ModelError):
    """FakeModel has no recorded reply for this call."""


class ModelNotWiredError(ModelError):
    """FoundryModel has its settings but no client yet."""


class Model(Protocol):
    async def respond(self, request: ModelRequest) -> dict[str, Any]:
        """Answer one request with a JSON object, or raise ModelError."""
        ...


Fixtures = Mapping[str, Mapping[str, Sequence[Mapping[str, Any]]]]
"""fixtures[agent][moment_id] is the list of replies for successive calls.

Each entry is {"reply": {...}}, optionally with "delay_s" to simulate a slow
model, or {"error": "..."} to simulate a failure.
"""


class FakeModel:
    """Replays recorded replies in order, per agent and moment."""

    def __init__(self, fixtures: Fixtures) -> None:
        self._fixtures = fixtures
        self._calls: Counter[tuple[str, str]] = Counter()
        self.requests: list[ModelRequest] = []

    @classmethod
    def from_file(cls, path: Path) -> FakeModel:
        data = json.loads(path.read_text(encoding="utf-8"))
        unknown = set(data) - set(MODEL_AGENTS) - {"_comment"}
        if unknown:
            raise ValueError(f"{path}: unknown agents {sorted(unknown)}")
        return cls({k: v for k, v in data.items() if k in MODEL_AGENTS})

    async def respond(self, request: ModelRequest) -> dict[str, Any]:
        self.requests.append(request)
        key = (request.agent, request.moment_id)
        n = self._calls[key]
        self._calls[key] += 1
        entries = self._fixtures.get(request.agent, {}).get(request.moment_id, ())
        if n >= len(entries):
            raise MissingFixtureError(
                f"no recorded reply #{n + 1} for {request.agent} on {request.moment_id}"
            )
        entry = entries[n]
        delay = float(entry.get("delay_s", 0))
        if delay > 0:
            await asyncio.sleep(delay)
        if "error" in entry:
            raise ModelError(str(entry["error"]))
        reply = entry.get("reply")
        if not isinstance(reply, Mapping):
            raise ModelError(f"fixture for {request.agent} on {request.moment_id} has no reply")
        return copy.deepcopy(dict(reply))

    def calls(self, agent: str, moment_id: str) -> int:
        return self._calls[(agent, moment_id)]


class RecordingModel:
    """Passes requests to another model and keeps its replies in fixture format."""

    def __init__(self, inner: Model) -> None:
        self.inner = inner
        self.fixtures: dict[str, dict[str, list[dict[str, Any]]]] = {}

    async def respond(self, request: ModelRequest) -> dict[str, Any]:
        started = time.perf_counter()
        reply = await self.inner.respond(request)
        latency_ms = round((time.perf_counter() - started) * 1000)
        per_agent = self.fixtures.setdefault(request.agent, {})
        per_agent.setdefault(request.moment_id, []).append(
            {"reply": copy.deepcopy(reply), "latency_ms": latency_ms}
        )
        return reply

    def save(self, path: Path) -> None:
        path.write_text(
            json.dumps(self.fixtures, indent=2, sort_keys=True) + "\n", encoding="utf-8"
        )


ENDPOINT_ENV = "FOUNDRY_PROJECT_ENDPOINT"
DEPLOYMENT_ENV: dict[AgentName, str] = {
    "moment_detector": "FOUNDRY_MOMENT_DETECTOR_DEPLOYMENT",
    "narrator": "FOUNDRY_NARRATOR_DEPLOYMENT",
    "verifier": "FOUNDRY_VERIFIER_DEPLOYMENT",
}


class ConfigError(Exception):
    """Required settings are missing from the environment."""


@dataclass(frozen=True, slots=True)
class FoundryConfig:
    """Where the models live. Nothing here is a secret: authentication will use
    Microsoft Entra ID (DefaultAzureCredential locally, managed identity in
    Azure), never a key in code or in the repository."""

    project_endpoint: str
    deployments: Mapping[AgentName, str]

    @classmethod
    def from_env(cls, env: Mapping[str, str] | None = None) -> FoundryConfig:
        source = os.environ if env is None else env
        names = [ENDPOINT_ENV, *DEPLOYMENT_ENV.values()]
        missing = [n for n in names if not source.get(n, "").strip()]
        if missing:
            raise ConfigError("set " + ", ".join(missing) + " (see docs/07-implementation.md)")
        return cls(
            project_endpoint=source[ENDPOINT_ENV].strip(),
            deployments={agent: source[var].strip() for agent, var in DEPLOYMENT_ENV.items()},
        )


class FoundryModel:
    """Microsoft Foundry models, one deployment per agent. Not wired yet.

    Wiring plan: create one Foundry agent per AgentName from its instructions
    file and deployment, send `request.input` as the user message, require a
    JSON reply, and return it parsed. Until then every call raises
    ModelNotWiredError, so the orchestrator publishes template fallbacks.
    """

    def __init__(self, config: FoundryConfig) -> None:
        self.config = config

    @classmethod
    def from_env(cls, env: Mapping[str, str] | None = None) -> FoundryModel:
        return cls(FoundryConfig.from_env(env))

    async def respond(self, request: ModelRequest) -> dict[str, Any]:
        deployment = self.config.deployments[request.agent]
        raise ModelNotWiredError(
            f"FoundryModel is not wired yet: {request.agent} would call deployment "
            f"{deployment!r} in {self.config.project_endpoint}"
        )
