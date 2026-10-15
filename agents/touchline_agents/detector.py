"""Moment Detector: decides whether a candidate moment deserves screen time."""

from __future__ import annotations

from typing import Any

from touchline_agents.config import ALWAYS_SURFACE, MAX_PRIORITY, MIN_PRIORITY, layout_for
from touchline_agents.contracts import Brief, Moment, Skip
from touchline_agents.evidence import Evidence
from touchline_agents.model import Model, ModelError, ModelRequest


class MomentDetector:
    def __init__(self, model: Model, instructions: str) -> None:
        self._model = model
        self._instructions = instructions

    async def decide(self, moment: Moment, evidence: Evidence) -> Brief | Skip:
        must_surface = moment.kind in ALWAYS_SURFACE
        reply = await self._model.respond(
            ModelRequest(
                agent="moment_detector",
                moment_id=moment.id,
                instructions=self._instructions,
                input={"evidence": evidence.for_model(), "must_surface": must_surface},
            )
        )
        return parse_decision(moment, reply)


def parse_decision(moment: Moment, reply: dict[str, Any]) -> Brief | Skip:
    """Turn a reply into a Brief or a Skip, enforcing what code owns.

    Goals cannot be skipped. Evidence IDs outside the moment are ignored, and
    when none are left the moment's own evidence is used, so a brief always
    cites something. A missing or out-of-range priority takes the kind's default.
    """
    decision = reply.get("decision")
    if decision not in {"surface", "skip"}:
        raise ModelError(f"moment_detector: decision must be surface or skip, got {decision!r}")

    default_priority = layout_for(moment.kind).priority
    if decision == "skip":
        if moment.kind not in ALWAYS_SURFACE:
            return Skip(moment.id, str(reply.get("reason") or "no reason given"))
        return Brief(
            moment_id=moment.id,
            headline_idea=moment.summary,
            why_it_matters=f"Every {moment.kind} reaches the screen.",
            evidence_event_ids=moment.event_ids,
            priority=default_priority,
            forced=True,
        )

    cited = [str(i) for i in reply.get("evidence_event_ids") or [] if str(i) in moment.event_ids]
    priority = reply.get("suggested_priority")
    valid_priority = (
        isinstance(priority, int)
        and not isinstance(priority, bool)
        and MIN_PRIORITY <= priority <= MAX_PRIORITY
    )
    return Brief(
        moment_id=moment.id,
        headline_idea=str(reply.get("headline_idea") or moment.summary),
        why_it_matters=str(reply.get("why_it_matters") or ""),
        evidence_event_ids=tuple(cited) or moment.event_ids,
        priority=priority if valid_priority and isinstance(priority, int) else default_priority,
    )
