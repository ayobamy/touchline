# ADR 0007: Contracts are Go types plus JSON Schemas, guarded by drift tests

- **Status:** Accepted, implemented
- **Date:** 2026-10-15

## Context

The event, match and overlay formats are shared by Go code, Python agents and a TypeScript viewer. The Go side needs typed validation, and the other languages need a language-neutral definition. Two definitions of one format tend to drift apart.

## Decision

Each contract has:

- a Go type with a `Validate` method that reports every problem at once (`errors.Join`);
- a JSON Schema in `schema/` for other languages and for agents;
- a test (`internal/schemacheck`) that fails if the Go type's JSON fields and the schema's properties differ.

Contracts gain fields; they do not rename or remove fields once agents depend on them.

## Consequences

- Python and TypeScript can validate against the schemas without reading Go.
- Drift between the two definitions is caught by CI.
- The drift test compares field names only, not types or constraints. Those still rely on review and on the validation tests.

## Alternatives considered

- **Generating the schemas from Go types.** Rejected: it needs a dependency, and hand-written schemas carry better descriptions for agents.
- **Schemas only, validated with a library at runtime.** Rejected: weaker typing in Go and a dependency.

## In the code

`internal/events`, `internal/overlay`, `schema/`, `internal/schemacheck`, and the `TestSchema*` tests.
