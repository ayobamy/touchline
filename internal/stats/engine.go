package stats

import (
	"sort"

	"github.com/ayobamy/touchline/internal/events"
)

// DefaultWindowMinutes is the length of the rolling window used for momentum,
// pressure, chaos and rhythm.
const DefaultWindowMinutes = 5

// Derived holds the per-event numbers the engine computes, so a caller (an
// agent or an overlay) can quote them next to the event they describe.
type Derived struct {
	PassDistanceM  float64 `json:"pass_distance_m,omitempty"`
	PassDifficulty float64 `json:"pass_difficulty,omitempty"`
	ExpectedGoals  float64 `json:"xg,omitempty"`
	ShotSpeedKmh   float64 `json:"shot_speed_kmh,omitempty"`
}

// TeamStats are cumulative statistics for one team.
type TeamStats struct {
	TeamID           string  `json:"team_id"`
	Passes           int     `json:"passes"`
	PassesCompleted  int     `json:"passes_completed"`
	PassAccuracyPct  float64 `json:"pass_accuracy_pct"`
	AvgPassDistanceM float64 `json:"avg_pass_distance_m"`
	Shots            int     `json:"shots"`
	ShotsOnTarget    int     `json:"shots_on_target"`
	Goals            int     `json:"goals"`
	ExpectedGoals    float64 `json:"xg"`
	MaxShotSpeedKmh  float64 `json:"max_shot_speed_kmh"`
	Tackles          int     `json:"tackles"`
	TacklesWon       int     `json:"tackles_won"`
	Interceptions    int     `json:"interceptions"`
	Pressures        int     `json:"pressures"`
	Fouls            int     `json:"fouls"`
	YellowCards      int     `json:"yellow_cards"`
	RedCards         int     `json:"red_cards"`
	PossessionPct    float64 `json:"possession_pct"`

	possessionMs int64
	passDistance float64
}

// PlayerStats are cumulative statistics for one player.
type PlayerStats struct {
	PlayerID          string  `json:"player_id"`
	TeamID            string  `json:"team_id"`
	Touches           int     `json:"touches"`
	Passes            int     `json:"passes"`
	PassesCompleted   int     `json:"passes_completed"`
	PassAccuracyPct   float64 `json:"pass_accuracy_pct"`
	AvgPassDifficulty float64 `json:"avg_pass_difficulty"`
	Shots             int     `json:"shots"`
	Goals             int     `json:"goals"`
	ExpectedGoals     float64 `json:"xg"`
	TacklesWon        int     `json:"tackles_won"`
	Interceptions     int     `json:"interceptions"`
	Pressures         int     `json:"pressures"`

	difficultySum float64
}

// Window summarises the last few minutes of play.
type Window struct {
	Minutes int `json:"minutes"`
	// Momentum runs from -1 (away team on top) to +1 (home team on top).
	Momentum float64 `json:"momentum"`
	// PressureIndex is pressures per 10 opponent passes, by team ID.
	PressureIndex map[string]float64 `json:"pressure_index"`
	// ChaosPerMin is possession changes per minute: high means a scrappy,
	// end-to-end spell, low means one team is in control.
	ChaosPerMin float64 `json:"chaos_per_min"`
	// RhythmPerMin is events of any kind per minute.
	RhythmPerMin float64 `json:"rhythm_per_min"`
}

// Snapshot is the engine's view of the match at one moment.
type Snapshot struct {
	MatchID string        `json:"match_id"`
	Period  int           `json:"period"`
	ClockMs int64         `json:"clock_ms"`
	Score   events.Score  `json:"score"`
	Teams   []TeamStats   `json:"teams"`
	Players []PlayerStats `json:"players"`
	Window  Window        `json:"window"`
}

type windowEntry struct {
	clock   int64
	typ     events.Type
	team    string
	outcome events.Outcome
	weight  float64 // momentum credit for team
}

// Engine consumes events in order and keeps running statistics.
type Engine struct {
	match         events.Match
	windowMinutes int

	teams   map[string]*TeamStats
	players map[string]*PlayerStats

	period        int
	clock         int64
	periodStartMs int64
	score         events.Score

	possessionTeam string
	lastClock      int64

	window []windowEntry
}

