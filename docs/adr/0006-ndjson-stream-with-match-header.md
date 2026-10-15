# ADR 0006: Newline-delimited JSON stream with a match header

- **Status:** Accepted, implemented
- **Date:** 2026-10-15

## Context

Events need to move between the simulator, the stats tools, the server, tests and, later, Azure Event Hubs. Consumers also need the fixture metadata (teams and players) before the first event.

## Decision

A stream is newline-delimited JSON. Each line is an envelope `{"kind": "match" | "event", ...}`. The first line is exactly one `match` message, and every following line is one `event`. On Event Hubs, each event will be one message, partitioned by `match_id`.

## Consequences

- Streams can be piped (`simulator | stats`), recorded to a file, inspected with standard tools and replayed by `cmd/server -in`.
- One format serves local and cloud modes.
- JSON is larger than a binary format. At about 1,800 events a match this does not matter.

## Alternatives considered

- **Protocol Buffers or Avro.** Rejected: schema tooling and harder debugging, for no gain at this volume.
- **Metadata in a separate file.** Rejected: two files to keep in step.

## In the code

`internal/events/match.go` (`Message`), `cmd/simulator`, `cmd/stats`, `cmd/server`.
