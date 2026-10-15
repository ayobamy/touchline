# 06 Agents

Four agents turn statistics into explained, verified overlays. They run on **Microsoft Foundry**, are orchestrated with **Microsoft Agent Framework** (Python), and read every number through the Touchline **MCP server**. None of them computes statistics; none of them can change match state.

## Roles

| Agent | Input | Output | Model size |
| --- | --- | --- | --- |
| **Moment Detector** | A candidate moment from the stats service | A moment brief: what happened, why it may matter, evidence event IDs; or "skip" | Small, fast |
| **Narrator** | A moment brief and the active viewer profiles | Draft overlays: one per audience and locale | Larger, better writing |
| **Verifier** | Draft overlays | Each draft approved, or rejected with reasons | Small, fast; strict instructions |
| **Director** | Verified overlays, the current screen schedule | Scheduled overlays (priority, slot, time-to-live), stale ones dropped | Code first; model only to break ties |

In the full design a separate **Tactical Analyst** explains the "why" and a **Localizer** handles languages. For the solo build both are folded into the Narrator (see [02 Plan](02-plan.md)); they can be split out again without changing any contract.

## Where moments come from

Deterministic rules in `internal/moments`, not an agent, propose candidate moments, so the agents are not woken for every pass. Thresholds live in `moments.DefaultThresholds()`:

| Kind | Trigger |
| --- | --- |
| `goal` | Any `goal` event; evidence is the shot and the goal |
| `big_chance` | A missed, saved or blocked shot with xG ≥ 0.25 |
| `key_pass` | A completed pass with difficulty ≥ 8 |
| `card` | Any `card` event |
| `momentum_swing` | A team is now clearly on top (momentum beyond ±0.5) *and* either just crossed that level or momentum moved by 0.6 or more in five minutes |
| `press_switch` | A team's pressure index rises by 3 or more in five minutes and reaches at least 6 |
| `chaos_spell` | Possession changing hands 4 or more times a minute, sustained for two minutes |
| `player_milestone` | A player's 40th completed pass, first saved shot on target, or fourth tackle won |

Window rules stay quiet for the first four minutes of each half, and the same rule for the same team stays quiet for six minutes after firing. A typical match produces 35–55 candidates; on seed 7 with the late-press scenario Ironbridge's press switch fires on 63 minutes. Each moment carries up to six evidence event IDs and a plain-English summary using player and club names.

The Moment Detector then decides which candidates deserve screen time. Expect it to skip most of them.

## MCP tools

`cmd/server` replays a match into the store and exposes these read-only tools at `/mcp` (streamable HTTP) or over stdio with `-stdio`. Agents must call them for any number they state. On connect, the server's instructions remind the model of that rule and of the clock and pitch conventions.

| Tool | Arguments | Returns |
| --- | --- | --- |
| `get_match` | — | Teams, players (ID, name, number, position), venue, scenario |
| `get_status` | — | Period, clock, score, event and moment counts, finished flag |
| `get_snapshot` | `include_players` (optional) | Cumulative team stats and the last-5-minutes window; player stats on request |
| `get_player_stats` | `player_id` | That player's cumulative stats |
| `get_window` | `minutes` (1–45) | Momentum, pressure index, chaos and rhythm over the last N minutes of the current half |
| `get_events` | optional `period`, `from_ms`, `to_ms`, `team_id`, `player_id`, `types[]`, `limit` (≤ 200) | Matching events with derived numbers, plus the total matched |
| `get_event` | `event_id` | One event and its derived numbers |
| `compare_periods` | `metric`, `a` and `b` as `{period, from_ms, to_ms}` | One metric for both teams over two spells; metrics: passes, passes_completed, pass_accuracy_pct, shots, shots_on_target, goals, xg, tackles_won, interceptions, pressures, pressure_index, momentum |
| `list_moments` | `after` (optional) | Candidate moments after the first `after`, and `next` to pass on the following call |

Bad arguments, unknown IDs and unknown fields come back as tool errors (`isError: true`) with a message the model can act on, not as protocol errors.

The server also offers a small JSON API for the orchestrator and viewer: `GET /api/status`, `/api/match`, `/api/snapshot`, `/api/moments?after=N`, and `/healthz`.

