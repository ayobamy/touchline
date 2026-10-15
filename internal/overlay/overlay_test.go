package overlay

import (
	"reflect"
	"strings"
	"testing"

	"github.com/ayobamy/touchline/internal/schemacheck"
)

func validOverlay() Overlay {
	return Overlay{
		ID: "ov-0001", MatchID: "m-0001", Period: 2, ShowAtMs: 3_900_000, TTLMs: 8000,
		Priority: 2, Slot: LowerThird, Audience: Casual, Locale: "pcm",
		TeamID:           "iro",
		Headline:         "Rovers don dey press high",
		Body:             "Since 60 minutes, Rovers dey win ball for Riverside half. Na why di goal come.",
		Metrics:          []Metric{{Label: "Pressure index", Value: "9.4", Unit: "per 10 passes"}},
		EvidenceEventIDs: []string{"m-0001-01201", "m-0001-01207"},
		Producer:         "narrator",
	}
}

func TestValidateAcceptsValidOverlay(t *testing.T) {
	if err := validOverlay().Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateRejectsBrokenOverlays(t *testing.T) {
	cases := map[string]struct {
		mutate func(*Overlay)
		want   string
	}{
		"no evidence":   {func(o *Overlay) { o.EvidenceEventIDs = nil }, "evidence_event_ids"},
		"long headline": {func(o *Overlay) { o.Headline = strings.Repeat("a", 61) }, "headline is 61"},
		"bad locale":    {func(o *Overlay) { o.Locale = "English" }, "locale"},
		"ttl too long":  {func(o *Overlay) { o.TTLMs = 60_000 }, "ttl_ms"},
		"bad priority":  {func(o *Overlay) { o.Priority = 0 }, "priority"},
		"bad slot":      {func(o *Overlay) { o.Slot = "corner" }, "slot"},
		"no producer":   {func(o *Overlay) { o.Producer = "" }, "producer"},
	}
	for name, tc := range cases {
		o := validOverlay()
		tc.mutate(&o)
		err := o.Validate()
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: want error containing %q, got %v", name, tc.want, err)
		}
	}
}

func TestSchemaMatchesGoType(t *testing.T) {
	diff, err := schemacheck.Diff("../../schema/overlay.schema.json", reflect.TypeFor[Overlay]())
	if err != nil {
		t.Fatal(err)
	}
	if diff != "" {
		t.Errorf("schema drift: %s", diff)
	}
}
