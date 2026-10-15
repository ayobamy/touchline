// Package stats turns match events into football statistics. Everything here
// is deterministic: the same events always give the same numbers. The agents
// explain these numbers; they never compute them.
package stats

import (
	"math"

	"github.com/ayobamy/touchline/internal/events"
)

const goalMouthWidth = 7.32 // metres between the posts

// attackingFrame converts an absolute pitch position into the frame of the
// team in possession: a is distance from that team's own goal line (0 to 105)
// and y is unchanged. The home team attacks towards x = 105.
func attackingFrame(p events.Point, home bool) (a, y float64) {
	if home {
		return p.X, p.Y
	}
	return events.PitchLength - p.X, events.PitchWidth - p.Y
}

// ExpectedGoals returns the probability that a shot from p scores, from a
// logistic model of distance to goal and the angle the goal mouth subtends.
// Calibrated so a central shot from 6 m is about 0.70, from the penalty spot
// about 0.31, and from 25 m about 0.06.
func ExpectedGoals(p events.Point, home bool) float64 {
	a, y := attackingFrame(p, home)
	dx := events.PitchLength - a
	dy := math.Abs(y - events.PitchWidth/2)
	dist := math.Hypot(dx, dy)
	half := goalMouthWidth / 2
	angle := math.Atan2(goalMouthWidth*dx, dx*dx+dy*dy-half*half)
	if angle < 0 {
		angle += math.Pi
	}
	logit := -1.2 + 2.2*angle - 0.09*dist
	return clamp(1/(1+math.Exp(-logit)), 0.01, 0.95)
}

// PassDifficulty rates a pass from 0 (trivial) to 10 (very hard). It adds up
// four documented components:
//
//   - length: up to 4 points, linear to 50 m
//   - under pressure: 2.5 points
//   - forward progress: up to 2 points, linear to 30 m gained
//   - target in the final third (more than 70 m from own goal): 1.5 points
func PassDifficulty(start, end events.Point, home, underPressure bool) float64 {
	d := math.Hypot(end.X-start.X, end.Y-start.Y)
	a0, _ := attackingFrame(start, home)
	a1, _ := attackingFrame(end, home)

	score := math.Min(d/50, 1) * 4
	if underPressure {
		score += 2.5
	}
	if gain := a1 - a0; gain > 0 {
		score += math.Min(gain/30, 1) * 2
	}
	if a1 > 70 {
		score += 1.5
	}
	return round1(math.Min(score, 10))
}

// Distance returns the straight-line distance between two points in metres.
func Distance(a, b events.Point) float64 { return math.Hypot(b.X-a.X, b.Y-a.Y) }

func clamp(v, lo, hi float64) float64 { return math.Max(lo, math.Min(hi, v)) }

func round1(v float64) float64 { return math.Round(v*10) / 10 }

func round2(v float64) float64 { return math.Round(v*100) / 100 }
