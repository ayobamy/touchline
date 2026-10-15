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
│   └── transport/        planned: Event Hubs source, Redis-backed moment queue
├── agents/               planned: Python orchestrator and agent instructions
├── web/                  planned: pitch view
├── schema/               JSON Schemas
├── scenarios/            scenario files
├── infra/                planned: Bicep for Azure resources
└── docs/
```

## Local development

Requirements: Go 1.24+. Later: Python 3.12+ for `agents/`, Node 20+ for `web/`, the Azure CLI for deployment.

```bash
make test        # go vet + go test
make run         # one match, seed 7, as fast as possible, to match.ndjson
make demo        # late-press scenario at 60x with stats every 5 minutes
make serve       # late-press match served over MCP at http://localhost:8080/mcp
```

## Coding standards

- `gofmt` and `go vet` clean; CI fails otherwise.
- No new Go dependencies without a reason written in the commit message. The data pipeline has none today.
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
| Agents | Verifier rejects a planted wrong number; Director drops a stale overlay; fallback on timeout. Model calls replaced by recorded fixtures | `agents/tests` |
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

Done: tests call every tool through the full JSON-RPC path, and `get_snapshot` matches the stats engine exactly. Still to check by hand: connect the MCP Inspector and a Foundry agent to `/mcp`.

### 5. Transport and shared state

`cmd/server` already reads from a file, stdin or the simulator and keeps state in memory. Remaining: an Event Hubs source, and publishing moments and the agent trace to Redis so the orchestrator and viewer can run as separate apps.

Done when: the server runs locally from a file and in Azure from Event Hubs, from the same binary and a flag.

### 6. Agents

`agents/`, Python with Microsoft Agent Framework:

1. Create a Foundry project and deploy two models: a small one for the Moment Detector and Verifier, a stronger one for the Narrator.
2. Write each agent's instructions in `agents/instructions/<agent>.md` so they are reviewed like code.
3. Connect the agents to the MCP server as a tool source.
4. Write the orchestrator as a loop over `match:{id}:moments`: Detector → Narrator → Verifier (with up to two rewrites) → Director, with a deadline per step from the latency budget in [03](03-architecture.md#latency-budget).
5. Validate every overlay against `schema/overlay.schema.json` before publishing.
6. Record each step to `match:{id}:trace`.

Get one agent answering one tool call on day 4 before building the rest.

Done when: a full replay of seed 7 late-press produces overlays for every goal, the planted-error test is rejected and fixed, and the forced-timeout test produces a fallback.

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
| `MCP_SERVER_URL` | agents | `https://server.<env>.azurecontainerapps.io/mcp` |

Connections use managed identity in Azure. Local secrets go in `.env`, which is git-ignored.
