// Package overlay defines the on-screen output contract. Every overlay the
// agents produce is validated against it before it reaches a screen.
//
// The rule that matters most: no evidence, no overlay. Each overlay must cite
// the event IDs that back its claim, so a broadcaster can trace any line on
// screen back to what happened on the pitch.
package overlay

import (
	"errors"
	"fmt"
	"regexp"
	"unicode/utf8"
)

// Slot is where an overlay sits on screen.
type Slot string

// Screen slots.
const (
	LowerThird Slot = "lower_third"
	TopBar     Slot = "top_bar"
	SidePanel  Slot = "side_panel"
	FullScreen Slot = "full_screen"
)

// Audience is the viewing mode an overlay was written for.
type Audience string

// Audiences.
const (
	Analyst Audience = "analyst"
	Casual  Audience = "casual"
	All     Audience = "all"
)

// Limits that keep overlays readable during live play.
const (
	MaxHeadlineRunes = 60
	MaxBodyRunes     = 160
	MaxMetrics       = 4
	MinTTLMs         = 2_000
	MaxTTLMs         = 30_000
)

var localePattern = regexp.MustCompile(`^[a-z]{2,3}(-[A-Za-z0-9]{2,8})*$`)

// Metric is one number shown on an overlay.
type Metric struct {
	Label string `json:"label"`
	Value string `json:"value"`
	Unit  string `json:"unit,omitempty"`
}

// Overlay is one timed, machine-readable graphic.
type Overlay struct {
	ID               string   `json:"id"`
	MatchID          string   `json:"match_id"`
	Period           int      `json:"period"`
	ShowAtMs         int64    `json:"show_at_ms"`
	TTLMs            int64    `json:"ttl_ms"`
	Priority         int      `json:"priority"`
	Slot             Slot     `json:"slot"`
	Audience         Audience `json:"audience"`
	Locale           string   `json:"locale"`
	TeamID           string   `json:"team_id,omitempty"`
	PlayerID         string   `json:"player_id,omitempty"`
	Headline         string   `json:"headline"`
	Body             string   `json:"body,omitempty"`
	Metrics          []Metric `json:"metrics,omitempty"`
	EvidenceEventIDs []string `json:"evidence_event_ids"`
	Producer         string   `json:"producer"`
	Fallback         bool     `json:"fallback,omitempty"`
}

// Validate reports every way the overlay breaks the contract.
func (o Overlay) Validate() error {
	var errs []error
	add := func(format string, args ...any) { errs = append(errs, fmt.Errorf(format, args...)) }

	if o.ID == "" {
		add("id is required")
	}
	if o.MatchID == "" {
		add("match_id is required")
	}
	if o.Period != 1 && o.Period != 2 {
		add("period must be 1 or 2, got %d", o.Period)
	}
	if o.ShowAtMs < 0 {
		add("show_at_ms must be >= 0")
	}
	if o.TTLMs < MinTTLMs || o.TTLMs > MaxTTLMs {
		add("ttl_ms must be between %d and %d, got %d", MinTTLMs, MaxTTLMs, o.TTLMs)
	}
	if o.Priority < 1 || o.Priority > 5 {
		add("priority must be 1 (highest) to 5, got %d", o.Priority)
	}
	switch o.Slot {
	case LowerThird, TopBar, SidePanel, FullScreen:
	default:
		add("unknown slot %q", o.Slot)
	}
	switch o.Audience {
	case Analyst, Casual, All:
	default:
		add("unknown audience %q", o.Audience)
	}
	if !localePattern.MatchString(o.Locale) {
		add("locale %q is not a BCP 47 tag such as en, pcm or yo", o.Locale)
	}
	if o.Headline == "" {
		add("headline is required")
	} else if n := utf8.RuneCountInString(o.Headline); n > MaxHeadlineRunes {
		add("headline is %d characters, max %d", n, MaxHeadlineRunes)
	}
	if n := utf8.RuneCountInString(o.Body); n > MaxBodyRunes {
		add("body is %d characters, max %d", n, MaxBodyRunes)
	}
	if len(o.Metrics) > MaxMetrics {
		add("at most %d metrics, got %d", MaxMetrics, len(o.Metrics))
	}
	for i, m := range o.Metrics {
		if m.Label == "" || m.Value == "" {
			add("metrics[%d] needs label and value", i)
		}
	}
	if len(o.EvidenceEventIDs) == 0 {
		add("evidence_event_ids must cite at least one event")
	}
	for i, id := range o.EvidenceEventIDs {
		if id == "" {
			add("evidence_event_ids[%d] is empty", i)
		}
	}
	if o.Producer == "" {
		add("producer is required (the agent or template that wrote it)")
	}
	return errors.Join(errs...)
}
