// Package sim generates synthetic, football-realistic match event streams.
//
// The model is deliberately simple and fully deterministic: a seeded random
// source drives a possession loop of pressure, tackles, passes, interceptions
// and shots. Scenarios shift the probabilities (a late press, a red card) so
// the same fixture can be replayed with a different story. No real match data
// is used anywhere.
package sim

import (
	"fmt"
	"math"
	"math/rand/v2"

	"github.com/ayobamy/touchline/internal/events"
	"github.com/ayobamy/touchline/internal/stats"
)

const halfMs = 45 * 60_000

type side int

const (
	home side = 0
	away side = 1
)

func (s side) other() side { return 1 - s }

func (s side) name() string {
	if s == home {
		return "home"
	}
	return "away"
}

// Config configures one simulated match.
type Config struct {
	MatchID  string
	Seed     uint64
	Scenario Scenario
}

type teamState struct {
	team     events.Team
	onPitch  []bool
	yellows  []int
	strength float64 // 1 at full strength, scaled down for each red card
}

// Simulator generates one match. The same Config always yields the same
// events, whatever speed the caller replays them at.
type Simulator struct {
	cfg   Config
	rng   *rand.Rand
	match events.Match
	teams [2]*teamState
	emit  func(events.Event)

	seq    int
	period int
	clock  int64
	score  events.Score

	possession side
	carrier    int
	ball       events.Point

	cardsDone []bool
}

// New prepares a match. It does not generate anything until Run is called.
func New(cfg Config) (*Simulator, error) {
	if cfg.MatchID == "" {
		cfg.MatchID = "m-0001"
	}
	if cfg.Scenario.Name == "" {
		cfg.Scenario = Baseline()
	}
	if err := cfg.Scenario.Validate(); err != nil {
		return nil, err
	}
	s := &Simulator{
		cfg: cfg,
		rng: rand.New(rand.NewPCG(cfg.Seed, cfg.Seed^0x9E3779B97F4A7C15)),
	}
	h, a := homeTeam.build(), awayTeam.build()
	s.match = events.Match{
		ID: cfg.MatchID, Home: h, Away: a, Venue: "Millbrook Stadium",
		Seed: cfg.Seed, Scenario: cfg.Scenario.Name,
	}
	for i, t := range []events.Team{h, a} {
		ts := &teamState{
			team:     t,
			onPitch:  make([]bool, len(t.Players)),
			yellows:  make([]int, len(t.Players)),
			strength: 1,
		}
		for j := range ts.onPitch {
			ts.onPitch[j] = true
		}
		s.teams[i] = ts
	}
	s.cardsDone = make([]bool, len(cfg.Scenario.Cards))
	return s, nil
}

// Match returns the fixture metadata: teams, players, venue, seed, scenario.
func (s *Simulator) Match() events.Match { return s.match }

// Run plays the whole match, calling emit once per event in order.
func (s *Simulator) Run(emit func(events.Event)) {
	s.emit = emit
	for period := 1; period <= 2; period++ {
		s.period = period
		s.clock = int64(period-1) * halfMs
		stoppage := int64(60_000 * (1 + s.rng.IntN(4))) // 1 to 4 minutes
		kicker := home
		if period == 2 {
			stoppage = int64(60_000 * (2 + s.rng.IntN(5))) // 2 to 6 minutes
			kicker = away
		}
		end := s.clock + halfMs + stoppage

		s.kickoff(kicker)
		for s.clock < end {
			s.applyScriptedCards()
			s.step()
		}

		typ := events.HalfTime
		if period == 2 {
			typ = events.FullTime
		}
		ev := s.newEvent(typ)
		score := s.score
		ev.Score = &score
		s.emit(ev)
	}
}

// step plays one action by the team in possession.
func (s *Simulator) step() {
	att, def := s.possession, s.possession.other()
	defPress, _ := s.mods(def)
	_, attack := s.mods(att)
	a, y := toAtt(att, s.ball)

	underPressure := false
	if s.rng.Float64() < clampf(0.28*defPress, 0.05, 0.85) {
		underPressure = true
		presser := s.nearest(def, s.ball, -1, false)
		ev := s.newEvent(events.Pressure)
		ev.TeamID = s.teamID(def)
		ev.PlayerID = s.playerID(def, presser)
		ev.OpponentID = s.playerID(att, s.carrier)
		ev.Start = ptr(s.ball)
		s.emit(ev)
		s.clock += 500 + s.rng.Int64N(700)

		if s.rng.Float64() < 0.35 && s.tackle(def, presser, defPress, attack) {
			return
		}
	}

	if a > 78 && math.Abs(y-events.PitchWidth/2) < 22 {
		pShot := clampf(0.10*attack*(1+(a-78)/27), 0.02, 0.6)
		if s.rng.Float64() < pShot {
			s.shoot(underPressure, attack)
			return
		}
	}
	s.pass(underPressure, defPress, attack)
}

