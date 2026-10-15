package events

import (
	"reflect"
	"strings"
	"testing"

	"github.com/ayobamy/touchline/internal/schemacheck"
)

func validPass() Event {
	return Event{
		ID: "m-0001-00002", MatchID: "m-0001", Seq: 2, Type: Pass, Period: 1, ClockMs: 4000,
		TeamID: "riv", PlayerID: "riv-09", RecipientID: "riv-08",
		Start: &Point{X: 52.5, Y: 34}, End: &Point{X: 60, Y: 40},
		Outcome: Complete, BallSpeedKmh: 41.2,
	}
}

func TestValidateAcceptsValidEvents(t *testing.T) {
	cases := map[string]Event{
		"pass": validPass(),
		"goal": {
			ID: "m-0001-00100", MatchID: "m-0001", Seq: 100, Type: Goal, Period: 2, ClockMs: 3_000_000,
			TeamID: "riv", PlayerID: "riv-09", RelatedID: "m-0001-00099", Score: &Score{Home: 1},
		},
		"full time": {
			ID: "m-0001-01500", MatchID: "m-0001", Seq: 1500, Type: FullTime, Period: 2, ClockMs: 5_600_000,
			Score: &Score{Home: 1, Away: 1},
		},
	}
	for name, ev := range cases {
		if err := ev.Validate(); err != nil {
			t.Errorf("%s: unexpected error: %v", name, err)
		}
	}
}

func TestValidateRejectsBrokenEvents(t *testing.T) {
	cases := map[string]struct {
		mutate func(*Event)
		want   string
	}{
		"off pitch":             {func(e *Event) { e.End = &Point{X: 120, Y: 34} }, "off the pitch"},
		"complete no recipient": {func(e *Event) { e.RecipientID = "" }, "recipient_id"},
		"wrong outcome":         {func(e *Event) { e.Outcome = Won }, "outcome must be one of"},
		"no speed":              {func(e *Event) { e.BallSpeedKmh = 0 }, "ball_speed_kmh"},
		"bad period":            {func(e *Event) { e.Period = 3 }, "period"},
		"unknown type":          {func(e *Event) { e.Type = "dribble" }, "unknown type"},
		"card on pass":          {func(e *Event) { e.Card = Red }, "only card events"},
	}
	for name, tc := range cases {
		ev := validPass()
		tc.mutate(&ev)
		err := ev.Validate()
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: want error containing %q, got %v", name, tc.want, err)
		}
	}
}

func TestSchemasMatchGoTypes(t *testing.T) {
	cases := []struct {
		path string
		typ  reflect.Type
	}{
		{"../../schema/event.schema.json", reflect.TypeFor[Event]()},
		{"../../schema/match.schema.json", reflect.TypeFor[Match]()},
	}
	for _, tc := range cases {
		diff, err := schemacheck.Diff(tc.path, tc.typ)
		if err != nil {
			t.Fatal(err)
		}
		if diff != "" {
			t.Errorf("schema drift: %s", diff)
		}
	}
}
