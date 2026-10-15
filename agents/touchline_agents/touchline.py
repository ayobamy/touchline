"""Client for the Touchline MCP tools.

`McpTools` talks to the Go server over MCP streamable HTTP with the official
Python SDK (ADR 0021). `RecordingTools` captures every call so a run can be
replayed by `FakeTools` in tests without a server.
"""

from __future__ import annotations

import json
from abc import ABC, abstractmethod
from collections.abc import Mapping
from contextlib import AsyncExitStack
from dataclasses import dataclass
from pathlib import Path
from types import TracebackType
from typing import Any, Self

from mcp import Client
from mcp.types import Implementation, TextContent

from touchline_agents import __version__
from touchline_agents.contracts import Moment


class ToolError(Exception):
    """A tool returned an error result, or could not be reached."""

    def __init__(self, tool: str, message: str) -> None:
        super().__init__(f"{tool}: {message}")
        self.tool = tool


@dataclass(frozen=True, slots=True)
class TimeRange:
    """A stretch of one half, as compare_periods expects it."""

    period: int
    from_ms: int
    to_ms: int

    def to_json(self) -> dict[str, int]:
        return {"period": self.period, "from_ms": self.from_ms, "to_ms": self.to_ms}


class Tools(ABC):
    """The Touchline tools the orchestrator uses, over any transport."""

    @abstractmethod
    async def call(self, name: str, arguments: Mapping[str, Any] | None = None) -> dict[str, Any]:
        """Call a tool and return its structured result, or raise ToolError."""

    async def get_match(self) -> dict[str, Any]:
        return await self.call("get_match")

    async def get_status(self) -> dict[str, Any]:
        return await self.call("get_status")

    async def get_event(self, event_id: str) -> dict[str, Any]:
        return await self.call("get_event", {"event_id": event_id})

    async def compare_periods(self, metric: str, a: TimeRange, b: TimeRange) -> dict[str, Any]:
        return await self.call(
            "compare_periods", {"metric": metric, "a": a.to_json(), "b": b.to_json()}
        )

    async def list_moments(self, after: int) -> tuple[list[Moment], int]:
        result = await self.call("list_moments", {"after": after})
        moments = [Moment.from_json(m) for m in result.get("moments") or []]
        return moments, int(result.get("next", after))


class McpTools(Tools):
    """Touchline tools over MCP streamable HTTP. Use as an async context manager."""

    def __init__(self, url: str, *, timeout_s: float = 2.0) -> None:
        self.url = url
        self._timeout_s = timeout_s
        self._stack: AsyncExitStack | None = None
        self._client: Client | None = None

    async def __aenter__(self) -> Self:
        stack = AsyncExitStack()
        client = Client(
            self.url,
            read_timeout_seconds=self._timeout_s,
            client_info=Implementation(name="touchline-agents", version=__version__),
        )
        try:
            self._client = await stack.enter_async_context(client)
        except BaseException:
            await stack.aclose()
            raise
        self._stack = stack
        return self

    async def __aexit__(
        self,
        exc_type: type[BaseException] | None,
        exc: BaseException | None,
        tb: TracebackType | None,
    ) -> None:
        stack, self._stack, self._client = self._stack, None, None
        if stack is not None:
            await stack.aclose()

    async def call(self, name: str, arguments: Mapping[str, Any] | None = None) -> dict[str, Any]:
        if self._client is None:
            raise RuntimeError("McpTools must be used inside `async with`")
        result = await self._client.call_tool(name, dict(arguments or {}))
        text = "".join(c.text for c in result.content if isinstance(c, TextContent))
        if result.is_error:
            raise ToolError(name, text or "tool returned an error")
        structured = result.structured_content
        if isinstance(structured, dict):
            return structured
        try:
            decoded = json.loads(text)
        except json.JSONDecodeError as exc:
            raise ToolError(name, f"result is not JSON: {text[:80]!r}") from exc
        if not isinstance(decoded, dict):
            raise ToolError(name, "result is not a JSON object")
        return decoded


def _key(name: str, arguments: Mapping[str, Any] | None) -> str:
    return json.dumps([name, dict(arguments or {})], sort_keys=True, separators=(",", ":"))


class RecordingTools(Tools):
    """Passes calls through to another Tools and remembers every answer."""

    def __init__(self, inner: Tools) -> None:
        self.inner = inner
        self.entries: dict[str, dict[str, Any]] = {}

    async def call(self, name: str, arguments: Mapping[str, Any] | None = None) -> dict[str, Any]:
        result = await self.inner.call(name, arguments)
        self.entries[_key(name, arguments)] = {
            "tool": name,
            "arguments": dict(arguments or {}),
            "result": result,
        }
        return result

    def save(self, path: Path) -> None:
        """Write the recording: one entry per line, sorted, so diffs stay readable."""
        lines = [json.dumps(self.entries[k], sort_keys=True) for k in sorted(self.entries)]
        path.write_text("[\n" + ",\n".join(lines) + "\n]\n", encoding="utf-8")


class FakeTools(Tools):
    """Answers from a recording made by RecordingTools. Unknown calls are tool errors."""

    def __init__(self, entries: list[Mapping[str, Any]]) -> None:
        self._results = {_key(e["tool"], e.get("arguments")): e["result"] for e in entries}
        self.calls: list[tuple[str, dict[str, Any]]] = []

    @classmethod
    def from_file(cls, path: Path) -> FakeTools:
        entries = json.loads(path.read_text(encoding="utf-8"))
        if not isinstance(entries, list):
            raise TypeError(f"{path}: expected a JSON list of recorded tool calls")
        return cls(entries)

    async def call(self, name: str, arguments: Mapping[str, Any] | None = None) -> dict[str, Any]:
        self.calls.append((name, dict(arguments or {})))
        try:
            result = self._results[_key(name, arguments)]
        except KeyError:
            raise ToolError(
                name, f"no recorded result for arguments {dict(arguments or {})}"
            ) from None
        # A copy, so a caller that mutates the result cannot change the recording.
        copied: dict[str, Any] = json.loads(json.dumps(result))
        return copied
