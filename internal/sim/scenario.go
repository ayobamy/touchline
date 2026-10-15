package sim

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

// Scenario steers a simulated match: who presses, who attacks and when, and
// any scripted cards. A scenario never fixes the result; it only shifts the
// probabilities, so the same scenario with a different seed plays out
// differently.
type Scenario struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Modifiers   []Modifier     `json:"modifiers,omitempty"`
	Cards       []ScriptedCard `json:"cards,omitempty"`
}

// Modifier scales one team's behaviour between two match minutes.
// Multipliers default to 1 when left out.
type Modifier struct {
	Team       string  `json:"team"` // "home" or "away"
	FromMinute int     `json:"from_minute"`
	ToMinute   int     `json:"to_minute"`
	Press      float64 `json:"press,omitempty"`  // pressing intensity and ball winning
	Attack     float64 `json:"attack,omitempty"` // forward passing and shooting
}

// ScriptedCard shows a card at a given minute. Number picks the player by
// shirt number; zero picks an outfield player at random.
type ScriptedCard struct {
	Team   string `json:"team"`
	Minute int    `json:"minute"`
	Card   string `json:"card"` // "yellow" or "red"
	Number int    `json:"number,omitempty"`
}

// Baseline is an even match with no scripting.
func Baseline() Scenario {
	return Scenario{Name: "baseline", Description: "An even match with no scripted events."}
}

// LoadScenario reads and validates a scenario file.
func LoadScenario(path string) (Scenario, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Scenario{}, err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var s Scenario
	if err := dec.Decode(&s); err != nil {
		return Scenario{}, fmt.Errorf("parse %s: %w", path, err)
	}
	if err := s.Validate(); err != nil {
		return Scenario{}, fmt.Errorf("invalid scenario %s: %w", path, err)
	}
	return s, nil
}

// Validate checks the scenario for mistakes.
func (s Scenario) Validate() error {
	var errs []error
	if s.Name == "" {
		errs = append(errs, errors.New("name is required"))
	}
	for i, m := range s.Modifiers {
		if m.Team != "home" && m.Team != "away" {
			errs = append(errs, fmt.Errorf("modifiers[%d]: team must be home or away", i))
		}
		if m.ToMinute <= m.FromMinute {
			errs = append(errs, fmt.Errorf("modifiers[%d]: to_minute must be after from_minute", i))
		}
		if m.Press < 0 || m.Attack < 0 {
			errs = append(errs, fmt.Errorf("modifiers[%d]: multipliers must be positive", i))
		}
	}
	for i, c := range s.Cards {
		if c.Team != "home" && c.Team != "away" {
			errs = append(errs, fmt.Errorf("cards[%d]: team must be home or away", i))
		}
		if c.Card != "yellow" && c.Card != "red" {
			errs = append(errs, fmt.Errorf("cards[%d]: card must be yellow or red", i))
		}
		if c.Minute < 0 || c.Minute > 95 {
			errs = append(errs, fmt.Errorf("cards[%d]: minute must be between 0 and 95", i))
		}
	}
	return errors.Join(errs...)
}
