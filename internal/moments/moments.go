// Package moments decides which parts of a match are worth an agent's
// attention. It wraps the stats engine and, after each event, applies simple
// deterministic rules: a goal, a big chance, a key pass, a card, a momentum
// swing, a switch to a high press, a chaotic spell, a player milestone.
//
// The rules are deliberately generous; the Moment Detector agent decides
// which candidates deserve screen time. Each moment carries the IDs of the
// events that justify it, so every later claim can be traced back.
package moments

import (
	"fmt"
	"math"

	"github.com/ayobamy/touchline/internal/events"
	"github.com/ayobamy/touchline/internal/stats"
)

// Kind is the rule that produced a moment.
type Kind string

// Moment kinds.
const (
	Goal            Kind = "goal"
	BigChance       Kind = "big_chance"
	KeyPass         Kind = "key_pass"
	Card            Kind = "card"
	MomentumSwing   Kind = "momentum_swing"
	PressSwitch     Kind = "press_switch"
	ChaosSpell      Kind = "chaos_spell"
	PlayerMilestone Kind = "player_milestone"
)

// Moment is a candidate for an overlay.
type Moment struct {
	ID       string             `json:"id"`
	MatchID  string             `json:"match_id"`
	Kind     Kind               `json:"kind"`
	Period   int                `json:"period"`
	ClockMs  int64              `json:"clock_ms"`
	TeamID   string             `json:"team_id,omitempty"`
	PlayerID string             `json:"player_id,omitempty"`
	Summary  string             `json:"summary"`
	Metrics  map[string]float64 `json:"metrics,omitempty"`
	EventIDs []string           `json:"event_ids"`
}

// Thresholds tune the rules. DefaultThresholds documents the defaults.
type Thresholds struct {
	BigChanceXG       float64 // a missed shot at or above this xG
	KeyPassDifficulty float64 // a completed pass at or above this difficulty
	MomentumLevel     float64 // momentum crossing ±this level
	MomentumShift     float64 // momentum moving by this much within the window
	PressRise         float64 // a team's pressure index rising by this much
	PressLevel        float64 // ...and reaching at least this level
	ChaosLevel        float64 // possession changes per minute
	ChaosHoldMs       int64   // how long chaos must stay at that level
	WarmupMs          int64   // window rules are quiet this long into each half
	CooldownMs        int64   // the same rule for the same team stays quiet this long
	PassMilestone     int     // completed passes by one player
	TackleMilestone   int     // tackles won by one player
	EvidenceLimit     int     // most event IDs attached to a window moment
}

// DefaultThresholds returns the thresholds used in the demo.
func DefaultThresholds() Thresholds {
	return Thresholds{
		BigChanceXG:       0.25,
		KeyPassDifficulty: 8,
		MomentumLevel:     0.5,
		MomentumShift:     0.6,
		PressRise:         3,
		PressLevel:        6,
		ChaosLevel:        4,
		ChaosHoldMs:       2 * 60_000,
		WarmupMs:          4 * 60_000,
		CooldownMs:        6 * 60_000,
		PassMilestone:     40,
		TackleMilestone:   4,
		EvidenceLimit:     6,
	}
}

type sample struct {
	clock    int64
	momentum float64
	press    map[string]float64
}

// Detector applies events to a stats engine and returns the moments each
// event triggers.
type Detector struct {
	engine *stats.Engine
	match  events.Match
	th     Thresholds

	seq           int
	period        int
	periodStartMs int64
	lastShotID    string

	recent    []events.Event // current period, for evidence
	history   []sample       // window metrics after each event, current period
	lastFired map[string]int64
	chaosFrom int64 // clock when chaos first reached the level; -1 when below

	completed map[string]int
	tackles   map[string]int
	onTarget  map[string]bool
}

// New returns a detector for the match.
func New(m events.Match, th Thresholds) *Detector {
	return &Detector{
		engine:    stats.New(m),
		match:     m,
		th:        th,
		period:    1,
		lastFired: map[string]int64{},
		chaosFrom: -1,
		completed: map[string]int{},
		tackles:   map[string]int{},
		onTarget:  map[string]bool{},
	}
}