// tackle resolves a challenge. It reports true when play moved on: the ball
// was won or a foul stopped play.
func (s *Simulator) tackle(def side, tackler int, defPress, attack float64) bool {
	att := def.other()
	ev := s.newEvent(events.Tackle)
	ev.TeamID = s.teamID(def)
	ev.PlayerID = s.playerID(def, tackler)
	ev.OpponentID = s.playerID(att, s.carrier)
	ev.Start = ptr(s.ball)

	if s.rng.Float64() < clampf(0.5*defPress/attack, 0.15, 0.85) {
		ev.Outcome = events.Won
		s.emit(ev)
		s.clock += 800
		s.changePossession(def, tackler, 1000+s.rng.Int64N(1500))
		return true
	}

	ev.Outcome = events.Lost
	s.emit(ev)
	s.clock += 600
	if s.rng.Float64() < 0.22 {
		foul := s.newEvent(events.Foul)
		foul.TeamID = s.teamID(def)
		foul.PlayerID = s.playerID(def, tackler)
		foul.OpponentID = s.playerID(att, s.carrier)
		foul.Start = ptr(s.ball)
		s.emit(foul)
		if s.rng.Float64() < 0.18 {
			s.giveCard(def, tackler, events.Yellow)
		}
		s.clock += 15_000 + s.rng.Int64N(20_000) // free kick
		return true
	}
	return false
}

func (s *Simulator) pass(underPressure bool, defPress, attack float64) {
	att, def := s.possession, s.possession.other()
	a, y := toAtt(att, s.ball)

	length := 6 + s.rng.Float64()*16
	if s.rng.Float64() < 0.12 {
		length = 25 + s.rng.Float64()*25
	}
	theta := s.rng.NormFloat64() * 0.9
	if s.rng.Float64() < 0.22/attack {
		theta += math.Pi // recycle the ball backwards
	}
	ta := clampf(a+length*math.Cos(theta), 1, events.PitchLength-1)
	ty := clampf(y+length*math.Sin(theta), 1, events.PitchWidth-1)
	target := roundPoint(toAbs(att, ta, ty))
	dist := stats.Distance(s.ball, target)

	success := 0.88 - 0.005*math.Max(0, dist-12)
	if underPressure {
		success -= 0.12
	}
	success -= 0.06 * (defPress - 1)
	success += 0.04 * (attack - 1)
	if ta > 75 {
		success -= 0.10
	}
	success = clampf(success, 0.35, 0.97)

	speed := clampf(28+dist*1.3+s.rng.NormFloat64()*6, 22, 115)
	travelMs := int64(dist / (speed / 3.6) * 1000)
	recipient := s.nearest(att, target, s.carrier, ta < 25)

	ev := s.newEvent(events.Pass)
	ev.TeamID = s.teamID(att)
	ev.PlayerID = s.playerID(att, s.carrier)
	ev.RecipientID = s.playerID(att, recipient)
	ev.Start = ptr(s.ball)
	ev.End = ptr(target)
	ev.BallSpeedKmh = round1(speed)
	ev.UnderPressure = underPressure

	if s.rng.Float64() < success {
		ev.Outcome = events.Complete
		s.emit(ev)
		s.ball = target
		s.carrier = recipient
		s.clock += travelMs + 1800 + s.rng.Int64N(2200)
		return
	}

	ev.Outcome = events.Incomplete
	s.emit(ev)
	s.clock += travelMs
	if s.rng.Float64() < 0.6 {
		ip := roundPoint(lerp(s.ball, target, 0.4+s.rng.Float64()*0.5))
		interceptor := s.nearest(def, ip, -1, false)
		iev := s.newEvent(events.Interception)
		iev.TeamID = s.teamID(def)
		iev.PlayerID = s.playerID(def, interceptor)
		iev.Start = ptr(ip)
		s.emit(iev)
		s.ball = ip
		s.changePossession(def, interceptor, 1200)
		return
	}
	// Ball runs loose or out of play.
	s.ball = target
	s.changePossession(def, s.nearest(def, target, -1, false), 6000+s.rng.Int64N(10_000))
}

