package store

import (
	"testing"

	"github.com/ayobamy/touchline/internal/events"
	"github.com/ayobamy/touchline/internal/moments"
	"github.com/ayobamy/touchline/internal/sim"
)

func TestReplayingEventsIsHarmless(t *testing.T) {
	s, _ := sim.New(sim.Config{Seed: 3})
	var evs []events.Event
	s.Run(func(ev events.Event) { evs = append(evs, ev) })

	st := New(s.Match(), moments.DefaultThresholds())
	half := len(evs) / 2
	for _, ev := range evs[:half] {
		if _, err := st.Apply(ev); err != nil {
			t.Fatal(err)
		}
	}
	before := st.Snapshot()
	for _, ev := range evs[:half] { // a redelivered partition
		if _, err := st.Apply(ev); err != nil {
			t.Fatal(err)
		}
	}
	if after := st.Snapshot(); after.Teams[0].Passes != before.Teams[0].Passes || st.Status().Events != half {
		t.Fatalf("replay double-counted: passes %d then %d, events %d", before.Teams[0].Passes, after.Teams[0].Passes, st.Status().Events)
	}
	for _, ev := range evs[half:] {
		if _, err := st.Apply(ev); err != nil {
			t.Fatal(err)
		}
	}
	if status := st.Status(); !status.Finished || status.Events != len(evs) {
		t.Fatalf("status after full replay: %+v", status)
	}
}

func TestInvalidEventsAreRejected(t *testing.T) {
	s, _ := sim.New(sim.Config{Seed: 3})
	st := New(s.Match(), moments.DefaultThresholds())
	if _, err := st.Apply(events.Event{ID: "x", MatchID: "m", Seq: 1, Type: "dribble", Period: 1}); err == nil {
		t.Fatal("invalid event accepted")
	}
}

func TestEventFilters(t *testing.T) {
	s, _ := sim.New(sim.Config{Seed: 3})
	st := New(s.Match(), moments.DefaultThresholds())
	s.Run(func(ev events.Event) { _, _ = st.Apply(ev) })

	got, total := st.Events(Filter{Period: 2, FromMs: 60 * 60_000, ToMs: 70 * 60_000, TeamID: "riv", Types: []events.Type{events.Pass}, Limit: 500})
	if len(got) != total || total == 0 || len(got) > MaxEvents {
		t.Fatalf("returned %d of %d", len(got), total)
	}
	for _, ev := range got {
		if ev.Period != 2 || ev.ClockMs < 60*60_000 || ev.ClockMs > 70*60_000 || ev.TeamID != "riv" || ev.Type != events.Pass {
			t.Fatalf("event outside the filter: %+v", ev.Event)
		}
	}
	if FormatClock(3_787_000) != "63:07" {
		t.Errorf("FormatClock = %s", FormatClock(3_787_000))
	}
}