// Engine returns the underlying stats engine.
func (d *Detector) Engine() *stats.Engine { return d.engine }

// Apply folds the event into the statistics and returns its derived numbers
// and any moments it triggers.
func (d *Detector) Apply(ev events.Event) (stats.Derived, []Moment) {
	if ev.Period != d.period {
		d.period = ev.Period
		d.periodStartMs = ev.ClockMs
		d.recent = d.recent[:0]
		d.history = d.history[:0]
		d.chaosFrom = -1
	}
	derived := d.engine.Apply(ev)
	d.recent = append(d.recent, ev)
	d.trimRecent(ev.ClockMs)

	var out []Moment
	add := func(m Moment) {
		d.seq++
		m.ID = fmt.Sprintf("%s-mo-%03d", d.match.ID, d.seq)
		m.MatchID = d.match.ID
		m.Period = ev.Period
		m.ClockMs = ev.ClockMs
		out = append(out, m)
	}

	d.eventRules(ev, derived, add)

	window := d.engine.Window()
	d.history = append(d.history, sample{clock: ev.ClockMs, momentum: window.Momentum, press: window.PressureIndex})
	if ev.ClockMs-d.periodStartMs >= d.th.WarmupMs {
		d.windowRules(ev, window, add)
	}
	return derived, out
}

func (d *Detector) eventRules(ev events.Event, derived stats.Derived, add func(Moment)) {
	who := d.playerName(ev.PlayerID)
	team := d.teamName(ev.TeamID)

	switch ev.Type {
	case events.Shot:
		d.lastShotID = ev.ID
		if ev.Outcome == events.OnTarget || ev.Outcome == events.Scored {
			if !d.onTarget[ev.PlayerID] {
				d.onTarget[ev.PlayerID] = true
				if ev.Outcome == events.OnTarget {
					add(Moment{
						Kind: PlayerMilestone, TeamID: ev.TeamID, PlayerID: ev.PlayerID,
						Summary:  fmt.Sprintf("%s's first shot on target (saved)", who),
						Metrics:  map[string]float64{"xg": derived.ExpectedGoals, "shot_speed_kmh": derived.ShotSpeedKmh},
						EventIDs: []string{ev.ID},
					})
				}
			}
		}
		if ev.Outcome != events.Scored && derived.ExpectedGoals >= d.th.BigChanceXG {
			add(Moment{
				Kind: BigChance, TeamID: ev.TeamID, PlayerID: ev.PlayerID,
				Summary:  fmt.Sprintf("Big chance for %s: %s's shot (xG %.2f) is %s", team, who, derived.ExpectedGoals, describeShot(ev.Outcome)),
				Metrics:  map[string]float64{"xg": derived.ExpectedGoals, "shot_speed_kmh": derived.ShotSpeedKmh},
				EventIDs: []string{ev.ID},
			})
		}
	case events.Goal:
		ids := []string{ev.ID}
		if ev.RelatedID != "" {
			ids = []string{ev.RelatedID, ev.ID}
		}
		score := events.Score{}
		if ev.Score != nil {
			score = *ev.Score
		}
		add(Moment{
			Kind: Goal, TeamID: ev.TeamID, PlayerID: ev.PlayerID,
			Summary: fmt.Sprintf("Goal for %s by %s; %s %d-%d %s", team, who,
				d.match.Home.ShortName, score.Home, score.Away, d.match.Away.ShortName),
			Metrics:  map[string]float64{"home_goals": float64(score.Home), "away_goals": float64(score.Away)},
			EventIDs: ids,
		})
	case events.Pass:
		if ev.Outcome != events.Complete {
			return
		}
		d.completed[ev.PlayerID]++
		if derived.PassDifficulty >= d.th.KeyPassDifficulty {
			add(Moment{
				Kind: KeyPass, TeamID: ev.TeamID, PlayerID: ev.PlayerID,
				Summary: fmt.Sprintf("%s completes a pass rated %.1f/10 difficulty to %s", who, derived.PassDifficulty, d.playerName(ev.RecipientID)),
				Metrics: map[string]float64{
					"pass_difficulty": derived.PassDifficulty, "pass_distance_m": derived.PassDistanceM, "ball_speed_kmh": ev.BallSpeedKmh,
				},
				EventIDs: []string{ev.ID},
			})
		}
		if d.completed[ev.PlayerID] == d.th.PassMilestone {
			add(Moment{
				Kind: PlayerMilestone, TeamID: ev.TeamID, PlayerID: ev.PlayerID,
				Summary:  fmt.Sprintf("%s completes their %s pass of the match", who, ordinal(d.th.PassMilestone)),
				Metrics:  map[string]float64{"passes_completed": float64(d.th.PassMilestone)},
				EventIDs: []string{ev.ID},
			})
		}
	case events.Tackle:
		if ev.Outcome != events.Won {
			return
		}
		d.tackles[ev.PlayerID]++
		if d.tackles[ev.PlayerID] == d.th.TackleMilestone {
			add(Moment{
				Kind: PlayerMilestone, TeamID: ev.TeamID, PlayerID: ev.PlayerID,
				Summary: fmt.Sprintf("%s wins their %s tackle", who, ordinal(d.th.TackleMilestone)),
				Metrics: map[string]float64{"tackles_won": float64(d.th.TackleMilestone)},
				EventIDs: d.evidence(ev.ClockMs, func(e events.Event) bool {
					return e.Type == events.Tackle && e.Outcome == events.Won && e.PlayerID == ev.PlayerID
				}),
			})
		}
	case events.Card:
		add(Moment{
			Kind: Card, TeamID: ev.TeamID, PlayerID: ev.PlayerID,
			Summary:  fmt.Sprintf("%s card for %s (%s)", capitalise(ev.Card), who, team),
			EventIDs: []string{ev.ID},
		})
	}
}

