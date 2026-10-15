"""Code checks Moment Detector replies: goals cannot be skipped; evidence stays in bounds."""

from __future__ import annotations

import pytest
from support import mid

from touchline_agents.contracts import Brief, Moment, Skip
from touchline_agents.detector import parse_decision
from touchline_agents.model import ModelError


def test_a_skip_is_honoured_for_ordinary_moments(moments: dict[str, Moment]) -> None:
    decision = parse_decision(moments[mid("mo-026")], {"decision": "skip", "reason": "routine"})
    assert decision == Skip(mid("mo-026"), "routine")


def test_a_goal_cannot_be_skipped(moments: dict[str, Moment]) -> None:
    goal = moments[mid("mo-003")]
    decision = parse_decision(goal, {"decision": "skip", "reason": "boring"})
    assert isinstance(decision, Brief)
    assert decision.forced
    assert decision.evidence_event_ids == goal.event_ids
    assert decision.priority == 1


def test_evidence_outside_the_moment_is_ignored(moments: dict[str, Moment]) -> None:
    moment = moments[mid("mo-018")]
    reply = {
        "decision": "surface",
        "evidence_event_ids": ["m-0001-00001", moment.event_ids[0]],
        "suggested_priority": 9,
    }
    decision = parse_decision(moment, reply)
    assert isinstance(decision, Brief)
    assert decision.evidence_event_ids == (moment.event_ids[0],)
    assert decision.priority == 2, "an out-of-range priority takes the kind's default"


def test_a_reply_with_no_valid_evidence_cites_the_whole_moment(moments: dict[str, Moment]) -> None:
    moment = moments[mid("mo-018")]
    decision = parse_decision(moment, {"decision": "surface", "evidence_event_ids": ["nope"]})
    assert isinstance(decision, Brief)
    assert decision.evidence_event_ids == moment.event_ids


def test_a_malformed_reply_is_a_model_error(moments: dict[str, Moment]) -> None:
    with pytest.raises(ModelError, match="decision"):
        parse_decision(moments[mid("mo-018")], {"answer": "yes"})
