# 07 Implementation guide

How to build, test and ship each part of Touchline. Each section ends with the checks that mean it is done.

## Repository layout

Current and planned:

```
touchline/
├── cmd/
│   ├── simulator/        built: match generator
│   ├── stats/            built: stats from a stream
│   └── server/           built: replays a match, serves MCP tools and a JSON API
├── internal/
│   ├── events/           built: event, match and stream contract
│   ├── sim/              built: simulator and scenarios
│   ├── stats/            built: metrics and stats engine
│   ├── overlay/          built: overlay contract
│   ├── schemacheck/      built: Go type vs JSON Schema drift check
│   ├── moments/          built: candidate-moment rules
│   ├── store/            built: live match state for concurrent readers
│   ├── mcp/              built: dependency-free MCP server (stdio, streamable HTTP)
│   ├── mcptools/         built: the Touchline tools
│   ├── apidocs/          built: OpenAPI document and the Scalar API reference at /docs
│   └── transport/        planned: Event Hubs source, Redis-backed moment queue
├── agents/               built: Python orchestrator, agent instructions, fixtures, tests
├── web/                  planned: pitch view
├── schema/               JSON Schemas (embedded by the server through schema.go)
├── scenarios/            scenario files
├── infra/                planned: Bicep for Azure resources
└── docs/
```

## Local development

Requirements: Go 1.24+; Python 3.12+ for `agents/`. Later: Node 20+ for `web/`, the Azure CLI for deployment.

```bash
make test          # gofmt check, go vet, go test
make run           # one match, seed 7, as fast as possible, to match.ndjson
make demo          # late-press scenario at 60x with stats every 5 minutes
make serve         # late-press match: MCP at :8080/mcp, API reference at :8080/docs (SPEED=60)
make agents-test   # agents: ruff, mypy, pytest with coverage (creates agents/.venv)
make agents        # agents end to end with recorded model replies; no Azure needed
```

## Coding standards