// New returns an engine for the given match.
func New(m events.Match) *Engine {
	e := &Engine{
		match:         m,
		windowMinutes: DefaultWindowMinutes,
		teams:         map[string]*TeamStats{},
		players:       map[string]*PlayerStats{},
		period:        1,
	}
	for _, t := range []events.Team{m.Home, m.Away} {
		e.teams[t.ID] = &TeamStats{TeamID: t.ID}
		for _, p := range t.Players {
			e.players[p.ID] = &PlayerStats{PlayerID: p.ID, TeamID: t.ID}
		}
	}
	return e
}

// Apply folds one event into the statistics and returns the numbers derived
// for that event. Events must arrive in (period, clock) order.
func (e *Engine) Apply(ev events.Event) Derived {
	if ev.Period != e.period {
		e.period = ev.Period
		e.periodStartMs = ev.ClockMs
		e.window = e.window[:0]
	}
	e.accruePossession(ev.ClockMs)
	e.clock = ev.ClockMs

	home := e.match.IsHome(ev.TeamID)
	team := e.teams[ev.TeamID]
	player := e.players[ev.PlayerID]
	var d Derived
	weight := 0.0

	switch ev.Type {
	case events.Kickoff:
		e.possessionTeam = ev.TeamID
	case events.PossessionChange:
		e.possessionTeam = ev.TeamID
	case events.HalfTime, events.FullTime:
		e.possessionTeam = ""
	case events.Pass:
		if ev.Start == nil || ev.End == nil {
			break
		}
		d.PassDistanceM = round1(Distance(*ev.Start, *ev.End))
		d.PassDifficulty = PassDifficulty(*ev.Start, *ev.End, home, ev.UnderPressure)
		complete := ev.Outcome == events.Complete
		if team != nil {
			team.Passes++
			team.passDistance += d.PassDistanceM
			if complete {
				team.PassesCompleted++
			}
		}
		if player != nil {
			player.Touches++
			player.Passes++
			player.difficultySum += d.PassDifficulty
			if complete {
				player.PassesCompleted++
			}
		}
		if a, _ := attackingFrame(*ev.End, home); complete && a > 70 {
			weight = 1
		}
	case events.Shot:
		if ev.Start != nil {
			d.ExpectedGoals = round2(ExpectedGoals(*ev.Start, home))
		}
		d.ShotSpeedKmh = round1(ev.BallSpeedKmh)
		onTarget := ev.Outcome == events.OnTarget || ev.Outcome == events.Scored
		weight = 3
		if onTarget {
			weight++
		}
		if team != nil {
			team.Shots++
			team.ExpectedGoals += d.ExpectedGoals
			if onTarget {
				team.ShotsOnTarget++
			}
			if d.ShotSpeedKmh > team.MaxShotSpeedKmh {
				team.MaxShotSpeedKmh = d.ShotSpeedKmh
			}
		}
		if player != nil {
			player.Touches++
			player.Shots++
			player.ExpectedGoals += d.ExpectedGoals
		}
	case events.Goal:
		weight = 2
		if ev.Score != nil {
			e.score = *ev.Score
		}
		if team != nil {
			team.Goals++
		}
		if player != nil {
			player.Goals++
		}
	case events.Tackle:
		if team != nil {
			team.Tackles++
			if ev.Outcome == events.Won {
				team.TacklesWon++
			}
		}
		if ev.Outcome == events.Won {
			weight = 1.5
			if player != nil {
				player.Touches++
				player.TacklesWon++
			}
		}
	case events.Interception:
		weight = 1.5
		if team != nil {
			team.Interceptions++
		}
		if player != nil {
			player.Touches++
			player.Interceptions++
		}
	case events.Pressure:
		weight = 0.5
		if team != nil {
			team.Pressures++
		}
		if player != nil {
			player.Pressures++
		}
	case events.Foul:
		if team != nil {
			team.Fouls++
		}
	case events.Card:
		if team != nil {
			if ev.Card == events.Red {
				team.RedCards++
			} else {
				team.YellowCards++
			}
		}
	}

	e.window = append(e.window, windowEntry{
		clock: ev.ClockMs, typ: ev.Type, team: ev.TeamID, outcome: ev.Outcome, weight: weight,
	})
	e.pruneWindow()
	return d
}

