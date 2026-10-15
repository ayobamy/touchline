# 05 Stats engine

The stats engine (`internal/stats`) turns events into numbers. It is deterministic and has no randomness and no model calls: the same events always produce the same statistics. Agents read these numbers through the MCP server and explain them; they never compute their own.

## Using it

```go
engine := stats.New(match)
for _, ev := range stream {
    derived := engine.Apply(ev) // per-event numbers, e.g. this pass's difficulty
    _ = derived
}
snapshot := engine.Snapshot() // cumulative team and player stats plus the rolling window
```

From the command line:

```bash
go run ./cmd/simulator -seed 7 | go run ./cmd/stats -every 15 -pretty
```

## Per-event metrics

Returned by `Apply` as `stats.Derived`.

### Pass distance

Straight-line distance from `start` to `end`, in metres, rounded to 0.1.

### Pass difficulty (0–10)

The sum of four components, capped at 10 and rounded to 0.1:

| Component | Points |
| --- | --- |
| Length | `min(distance / 50, 1) × 4`, so up to 4 for passes of 50 m or more |
| Under pressure | 2.5 if a `pressure` event came just before the pass |
| Forward progress | `min(gain / 30, 1) × 2`, where gain is metres moved towards the opponent's goal; 0 for backward passes |
| Into the final third | 1.5 if the pass ends more than 70 m from the passer's own goal line |

A 5 m sideways pass in midfield scores about 0.4. A 45 m pressured ball forward into the final third scores about 9.6.

### Expected goals (xG)

A logistic model of shot distance and the angle the goal mouth subtends from the shot position:

```
angle = atan2(7.32 × dx, dx² + dy² − 3.66²)     (radians; dx, dy in metres from the goal centre)
xG    = 1 / (1 + e^−(−1.2 + 2.2 × angle − 0.09 × distance))
```

clamped to between 0.01 and 0.95.

| Shot from | xG |
| --- | --- |
| 6 m, central | about 0.70 |
| Penalty spot (11 m), central | about 0.31 |
| 25 m, central | about 0.06 |

### Shot speed

The shot's `ball_speed_kmh`, rounded to 0.1.

## Cumulative statistics

Returned by `Snapshot`, for each team and player:

| Statistic | Definition |
| --- | --- |
| Passes, passes completed | Count of `pass` events, and those with outcome `complete` |
| Pass accuracy % | Completed ÷ attempted × 100 |
| Average pass distance | Mean pass distance in metres (team) |
| Average pass difficulty | Mean pass difficulty (player) |
| Shots, shots on target | `on_target` and `goal` outcomes count as on target |
| Goals, xG | Sum of goals and of shot xG |
| Max shot speed | Highest `ball_speed_kmh` on a shot (team) |
| Tackles, tackles won, interceptions, pressures, fouls, cards | Counts of those events |
| Possession % | Share of match-clock time each team had the ball, measured from `kickoff` and `possession_change` events; stoppages after a turnover count for the team that won the ball |
| Touches | Passes, shots, tackles won and interceptions by the player |

## The rolling window

The window covers the last **5 match minutes** of the current period and is reset at half time. It powers the explainability metrics the rules ask for: control vs chaos, pressure changes and game rhythm.

### Momentum (−1 to +1)

Each event in the window credits the team that made it:

| Event | Credit |
| --- | --- |
| Completed pass ending in the final third | 1 |
| Shot | 3, plus 1 if on target |
| Goal | 2 (on top of the shot) |
| Tackle won, interception | 1.5 |
| Pressure | 0.5 |

`momentum = (home credit − away credit) ÷ (home credit + away credit)`. +1 means the home team has had everything; −1 means the away team has.

### Pressure index

`pressures by a team ÷ passes by the opponent × 10`, so "pressures per 10 opponent passes". Rises sharply when a team starts pressing high; with seed 7 and `scenarios/late-press.json`, the away team's index sits around 4–5 in the first half and climbs above 9 in the last ten minutes.

### Chaos

Possession changes per minute. A high value means a scrappy, end-to-end spell; a low value means one team is in control.

### Rhythm

Events of any kind per minute: how busy the game is.

## How the simulator relates

The simulator uses the same `ExpectedGoals` function to decide whether a shot scores, multiplied by a finishing factor of 0.55. That factor brings scoring down to a realistic average of about 2.9 goals a match over 60 seeded matches. The stats engine reports the unscaled xG, as real xG models do, so teams typically "underperform" their xG slightly. This is expected, not a bug.

## Known limitations

- Player positions are not tracked between events, so there are no distance-covered or sprint metrics yet. Threshold-triggered player tags are a stretch goal (see [02 Plan](02-plan.md)).
- Possession during stoppages counts for the team that will restart play.
- The window is a simple time window; it does not weight recent events more heavily.