- `gofmt` and `go vet` clean; CI fails otherwise.
- No new Go dependencies without an ADR ([ADR 0020](adr/0020-zero-go-dependencies.md)). The data pipeline has none today. Python dependencies for `agents/` are pinned exactly, with hashes.
- Contracts change in one commit with their schema and docs (see [04](04-data-contracts.md#changing-a-contract)).
- Every behaviour the demo relies on has a test.
- Small commits with plain messages: "Add pressure-switch moment rule", not "updates".

## Testing strategy

| Layer | What is tested | Where |
| --- | --- | --- |
| Contracts | Validation accepts good and rejects bad events and overlays; Go types match schemas | `internal/events`, `internal/overlay` |
| Simulator | Determinism; 20 seeds produce valid, ordered streams with realistic volumes; scenarios do what they say | `internal/sim` |
| Stats | xG calibration; pass difficulty ordering; possession and accuracy on hand-built streams; window pruning | `internal/stats` |
| Moments | Every goal and card becomes a moment; the late press is detected; evidence IDs exist and are never from the future; cooldowns hold | `internal/moments` |
| MCP protocol | Version negotiation, tools list and call, tool errors vs protocol errors, stdio and HTTP transports | `internal/mcp` |
| MCP tools | Each tool returns the engine's numbers for a replayed match; bad arguments become tool errors | `internal/mcptools` |
| Agents | Verifier rejects a planted wrong number and the Narrator fixes it; at most two rewrites; fallback on timeout; Director drops stale overlays and resolves slots; every published overlay passes the schema. Model calls use recorded fixtures, tool calls a recording of the Go server | `agents/tests` |
| Agents against the server | A full replay over MCP with the official Python SDK; the recorded tool results still match the server | `agents/tests/test_integration.py` (CI starts the server) |
| API reference | Every route documented and answering; every `$ref` resolves; components match the Go types | `internal/apidocs`, `cmd/server` |
| End to end | Replay seed 7 late-press locally; at least one overlay per goal; zero invalid overlays | `make e2e` (planned) |

## Component build notes

### 1. Simulator (built)

`internal/sim`. A possession loop: pressure → maybe a tackle → shot if in range → otherwise a pass that is completed, intercepted or lost. Positions come from a 4-3-3 zone per player, shifted up and down with the ball. Scenarios multiply a team's press and attack between two minutes; red cards remove a player and weaken the team by 15%.

Done when: `go test ./internal/sim` passes; `make demo` shows the late press in the pressure index.

### 2. Stats engine (built)

`internal/stats`. See [05](05-stats-engine.md).

Done when: `go test ./internal/stats` passes.

### 3. Candidate moments (built)

`internal/moments`: a `Detector` wraps the engine and returns `[]Moment` from each `Apply`, using the rules in [06](06-agents.md#where-moments-come-from). Thresholds are one struct, `moments.Thresholds`.

Done: on seed 7 late-press it emits every goal and card and Ironbridge's press switch on 63 minutes, with 42 candidates in the match.

### 4. MCP server (built)

`internal/mcp` implements the protocol (initialize, ping, tools/list, tools/call over stdio and streamable HTTP with JSON responses) using only the standard library; see [ADR 0014](adr/0014-own-mcp-server-implementation.md). `internal/store` holds the live match; `internal/mcptools` registers the nine tools in [06](06-agents.md#mcp-tools); `cmd/server` wires them together and replays a match from a file, stdin or the simulator.

```bash
go run ./cmd/server -simulate -seed 7 -scenario scenarios/late-press.json -speed 60   # HTTP on :8080
go run ./cmd/server -simulate -stdio                                                   # stdio
```

Done: tests call every tool through the full JSON-RPC path, and `get_snapshot` matches the stats engine exactly. The official MCP Python SDK works against it unchanged ([ADR 0021](adr/0021-official-mcp-python-sdk-client.md)). Still to check by hand: a Foundry agent connected to `/mcp`.

The server also serves its OpenAPI document at `/openapi.json` and an API reference at `/docs`; see [API reference](#api-reference) below.

### 5. Transport and shared state

`cmd/server` already reads from a file, stdin or the simulator and keeps state in memory. Remaining: an Event Hubs source, and publishing moments and the agent trace to Redis so the orchestrator and viewer can run as separate apps.

Done when: the server runs locally from a file and in Azure from Event Hubs, from the same binary and a flag.

### 6. Agents (skeleton built; Foundry models to wire)

`agents/`, Python 3.12+. The orchestrator owns control flow in plain asyncio and talks to models through one interface ([ADR 0022](adr/0022-orchestrator-asyncio-pipeline.md)):

| Module | Role |
| --- | --- |
| `orchestrator.py` | Polls `list_moments` over MCP; runs Detector → Narrator → Verifier (up to two rewrites) → Director with a deadline per step; traces every step |
| `evidence.py` | The evidence pack per moment, anchored to the moment's time: the moment, `get_event` for each evidence ID, `compare_periods` for pressure index and momentum ([ADR 0023](adr/0023-time-anchored-evidence-code-first-verifier.md)) |
| `detector.py`, `narrator.py`, `verifier.py` | The model-backed agents; the Verifier checks schema, numbers, metric labels and evidence in code before asking its model |
| `director.py`, `fallback.py`, `publish.py` | Director rules and template overlays in code; every overlay passes `schema/overlay.schema.json` before publishing ([ADR 0024](adr/0024-director-and-fallbacks-in-code.md)) |
| `model.py` | `FakeModel` (recorded replies), `RecordingModel`, and the `FoundryModel` stub |
| `touchline.py` | The MCP client, using the official SDK ([ADR 0021](adr/0021-official-mcp-python-sdk-client.md)) |
| `instructions/*.md` | One instruction file per agent; the Narrator's includes a Nigerian Pidgin style guide |

#### Run the agents locally

```bash
make agents
```

This builds the server, replays seed 7 late-press, and runs all four agents with the recorded replies in `agents/fixtures/late_press_model.json`, printing the agent trace to stderr and writing overlays to `bin/overlays.ndjson`. It shows a planted wrong number rejected and rewritten (63:38, Ironbridge's press), a Narrator timeout replaced by a template (56:48, Riverside's goal), Moment Detector skips, and templates for goals without recorded replies. No Azure account is needed.

Against a live server instead:

```bash
make serve SPEED=4                     # terminal 1: four match seconds per second
cd agents && .venv/bin/python -m touchline_agents          # terminal 2
```

With `--clock server` (the default) the Director uses the live match clock, so anything that reaches it after `show_at_ms + ttl_ms` is dropped. At `SPEED=60` the poll interval alone costs 15 match seconds, so most overlays arrive stale; that is the rule working. Useful flags: `--out FILE`, `--trace FILE` (NDJSON), `--profiles FILE`, `--record-model FILE`, `--record-tools FILE`, `--quiet`.

#### Wiring Foundry

1. Create a Foundry project and deploy two models: a small one for the Moment Detector and Verifier, a stronger one for the Narrator.
2. Set `FOUNDRY_PROJECT_ENDPOINT`, `FOUNDRY_MOMENT_DETECTOR_DEPLOYMENT`, `FOUNDRY_NARRATOR_DEPLOYMENT` and `FOUNDRY_VERIFIER_DEPLOYMENT` (in `.env` locally). Authentication will use Microsoft Entra ID (`az login` locally, managed identity in Azure); no keys.
3. Implement `FoundryModel.respond` in `agents/touchline_agents/model.py`: one Foundry agent per role from its instructions file and deployment, the request's JSON as the user message, a JSON reply. Read the Foundry and Agent Framework docs for the current SDK first.
4. Run `python -m touchline_agents --model foundry --replay --record-model fixtures/foundry.json` to check a full match and record real replies for the tests.

Until then `--model foundry` reads the settings and every agent call fails, so the Director publishes templates.

Done when: a full replay of seed 7 late-press on Foundry produces overlays for every goal, the planted-error test is rejected and fixed, and the forced-timeout test produces a fallback. The last two already pass with recorded replies.

### API reference

`internal/apidocs` serves `GET /openapi.json` (OpenAPI 3.1, with the Event, Match and Overlay schemas from `schema/` embedded as components) and `GET /docs`, an API reference rendered by Scalar from a pinned, integrity-checked script with telemetry turned off ([ADR 0025](adr/0025-openapi-and-scalar-api-reference.md)). Run `make serve` and open <http://localhost:8080/docs>. When a route or a response type changes, update `internal/apidocs/openapi.json` in the same commit; the tests fail until it matches.

### 7. Pitch view

`web/`: one page.

- A canvas pitch, 105 × 68 scaled to the viewport, drawing the ball and the last few passes.
- An overlay layer with the four slots; overlays fade in at `show_at_ms` and out after `ttl_ms`.
- A profile switcher, and a split-screen mode showing two profiles side by side.
- An agent-trace panel listing each moment's steps, including rejections and fallbacks.
- A "replay match" button that starts a seeded replay, so judges can try it without setup.

Done when: it works at phone width and on a laptop, and a judge can press one button and see overlays within 30 seconds.

### 8. Deployment

`infra/main.bicep` provisions: a Container Apps environment, three container apps (server, agents, web), Event Hubs (one hub, two partitions), Azure Managed Redis (smallest tier), Azure SignalR Service (free tier if limits allow), Application Insights, and managed identities with role assignments.

```bash
az deployment group create -g touchline -f infra/main.bicep
```

Scale the apps to zero outside demos and set a budget alert.

Done when: the public URL replays a match end to end from a cold start.

## Configuration

| Variable | Used by | Example |
| --- | --- | --- |
| `TOUCHLINE_SOURCE` | server | `stdin` or `eventhubs` |
| `TOUCHLINE_STORE` | server, agents | `memory` or `redis` |
| `EVENTHUBS_NAMESPACE` | server, simulator | `touchline-eh.servicebus.windows.net` |
| `REDIS_HOST` | server, agents | `touchline.redis.cache.windows.net` |
| `FOUNDRY_PROJECT_ENDPOINT` | agents | from the Foundry portal |
| `FOUNDRY_MOMENT_DETECTOR_DEPLOYMENT` | agents | the small model's deployment name |
| `FOUNDRY_NARRATOR_DEPLOYMENT` | agents | the stronger model's deployment name |
| `FOUNDRY_VERIFIER_DEPLOYMENT` | agents | the small model's deployment name |
| `MCP_SERVER_URL` | agents | default `http://localhost:8080/mcp`; in Azure `https://server.<env>.azurecontainerapps.io/mcp` |
| `TOUCHLINE_OVERLAY_SCHEMA` | agents | path to `overlay.schema.json` when not run from the repository |

Connections use managed identity in Azure. Local secrets go in `.env`, which is git-ignored.
