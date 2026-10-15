# 04 Data contracts

Three formats connect every part of Touchline. Each has a Go type, a JSON Schema in [`schema/`](../schema), and a test that fails if the two drift apart.

| Contract | Go type | Schema |
| --- | --- | --- |
| Match event | `events.Event` | [`schema/event.schema.json`](../schema/event.schema.json) |
| Match metadata | `events.Match` | [`schema/match.schema.json`](../schema/match.schema.json) |
| Overlay | `overlay.Overlay` | [`schema/overlay.schema.json`](../schema/overlay.schema.json) |

## Coordinates and time

- The pitch is **105 m × 68 m**. `x` runs from the home team's goal line (0) to the away team's goal line (105); `y` runs across the pitch from 0 to 68.
- The **home team always attacks towards x = 105**, in both halves. Teams do not switch ends. This keeps every calculation free of half-time special cases.
- `clock_ms` is the match clock in milliseconds. The second half starts at 2,700,000 (45:00) even if first-half stoppage time ran past it, so events are ordered by **(period, clock_ms)**, not by clock alone.
- `seq` is strictly increasing within a match and is the tie-breaker for events at the same clock time.

## The stream

A stream is newline-delimited JSON. The first line is the match; every later line is one event, in order.

```json
{"kind":"match","match":{"id":"m-0001","home":{"id":"riv","name":"Riverside Athletic","short_name":"RIV","players":[...]},"away":{...},"venue":"Millbrook Stadium","seed":7,"scenario":"baseline"}}
{"kind":"event","event":{"id":"m-0001-00001","match_id":"m-0001","seq":1,"type":"kickoff","period":1,"clock_ms":0,"team_id":"riv","player_id":"riv-09","start":{"x":52.5,"y":34}}}
{"kind":"event","event":{"id":"m-0001-00002","match_id":"m-0001","seq":2,"type":"pass","period":1,"clock_ms":2000,"team_id":"riv","player_id":"riv-09","recipient_id":"riv-08","start":{"x":52.5,"y":34},"end":{"x":47.6,"y":45},"outcome":"complete","ball_speed_kmh":36.3}}
```

The same messages travel over Azure Event Hubs in cloud mode, one message per event, partitioned by `match_id`.

## Event fields

| Field | Type | Notes |
| --- | --- | --- |
| `id` | string | `<match_id>-<seq, 5 digits>`; unique |
| `match_id` | string | |
| `seq` | integer ≥ 1 | Strictly increasing |
| `type` | enum | See the table below |
| `period` | 1 or 2 | |
| `clock_ms` | integer ≥ 0 | Match clock |
| `team_id` | string | Acting team; for `kickoff` and `possession_change`, the team that now has the ball |
| `player_id` | string | Acting player |
| `recipient_id` | string | Intended pass target; required on a complete pass |
| `opponent_id` | string | The opposing player pressed, tackled or fouled |
| `start`, `end` | point | Metres; `end` is where a pass or shot finished |
| `outcome` | enum | Only on passes, shots and tackles |
| `ball_speed_kmh` | number > 0 | On passes and shots |
| `under_pressure` | boolean | True when a `pressure` event preceded the action |
| `card` | `yellow` or `red` | Only on card events |
| `score` | `{home, away}` | On `goal`, `half_time`, `full_time` |
| `related_id` | string | On a goal: the ID of the shot that scored |

## Event types

| Type | Required beyond the basics | Outcome |
| --- | --- | --- |
| `kickoff` | `team_id` | — |
| `pass` | `team_id`, `player_id`, `start`, `end`, `ball_speed_kmh`; `recipient_id` when complete | `complete`, `incomplete` |
| `shot` | `team_id`, `player_id`, `start`, `end`, `ball_speed_kmh` | `on_target`, `off_target`, `blocked`, `goal` |
| `tackle` | `team_id`, `player_id`, `start` | `won`, `lost` |
| `interception` | `team_id`, `player_id`, `start` | — |
| `pressure` | `team_id`, `player_id`, `start`, `opponent_id` | — |
| `possession_change` | `team_id` (new owner) | — |
| `foul` | `team_id`, `player_id`, `start` | — |
| `card` | `team_id`, `player_id`, `start`, `card` | — |
| `goal` | `team_id`, `player_id`, `related_id`, `score` | — |
| `half_time`, `full_time` | `score` | — |

A shot with outcome `goal` is always followed by a `goal` event pointing back to it. A `shot` with outcome `on_target` was saved.

## Match metadata

```json
{
  "id": "m-0001",
  "home": {
    "id": "riv", "name": "Riverside Athletic", "short_name": "RIV",
    "players": [{ "id": "riv-09", "name": "Tunde Quinlan", "number": 9, "position": "ST" }]
  },
  "away": { "id": "iro", "name": "Ironbridge Rovers", "short_name": "IRO", "players": [] },
  "venue": "Millbrook Stadium",
  "seed": 7,
  "scenario": "late-press"
}
```

Player IDs are `<team_id>-<shirt number, 2 digits>`. Both teams play a 4-3-3. Every name is fictional.

## Overlays

An overlay is one graphic, timed against the match clock.

```json
{
  "id": "ov-0042",
  "match_id": "m-0001",
  "period": 2,
  "show_at_ms": 3905000,
  "ttl_ms": 8000,
  "priority": 2,
  "slot": "lower_third",
  "audience": "casual",
  "locale": "pcm",
  "team_id": "iro",
  "headline": "Rovers don dey press high",
  "body": "Since 60 minutes, Rovers dey win ball for Riverside half. Na why di goal come.",
  "metrics": [{ "label": "Pressure index", "value": "9.4", "unit": "per 10 passes" }],
  "evidence_event_ids": ["m-0001-01201", "m-0001-01207"],
  "producer": "narrator"
}
```

| Field | Rule |
| --- | --- |
| `show_at_ms`, `ttl_ms` | Show at this match-clock time for 2–30 seconds |
| `priority` | 1 (most urgent) to 5; the Director drops lower priorities when two overlays want the same slot |
| `slot` | `lower_third`, `top_bar`, `side_panel` or `full_screen` |
| `audience` | `analyst`, `casual` or `all` |
| `locale` | BCP 47 tag: `en`, `pcm` (Nigerian Pidgin), `yo` (Yoruba) |
| `headline` | Required, at most 60 characters |
| `body` | At most 160 characters |
| `metrics` | At most 4 |
| `evidence_event_ids` | **At least one.** No evidence, no overlay |
| `producer` | The agent or template that wrote it |
| `fallback` | True when the Director substituted a template |

## Changing a contract

1. Change the Go type and the JSON Schema in the same commit.
2. Run `go test ./...`; the drift tests will fail until both agree.
3. Update this document and any agent instructions that quote the field.
4. Add fields; do not rename or remove them once agents depend on them.
