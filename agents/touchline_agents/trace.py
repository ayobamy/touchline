"""The agent trace: one record per step, keyed by moment, for the trace panel and the demo."""

from __future__ import annotations

import json
import time
from collections.abc import Callable, Mapping
from dataclasses import asdict, dataclass, field
from datetime import UTC, datetime
from typing import Any, TextIO


@dataclass(frozen=True, slots=True)
class TraceRecord:
    moment_id: str
    step: str
    status: str
    duration_ms: int
    detail: Mapping[str, Any] = field(default_factory=dict)
    at: str = ""

    def describe(self) -> str:
        """One readable line, e.g. for a terminal."""
        parts = [f"{self.moment_id} {self.step:<12} {self.status:<10} {self.duration_ms:>5} ms"]
        parts.extend(
            f"{key}={self.detail[key]}"
            for key in ("overlay_id", "reason", "error")
            if key in self.detail
        )
        for rejected in self.detail.get("rejected", []):
            claims = ", ".join(p["claim"] for p in rejected.get("problems", []))
            parts.append(f"rejected {rejected['overlay_id']} ({rejected['checked_by']}): {claims}")
        return "  ".join(parts)


Listener = Callable[[TraceRecord], None]


class Trace:
    def __init__(self, sink: TextIO | None = None) -> None:
        self.records: list[TraceRecord] = []
        self.listeners: list[Listener] = []
        self._sink = sink

    def add(
        self, moment_id: str, step: str, status: str, *, since: float | None = None, **detail: Any
    ) -> TraceRecord:
        """Record a step. `since` is a time.perf_counter() reading from the step's start."""
        duration = round((time.perf_counter() - since) * 1000) if since is not None else 0
        record = TraceRecord(
            moment_id=moment_id,
            step=step,
            status=status,
            duration_ms=duration,
            detail=detail,
            at=datetime.now(UTC).isoformat(timespec="milliseconds"),
        )
        self.records.append(record)
        if self._sink is not None:
            self._sink.write(json.dumps(asdict(record), ensure_ascii=False, default=str) + "\n")
            self._sink.flush()
        for listener in self.listeners:
            listener(record)
        return record

    def for_moment(self, moment_id: str) -> list[TraceRecord]:
        return [r for r in self.records if r.moment_id == moment_id]

    def find(self, moment_id: str, step: str) -> list[TraceRecord]:
        return [r for r in self.records if r.moment_id == moment_id and r.step == step]
