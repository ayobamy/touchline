"""Narrator: writes one draft overlay per (audience, locale), and rewrites rejected ones.

The Narrator writes words. Code fills everything else - ID, timing, slot,
priority, team and player, and the evidence when the draft does not narrow it
- so a model never decides when or where something appears.
"""

from __future__ import annotations

from collections.abc import Sequence
from dataclasses import dataclass
from typing import Any

from touchline_agents.config import layout_for
from touchline_agents.contracts import Brief, Channel, Metric, Moment, Overlay, Problem
from touchline_agents.evidence import Evidence
from touchline_agents.model import Model, ModelError, ModelRequest


@dataclass(frozen=True, slots=True)
class Limits:
    """On-screen size limits, read from the overlay schema."""

    headline: int
    body: int
    metrics: int

    def to_json(self) -> dict[str, int]:
        return {"headline_chars": self.headline, "body_chars": self.body, "metrics": self.metrics}


@dataclass(frozen=True, slots=True)
class Draft:
    channel: Channel
    headline: str
    body: str
    metrics: tuple[Metric, ...]
    evidence_event_ids: tuple[str, ...]


def overlay_id(moment: Moment, channel: Channel, *, fallback: bool = False) -> str:
    suffix = "-fallback" if fallback else ""
    return f"ov-{moment.id}-{channel.audience}-{channel.locale}{suffix}"


def build_overlay(draft: Draft, moment: Moment, brief: Brief) -> Overlay:
    layout = layout_for(moment.kind)
    overlay: Overlay = {
        "id": overlay_id(moment, draft.channel),
        "match_id": moment.match_id,
        "period": moment.period,
        "show_at_ms": moment.clock_ms,
        "ttl_ms": layout.ttl_ms,
        "priority": brief.priority,
        "slot": layout.slot,
        "audience": draft.channel.audience,
        "locale": draft.channel.locale,
        "headline": draft.headline,
        "evidence_event_ids": list(draft.evidence_event_ids or brief.evidence_event_ids),
        "producer": "narrator",
    }
    if moment.team_id:
        overlay["team_id"] = moment.team_id
    if moment.player_id:
        overlay["player_id"] = moment.player_id
    if draft.body:
        overlay["body"] = draft.body
    if draft.metrics:
        overlay["metrics"] = list(draft.metrics)
    return overlay


class Narrator:
    def __init__(self, model: Model, instructions: str, limits: Limits) -> None:
        self._model = model
        self._instructions = instructions
        self._limits = limits

    async def draft(
        self, moment: Moment, brief: Brief, evidence: Evidence, channels: Sequence[Channel]
    ) -> dict[Channel, Draft]:
        reply = await self._ask(
            moment,
            {
                "brief": brief.to_json(),
                "evidence": evidence.for_model(),
                "channels": [c.to_json() for c in channels],
                "limits": self._limits.to_json(),
            },
        )
        return parse_drafts(reply, channels)

    async def rewrite(
        self,
        moment: Moment,
        brief: Brief,
        evidence: Evidence,
        rejected: Sequence[tuple[Overlay, Sequence[Problem]]],
    ) -> dict[Channel, Draft]:
        channels = [Channel(o["audience"], o["locale"]) for o, _ in rejected]
        reply = await self._ask(
            moment,
            {
                "brief": brief.to_json(),
                "evidence": evidence.for_model(),
                "channels": [c.to_json() for c in channels],
                "limits": self._limits.to_json(),
                "rejected": [
                    {"draft": _draft_view(o), "problems": [p.to_json() for p in problems]}
                    for o, problems in rejected
                ],
            },
        )
        return parse_drafts(reply, channels)

    async def _ask(self, moment: Moment, payload: dict[str, Any]) -> dict[str, Any]:
        request = ModelRequest("narrator", moment.id, self._instructions, payload)
        return await self._model.respond(request)


def _draft_view(overlay: Overlay) -> dict[str, Any]:
    """The parts of a rejected overlay the Narrator wrote, to send back for a rewrite."""
    view: dict[str, Any] = {
        "audience": overlay["audience"],
        "locale": overlay["locale"],
        "headline": overlay["headline"],
        "evidence_event_ids": overlay["evidence_event_ids"],
    }
    if "body" in overlay:
        view["body"] = overlay["body"]
    if "metrics" in overlay:
        view["metrics"] = overlay["metrics"]
    return view


def parse_drafts(reply: dict[str, Any], channels: Sequence[Channel]) -> dict[Channel, Draft]:
    """Drafts for the requested channels. Extra channels are ignored; a missing
    channel is simply absent, and the orchestrator gives it a template."""
    raw = reply.get("drafts")
    if not isinstance(raw, list):
        raise ModelError("narrator: reply needs a drafts list")
    wanted = set(channels)
    out: dict[Channel, Draft] = {}
    for item in raw:
        if not isinstance(item, dict):
            raise ModelError("narrator: every draft must be an object")
        channel = Channel(str(item.get("audience", "")), str(item.get("locale", "")))
        if channel not in wanted:
            continue
        out[channel] = Draft(
            channel=channel,
            headline=str(item.get("headline") or "").strip(),
            body=str(item.get("body") or "").strip(),
            metrics=_metrics(item.get("metrics")),
            evidence_event_ids=tuple(str(i) for i in item.get("evidence_event_ids") or ()),
        )
    return out


def _metrics(raw: Any) -> tuple[Metric, ...]:
    if raw is None:
        return ()
    if not isinstance(raw, list):
        raise ModelError("narrator: metrics must be a list")
    out: list[Metric] = []
    for item in raw:
        if not isinstance(item, dict):
            raise ModelError("narrator: every metric must be an object")
        metric: Metric = {"label": str(item.get("label", "")), "value": str(item.get("value", ""))}
        if item.get("unit"):
            metric["unit"] = str(item["unit"])
        out.append(metric)
    return tuple(out)