func (d *Detector) windowRules(ev events.Event, w stats.Window, add func(Moment)) {
	past := d.sampleAgo(ev.ClockMs)
	homeID, awayID := d.match.Home.ID, d.match.Away.ID

	// Momentum: crossing the level, or a big shift within the window.
	if past != nil {
		// Only a team that is now clearly on top counts: either it has just
		// crossed the level, or momentum has moved sharply towards it.
		dominant := math.Abs(w.Momentum) >= d.th.MomentumLevel
		crossed := math.Abs(past.momentum) < d.th.MomentumLevel
		shifted := math.Abs(w.Momentum-past.momentum) >= d.th.MomentumShift
		if dominant && (crossed || shifted) {
			team := homeID
			if w.Momentum < 0 {
				team = awayID
			}
			if d.cool(MomentumSwing, team, ev.ClockMs) {
				add(Moment{
					Kind: MomentumSwing, TeamID: team,
					Summary: fmt.Sprintf("Momentum swings to %s: %+.2f, from %+.2f %d minutes earlier",
						d.teamName(team), w.Momentum, past.momentum, stats.DefaultWindowMinutes),
					Metrics: map[string]float64{"momentum": w.Momentum, "momentum_before": past.momentum},
					EventIDs: d.evidence(ev.ClockMs, func(e events.Event) bool {
						return e.TeamID == team && stats.MomentumCredit(e, d.match.IsHome(team)) >= 1
					}),
				})
			}
		}
	}

	// Press switch: a team's pressure index rises sharply to a high level.
	if past != nil {
		for _, team := range []string{homeID, awayID} {
			now, before := w.PressureIndex[team], past.press[team]
			if now-before >= d.th.PressRise && now >= d.th.PressLevel && d.cool(PressSwitch, team, ev.ClockMs) {
				add(Moment{
					Kind: PressSwitch, TeamID: team,
					Summary: fmt.Sprintf("%s step up their press: pressure index %.1f, up from %.1f", d.teamName(team), now, before),
					Metrics: map[string]float64{"pressure_index": now, "pressure_index_before": before},
					EventIDs: d.evidence(ev.ClockMs, func(e events.Event) bool {
						return e.TeamID == team && (e.Type == events.Pressure || (e.Type == events.Tackle && e.Outcome == events.Won))
					}),
				})
			}
		}
	}

	// Chaos: possession changing hands fast for a sustained spell.
	if w.ChaosPerMin >= d.th.ChaosLevel {
		if d.chaosFrom < 0 {
			d.chaosFrom = ev.ClockMs
		}
		if ev.ClockMs-d.chaosFrom >= d.th.ChaosHoldMs && d.cool(ChaosSpell, "", ev.ClockMs) {
			add(Moment{
				Kind:    ChaosSpell,
				Summary: fmt.Sprintf("A scrappy spell: possession changing hands %.1f times a minute", w.ChaosPerMin),
				Metrics: map[string]float64{"chaos_per_min": w.ChaosPerMin, "rhythm_per_min": w.RhythmPerMin},
				EventIDs: d.evidence(ev.ClockMs, func(e events.Event) bool {
					return e.Type == events.PossessionChange
				}),
			})
		}
	} else {
		d.chaosFrom = -1
	}
}

