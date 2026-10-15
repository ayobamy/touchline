"""Verifier: checks every draft before it can reach the Director.

Code checks first, deterministically (ADR 0023):

1. the overlay passes schema/overlay.schema.json;
2. every number in the headline, body and metric values is a number from the
   moment's tool results, rounded as written;
3. a metric whose label names a statistic (pressure index, xG, ...) carries a
   value of that statistic;
4. every cited event is in the moment's evidence.

Only drafts that pass all four go to the Verifier model, which judges what
code cannot: whether the cited events support the claim in words.
"""

from __future__ import annotations

import re
from collections.abc import Iterator, Sequence
from typing import Any

from touchline_agents.contracts import Moment, Overlay, Problem, Verdict
from touchline_agents.evidence import Evidence, Fact, matching
from touchline_agents.model import Model, ModelError, ModelRequest
from touchline_agents.schema import OverlayValidator

NUMBER = re.compile(r"\d+(?:\.\d+)?")

# Words in a metric label, and the statistic the value must then be.
LABEL_STATS: tuple[tuple[str, str], ...] = (
    ("pressure", "pressure_index"),
    ("expected goals", "xg"),
    ("xg", "xg"),
    ("momentum", "momentum"),
    ("difficulty", "pass_difficulty"),
    ("shot speed", "shot_speed_kmh"),
    ("ball speed", "ball_speed_kmh"),
    ("distance", "pass_distance_m"),
    ("possession change", "chaos_per_min"),
    ("chaos", "chaos_per_min"),
    ("rhythm", "rhythm_per_min"),
    ("tackles", "tackles_won"),
    ("passes", "passes_completed"),
)


def stat_for_label(label: str) -> str | None:
    lowered = label.lower()
    for words, stat in LABEL_STATS:
        if words in lowered:
            return stat
    return None


def _texts(overlay: Overlay) -> Iterator[tuple[str, str]]:
    yield "headline", overlay["headline"]
    yield "body", overlay.get("body", "")
    for i, metric in enumerate(overlay.get("metrics", [])):
        yield f"metrics[{i}].value", metric["value"]


def _correct_value(stat: str, claim: str, evidence: Evidence) -> str | None:
    value = evidence.moment.metrics.get(stat)
    if value is None:
        return None
    places = len(claim.split(".")[1]) if "." in claim else 0
    return f"{abs(value):.{places}f}"


def check(overlay: Overlay, evidence: Evidence, validator: OverlayValidator) -> list[Problem]:
    """Every problem code can prove. Empty means the draft may go to the model."""
    problems = [
        Problem(claim=line.split(":")[0], reason=line) for line in validator.errors(overlay)
    ]
    facts = evidence.facts()

    problems.extend(
        Problem(claim, f"{claim} in the {where} is not in any tool result for this moment")
        for where, text in _texts(overlay)
        for claim in NUMBER.findall(text)
        if matching(claim, facts) is None
    )
    for metric in overlay.get("metrics", []):
        problems.extend(_label_problems(metric["label"], metric["value"], facts, evidence))

    known = set(evidence.events)
    problems.extend(
        Problem(event_id, "cites an event that is not in this moment's evidence")
        for event_id in overlay["evidence_event_ids"]
        if event_id not in known
    )
    return problems


def _label_problems(label: str, value: str, facts: list[Fact], evidence: Evidence) -> list[Problem]:
    stat = stat_for_label(label)
    numbers = NUMBER.findall(value)
    if stat is None or len(numbers) != 1:
        return []
    of_stat = [
        f
        for f in facts
        if not f.name.endswith("_minute")
        and (f.name.split(".")[-1].startswith(stat) or f".{stat}." in f.name)
    ]
    if matching(numbers[0], of_stat) is not None:
        return []
    return [
        Problem(
            claim=numbers[0],
            reason=(
                f"the {label!r} metric says {numbers[0]}, "
                f"but no {stat} in the tool results has that value"
            ),
            correct_value=_correct_value(stat, numbers[0], evidence),
        )
    ]


class Verifier:
    def __init__(self, model: Model, instructions: str, validator: OverlayValidator) -> None:
        self._model = model
        self._instructions = instructions
        self._validator = validator

    def check(self, overlay: Overlay, evidence: Evidence) -> list[Problem]:
        return check(overlay, evidence, self._validator)

    async def review(
        self, moment: Moment, evidence: Evidence, overlays: Sequence[Overlay]
    ) -> dict[str, Verdict]:
        verdicts: dict[str, Verdict] = {}
        for_model: list[Overlay] = []
        for overlay in overlays:
            problems = self.check(overlay, evidence)
            if problems:
                verdicts[overlay["id"]] = Verdict(overlay["id"], False, tuple(problems), "code")
            else:
                for_model.append(overlay)
        if not for_model:
            return verdicts

        reply = await self._model.respond(
            ModelRequest(
                agent="verifier",
                moment_id=moment.id,
                instructions=self._instructions,
                input={"evidence": evidence.for_model(), "drafts": list(for_model)},
            )
        )
        judged = parse_verdicts(reply)
        for overlay in for_model:
            verdicts[overlay["id"]] = judged.get(overlay["id"]) or Verdict(
                overlay["id"],
                False,
                (Problem("verdict", "the Verifier gave no verdict for this draft"),),
            )
        return verdicts


def parse_verdicts(reply: dict[str, Any]) -> dict[str, Verdict]:
    raw = reply.get("verdicts")
    if not isinstance(raw, list):
        raise ModelError("verifier: reply needs a verdicts list")
    out: dict[str, Verdict] = {}
    for item in raw:
        if not isinstance(item, dict) or not isinstance(item.get("approved"), bool):
            raise ModelError("verifier: every verdict needs overlay_id and approved true or false")
        problems = tuple(
            Problem(
                claim=str(p.get("claim", "")),
                reason=str(p.get("reason", "")),
                correct_value=None if p.get("correct_value") is None else str(p["correct_value"]),
            )
            for p in item.get("problems") or []
            if isinstance(p, dict)
        )
        overlay_id = str(item.get("overlay_id", ""))
        out[overlay_id] = Verdict(overlay_id, item["approved"], problems, "model")
    return out
