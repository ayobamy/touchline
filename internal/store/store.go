// Package store keeps the live state of one match — every event, its derived
// numbers, the running statistics and the candidate moments — and answers
// read-only questions about it. It is safe for one writer (the replay) and
// many concurrent readers (MCP tool calls, HTTP requests).
package store

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"sync"

	"github.com/ayobamy/touchline/internal/events"
	"github.com/ayobamy/touchline/internal/moments"
	"github.com/ayobamy/touchline/internal/stats"
)

// EventView is an event together with the numbers derived from it.
type EventView struct {
	events.Event
	Derived stats.Derived `json:"derived"`
}

// Status is a one-line view of where the match is.
type Status struct {
	MatchID  string       `json:"match_id"`
	Period   int          `json:"period"`
	ClockMs  int64        `json:"clock_ms"`
	Clock    string       `json:"clock"`
	Score    events.Score `json:"score"`
	Events   int          `json:"events"`
	Moments  int          `json:"moments"`
	Finished bool         `json:"finished"`
}

// Store holds one match.
type Store struct {
	mu          sync.RWMutex
	match       events.Match
	det         *moments.Detector
	evs         []events.Event
	index       map[string]int
	derived     map[string]stats.Derived
	moments     []moments.Moment
	periodStart map[int]int64
	finished    bool
}

// New returns an empty store for the match.
func New(m events.Match, th moments.Thresholds) *Store {
	return &Store{
		match:       m,
		det:         moments.New(m, th),
		index:       map[string]int{},
		derived:     map[string]stats.Derived{},
		periodStart: map[int]int64{},
	}
}

// Apply adds the next event and returns any moments it triggered. Events
// already applied (same seq or earlier) are ignored, so a replayed stream is
// harmless.
func (s *Store) Apply(ev events.Event) ([]moments.Moment, error) {
	if err := ev.Validate(); err != nil {
		return nil, fmt.Errorf("event %s: %w", ev.ID, err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if n := len(s.evs); n > 0 && ev.Seq <= s.evs[n-1].Seq {
		return nil, nil
	}
	if _, ok := s.periodStart[ev.Period]; !ok {
		s.periodStart[ev.Period] = ev.ClockMs
	}
	d, ms := s.det.Apply(ev)
	s.index[ev.ID] = len(s.evs)
	s.evs = append(s.evs, ev)
	s.derived[ev.ID] = d
	s.moments = append(s.moments, ms...)
	if ev.Type == events.FullTime {
		s.finished = true
	}
	return ms, nil
}

// Match returns the fixture.
func (s *Store) Match() events.Match { return s.match }

// Status reports the clock, score and counts.
func (s *Store) Status() Status {
	s.mu.RLock()
	defer s.mu.RUnlock()
	snap := s.det.Engine().Snapshot()
	return Status{
		MatchID: s.match.ID, Period: snap.Period, ClockMs: snap.ClockMs, Clock: FormatClock(snap.ClockMs),
		Score: snap.Score, Events: len(s.evs), Moments: len(s.moments), Finished: s.finished,
	}
}

// Snapshot returns the running statistics.
func (s *Store) Snapshot() stats.Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.det.Engine().Snapshot()
}

// Player returns one player's statistics.
func (s *Store) Player(id string) (stats.PlayerStats, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.det.Engine().Player(id)
}

// Window summarises the last `minutes` of the current period.
func (s *Store) Window(minutes int) (stats.Window, error) {
	if minutes < 1 || minutes > 45 {
		return stats.Window{}, errors.New("minutes must be between 1 and 45")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.evs) == 0 {
		return stats.Window{Minutes: minutes, PressureIndex: map[string]float64{}}, nil
	}
	last := s.evs[len(s.evs)-1]
	return stats.SummarizeWindow(s.match, s.periodEvents(last.Period), s.periodStart[last.Period], last.ClockMs, minutes), nil
}

// Event returns one event and its derived numbers.
func (s *Store) Event(id string) (EventView, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	i, ok := s.index[id]
	if !ok {
		return EventView{}, false
	}
	ev := s.evs[i]
	return EventView{Event: ev, Derived: s.derived[ev.ID]}, true
}

// Filter selects events. Zero values mean "any".
type Filter struct {
	Period   int
	FromMs   int64
	ToMs     int64 // 0 means up to now
	TeamID   string
	PlayerID string // matches the acting player or the recipient
	Types    []events.Type
	Limit    int // most recent N matching events; 0 means 50
}

// MaxEvents caps one Events call.
const MaxEvents = 200

// Events returns matching events, oldest first, keeping the most recent
// Limit of them, and the total number that matched.
func (s *Store) Events(f Filter) ([]EventView, int) {
	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}
	limit = min(limit, MaxEvents)

	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []EventView
	total := 0
	for _, ev := range s.evs {
		if f.Period != 0 && ev.Period != f.Period {
			continue
		}
		if ev.ClockMs < f.FromMs || (f.ToMs > 0 && ev.ClockMs > f.ToMs) {
			continue
		}
		if f.TeamID != "" && ev.TeamID != f.TeamID {
			continue
		}
		if f.PlayerID != "" && ev.PlayerID != f.PlayerID && ev.RecipientID != f.PlayerID {
			continue
		}
		if len(f.Types) > 0 && !slices.Contains(f.Types, ev.Type) {
			continue
		}
		total++
		out = append(out, EventView{Event: ev, Derived: s.derived[ev.ID]})
	}
	if len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out, total
}