func (s *Simulator) shoot(underPressure bool, attack float64) {
	att, def := s.possession, s.possession.other()
	start := s.ball
	// Real finishing falls short of the xG model's chance quality once
	// blocks, saves and defenders closing down are accounted for; 0.55 keeps
	// scoring near a real league average of about 2.8 goals a match.
	xg := stats.ExpectedGoals(start, att == home) * 0.55
	if underPressure {
		xg *= 0.8
	}
	speed := clampf(72+s.rng.NormFloat64()*14+(attack-1)*10, 45, 130)
	mouthY := events.PitchWidth/2 + (s.rng.Float64()*2-1)*3.4

	ev := s.newEvent(events.Shot)
	ev.TeamID = s.teamID(att)
	ev.PlayerID = s.playerID(att, s.carrier)
	ev.Start = ptr(start)
	ev.BallSpeedKmh = round1(speed)
	ev.UnderPressure = underPressure

	saved := 0.30 * (1 - xg)
	blocked := 0.25 * (1 - xg)
	goal := toAbs(att, events.PitchLength, events.PitchWidth/2)
	switch r := s.rng.Float64(); {
	case r < xg:
		ev.Outcome = events.Scored
		ev.End = ptr(roundPoint(toAbs(att, events.PitchLength, mouthY)))
	case r < xg+saved:
		ev.Outcome = events.OnTarget
		ev.End = ptr(roundPoint(toAbs(att, events.PitchLength-0.5, mouthY)))
	case r < xg+saved+blocked:
		ev.Outcome = events.Blocked
		ev.End = ptr(roundPoint(lerp(start, goal, 0.25)))
	default:
		ev.Outcome = events.OffTarget
		wide := 4.5 + s.rng.Float64()*8
		if s.rng.IntN(2) == 0 {
			wide = -wide
		}
		ev.End = ptr(roundPoint(toAbs(att, events.PitchLength, events.PitchWidth/2+wide)))
	}
	s.emit(ev)
	s.clock += int64(stats.Distance(start, *ev.End)/(speed/3.6)*1000) + 300

	switch ev.Outcome {
	case events.Scored:
		if att == home {
			s.score.Home++
		} else {
			s.score.Away++
		}
		g := s.newEvent(events.Goal)
		g.TeamID = ev.TeamID
		g.PlayerID = ev.PlayerID
		g.RelatedID = ev.ID
		g.Start = ptr(start)
		score := s.score
		g.Score = &score
		s.emit(g)
		s.clock += 45_000 + s.rng.Int64N(30_000) // celebration
		s.kickoff(def)
	case events.OnTarget, events.OffTarget:
		s.ball = toAbs(def, 6, events.PitchWidth/2)
		s.changePossession(def, s.keeper(def), 8000+s.rng.Int64N(12_000))
	case events.Blocked:
		s.ball = *ev.End
		if s.rng.Float64() < 0.5 {
			s.changePossession(def, s.nearest(def, s.ball, -1, false), 800)
		} else {
			s.carrier = s.nearest(att, s.ball, -1, false)
			s.clock += 1500
		}
	}
}

func (s *Simulator) kickoff(sd side) {
	s.possession = sd
	s.ball = events.Point{X: events.PitchLength / 2, Y: events.PitchWidth / 2}
	s.carrier = strikerIndex
	if !s.teams[sd].onPitch[strikerIndex] {
		s.carrier = s.nearest(sd, s.ball, -1, false)
	}
	ev := s.newEvent(events.Kickoff)
	ev.TeamID = s.teamID(sd)
	ev.PlayerID = s.playerID(sd, s.carrier)
	ev.Start = ptr(s.ball)
	s.emit(ev)
	s.clock += 2000
}

func (s *Simulator) changePossession(sd side, player int, delayMs int64) {
	s.possession = sd
	s.carrier = player
	ev := s.newEvent(events.PossessionChange)
	ev.TeamID = s.teamID(sd)
	ev.PlayerID = s.playerID(sd, player)
	ev.Start = ptr(s.ball)
	s.emit(ev)
	s.clock += delayMs
}

func (s *Simulator) applyScriptedCards() {
	minute := int(s.clock / 60_000)
	for i, c := range s.cfg.Scenario.Cards {
		if s.cardsDone[i] || minute < c.Minute || (c.Minute >= 45) != (s.period == 2) {
			continue
		}
		s.cardsDone[i] = true
		sd := home
		if c.Team == "away" {
			sd = away
		}
		if idx := s.pickPlayer(sd, c.Number); idx >= 0 {
			s.giveCard(sd, idx, c.Card)
		}
	}
}

func (s *Simulator) giveCard(sd side, idx int, colour string) {
	ts := s.teams[sd]
	if !ts.onPitch[idx] {
		return
	}
	s.emitCard(sd, idx, colour)
	if colour == events.Yellow {
		ts.yellows[idx]++
		if ts.yellows[idx] < 2 {
			return
		}
		s.emitCard(sd, idx, events.Red) // second yellow
	}
	ts.onPitch[idx] = false
	ts.strength *= 0.85
	if s.possession == sd && s.carrier == idx {
		s.carrier = s.nearest(sd, s.ball, idx, false)
	}
	s.clock += 30_000 + s.rng.Int64N(30_000)
}