## Shared state

Shared state lives in Redis, keyed by match. Agents never talk to each other directly; each handoff is a typed message on a list.

| Key | Contents | Written by |
| --- | --- | --- |
| `match:{id}:snapshot` | Latest stats snapshot | Stats service |
| `match:{id}:moments` | Candidate moments (list) | Stats service |
| `match:{id}:briefs` | Moment briefs (list) | Moment Detector |
| `match:{id}:drafts` | Draft overlays (list) | Narrator |
| `match:{id}:verified` | Verified overlays (list) | Verifier |
| `match:{id}:schedule` | What is on screen per slot, per profile | Director |
| `match:{id}:trace` | One record per agent step, for the trace panel | Orchestrator |

Every message carries the `moment_id`, so the trace panel can show one moment's journey from trigger to screen.

## Handoffs

1. **Stats service → Moment Detector:** `{moment_id, kind, clock_ms, period, event_ids[], snapshot_ref}`.
2. **Moment Detector → Narrator:** `{moment_id, headline_idea, why_it_matters, evidence_event_ids[], suggested_priority}`, or `skip` with a reason.
3. **Narrator → Verifier:** a list of draft `Overlay` objects (see [04](04-data-contracts.md)), one per audience and locale, all sharing the moment's evidence.
4. **Verifier → Narrator (rejection):** `{moment_id, overlay_id, problems: [{claim, reason, correct_value?}]}`.
5. **Verifier → Director:** approved overlays.
6. **Director → viewers:** scheduled overlays, each passing `overlay.Validate`.

## Failure recovery

These are the behaviours the Best Multi-Agent System judges look for, so the demo shows them on purpose.

| Failure | Detection | Recovery |
| --- | --- | --- |
| Narrator states a number that does not match the stats | Verifier compares each number with an MCP tool result | Rejected with the correct value; Narrator rewrites; at most two rewrites, then the moment is dropped |
| Narrator cites an event that does not support the claim | Verifier calls `get_event` for every cited ID | Rejected; same loop |
| An agent misses its deadline | Orchestrator timeout | Director publishes a template overlay built only from stats-engine numbers, marked `fallback: true` |
| Overlay arrives after its moment has passed | Director compares `show_at_ms + ttl_ms` with the match clock | Dropped silently |
| Two overlays want the same slot | Director | Higher priority wins; the other waits or is dropped |
| Output fails `overlay.Validate` | Overlay publisher | Never published; logged to the trace |

## Personalization

Each viewer has a profile:

```json
{
  "profile_id": "fan-1",
  "audience": "casual",
  "locale": "pcm",
  "club_id": "riv",
  "focus_player_id": "riv-10",
  "favourite_metric": "pressure_index",
  "explain_simply": true
}
```

- The Narrator writes one draft per distinct (audience, locale) pair among the connected profiles.
- The Director filters by profile: club focus raises the priority of that club's moments; player focus adds a lower-third whenever the focus player is involved; favourite metric pins that metric to the side panel.
- At full time the Narrator writes a recap per profile from the moments that were shown to it.

## Instructions for the Narrator

Keep these in the agent's system instructions:

- Every number must come from an MCP tool result in this conversation. If you do not have it, call a tool or leave the number out.
- Explain *why* the moment matters, not only *what* happened.
- Headline: at most 60 characters. Body: at most 160.
- Casual audience: no jargon; one idea per overlay. Analyst audience: lead with the metric.
- Pidgin (`pcm`): natural Nigerian Pidgin as spoken by football fans, not translated English. Keep player and club names unchanged.
- Never mention real clubs, real players, betting or odds.

## Evaluation

- A fixed set of 30 moments from seeded matches, each with a hand-checked list of true facts.
- **Claim accuracy:** share of numbers in approved overlays that match the stats engine. Target: 100%.
- **Groundedness:** Foundry evaluation of each overlay against its cited events.
- **Rejection rate:** share of drafts the Verifier sends back. Track it; a rising rate means the Narrator's instructions need work.
- **Latency:** median and 95th percentile from triggering event to published overlay.

Publish the latest scores in the README before submission.
