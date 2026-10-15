"""The orchestrator: polls candidate moments and runs each one through
Moment Detector -> Narrator -> Verifier (up to two rewrites) -> Director.

Control flow lives here, in plain asyncio, not in any model (ADR 0022):

- every step has a deadline from the latency budget, capped by what is left
  of the moment's total;
- a missed deadline or a failed agent gives the affected channels a template
  overlay, marked fallback: true;
- a draft still rejected after two rewrites is dropped (for a goal, the
  template is published instead, because every goal reaches the screen);
- the Director places overlays and drops stale ones; the publisher refuses
  anything that fails the overlay schema;
- every step is written to the trace.
"""

from __future__ import annotations

import asyncio
import logging
import time
from collections import Counter
from collections.abc import Callable, Mapping, Sequence
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any, TextIO

from touchline_agents.clock import MatchClock
from touchline_agents.config import (
    ALWAYS_SURFACE,
    FALLBACK_KINDS,
    MAX_REWRITES,
    Budget,
    layout_for,
)
from touchline_agents.contracts import (
    DEFAULT_PROFILES,
    Brief,
    Channel,
    Moment,
    Overlay,
    Profile,
    Skip,
    Verdict,
    channels_for,
)
from touchline_agents.detector import MomentDetector
from touchline_agents.director import Director, MatchTime
from touchline_agents.evidence import Evidence, gather
from touchline_agents.fallback import fallback_overlays
from touchline_agents.model import Model, ModelError, load_instructions
from touchline_agents.narrator import Limits, Narrator, build_overlay
from touchline_agents.publish import InvalidOverlayError, Publisher, ValidatingPublisher
from touchline_agents.schema import OverlayValidator
from touchline_agents.touchline import ToolError, Tools
from touchline_agents.trace import Trace
from touchline_agents.verifier import Verifier

log = logging.getLogger(__name__)

AGENT_FAILURES = (TimeoutError, ModelError, ToolError)
"""What a step may fail with and still be recovered by a template."""

Deadline = Callable[[float], float]


def _failure(exc: BaseException) -> tuple[str, str]:
    status = "timeout" if isinstance(exc, TimeoutError) else "error"
    return status, str(exc) or type(exc).__name__


@dataclass(frozen=True, slots=True)
class Written:
    """What came out of the Narrator and Verifier for one moment."""

    approved: list[Overlay]
    failed: list[Channel]
    """Channels whose agent timed out or failed: they get a template."""
    exhausted: list[Channel]
    """Channels still rejected after the last rewrite: dropped, except for goals."""


AGENT_STEPS = frozenset({"detector", "narrator", "verifier", "rewrite"})


@dataclass(frozen=True, slots=True)
class RunSummary:
    moments: int
    published: int
    fallbacks: int
    skipped: int
    dropped: int
    agent_failures: int
    """Agent steps that timed out or failed; each was recovered by a template or a drop."""
    errors: int
    """Failures in the orchestrator itself. Anything above zero is a bug."""

    @classmethod
    def from_trace(cls, trace: Trace) -> RunSummary:
        counts = Counter((r.step, r.status) for r in trace.records)
        failed = {"timeout", "error"}
        return cls(
            moments=len({r.moment_id for r in trace.records}),
            published=counts[("publish", "published")],
            fallbacks=sum(
                1 for r in trace.records if r.step == "publish" and r.detail.get("fallback")
            ),
            skipped=counts[("detector", "skip")] + counts[("detector", "stale")],
            dropped=counts[("director", "dropped")]
            + counts[("publish", "invalid")]
            + counts[("rewrite", "exhausted")],
            agent_failures=sum(
                n
                for (step, status), n in counts.items()
                if step in AGENT_STEPS and status in failed
            ),
            errors=sum(
                n
                for (step, status), n in counts.items()
                if step not in AGENT_STEPS and status in failed
            ),
        )


@dataclass(frozen=True, slots=True)
class Settings:
    """How the orchestrator runs. The defaults match docs/03 and docs/06."""

    profiles: tuple[Profile, ...] = DEFAULT_PROFILES
    budget: Budget = field(default_factory=Budget)
    concurrency: int = 4
    poll_interval_s: float = 0.25
    instructions_dir: Path | None = None
    schema_path: Path | None = None
    trace_sink: TextIO | None = None


