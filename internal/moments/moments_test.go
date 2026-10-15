package moments

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ayobamy/touchline/internal/events"
	"github.com/ayobamy/touchline/internal/sim"
)

func run(t *testing.T, seed uint64, scenario string) ([]events.Event, []Moment) {
	t.Helper()
	sc := sim.Baseline()
	if scenario != "" {
		var err error
		sc, err = sim.LoadScenario(filepath.Join("..", "..", "scenarios", scenario+".json"))
		if err != nil {
			t.Fatal(err)
		}
	}
	s, err := sim.New(sim.Config{Seed: seed, Scenario: sc})
	if err != nil {
		t.Fatal(err)
	}
	det := New(s.Match(), DefaultThresholds())
	var evs []events.Event
	var ms []Moment
	s.Run(func(ev events.Event) {
		evs = append(evs, ev)
		_, got := det.Apply(ev)
		ms = append(ms, got...)
	})
	return evs, ms
}

func TestEveryGoalAndCardBecomesAMoment(t *testing.T) {
	for _, sc := range []string{"", "late-press", "red-card"} {
		evs, ms := run(t, 7, sc)
		want := map[string]bool{}
		for _, ev := range evs {
			if ev.Type == events.Goal || ev.Type == events.Card {
				want[ev.ID] = true
			}
		}
		for _, m := range ms {
			if m.Kind == Goal || m.Kind == Card {
				delete(want, m.EventIDs[len(m.EventIDs)-1])
			}
		}
		if len(want) > 0 {
			t.Errorf("scenario %q: goals or cards without a moment: %v", sc, want)
		}
	}
}

func TestLatePressIsDetected(t *testing.T) {
	_, ms := run(t, 7, "late-press")
	for _, m := range ms {
		if m.Kind == PressSwitch && m.TeamID == "iro" && m.ClockMs >= 60*60_000 {
			return
		}
	}
	t.Fatal("no press switch by Ironbridge Rovers after the hour")
}

func TestMomentsAreWellFormedAndNotTooMany(t *testing.T) {
	for seed := uint64(1); seed <= 6; seed++ {
		evs, ms := run(t, seed, "late-press")
		ids := map[string]events.Event{}
		for _, ev := range evs {
			ids[ev.ID] = ev
		}
		if n := len(ms); n < 20 || n > 70 {
			t.Errorf("seed %d: %d moments, want 20 to 70", seed, n)
		}
		seen := map[string]bool{}
		for _, m := range ms {
			if seen[m.ID] {
				t.Fatalf("seed %d: duplicate moment id %s", seed, m.ID)
			}
			seen[m.ID] = true
			if m.Summary == "" || len(m.EventIDs) == 0 {
				t.Fatalf("seed %d: moment without summary or evidence: %+v", seed, m)
			}
			for _, id := range m.EventIDs {
				ev, ok := ids[id]
				if !ok {
					t.Fatalf("seed %d: moment %s cites unknown event %s", seed, m.ID, id)
				}
				if ev.Period != m.Period || ev.ClockMs > m.ClockMs {
					t.Fatalf("seed %d: moment %s cites event %s from the future or another half", seed, m.ID, id)
				}
			}
		}
	}
}

func TestMomentumSwingsNeedADominantTeam(t *testing.T) {
	th := DefaultThresholds()
	for seed := uint64(1); seed <= 6; seed++ {
		_, ms := run(t, seed, "")
		for _, m := range ms {
			if m.Kind != MomentumSwing {
				continue
			}
			mom := m.Metrics["momentum"]
			if mom > -th.MomentumLevel && mom < th.MomentumLevel {
				t.Errorf("seed %d: swing at momentum %.2f, below the %.2f level", seed, mom, th.MomentumLevel)
			}
			if (mom > 0) != (m.TeamID == "riv") {
				t.Errorf("seed %d: swing credited to %s at momentum %.2f", seed, m.TeamID, mom)
			}
		}
	}
}

func TestCooldownLimitsRepeats(t *testing.T) {
	th := DefaultThresholds()
	_, ms := run(t, 7, "late-press")
	last := map[string]int64{}
	for _, m := range ms {
		if m.Kind != MomentumSwing && m.Kind != PressSwitch && m.Kind != ChaosSpell {
			continue
		}
		key := string(m.Kind) + "/" + m.TeamID + "/" + string(rune('0'+m.Period))
		if prev, ok := last[key]; ok && m.ClockMs-prev < th.CooldownMs {
			t.Errorf("%s fired %d s after the previous one", key, (m.ClockMs-prev)/1000)
		}
		last[key] = m.ClockMs
	}
}

func TestSummariesUseNames(t *testing.T) {
	_, ms := run(t, 7, "late-press")
	for _, m := range ms {
		if strings.Contains(m.Summary, "riv-") || strings.Contains(m.Summary, "iro-") {
			t.Errorf("summary shows a raw ID instead of a name: %q", m.Summary)
		}
	}
}
