package stats

import (
	"testing"

	"github.com/ayobamy/touchline/internal/events"
)

func TestExpectedGoalsCalibration(t *testing.T) {
	cases := []struct {
		name   string
		p      events.Point
		lo, hi float64
	}{
		{"six yards, central", events.Point{X: 99.5, Y: 34}, 0.6, 0.8},
		{"penalty spot", events.Point{X: 94, Y: 34}, 0.25, 0.38},
		{"25 m, central", events.Point{X: 80, Y: 34}, 0.03, 0.09},
		{"tight angle", events.Point{X: 103, Y: 52}, 0.01, 0.08},
	}
	for _, tc := range cases {
		got := ExpectedGoals(tc.p, true)
		if got < tc.lo || got > tc.hi {
			t.Errorf("%s: xG %.3f, want %.2f to %.2f", tc.name, got, tc.lo, tc.hi)
		}
	}
}

func TestExpectedGoalsIsMirroredForAwayTeam(t *testing.T) {
	home := ExpectedGoals(events.Point{X: 94, Y: 30}, true)
	away := ExpectedGoals(events.Point{X: 11, Y: 38}, false)
	if home != away {
		t.Fatalf("mirrored shots differ: home %.4f, away %.4f", home, away)
	}
}

func TestPassDifficulty(t *testing.T) {
	short := PassDifficulty(events.Point{X: 40, Y: 34}, events.Point{X: 45, Y: 34}, true, false)
	long := PassDifficulty(events.Point{X: 40, Y: 34}, events.Point{X: 85, Y: 34}, true, false)
	pressed := PassDifficulty(events.Point{X: 40, Y: 34}, events.Point{X: 45, Y: 34}, true, true)
	backwards := PassDifficulty(events.Point{X: 45, Y: 34}, events.Point{X: 40, Y: 34}, true, false)

	if !(long > short) {
		t.Errorf("long pass (%.1f) should be harder than short (%.1f)", long, short)
	}
	if pressed != short+2.5 {
		t.Errorf("pressure should add 2.5: short %.1f, pressed %.1f", short, pressed)
	}
	if !(backwards < short) {
		t.Errorf("backwards pass (%.1f) should be easier than forward (%.1f)", backwards, short)
	}
	if long > 10 {
		t.Errorf("difficulty capped at 10, got %.1f", long)
	}
}

func testMatch() events.Match {
	return events.Match{
		ID:   "m-test",
		Home: events.Team{ID: "h", Players: []events.Player{{ID: "h-1"}, {ID: "h-2"}}},
		Away: events.Team{ID: "a", Players: []events.Player{{ID: "a-1"}}},
	}
}

type builder struct{ seq int }

func (b *builder) ev(t events.Type, clock int64, team, player string) events.Event {
	b.seq++
	return events.Event{ID: "e", MatchID: "m-test", Seq: b.seq, Type: t, Period: 1, ClockMs: clock, TeamID: team, PlayerID: player}
}

func pass(b *builder, clock int64, team, player string, complete bool) events.Event {
	ev := b.ev(events.Pass, clock, team, player)
	ev.Start, ev.End = &events.Point{X: 40, Y: 34}, &events.Point{X: 50, Y: 34}
	ev.BallSpeedKmh = 40
	ev.Outcome = events.Incomplete
	if complete {
		ev.Outcome = events.Complete
		ev.RecipientID = "h-2"
	}
	return ev
}

func TestEngineCountsPassesAndPossession(t *testing.T) {
	b := &builder{}
	e := New(testMatch())
	e.Apply(b.ev(events.Kickoff, 0, "h", "h-1"))
	e.Apply(pass(b, 10_000, "h", "h-1", true))
	e.Apply(pass(b, 20_000, "h", "h-1", true))
	e.Apply(pass(b, 30_000, "h", "h-1", false))
	e.Apply(b.ev(events.PossessionChange, 30_000, "a", "a-1")) // home had it 30 s
	e.Apply(pass(b, 40_000, "a", "a-1", true))                 // away has it 10 s

	s := e.Snapshot()
	home, away := s.Teams[0], s.Teams[1]
	if home.Passes != 3 || home.PassesCompleted != 2 {
		t.Fatalf("home passes %d/%d, want 2/3", home.PassesCompleted, home.Passes)
	}
	if home.PassAccuracyPct != 66.7 {
		t.Errorf("home accuracy %.1f, want 66.7", home.PassAccuracyPct)
	}
	if home.AvgPassDistanceM != 10 {
		t.Errorf("home avg pass distance %.1f, want 10", home.AvgPassDistanceM)
	}
	if home.PossessionPct != 75 || away.PossessionPct != 25 {
		t.Errorf("possession %.1f/%.1f, want 75/25", home.PossessionPct, away.PossessionPct)
	}
	p, ok := e.Player("h-1")
	if !ok || p.Passes != 3 || p.Touches != 3 {
		t.Errorf("player h-1: %+v", p)
	}
}

func TestEngineMomentumFavoursTheTeamOnTop(t *testing.T) {
	b := &builder{}
	e := New(testMatch())
	e.Apply(b.ev(events.Kickoff, 0, "a", "a-1"))
	for i := range 5 {
		ev := b.ev(events.Shot, int64(10_000*(i+1)), "a", "a-1")
		ev.Start, ev.End = &events.Point{X: 12, Y: 34}, &events.Point{X: 0, Y: 34}
		ev.Outcome, ev.BallSpeedKmh = events.OnTarget, 80
		e.Apply(ev)
	}
	pr := b.ev(events.Pressure, 70_000, "h", "h-1")
	pr.Start, pr.OpponentID = &events.Point{X: 20, Y: 34}, "a-1"
	e.Apply(pr)

	w := e.Snapshot().Window
	if w.Momentum >= -0.5 {
		t.Errorf("away team dominating, momentum %.2f, want below -0.5", w.Momentum)
	}
	s := e.Snapshot()
	if s.Teams[1].Shots != 5 || s.Teams[1].ShotsOnTarget != 5 {
		t.Errorf("away shots %+v", s.Teams[1])
	}
}

func TestEngineWindowDropsOldEvents(t *testing.T) {
	b := &builder{}
	e := New(testMatch())
	e.Apply(b.ev(events.Kickoff, 0, "h", "h-1"))
	e.Apply(pass(b, 10_000, "h", "h-1", true))
	e.Apply(pass(b, 20*60_000, "a", "a-1", true)) // 20 minutes later
	if got := len(e.window); got != 1 {
		t.Fatalf("window holds %d events, want 1", got)
	}
}