class Orchestrator:
    def __init__(
        self,
        *,
        tools: Tools,
        model: Model,
        publisher: Publisher,
        clock: MatchClock,
        settings: Settings | None = None,
    ) -> None:
        config = settings or Settings()
        self.tools = tools
        self.clock = clock
        self.budget = config.budget
        self.trace = Trace(config.trace_sink)
        self.validator = OverlayValidator(config.schema_path)
        self.channels = channels_for(list(config.profiles))
        self.director = Director()
        self.publisher = ValidatingPublisher(publisher, self.validator)
        limits = Limits(
            headline=self.validator.max_length("headline"),
            body=self.validator.max_length("body"),
            metrics=self.validator.max_items("metrics"),
        )
        instructions = config.instructions_dir
        self.detector = MomentDetector(model, load_instructions("moment_detector", instructions))
        self.narrator = Narrator(model, load_instructions("narrator", instructions), limits)
        self.verifier = Verifier(model, load_instructions("verifier", instructions), self.validator)
        self._concurrency = 1 if clock.sequential else max(1, config.concurrency)
        self._poll_interval_s = config.poll_interval_s
        self._match: Mapping[str, Any] | None = None
        self._director_lock = asyncio.Lock()

    async def run(self, *, stop_at_full_time: bool = True) -> RunSummary:
        """Poll list_moments and process every moment until full time."""
        gate = asyncio.Semaphore(self._concurrency)
        tasks: set[asyncio.Task[None]] = set()

        async def limited(moment: Moment) -> None:
            async with gate:
                await self.process_safely(moment)

        async def dispatch(moments: list[Moment]) -> None:
            for moment in moments:
                if self.clock.sequential:
                    await self.process_safely(moment)
                else:
                    task = asyncio.create_task(limited(moment), name=moment.id)
                    tasks.add(task)
                    task.add_done_callback(tasks.discard)

        after = 0
        while True:
            moments, after = await self.tools.list_moments(after)
            await dispatch(moments)
            if moments:
                continue
            if stop_at_full_time and (await self.tools.get_status()).get("finished"):
                final, after = await self.tools.list_moments(after)
                await dispatch(final)
                break
            await asyncio.sleep(self._poll_interval_s)
        if tasks:
            await asyncio.gather(*tasks)
        return RunSummary.from_trace(self.trace)

    async def process_safely(self, moment: Moment) -> list[Overlay]:
        """process(), but a bug in one moment is logged and traced instead of
        stopping the match."""
        try:
            return await self.process(moment)
        except Exception as exc:
            log.exception("moment %s failed", moment.id)
            self.trace.add(moment.id, "orchestrator", "error", error=repr(exc))
            return []

    async def process(self, moment: Moment) -> list[Overlay]:
        """Run one moment through the four agents. Returns what was published."""
        self.clock.observe(moment)
        loop = asyncio.get_running_loop()
        deadline_at = loop.time() + self.budget.total

        def left(step_budget: float) -> float:
            return max(0.0, min(step_budget, deadline_at - loop.time()))

        now = await self.clock.now()
        layout = layout_for(moment.kind)
        if MatchTime(moment.period, moment.clock_ms + layout.ttl_ms) <= now:
            self.trace.add(
                moment.id, "detector", "stale", reason=f"its time on screen ended before {now}"
            )
            return []

        started = time.perf_counter()
        try:
            async with asyncio.timeout(left(self.budget.detector)):
                evidence = await gather(self.tools, moment)
                decision = await self.detector.decide(moment, evidence)
        except AGENT_FAILURES as exc:
            status, error = _failure(exc)
            self.trace.add(moment.id, "detector", status, since=started, error=error)
            if moment.kind not in FALLBACK_KINDS:
                return []
            return await self._direct(moment, await self._fallbacks(moment, self.channels, None))

        if isinstance(decision, Skip):
            self.trace.add(moment.id, "detector", "skip", since=started, reason=decision.reason)
            return []
        brief = decision
        self.trace.add(
            moment.id,
            "detector",
            "surface",
            since=started,
            priority=brief.priority,
            forced=brief.forced,
            evidence_event_ids=list(brief.evidence_event_ids),
        )

        written = await self._write_and_verify(moment, brief, evidence, left)
        overlays = list(written.approved)
        templated = list(written.failed)
        if moment.kind in ALWAYS_SURFACE:
            templated += written.exhausted
        if templated:
            overlays += await self._fallbacks(moment, templated, brief.priority)
        return await self._direct(moment, overlays)

    async def _write_and_verify(
        self, moment: Moment, brief: Brief, evidence: Evidence, left: Deadline
    ) -> Written:
        started = time.perf_counter()
        try:
            async with asyncio.timeout(left(self.budget.narrator)):
                drafts = await self.narrator.draft(moment, brief, evidence, self.channels)
        except AGENT_FAILURES as exc:
            status, error = _failure(exc)
            self.trace.add(moment.id, "narrator", status, since=started, error=error)
            return Written([], list(self.channels), [])
        missing = [c for c in self.channels if c not in drafts]
        self.trace.add(
            moment.id,
            "narrator",
            "drafted",
            since=started,
            drafts=len(drafts),
            missing=[c.to_json() for c in missing],
        )

        pending = {c: build_overlay(d, moment, brief) for c, d in drafts.items()}
        approved: list[Overlay] = []
        rewrites = 0
        while pending:
            started = time.perf_counter()
            try:
                async with asyncio.timeout(left(self.budget.verifier)):
                    verdicts = await self.verifier.review(moment, evidence, list(pending.values()))
            except AGENT_FAILURES as exc:
                status, error = _failure(exc)
                self.trace.add(moment.id, "verifier", status, since=started, error=error)
                return Written(approved, missing + list(pending), [])

            rejected: list[tuple[Overlay, Verdict]] = []
            for channel, overlay in list(pending.items()):
                verdict = verdicts[overlay["id"]]
                if verdict.approved:
                    approved.append(pending.pop(channel))
                else:
                    rejected.append((overlay, verdict))
            self._trace_verdicts(moment, started, rewrites, verdicts)
            if not rejected:
                break
            if rewrites == MAX_REWRITES:
                self.trace.add(
                    moment.id,
                    "rewrite",
                    "exhausted",
                    rewrites=rewrites,
                    overlay_ids=[o["id"] for o, _ in rejected],
                )
                return Written(approved, missing, list(pending))

            rewrites += 1
            started = time.perf_counter()
            try:
                async with asyncio.timeout(left(self.budget.rewrite)):
                    fixed = await self.narrator.rewrite(
                        moment, brief, evidence, [(o, v.problems) for o, v in rejected]
                    )
            except AGENT_FAILURES as exc:
                status, error = _failure(exc)
                self.trace.add(
                    moment.id, "rewrite", status, since=started, attempt=rewrites, error=error
                )
                return Written(approved, missing + list(pending), [])
            for channel, draft in fixed.items():
                if channel in pending:
                    pending[channel] = build_overlay(draft, moment, brief)
            self.trace.add(
                moment.id,
                "rewrite",
                "rewritten",
                since=started,
                attempt=rewrites,
                drafts=len(fixed),
            )
        return Written(approved, missing, [])

    def _trace_verdicts(
        self, moment: Moment, started: float, attempt: int, verdicts: Mapping[str, Verdict]
    ) -> None:
        rejected = [v for v in verdicts.values() if not v.approved]
        self.trace.add(
            moment.id,
            "verifier",
            "rejected" if rejected else "approved",
            since=started,
            attempt=attempt,
            approved=[v.overlay_id for v in verdicts.values() if v.approved],
            rejected=[
                {
                    "overlay_id": v.overlay_id,
                    "checked_by": v.checked_by,
                    "problems": [p.to_json() for p in v.problems],
                }
                for v in rejected
            ],
        )

    async def _fallbacks(
        self, moment: Moment, channels: Sequence[Channel], priority: int | None
    ) -> list[Overlay]:
        overlays = fallback_overlays(moment, channels, await self._get_match(), priority=priority)
        self.trace.add(
            moment.id,
            "fallback",
            "template",
            overlay_ids=[o["id"] for o in overlays],
            channels=[c.to_json() for c in channels],
        )
        return overlays

    async def _get_match(self) -> Mapping[str, Any]:
        """Team and player names for templates, fetched once. Templates fall back
        to IDs if the server cannot answer."""
        if self._match is None:
            try:
                self._match = await self.tools.get_match()
            except ToolError:
                log.warning("get_match failed; templates will use IDs instead of names")
                return {}
        return self._match

    async def _direct(self, moment: Moment, overlays: list[Overlay]) -> list[Overlay]:
        """Director and publish, under the Director's own deadline. It is not
        capped by the moment's total: a template must still get its chance."""
        if not overlays:
            return []
        started = time.perf_counter()
        published: list[Overlay] = []
        placed: list[str] = []
        try:
            async with asyncio.timeout(self.budget.director), self._director_lock:
                now = await self.clock.now()
                for decision in self.director.schedule(overlays, now):
                    overlay = decision.overlay
                    if decision.action == "drop":
                        self.trace.add(
                            moment.id,
                            "director",
                            "dropped",
                            overlay_id=overlay["id"],
                            reason=decision.reason,
                        )
                        continue
                    placed.append(overlay["id"])
                    await self._publish(moment, overlay, decision.reason, started, published)
        except (TimeoutError, ToolError) as exc:
            status, error = _failure(exc)
            self.trace.add(moment.id, "director", status, since=started, error=error)
            done = {o["id"] for o in published}
            for overlay_id in placed:
                if overlay_id not in done:
                    self.director.release(overlay_id)
        return published

    async def _publish(
        self,
        moment: Moment,
        overlay: Overlay,
        reason: str,
        started: float,
        published: list[Overlay],
    ) -> None:
        try:
            await self.publisher.publish(overlay)
        except InvalidOverlayError as exc:
            self.director.release(overlay["id"])
            self.trace.add(
                moment.id, "publish", "invalid", overlay_id=overlay["id"], errors=exc.errors
            )
            return
        published.append(overlay)
        self.trace.add(
            moment.id,
            "publish",
            "published",
            since=started,
            overlay_id=overlay["id"],
            fallback=bool(overlay.get("fallback")),
            reason=reason,
        )
