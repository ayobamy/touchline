package sim

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ayobamy/touchline/internal/events"
)

func play(t *testing.T, seed uint64, sc Scenario) (events.Match, []events.Event) {
	t.Helper()
	s, err := New(Config{Seed: seed, Scenario: sc})
	if err != nil {
		t.Fatal(err)
	}
	var evs []events.Event
	s.Run(func(ev events.Event) { evs = append(evs, ev) })
	return s.Match(), evs
}

func TestSameSeedSameMatch(t *testing.T) {
	_, a := play(t, 7, Baseline())
	_, b := play(t, 7, Baseline())
	if !reflect.DeepEqual(a, b) {
		t.Fatal("same seed produced different matches")
	}
	_, c := play(t, 8, Baseline())
	if reflect.DeepEqual(a, c) {
		t.Fatal("different seeds produced the same match")
	}
}

func TestStreamsHonourTheContract(t *testing.T) {
	for seed := uint64(1); seed <= 20; seed++ {
		m, evs := play(t, seed, Baseline())
		checkStream(t, seed, m, evs)
	}
}

func checkStream(t *testing.T, seed uint64, m events.Match, evs []events.Event) {
	t.Helper()
	if len(evs) < 500 {
		t.Fatalf("seed %d: only %d events", seed, len(evs))
	}
	ids := map[string]bool{}
	goals := events.Score{}
	passes := 0
	for i, ev := range evs {
		if err := ev.Validate(); err != nil {
			t.Fatalf("seed %d: event %d invalid: %v\n%+v", seed, i, err, ev)
		}
		if ev.Seq != i+1 {
			t.Fatalf("seed %d: event %d has seq %d", seed, i, ev.Seq)
		}
		if ids[ev.ID] {
			t.Fatalf("seed %d: duplicate id %s", seed, ev.ID)
		}
		ids[ev.ID] = true
		if i > 0 {
			prev := evs[i-1]
			if ev.Period < prev.Period || (ev.Period == prev.Period && ev.ClockMs < prev.ClockMs) {
				t.Fatalf("seed %d: event %d goes back in time", seed, i)
			}
		}
		switch ev.Type {
		case events.Goal:
			if m.IsHome(ev.TeamID) {
				goals.Home++
			} else {
				goals.Away++
			}
			if *ev.Score != goals {
				t.Fatalf("seed %d: goal score %+v, counted %+v", seed, *ev.Score, goals)
			}
		case events.Pass:
			passes++
		}
	}
	last := evs[len(evs)-1]
	if last.Type != events.FullTime || *last.Score != goals {
		t.Fatalf("seed %d: last event %+v, want full_time with score %+v", seed, last, goals)
	}
	if passes < 600 || passes > 1500 {
		t.Errorf("seed %d: %d passes, want a realistic 600 to 1500", seed, passes)
	}
	if goals.Home+goals.Away > 12 {
		t.Errorf("seed %d: %d goals is not realistic", seed, goals.Home+goals.Away)
	}
}

func TestRedCardScenario(t *testing.T) {
	sc, err := LoadScenario(filepath.Join("..", "..", "scenarios", "red-card.json"))
	if err != nil {
		t.Fatal(err)
	}
	m, evs := play(t, 7, sc)
	checkStream(t, 7, m, evs)

	sentOff := ""
	for _, ev := range evs {
		if sentOff != "" && (ev.PlayerID == sentOff || ev.RecipientID == sentOff) {
			t.Fatalf("%s was sent off but still involved: %+v", sentOff, ev)
		}
		if ev.Type == events.Card && ev.Card == events.Red && ev.TeamID == "riv" && sentOff == "" {
			sentOff = ev.PlayerID
			if ev.PlayerID != "riv-05" {
				t.Errorf("scripted red for #5, got %s", ev.PlayerID)
			}
			if minute := ev.ClockMs / 60_000; minute < 58 || minute > 60 {
				t.Errorf("red card at minute %d, want about 58", minute)
			}
		}
	}
	if sentOff == "" {
		t.Fatal("no red card shown")
	}
}

func TestScenarioFilesAreValid(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("..", "..", "scenarios", "*.json"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no scenarios found: %v", err)
	}
	for _, p := range paths {
		if _, err := LoadScenario(p); err != nil {
			t.Error(err)
		}
	}
}

func TestMatchMetadataRoundTrips(t *testing.T) {
	m, _ := play(t, 7, Baseline())
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	var back events.Match
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(m, back) || len(m.Home.Players) != 11 || len(m.Away.Players) != 11 {
		t.Fatalf("match metadata did not round-trip: %+v", back)
	}
}
