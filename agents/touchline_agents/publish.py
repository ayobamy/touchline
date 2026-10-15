"""Overlay publishers. Every publisher the orchestrator uses is wrapped in
`ValidatingPublisher`, so nothing reaches a screen without passing
schema/overlay.schema.json (ADR 0008)."""

from __future__ import annotations

import json
from typing import Protocol, TextIO

from touchline_agents.contracts import Overlay
from touchline_agents.schema import OverlayValidator


class Publisher(Protocol):
    async def publish(self, overlay: Overlay) -> None: ...


class InvalidOverlayError(Exception):
    def __init__(self, overlay_id: str, errors: list[str]) -> None:
        super().__init__(f"{overlay_id}: " + "; ".join(errors))
        self.overlay_id = overlay_id
        self.errors = errors


class ValidatingPublisher:
    def __init__(self, inner: Publisher, validator: OverlayValidator) -> None:
        self.inner = inner
        self._validator = validator

    async def publish(self, overlay: Overlay) -> None:
        errors = self._validator.errors(overlay)
        if errors:
            raise InvalidOverlayError(str(overlay.get("id", "?")), errors)
        await self.inner.publish(overlay)


class MemoryPublisher:
    """Keeps published overlays in a list, for tests."""

    def __init__(self) -> None:
        self.overlays: list[Overlay] = []

    async def publish(self, overlay: Overlay) -> None:
        self.overlays.append(overlay)


class NdjsonPublisher:
    """Writes one overlay per line, e.g. to stdout or a file."""

    def __init__(self, stream: TextIO) -> None:
        self._stream = stream

    async def publish(self, overlay: Overlay) -> None:
        self._stream.write(json.dumps(overlay, ensure_ascii=False) + "\n")
        self._stream.flush()
