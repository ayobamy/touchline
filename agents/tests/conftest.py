"""Pytest fixtures shared by the agent tests. Helpers live in support.py."""

from __future__ import annotations

import pytest
from support import recorded_moments

from touchline_agents.contracts import Moment
from touchline_agents.schema import OverlayValidator


@pytest.fixture(scope="session")
def validator() -> OverlayValidator:
    return OverlayValidator()


@pytest.fixture(scope="session")
def moments() -> dict[str, Moment]:
    return recorded_moments()
