# Touchline: notes for Claude Code

Touchline is Ahmed Olawale's solo entry for the Microsoft *Inside the Game* developer hackathon: AI
agents that turn a synthetic football match into explained, verified, timed overlays. Submissions
close **27 October 2026, 11:59 PM Pacific** (about 8 AM on 28 October in Lagos, where the author
is). Treat the evening of 27 October in Lagos as the real deadline.

## Read first

- [docs/README.md](docs/README.md): index and the status table
- [docs/02-plan.md](docs/02-plan.md): the plan, milestones (M1 to M5) and risks
- [docs/03-architecture.md](docs/03-architecture.md): components, latency budget, reliability
- [docs/06-agents.md](docs/06-agents.md): agent roles, MCP tools, handoffs, failure recovery
- [docs/07-implementation.md](docs/07-implementation.md): layout, local runs, build notes
- [docs/adr/README.md](docs/adr/README.md): every significant decision and why

## Rules for every change

1. **Commits show only the author:** `Ahmed Olawale <ayobamyahmed@gmail.com>`. No
   `Co-Authored-By`, "Generated with" or any other AI attribution in commits or PRs.
2. **Work on a branch, never directly on `main`.** Run `make test` before every commit, and
   `make agents-test` as well when `agents/` changed. Small commits with plain messages ("Add
   pressure-switch moment rule", not "updates").
3. **The code computes every number; the AI only explains** ([ADR 0002](docs/adr/0002-code-computes-ai-explains.md)).
   Agents get numbers only from the MCP tools.
4. **No evidence, no overlay** ([ADR 0008](docs/adr/0008-no-evidence-no-overlay.md)). Every
   overlay must pass `schema/overlay.schema.json` before it is published.
5. **No new Go dependencies without an ADR** ([ADR 0020](docs/adr/0020-zero-go-dependencies.md)).
   Python dependencies for `agents/` are fine, pinned exactly in `agents/pyproject.toml` and the
   compiled `agents/requirements*.txt`.
6. **Every significant decision gets an ADR** in `docs/adr/`, using the template in
   [ADR 0001](docs/adr/0001-record-architecture-decisions.md), plus a row in
   `docs/adr/README.md`. Keep the status table in `docs/README.md` and the root `README.md`
   current as work lands.
7. **All clubs and players are fictional.** No real league names, crests, clubs, players or real
   data anywhere.

## Repository layout

```text
cmd/simulator      synthetic match generator (NDJSON)
cmd/stats          reads a stream, prints statistics snapshots
cmd/server         replays a match; MCP tools at /mcp, JSON API at /api/*, API docs at /docs
internal/events    event, match and stream contract
internal/sim       simulator and scenarios
internal/stats     deterministic metrics and the running stats engine
internal/moments   rules that flag candidate moments
internal/store     live match state for concurrent readers
internal/mcp       dependency-free MCP server (stdio and streamable HTTP)
internal/mcptools  the nine Touchline tools
internal/overlay   overlay contract and validation
internal/apidocs   OpenAPI document and the Scalar API reference page
schema/            JSON Schemas (event, match, overlay), embedded by the server
scenarios/         scenario files (baseline, late-press, red-card)
agents/            Python orchestrator, agent instructions and tests
web/               pitch view (planned, milestone 3)
docs/              plan, architecture, contracts, ADRs
```

## Commands

```bash
make test          # gofmt check, go vet, go test ./...
make demo          # late-press scenario at 60x with stats every 5 minutes
make serve         # late-press match: MCP at :8080/mcp, API docs at :8080/docs (SPEED=60 default)
make agents-test   # agents/: ruff, mypy and pytest in agents/.venv
make agents        # run the orchestrator against `make serve` with recorded fixtures
```

Go 1.24+ is the only requirement for the data pipeline. The agents need Python 3.12+.

## Working notes

- The same seed and scenario always give the same match, so moment IDs such as
  `m-0001-mo-018` (Ironbridge's press switch on 63 minutes, seed 7 late-press) are stable and
  usable in fixtures.
- Agents never call `get_window`, `get_status` or `get_player_stats` for evidence: those return
  values for *now*, not for the moment. Evidence comes from the moment, `get_event` and
  `compare_periods` with explicit ranges (ADR 0023).
- Propose commit messages instead of committing: the author reviews, commits and pushes. Commit
  only when asked.
