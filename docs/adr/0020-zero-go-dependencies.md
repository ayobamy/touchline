# ADR 0020: No third-party Go dependencies, enforced by CI

- **Status:** Accepted, implemented
- **Date:** 2026-10-15

## Context

Judges and other developers should be able to clone the repository and run it with nothing but Go installed. Every dependency is also something to vet, update and secure. The build environment could not fetch modules anyway ([ADR 0014](0014-own-mcp-server-implementation.md)).

## Decision

The Go module has no third-party dependencies. Adding one needs a reason in the commit message and an ADR if it is significant. CI (GitHub Actions) runs `gofmt`, `go vet`, all tests and a full simulated match through the stats engine on every push to `main` and every pull request.

## Consequences

- `go run ./cmd/server -simulate` works on any machine with Go 1.24 or later.
- Some things are written by hand, such as the MCP server and the schema drift check.
- The Azure SDKs for Event Hubs and Redis will be the first dependencies. They will be added under their own ADR.

## In the code

`go.mod`, `Makefile`, `.github/workflows/ci.yml`.
