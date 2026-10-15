"""Run the orchestrator against a Touchline server.

    python -m touchline_agents --replay           # start the Go server, replay seed 7, run once
    python -m touchline_agents                    # against `make serve` on :8080
    python -m touchline_agents --model foundry    # Microsoft Foundry (not wired yet)

The default model is FakeModel with the recorded fixtures, so no Azure
account is needed. Overlays go to stdout (or --out) as NDJSON; the readable
agent trace and the summary go to stderr.
"""

from __future__ import annotations

import argparse
import asyncio
import contextlib
import json
import logging
import os
import sys
from collections.abc import AsyncIterator, Iterator, Sequence
from pathlib import Path
from typing import TextIO

from touchline_agents.clock import MatchClock, MomentClock, ServerClock
from touchline_agents.contracts import DEFAULT_PROFILES, Profile
from touchline_agents.devserver import DevServerError, replay_server
from touchline_agents.model import ConfigError, FakeModel, FoundryModel, Model, RecordingModel
from touchline_agents.orchestrator import Orchestrator, RunSummary, Settings
from touchline_agents.publish import NdjsonPublisher
from touchline_agents.touchline import McpTools, RecordingTools, Tools
from touchline_agents.trace import TraceRecord

log = logging.getLogger("touchline_agents")
trace_log = logging.getLogger("touchline_agents.trace")

AGENTS_DIR = Path(__file__).resolve().parent.parent
DEFAULT_FIXTURES = AGENTS_DIR / "fixtures" / "late_press_model.json"
DEFAULT_MCP_URL = "http://localhost:8080/mcp"
NOISY_LOGGERS = ("httpx", "httpx2", "httpcore", "httpcore2", "mcp")


def parse_args(argv: Sequence[str] | None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        prog="python -m touchline_agents",
        description=__doc__,
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )
    parser.add_argument(
        "--mcp-url",
        default=os.environ.get("MCP_SERVER_URL", DEFAULT_MCP_URL),
        help="Touchline MCP endpoint (default: $MCP_SERVER_URL or %(default)s)",
    )
    parser.add_argument(
        "--replay",
        action="store_true",
        help="start the Go server, replay seed 7 late-press, use --clock moments; needs Go",
    )
    parser.add_argument(
        "--model",
        choices=("fake", "foundry"),
        default="fake",
        help="fake replays --fixtures; foundry reads the FOUNDRY_* settings",
    )
    parser.add_argument(
        "--fixtures",
        type=Path,
        default=DEFAULT_FIXTURES,
        help="recorded model replies for --model fake",
    )
    parser.add_argument("--profiles", type=Path, help="JSON list of viewer profiles")
    parser.add_argument(
        "--clock",
        choices=("server", "moments"),
        default="server",
        help="server: the live match clock; moments: each moment's own time (no lateness)",
    )
    parser.add_argument("--out", type=Path, help="write overlays here instead of stdout")
    parser.add_argument("--trace", type=Path, help="also write the agent trace here as NDJSON")
    parser.add_argument("--concurrency", type=int, default=4, help="moments in flight at once")
    parser.add_argument(
        "--keep-running",
        action="store_true",
        help="keep polling after full time instead of exiting",
    )
    parser.add_argument("--record-model", type=Path, help="save model replies as fixtures")
    parser.add_argument("--record-tools", type=Path, help="save tool results for FakeTools")
    parser.add_argument("-q", "--quiet", action="store_true", help="no trace lines, summary only")
    parser.add_argument("-v", "--verbose", action="store_true", help="debug logging")
    return parser.parse_args(argv)


def load_profiles(path: Path | None) -> tuple[Profile, ...]:
    if path is None:
        return DEFAULT_PROFILES
    raw = json.loads(path.read_text(encoding="utf-8"))
    if not isinstance(raw, list) or not raw:
        raise ValueError(f"{path}: expected a non-empty JSON list of profiles")
    return tuple(Profile.from_json(p) for p in raw)


