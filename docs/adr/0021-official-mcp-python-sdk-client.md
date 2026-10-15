# ADR 0021: The official MCP Python SDK for the orchestrator's tool client

- **Status:** Accepted, implemented
- **Date:** 2026-10-15

## Context

The orchestrator calls the Touchline MCP tools itself: to poll candidate moments, to fetch the
evidence for each one, and to read the match clock. The server is our own small implementation
of the protocol ([ADR 0014](0014-own-mcp-server-implementation.md)), which had only been
tested with its own unit tests and a hand-written client. ADR 0014 left a check against an
independent client open.

Two options for the Python side: the official SDK (`mcp`, MIT, maintained by the protocol's
authors; version 2.3.0 released 2 October 2026), or a hand-written JSON-RPC client of about a
hundred lines, mirroring the server.

## Decision

Use the official SDK, pinned at `mcp==2.3.0`, through its high-level `Client` over streamable
HTTP. `touchline_agents/touchline.py` wraps it in a small `Tools` interface with typed helpers
(`get_event`, `compare_periods`, `list_moments`, ...), so nothing else in the orchestrator
imports the SDK. Tool errors (`isError: true`) become `ToolError`.

Before choosing, the SDK was run against `cmd/server`: initialize, `tools/list`, `tools/call`,
structured content and tool errors all worked unchanged, including the SDK's handling of our
405 on GET and 204 on DELETE.

## Consequences

- Every orchestrator run, and the integration test in CI, is now an interoperability check of
  our Go server against the reference client. This closes the open check in ADR 0014 for the
  Python SDK; Foundry's own MCP client still needs a manual check.
- The SDK brings pydantic, anyio, httpx2 and starlette as transitive dependencies (about 28
  packages, all pinned with hashes in `agents/requirements.txt`).
- SDK major versions move quickly (v2 renamed result fields to snake_case). The `Tools`
  wrapper keeps any future change to one file.

## Alternatives considered

- **Hand-written client.** Smaller, but would test our server only against our own reading of
  the specification, the weakness ADR 0014 already noted.
- **Calling the JSON API (`/api/moments`) instead of MCP.** Fine for polling, but evidence
  needs `get_event` and `compare_periods`, which only exist as tools, and agents should read
  numbers through the same tools the models will use.

## In the code

`agents/touchline_agents/touchline.py`, `agents/tests/test_integration.py`,
`agents/pyproject.toml`.
