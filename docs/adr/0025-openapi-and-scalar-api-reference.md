# ADR 0025: An OpenAPI document and a Scalar API reference, served by the server

- **Status:** Accepted, implemented
- **Date:** 2026-10-15

## Context

The server's HTTP surface (the JSON API and the MCP endpoint) was documented only in prose.
Judges, the viewer and anyone wiring Foundry to `/mcp` need a precise, browsable reference, and
the hackathon scores documentation. The data pipeline must stay free of Go dependencies
([ADR 0020](0020-zero-go-dependencies.md)).

## Decision

- **A hand-written OpenAPI 3.1 document**, `internal/apidocs/openapi.json`, describing every
  route, with examples for the JSON API and for MCP messages. At start-up the server adds the
  published contracts from `schema/` (Event, Match, Overlay) as components, hoisting their
  `$defs` and rewriting references, and sets `info.version` to the server version. A small
  `schema` package embeds the schema files.
- **Served by the server** at `GET /openapi.json`, and rendered at `GET /docs` by
  [Scalar](https://github.com/scalar/scalar) (MIT), loaded from jsDelivr at a pinned version
  (`@scalar/api-reference@1.72.2`) with a Subresource Integrity hash. The page turns off
  Scalar's telemetry, hosted fonts, AI agent and MCP helper, so viewing the docs sends nothing
  to third parties beyond fetching the pinned script.
- **Guarded by tests:** every registered route is documented and every documented operation
  answers; every `$ref` resolves; the hand-written components (Status, Snapshot, Moment, ...)
  match the Go types through the existing drift check; the contracts are embedded unchanged.

## Consequences

- No new Go dependency; Scalar runs only in the browser.
- The reference cannot drift from the schemas, and route or field drift fails CI.
- `/docs` needs internet access for the script; `/openapi.json` works offline with any OpenAPI
  tool.
- Bumping Scalar means updating the version and the integrity hash together.
- Building the document turned up two small API bugs, now fixed: `/api/moments` returned
  `null` instead of `[]` before the first moment, and ignored a malformed `after` (it now
  returns 400).

## Alternatives considered

- **Swagger UI or Redoc.** Both work; Scalar renders OpenAPI 3.1 well, includes a request
  client, and is a single script.
- **Generating the document from Go code.** Needs a dependency or a lot of reflection code for
  a surface of eight paths.
- **Vendoring Scalar into the binary.** About 4 MB of JavaScript in the repository; the CDN
  with SRI gives the same integrity guarantee.

## In the code

`internal/apidocs/`, `schema/schema.go`, `cmd/server/main.go`, `cmd/server/main_test.go`,
`internal/schemacheck/schemacheck.go`.
