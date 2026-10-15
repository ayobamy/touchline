# ADR 0014: Our own small MCP server, standard library only

- **Status:** Accepted, implemented
- **Date:** 2026-10-15

## Context

Agents reach the statistics through the Model Context Protocol (MCP). The obvious choice was the official Go SDK, but the build environment could not download Go modules (the module proxy returned 403). Touchline needs only a small part of the protocol: tools, read-only, over stdio for local clients and streamable HTTP for Foundry agents.

## Decision

Implement the server side of MCP in `internal/mcp` (about 300 lines) with the standard library:

- JSON-RPC 2.0 with `initialize` (version negotiation across 2024-11-05, 2025-03-26, 2025-06-18 and 2025-11-25; unknown versions get 2025-06-18), `notifications/initialized`, `ping`, `tools/list` and `tools/call`.
- **stdio:** newline-delimited messages.
- **Streamable HTTP:** one JSON-RPC message per POST, answered with plain JSON, which the transport allows. Notifications get 202, GET gets 405, and an `Mcp-Session-Id` is issued on initialize.
- Tool results include both text and `structuredContent`. Panics in a tool become tool errors.

## Consequences

- No third-party Go dependencies ([ADR 0020](0020-zero-go-dependencies.md)).
- Full control over error behaviour ([ADR 0015](0015-mcp-tool-design.md)).
- No resources, prompts, server-initiated messages or SSE streaming. None are needed for read-only tools.
- Protocol compliance is ours to maintain. It is tested with unit tests and a live run over both transports. It still needs a check against a Foundry agent and the MCP Inspector, planned for milestone 2.
- Switching to the official SDK later touches only `internal/mcp` and `internal/mcptools`.

## Alternatives considered

- **Official Go SDK.** Preferred in principle, but not obtainable here.
- **Python MCP server.** Rejected: it would split the stats engine across two languages.

## In the code

`internal/mcp/mcp.go`, `internal/mcp/http.go`, `internal/mcp/mcp_test.go`.
