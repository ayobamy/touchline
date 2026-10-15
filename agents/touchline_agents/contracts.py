"""The messages that pass between the agents.

Overlays are TypedDicts, so they serialise straight to the JSON described by
schema/overlay.schema.json; that schema, not these types, is the contract that
gates publishing. Everything else is a small frozen dataclass.
"""

from __future__ import annotations

from collections.abc import Mapping
from dataclasses import dataclass, field
from typing import Any, Literal, NotRequired, TypedDict

AgentName = Literal["moment_detector", "narrator", "verifier"]
"""The agents that call a model. The Director is code only (ADR 0024)."""

MODEL_AGENTS: tuple[AgentName, ...] = ("moment_detector", "narrator", "verifier")


class Metric(TypedDict):
    label: str
    value: str
    unit: NotRequired[str]


class Overlay(TypedDict):
    id: str
    match_id: str
    period: int
    show_at_ms: int
    ttl_ms: int
    priority: int
    slot: str
    audience: str
    locale: str
    team_id: NotRequired[str]
    player_id: NotRequired[str]
    headline: str
    body: NotRequired[str]
    metrics: NotRequired[list[Metric]]
    evidence_event_ids: list[str]
    producer: str
    fallback: NotRequired[bool]


@dataclass(frozen=True, slots=True)
class Moment:
    """A candidate moment from the rules engine (internal/moments)."""

    id: str
    match_id: str
    kind: str
    period: int
    clock_ms: int
    summary: str
    event_ids: tuple[str, ...]
    team_id: str | None = None
    player_id: str | None = None
    metrics: Mapping[str, float] = field(default_factory=dict)

    @classmethod
    def from_json(cls, raw: Mapping[str, Any]) -> Moment:
        return cls(
            id=str(raw["id"]),
            match_id=str(raw["match_id"]),
            kind=str(raw["kind"]),
            period=int(raw["period"]),
            clock_ms=int(raw["clock_ms"]),
            summary=str(raw.get("summary", "")),
            event_ids=tuple(str(i) for i in raw.get("event_ids") or ()),
            team_id=raw.get("team_id") or None,
            player_id=raw.get("player_id") or None,
            metrics={str(k): float(v) for k, v in (raw.get("metrics") or {}).items()},
        )

    def to_json(self) -> dict[str, Any]:
        out: dict[str, Any] = {
            "id": self.id,
            "match_id": self.match_id,
            "kind": self.kind,
            "period": self.period,
            "clock_ms": self.clock_ms,
            "summary": self.summary,
            "event_ids": list(self.event_ids),
        }
        if self.team_id:
            out["team_id"] = self.team_id
        if self.player_id:
            out["player_id"] = self.player_id
        if self.metrics:
            out["metrics"] = dict(self.metrics)
        return out


@dataclass(frozen=True, slots=True, order=True)
class Channel:
    """One (audience, locale) pair. The Narrator writes one draft per channel."""

    audience: str
    locale: str

    def to_json(self) -> dict[str, str]:
        return {"audience": self.audience, "locale": self.locale}


@dataclass(frozen=True, slots=True)
class Profile:
    """A viewer profile, as in docs/06-agents.md#personalization."""

    profile_id: str
    audience: str
    locale: str
    club_id: str | None = None
    focus_player_id: str | None = None
    favourite_metric: str | None = None
    explain_simply: bool = False

    @property
    def channel(self) -> Channel:
        return Channel(self.audience, self.locale)

    @classmethod
    def from_json(cls, raw: Mapping[str, Any]) -> Profile:
        return cls(
            profile_id=str(raw["profile_id"]),
            audience=str(raw["audience"]),
            locale=str(raw["locale"]),
            club_id=raw.get("club_id"),
            focus_player_id=raw.get("focus_player_id"),
            favourite_metric=raw.get("favourite_metric"),
            explain_simply=bool(raw.get("explain_simply", False)),
        )


DEFAULT_PROFILES: tuple[Profile, ...] = (
    Profile("analyst-1", "analyst", "en", favourite_metric="xg"),
    Profile(
        "fan-1",
        "casual",
        "pcm",
        club_id="riv",
        focus_player_id="riv-10",
        favourite_metric="pressure_index",
        explain_simply=True,
    ),
)


def channels_for(profiles: tuple[Profile, ...] | list[Profile]) -> list[Channel]:
    """The distinct channels among the profiles, in a stable order."""
    return sorted({p.channel for p in profiles})


@dataclass(frozen=True, slots=True)
class Brief:
    """Moment Detector to Narrator: why this moment deserves screen time."""

    moment_id: str
    headline_idea: str
    why_it_matters: str
    evidence_event_ids: tuple[str, ...]
    priority: int
    forced: bool = False
    """True when code overrode a skip, because the kind must always surface."""

    def to_json(self) -> dict[str, Any]:
        return {
            "moment_id": self.moment_id,
            "headline_idea": self.headline_idea,
            "why_it_matters": self.why_it_matters,
            "evidence_event_ids": list(self.evidence_event_ids),
            "suggested_priority": self.priority,
        }


@dataclass(frozen=True, slots=True)
class Skip:
    """Moment Detector decided the moment does not deserve screen time."""

    moment_id: str
    reason: str


@dataclass(frozen=True, slots=True)
class Problem:
    """One reason a draft was rejected, sent back to the Narrator."""

    claim: str
    reason: str
    correct_value: str | None = None

    def to_json(self) -> dict[str, str]:
        out = {"claim": self.claim, "reason": self.reason}
        if self.correct_value is not None:
            out["correct_value"] = self.correct_value
        return out


@dataclass(frozen=True, slots=True)
class Verdict:
    overlay_id: str
    approved: bool
    problems: tuple[Problem, ...] = ()
    checked_by: Literal["code", "model"] = "model"