func (s *Simulator) emitCard(sd side, idx int, colour string) {
	ev := s.newEvent(events.Card)
	ev.TeamID = s.teamID(sd)
	ev.PlayerID = s.playerID(sd, idx)
	ev.Start = ptr(s.ball)
	ev.Card = colour
	s.emit(ev)
}

// pickPlayer finds a player for a scripted card: by shirt number, or else a
// random outfield player who is on the pitch and not on the ball.
func (s *Simulator) pickPlayer(sd side, number int) int {
	ts := s.teams[sd]
	if number > 0 {
		for i, p := range ts.team.Players {
			if p.Number == number && ts.onPitch[i] {
				return i
			}
		}
		return -1
	}
	var candidates []int
	for i := 1; i < len(ts.team.Players); i++ {
		if ts.onPitch[i] && !(s.possession == sd && s.carrier == i) {
			candidates = append(candidates, i)
		}
	}
	if len(candidates) == 0 {
		return -1
	}
	return candidates[s.rng.IntN(len(candidates))]
}

// mods returns a team's current press and attack multipliers, including the
// scenario and any red cards.
func (s *Simulator) mods(sd side) (press, attack float64) {
	press, attack = 1, 1
	minute := int(s.clock / 60_000)
	for _, m := range s.cfg.Scenario.Modifiers {
		if m.Team != sd.name() || minute < m.FromMinute || minute >= m.ToMinute {
			continue
		}
		if m.Press > 0 {
			press *= m.Press
		}
		if m.Attack > 0 {
			attack *= m.Attack
		}
	}
	strength := s.teams[sd].strength
	return press * strength, attack * strength
}

// playerPos estimates where a player is: their formation zone, with the whole
// team shifted up or down the pitch to follow the ball.
func (s *Simulator) playerPos(sd side, idx int) events.Point {
	z := formation[idx]
	ballA, _ := toAtt(sd, s.ball)
	shift := (ballA - events.PitchLength/2) * 0.35
	if idx == 0 {
		shift *= 0.15
	}
	return toAbs(sd, clampf(z.a+shift, 1, events.PitchLength-1), z.y)
}

// nearest returns the on-pitch player of side sd closest to p, skipping
// exclude, and skipping the goalkeeper unless allowGK.
func (s *Simulator) nearest(sd side, p events.Point, exclude int, allowGK bool) int {
	best, bestD := -1, math.Inf(1)
	ts := s.teams[sd]
	for i := range ts.team.Players {
		if !ts.onPitch[i] || i == exclude || (i == 0 && !allowGK) {
			continue
		}
		if d := stats.Distance(s.playerPos(sd, i), p); d < bestD {
			best, bestD = i, d
		}
	}
	if best < 0 {
		return s.keeper(sd)
	}
	return best
}

func (s *Simulator) keeper(sd side) int {
	if s.teams[sd].onPitch[0] {
		return 0
	}
	return s.nearest(sd, toAbs(sd, 6, events.PitchWidth/2), 0, false)
}

func (s *Simulator) newEvent(t events.Type) events.Event {
	s.seq++
	return events.Event{
		ID:      fmt.Sprintf("%s-%05d", s.match.ID, s.seq),
		MatchID: s.match.ID,
		Seq:     s.seq,
		Type:    t,
		Period:  s.period,
		ClockMs: s.clock,
	}
}

func (s *Simulator) teamID(sd side) string { return s.teams[sd].team.ID }

func (s *Simulator) playerID(sd side, idx int) string { return s.teams[sd].team.Players[idx].ID }

// toAbs converts a position in a team's attacking frame to absolute pitch
// coordinates. The transform is its own inverse, so toAtt uses it too.
func toAbs(sd side, a, y float64) events.Point {
	if sd == home {
		return events.Point{X: a, Y: y}
	}
	return events.Point{X: events.PitchLength - a, Y: events.PitchWidth - y}
}

func toAtt(sd side, p events.Point) (a, y float64) {
	q := toAbs(sd, p.X, p.Y)
	return q.X, q.Y
}

func lerp(a, b events.Point, t float64) events.Point {
	return events.Point{X: a.X + (b.X-a.X)*t, Y: a.Y + (b.Y-a.Y)*t}
}

func ptr(p events.Point) *events.Point { return &p }

func roundPoint(p events.Point) events.Point { return events.Point{X: round1(p.X), Y: round1(p.Y)} }

func round1(v float64) float64 { return math.Round(v*10) / 10 }

func clampf(v, lo, hi float64) float64 { return math.Max(lo, math.Min(hi, v)) }
