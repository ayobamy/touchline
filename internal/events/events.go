// Package events defines the synthetic match event contract shared by the
// simulator, the stats engine, the MCP server and the agents.
//
// Coordinates are metres on a 105 x 68 pitch. X runs from the home team's
// goal line (0) to the away team's goal line (105), so the home team always
// attacks towards +X and the away team towards -X. Teams do not switch ends at
// half time; that keeps every downstream calculation simple. Y runs from one
// touchline (0) to the other (68).
//
// Events are ordered by (period, clock_ms). clock_ms is the match clock, so
// the second half starts at 45:00 even if first-half stoppage time ran past it.
package events

import (
	"errors"
	"fmt"
	"slices"
)

// Pitch dimensions in metres.
const (
	PitchLength = 105.0
	PitchWidth  = 68.0
)

// Type is the kind of match event.
type Type string

// Event types emitted by the simulator.
const (
	Kickoff          Type = "kickoff"
	Pass             Type = "pass"
	Shot             Type = "shot"
	Tackle           Type = "tackle"
	Interception     Type = "interception"
	Pressure         Type = "pressure"
	PossessionChange Type = "possession_change"
	Foul             Type = "foul"
	Card             Type = "card"
	Goal             Type = "goal"
	HalfTime         Type = "half_time"
	FullTime         Type = "full_time"
)

var knownTypes = map[Type]bool{
	Kickoff: true, Pass: true, Shot: true, Tackle: true, Interception: true,
	Pressure: true, PossessionChange: true, Foul: true, Card: true, Goal: true,
	HalfTime: true, FullTime: true,
}

// Outcome describes how an event ended.
type Outcome string

// Outcomes. Only passes, shots and tackles carry an outcome.
const (
	Complete   Outcome = "complete"
	Incomplete Outcome = "incomplete"
	OnTarget   Outcome = "on_target"
	OffTarget  Outcome = "off_target"
	Blocked    Outcome = "blocked"
	Scored     Outcome = "goal"
	Won        Outcome = "won"
	Lost       Outcome = "lost"
)

var validOutcomes = map[Type][]Outcome{
	Pass:   {Complete, Incomplete},
	Shot:   {OnTarget, OffTarget, Blocked, Scored},
	Tackle: {Won, Lost},
}

// Card colours.
const (
	Yellow = "yellow"
	Red    = "red"
)

// Point is a position on the pitch in metres.
type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// InPitch reports whether p lies on the pitch, lines included.
func (p Point) InPitch() bool {
	return p.X >= 0 && p.X <= PitchLength && p.Y >= 0 && p.Y <= PitchWidth
}

// Score is the match score.
type Score struct {
	Home int `json:"home"`
	Away int `json:"away"`
}

// Event is one thing that happened in the match.
type Event struct {
	ID            string  `json:"id"`
	MatchID       string  `json:"match_id"`
	Seq           int     `json:"seq"`
	Type          Type    `json:"type"`
	Period        int     `json:"period"`
	ClockMs       int64   `json:"clock_ms"`
	TeamID        string  `json:"team_id,omitempty"`
	PlayerID      string  `json:"player_id,omitempty"`
	RecipientID   string  `json:"recipient_id,omitempty"`
	OpponentID    string  `json:"opponent_id,omitempty"`
	Start         *Point  `json:"start,omitempty"`
	End           *Point  `json:"end,omitempty"`
	Outcome       Outcome `json:"outcome,omitempty"`
	BallSpeedKmh  float64 `json:"ball_speed_kmh,omitempty"`
	UnderPressure bool    `json:"under_pressure,omitempty"`
	Card          string  `json:"card,omitempty"`
	Score         *Score  `json:"score,omitempty"`
	RelatedID     string  `json:"related_id,omitempty"`
}

// needsActor lists the event types that must name a team, a player and a
// start position.
var needsActor = map[Type]bool{
	Pass: true, Shot: true, Tackle: true, Interception: true,
	Pressure: true, Foul: true, Card: true,
}

// Validate reports every way the event breaks the contract.
func (e Event) Validate() error {
	var errs []error
	add := func(format string, args ...any) { errs = append(errs, fmt.Errorf(format, args...)) }

	if e.ID == "" {
		add("id is required")
	}
	if e.MatchID == "" {
		add("match_id is required")
	}
	if e.Seq < 1 {
		add("seq must be >= 1, got %d", e.Seq)
	}
	if !knownTypes[e.Type] {
		add("unknown type %q", e.Type)
	}
	if e.Period != 1 && e.Period != 2 {
		add("period must be 1 or 2, got %d", e.Period)
	}
	if e.ClockMs < 0 {
		add("clock_ms must be >= 0, got %d", e.ClockMs)
	}
	if e.Start != nil && !e.Start.InPitch() {
		add("start %+v is off the pitch", *e.Start)
	}
	if e.End != nil && !e.End.InPitch() {
		add("end %+v is off the pitch", *e.End)
	}

	if allowed, ok := validOutcomes[e.Type]; ok {
		if !slices.Contains(allowed, e.Outcome) {
			add("%s outcome must be one of %v, got %q", e.Type, allowed, e.Outcome)
		}
	} else if e.Outcome != "" {
		add("%s must not carry an outcome, got %q", e.Type, e.Outcome)
	}

	if needsActor[e.Type] {
		if e.TeamID == "" || e.PlayerID == "" {
			add("%s needs team_id and player_id", e.Type)
		}
		if e.Start == nil {
			add("%s needs start", e.Type)
		}
	}

	switch e.Type {
	case Pass:
		if e.End == nil {
			add("pass needs end")
		}
		if e.Outcome == Complete && e.RecipientID == "" {
			add("complete pass needs recipient_id")
		}
		if e.BallSpeedKmh <= 0 {
			add("pass needs ball_speed_kmh > 0")
		}
	case Shot:
		if e.End == nil {
			add("shot needs end")
		}
		if e.BallSpeedKmh <= 0 {
			add("shot needs ball_speed_kmh > 0")
		}
	case Pressure:
		if e.OpponentID == "" {
			add("pressure needs opponent_id (the player being pressed)")
		}
	case Card:
		if e.Card != Yellow && e.Card != Red {
			add("card must be %q or %q, got %q", Yellow, Red, e.Card)
		}
	case Goal:
		if e.TeamID == "" || e.PlayerID == "" || e.RelatedID == "" || e.Score == nil {
			add("goal needs team_id, player_id, related_id (the shot) and score")
		}
	case Kickoff, PossessionChange:
		if e.TeamID == "" {
			add("%s needs team_id", e.Type)
		}
	case HalfTime, FullTime:
		if e.Score == nil {
			add("%s needs score", e.Type)
		}
	}
	if e.Type != Card && e.Card != "" {
		add("only card events carry card")
	}

	return errors.Join(errs...)
}