// sampleAgo returns the window sample from about one window length ago, or
// nil when the period is younger than that.
func (d *Detector) sampleAgo(now int64) *sample {
	target := now - int64(stats.DefaultWindowMinutes)*60_000
	if len(d.history) == 0 || d.history[0].clock > target {
		return nil
	}
	var found *sample
	for i := range d.history {
		if d.history[i].clock > target {
			break
		}
		found = &d.history[i]
	}
	return found
}

// cool reports whether the rule may fire for the team now, and if so starts
// its cooldown.
func (d *Detector) cool(kind Kind, team string, now int64) bool {
	key := fmt.Sprintf("%d/%s/%s", d.period, kind, team)
	if last, ok := d.lastFired[key]; ok && now-last < d.th.CooldownMs {
		return false
	}
	d.lastFired[key] = now
	return true
}

// evidence returns the IDs of the most recent matching events within the
// window, oldest first.
func (d *Detector) evidence(now int64, match func(events.Event) bool) []string {
	cutoff := now - int64(stats.DefaultWindowMinutes)*60_000
	var ids []string
	for i := len(d.recent) - 1; i >= 0 && len(ids) < d.th.EvidenceLimit; i-- {
		e := d.recent[i]
		if e.ClockMs < cutoff {
			break
		}
		if match(e) {
			ids = append(ids, e.ID)
		}
	}
	for i, j := 0, len(ids)-1; i < j; i, j = i+1, j-1 {
		ids[i], ids[j] = ids[j], ids[i]
	}
	return ids
}

func (d *Detector) trimRecent(now int64) {
	cutoff := now - int64(stats.DefaultWindowMinutes+1)*60_000
	i := 0
	for i < len(d.recent) && d.recent[i].ClockMs < cutoff {
		i++
	}
	d.recent = d.recent[i:]

	j := 0
	for j < len(d.history)-1 && d.history[j+1].clock < cutoff {
		j++
	}
	d.history = d.history[j:]
}

func (d *Detector) teamName(id string) string {
	for _, t := range []events.Team{d.match.Home, d.match.Away} {
		if t.ID == id {
			return t.Name
		}
	}
	return id
}

func (d *Detector) playerName(id string) string {
	for _, t := range []events.Team{d.match.Home, d.match.Away} {
		for _, p := range t.Players {
			if p.ID == id {
				return p.Name
			}
		}
	}
	return id
}

func describeShot(o events.Outcome) string {
	switch o {
	case events.OnTarget:
		return "saved"
	case events.Blocked:
		return "blocked"
	default:
		return "off target"
	}
}

func capitalise(s string) string {
	if s == "" {
		return s
	}
	return string(s[0]-'a'+'A') + s[1:]
}

func ordinal(n int) string {
	suffix := "th"
	switch {
	case n%100 >= 11 && n%100 <= 13:
	case n%10 == 1:
		suffix = "st"
	case n%10 == 2:
		suffix = "nd"
	case n%10 == 3:
		suffix = "rd"
	}
	return fmt.Sprintf("%d%s", n, suffix)
}
