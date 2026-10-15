// Command simulator generates a synthetic match and writes it as
// newline-delimited JSON: one match message, then one message per event.
//
//	go run ./cmd/simulator -seed 7 -scenario scenarios/late-press.json -speed 60
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/ayobamy/touchline/internal/events"
	"github.com/ayobamy/touchline/internal/sim"
)

func main() {
	seed := flag.Uint64("seed", 7, "random seed; the same seed and scenario always give the same match")
	matchID := flag.String("match", "m-0001", "match ID stamped on every event")
	scenarioPath := flag.String("scenario", "", "path to a scenario JSON file (default: an even match)")
	speed := flag.Float64("speed", 0, "playback speed: 0 = as fast as possible, 1 = real time, 60 = one match minute per second")
	outPath := flag.String("out", "-", "output file, or - for stdout")
	flag.Parse()

	if err := run(*seed, *matchID, *scenarioPath, *speed, *outPath); err != nil {
		fmt.Fprintln(os.Stderr, "simulator:", err)
		os.Exit(1)
	}
}

func run(seed uint64, matchID, scenarioPath string, speed float64, outPath string) error {
	scenario := sim.Baseline()
	if scenarioPath != "" {
		var err error
		if scenario, err = sim.LoadScenario(scenarioPath); err != nil {
			return err
		}
	}
	s, err := sim.New(sim.Config{MatchID: matchID, Seed: seed, Scenario: scenario})
	if err != nil {
		return err
	}

	var out io.Writer = os.Stdout
	if outPath != "-" {
		f, err := os.Create(outPath)
		if err != nil {
			return err
		}
		defer f.Close()
		out = f
	}
	w := bufio.NewWriter(out)
	defer w.Flush()
	enc := json.NewEncoder(w)

	m := s.Match()
	if err := enc.Encode(events.Message{Kind: events.KindMatch, Match: &m}); err != nil {
		return err
	}

	var writeErr error
	var prev *events.Event
	s.Run(func(ev events.Event) {
		if writeErr != nil {
			return
		}
		if speed > 0 && prev != nil && prev.Period == ev.Period && ev.ClockMs > prev.ClockMs {
			time.Sleep(time.Duration(float64(ev.ClockMs-prev.ClockMs)/speed) * time.Millisecond)
		}
		prev = &ev
		if writeErr = enc.Encode(events.Message{Kind: events.KindEvent, Event: &ev}); writeErr != nil {
			return
		}
		if speed > 0 {
			writeErr = w.Flush()
		}
	})
	return writeErr
}