func (e *Engine) accruePossession(now int64) {
	if e.possessionTeam != "" && now > e.lastClock {
		if t := e.teams[e.possessionTeam]; t != nil {
			t.possessionMs += now - e.lastClock
		}
	}
	e.lastClock = now
}

func (e *Engine) pruneWindow() {
	cutoff := e.clock - int64(e.windowMinutes)*60_000
	i := 0
	for i < len(e.window) && e.window[i].clock < cutoff {
		i++
	}
	e.window = e.window[i:]
}

// Snapshot returns the current statistics. Teams are listed home first;
// players are sorted by ID.
func (e *Engine) Snapshot() Snapshot {
	s := Snapshot{
		MatchID: e.match.ID,
		Period:  e.period,
		ClockMs: e.clock,
		Score:   e.score,
		Window:  e.windowSummary(),
	}
	var totalPossession int64
	for _, t := range e.teams {
		totalPossession += t.possessionMs
	}
	for _, id := range []string{e.match.Home.ID, e.match.Away.ID} {
		t := *e.teams[id]
		if t.Passes > 0 {
			t.PassAccuracyPct = round1(100 * float64(t.PassesCompleted) / float64(t.Passes))
			t.AvgPassDistanceM = round1(t.passDistance / float64(t.Passes))
		}
		if totalPossession > 0 {
			t.PossessionPct = round1(100 * float64(t.possessionMs) / float64(totalPossession))
		}
		t.ExpectedGoals = round2(t.ExpectedGoals)
		s.Teams = append(s.Teams, t)
	}
	for _, p := range e.players {
		s.Players = append(s.Players, finishPlayer(*p))
	}
	sort.Slice(s.Players, func(i, j int) bool { return s.Players[i].PlayerID < s.Players[j].PlayerID })
	return s
}

// Player returns the statistics for one player.
func (e *Engine) Player(id string) (PlayerStats, bool) {
	p, ok := e.players[id]
	if !ok {
		return PlayerStats{}, false
	}
	return finishPlayer(*p), true
}

func finishPlayer(p PlayerStats) PlayerStats {
	if p.Passes > 0 {
		p.PassAccuracyPct = round1(100 * float64(p.PassesCompleted) / float64(p.Passes))
		p.AvgPassDifficulty = round1(p.difficultySum / float64(p.Passes))
	}
	p.ExpectedGoals = round2(p.ExpectedGoals)
	return p
}

func (e *Engine) windowSummary() Window {
	w := Window{Minutes: e.windowMinutes, PressureIndex: map[string]float64{}}
	homeID, awayID := e.match.Home.ID, e.match.Away.ID

	var credit = map[string]float64{}
	pressures := map[string]int{}
	passes := map[string]int{}
	changes := 0
	for _, en := range e.window {
		credit[en.team] += en.weight
		switch en.typ {
		case events.Pressure:
			pressures[en.team]++
		case events.Pass:
			passes[en.team]++
		case events.PossessionChange:
			changes++
		}
	}

	if total := credit[homeID] + credit[awayID]; total > 0 {
		w.Momentum = round2((credit[homeID] - credit[awayID]) / total)
	}
	for _, id := range []string{homeID, awayID} {
		opp := passes[e.match.Opponent(id)]
		w.PressureIndex[id] = round1(10 * float64(pressures[id]) / float64(max(opp, 1)))
	}

	elapsedMin := float64(e.clock-e.periodStartMs) / 60_000
	minutes := min(float64(e.windowMinutes), elapsedMin)
	if minutes < 1 {
		minutes = 1
	}
	w.ChaosPerMin = round1(float64(changes) / minutes)
	w.RhythmPerMin = round1(float64(len(e.window)) / minutes)
	return w
}
