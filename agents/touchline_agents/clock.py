"""Where the Director reads the match clock.

- `ServerClock` asks the server (get_status). Use it against a live replay.
- `MomentClock` treats each moment's own time as "now", as if the agents
  answered instantly. Use it to process a finished match quickly; it needs
  moments processed one at a time, in order.
- `ManualClock` is set by tests.
"""

from __future__ import annotations

from typing import Protocol

from touchline_agents.contracts import Moment
from touchline_agents.director import MatchTime
from touchline_agents.touchline import Tools


class MatchClock(Protocol):
    @property
    def sequential(self) -> bool:
        """True when moments must be processed one at a time, in order."""
        ...

    def observe(self, moment: Moment) -> None:
        """Called when processing of a moment starts."""
        ...

    async def now(self) -> MatchTime: ...


class ServerClock:
    sequential = False

    def __init__(self, tools: Tools) -> None:
        self._tools = tools

    def observe(self, moment: Moment) -> None:
        """The server's clock moves on its own."""

    async def now(self) -> MatchTime:
        return MatchTime.from_status(await self._tools.get_status())


class MomentClock:
    sequential = True

    def __init__(self) -> None:
        self._now = MatchTime(1, 0)

    def observe(self, moment: Moment) -> None:
        self._now = max(self._now, MatchTime(moment.period, moment.clock_ms))

    async def now(self) -> MatchTime:
        return self._now


class ManualClock:
    sequential = True

    def __init__(self, period: int = 1, clock_ms: int = 0) -> None:
        self.time = MatchTime(period, clock_ms)

    def observe(self, moment: Moment) -> None:
        """Tests move this clock by hand."""

    def set(self, period: int, clock_ms: int) -> None:
        self.time = MatchTime(period, clock_ms)

    async def now(self) -> MatchTime:
        return self.time
