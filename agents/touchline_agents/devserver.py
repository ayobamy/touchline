"""Start the Go server for a local replay, so the agents run with one command.

Used by `python -m touchline_agents --replay` and the integration test. It
builds cmd/server into a temporary directory, replays the match at full speed
(the whole match is in the store within a second), and stops the server on
exit. Needs Go on PATH.
"""

from __future__ import annotations

import asyncio
import contextlib
import http.client
import json
import shutil
import socket
import tempfile
from collections.abc import AsyncIterator
from dataclasses import dataclass
from http import HTTPStatus
from pathlib import Path

STARTUP_TIMEOUT_S = 30.0
POLL_S = 0.1


class DevServerError(Exception):
    """The Go server could not be built or did not start."""


@dataclass(frozen=True, slots=True)
class Replay:
    seed: int = 7
    scenario: str = "scenarios/late-press.json"


def repo_root() -> Path:
    for parent in Path(__file__).resolve().parents:
        if (parent / "go.mod").is_file() and (parent / "cmd" / "server").is_dir():
            return parent
    raise DevServerError("cannot find the repository root (go.mod with cmd/server)")


def _free_port() -> int:
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as sock:
        sock.bind(("127.0.0.1", 0))
        port: int = sock.getsockname()[1]
        return port


def _replayed(port: int) -> bool:
    """True once the server answers and the whole match is in its store."""
    conn = http.client.HTTPConnection("127.0.0.1", port, timeout=1)
    try:
        conn.request("GET", "/api/status")
        response = conn.getresponse()
        if response.status != HTTPStatus.OK:
            return False
        return bool(json.loads(response.read()).get("finished"))
    except (OSError, ValueError):
        return False
    finally:
        conn.close()


async def _build(go: str, root: Path, binary: Path) -> None:
    proc = await asyncio.create_subprocess_exec(
        go,
        "build",
        "-o",
        str(binary),
        "./cmd/server",
        cwd=root,
        stdout=asyncio.subprocess.PIPE,
        stderr=asyncio.subprocess.STDOUT,
    )
    output = (await proc.communicate())[0]
    if proc.returncode != 0:
        raise DevServerError(f"go build failed: {output.decode().strip()}")


async def _wait_replayed(port: int, proc: asyncio.subprocess.Process) -> None:
    async with asyncio.timeout(STARTUP_TIMEOUT_S):
        while not await asyncio.to_thread(_replayed, port):
            if proc.returncode is not None:
                raise DevServerError(f"the server exited with code {proc.returncode}")
            await asyncio.sleep(POLL_S)


@contextlib.asynccontextmanager
async def replay_server(replay: Replay | None = None) -> AsyncIterator[str]:
    """Yield the MCP URL of a freshly started server, once the match is fully replayed."""
    go = shutil.which("go")
    if go is None:
        raise DevServerError("Go is not on PATH; install Go 1.24+ or start the server yourself")
    spec = replay or Replay()
    root = repo_root()
    with tempfile.TemporaryDirectory(prefix="touchline-") as tmp:
        binary = Path(tmp) / "touchline-server"
        await _build(go, root, binary)
        port = _free_port()
        proc = await asyncio.create_subprocess_exec(
            str(binary),
            "-simulate",
            "-seed",
            str(spec.seed),
            "-scenario",
            str(root / spec.scenario),
            "-speed",
            "0",
            "-addr",
            f"127.0.0.1:{port}",
            cwd=root,
            stdout=asyncio.subprocess.DEVNULL,
            stderr=asyncio.subprocess.DEVNULL,
        )
        try:
            await _wait_replayed(port, proc)
            yield f"http://127.0.0.1:{port}/mcp"
        finally:
            if proc.returncode is None:
                proc.terminate()
                try:
                    async with asyncio.timeout(5):
                        await proc.wait()
                except TimeoutError:
                    proc.kill()
                    await proc.wait()