// Moments returns the moments after the first `after`, and the count to pass
// as `after` next time.
func (s *Store) Moments(after int) ([]moments.Moment, int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	after = max(0, min(after, len(s.moments)))
	return slices.Clone(s.moments[after:]), len(s.moments)
}

// Range is a stretch of one half.
type Range struct {
	Period int   `json:"period"`
	FromMs int64 `json:"from_ms"`
	ToMs   int64 `json:"to_ms"`
}

// Comparable metrics for Compare.
var Metrics = []string{
	"passes", "passes_completed", "pass_accuracy_pct", "shots", "shots_on_target",
	"goals", "xg", "tackles_won", "interceptions", "pressures", "pressure_index", "momentum",
}

// RangeValues is a metric for each team over one range.
type RangeValues struct {
	Range  Range              `json:"range"`
	Clock  string             `json:"clock"`
	Values map[string]float64 `json:"values"`
}

// Comparison is the result of Compare.
type Comparison struct {
	Metric string      `json:"metric"`
	A      RangeValues `json:"a"`
	B      RangeValues `json:"b"`
}

// Compare computes a metric for both teams over two ranges, to support claims
// such as "since the 60th minute".
func (s *Store) Compare(metric string, a, b Range) (Comparison, error) {
	if !slices.Contains(Metrics, metric) {
		return Comparison{}, fmt.Errorf("unknown metric %q; choose one of %v", metric, Metrics)
	}
	for _, r := range []Range{a, b} {
		if r.Period != 1 && r.Period != 2 {
			return Comparison{}, errors.New("each range needs period 1 or 2")
		}
		if r.ToMs <= r.FromMs {
			return Comparison{}, errors.New("each range needs to_ms after from_ms")
		}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return Comparison{Metric: metric, A: s.rangeValues(metric, a), B: s.rangeValues(metric, b)}, nil
}

func (s *Store) rangeValues(metric string, r Range) RangeValues {
	var evs []events.Event
	for _, ev := range s.periodEvents(r.Period) {
		if ev.ClockMs >= r.FromMs && ev.ClockMs <= r.ToMs {
			evs = append(evs, ev)
		}
	}
	values := map[string]float64{}
	home, away := s.match.Home.ID, s.match.Away.ID

	if metric == "momentum" {
		minutes := int((r.ToMs-r.FromMs)/60_000) + 1
		w := stats.SummarizeWindow(s.match, evs, r.FromMs, r.ToMs, minutes)
		values[home] = w.Momentum
		values[away] = -w.Momentum
	} else {
		eng := stats.New(s.match)
		for _, ev := range evs {
			eng.Apply(ev)
		}
		teams := eng.Snapshot().Teams
		for i, t := range teams {
			opp := teams[1-i]
			switch metric {
			case "passes":
				values[t.TeamID] = float64(t.Passes)
			case "passes_completed":
				values[t.TeamID] = float64(t.PassesCompleted)
			case "pass_accuracy_pct":
				values[t.TeamID] = t.PassAccuracyPct
			case "shots":
				values[t.TeamID] = float64(t.Shots)
			case "shots_on_target":
				values[t.TeamID] = float64(t.ShotsOnTarget)
			case "goals":
				values[t.TeamID] = float64(t.Goals)
			case "xg":
				values[t.TeamID] = t.ExpectedGoals
			case "tackles_won":
				values[t.TeamID] = float64(t.TacklesWon)
			case "interceptions":
				values[t.TeamID] = float64(t.Interceptions)
			case "pressures":
				values[t.TeamID] = float64(t.Pressures)
			case "pressure_index":
				values[t.TeamID] = math.Round(100*float64(t.Pressures)/float64(max(opp.Passes, 1))) / 10
			}
		}
	}
	return RangeValues{Range: r, Clock: FormatClock(r.FromMs) + " to " + FormatClock(r.ToMs), Values: values}
}

func (s *Store) periodEvents(period int) []events.Event {
	lo, _ := slices.BinarySearchFunc(s.evs, period, func(e events.Event, p int) int { return e.Period - p })
	hi, _ := slices.BinarySearchFunc(s.evs, period+1, func(e events.Event, p int) int { return e.Period - p })
	return s.evs[lo:hi]
}

// FormatClock renders a match clock as minutes and seconds, e.g. "63:07".
func FormatClock(ms int64) string {
	return fmt.Sprintf("%02d:%02d", ms/60_000, (ms/1000)%60)
}