def build_model(args: argparse.Namespace) -> Model:
    if args.model == "foundry":
        model = FoundryModel.from_env()
        log.warning("FoundryModel is a stub: every agent call fails and templates are published")
        return model
    return FakeModel.from_file(args.fixtures)


@contextlib.contextmanager
def _open_out(path: Path | None, default: TextIO | None) -> Iterator[TextIO | None]:
    if path is None:
        yield default
        return
    with path.open("w", encoding="utf-8") as stream:
        yield stream


@contextlib.asynccontextmanager
async def _server_url(args: argparse.Namespace) -> AsyncIterator[str]:
    if not args.replay:
        yield args.mcp_url
        return
    async with replay_server() as url:
        log.info("replaying seed 7 late-press on %s", url)
        yield url


def _echo(record: TraceRecord) -> None:
    trace_log.info("%s", record.describe())


async def run(args: argparse.Namespace) -> int:
    try:
        model = build_model(args)
        profiles = load_profiles(args.profiles)
    except (ConfigError, OSError, ValueError, TypeError) as exc:
        return _fail(str(exc))
    recording_model = RecordingModel(model) if args.record_model else None

    try:
        async with _server_url(args) as url:
            summary = await _orchestrate(args, url, recording_model or model, profiles)
    except DevServerError as exc:
        return _fail(str(exc))
    if summary is None:
        return 2
    if recording_model is not None:
        recording_model.save(args.record_model)
    report(summary)
    return 1 if summary.errors else 0


async def _orchestrate(
    args: argparse.Namespace, url: str, model: Model, profiles: tuple[Profile, ...]
) -> RunSummary | None:
    mcp = McpTools(url)
    try:
        await mcp.__aenter__()
    except Exception as exc:
        log.debug("connection failure", exc_info=exc)
        _fail(f"cannot reach the MCP server at {url} ({exc}); start it with `make serve`")
        return None
    try:
        tools: Tools = RecordingTools(mcp) if args.record_tools else mcp
        moments_clock = args.replay or args.clock == "moments"
        clock: MatchClock = MomentClock() if moments_clock else ServerClock(tools)
        with _open_out(args.out, sys.stdout) as out, _open_out(args.trace, None) as sink:
            orchestrator = Orchestrator(
                tools=tools,
                model=model,
                publisher=NdjsonPublisher(out or sys.stdout),
                clock=clock,
                settings=Settings(profiles=profiles, concurrency=args.concurrency, trace_sink=sink),
            )
            if not args.quiet:
                orchestrator.trace.listeners.append(_echo)
            summary = await orchestrator.run(stop_at_full_time=not args.keep_running)
    finally:
        await mcp.__aexit__(None, None, None)
    if isinstance(tools, RecordingTools):
        tools.save(args.record_tools)
    return summary


def _fail(message: str) -> int:
    """Report an expected problem (bad settings, server not running) without a traceback."""
    log.error("%s", message)
    return 2


def report(summary: RunSummary) -> None:
    log.info(
        "%d moments: published %d overlays (%d templates), skipped %d, dropped %d, "
        "agent failures %d, errors %d",
        summary.moments,
        summary.published,
        summary.fallbacks,
        summary.skipped,
        summary.dropped,
        summary.agent_failures,
        summary.errors,
    )


def main(argv: Sequence[str] | None = None) -> int:
    args = parse_args(argv)
    logging.basicConfig(
        level=logging.DEBUG if args.verbose else logging.INFO,
        format="%(asctime)s %(levelname)-7s %(message)s",
        datefmt="%H:%M:%S",
        stream=sys.stderr,
    )
    if not args.verbose:
        for name in NOISY_LOGGERS:
            logging.getLogger(name).setLevel(logging.WARNING)
    try:
        return asyncio.run(run(args))
    except KeyboardInterrupt:
        return 130


if __name__ == "__main__":
    sys.exit(main())
