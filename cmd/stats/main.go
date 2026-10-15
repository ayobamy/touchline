// Command stats reads a match stream on stdin and prints statistics
// snapshots as JSON.
//
//	go run ./cmd/simulator -seed 7 | go run ./cmd/stats -every 15 -pretty
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/ayobamy/touchline/internal/events"
	"github.com/ayobamy/touchline/internal/stats"
)

func main() {
	every := flag.Int("every", 15, "print a snapshot every N match minutes (0 = only at full time)")
	pretty := flag.Bool("pretty", false, "indent the JSON output")
	flag.Parse()

	if err := run(*every, *pretty); err != nil {
		fmt.Fprintln(os.Stderr, "stats:", err)
		os.Exit(1)
	}
}

func run(every int, pretty bool) error {
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	out := json.NewEncoder(os.Stdout)
	if pretty {
		out.SetIndent("", "  ")
	}

	var engine *stats.Engine
	lastBucket := map[int]int{}
	for in.Scan() {
		var msg events.Message
		if err := json.Unmarshal(in.Bytes(), &msg); err != nil {
			return fmt.Errorf("decode line: %w", err)
		}
		switch msg.Kind {
		case events.KindMatch:
			if msg.Match == nil {
				return errors.New("match message without match")
			}
			engine = stats.New(*msg.Match)
		case events.KindEvent:
			if engine == nil {
				return errors.New("event before match message")
			}
			ev := *msg.Event
			engine.Apply(ev)
			if ev.Type == events.FullTime {
				return out.Encode(engine.Snapshot())
			}
			if every > 0 {
				bucket := int(ev.ClockMs/60_000) / every
				if prev, seen := lastBucket[ev.Period]; seen && bucket > prev {
					if err := out.Encode(engine.Snapshot()); err != nil {
						return err
					}
				}
				lastBucket[ev.Period] = bucket
			}
		default:
			return fmt.Errorf("unknown message kind %q", msg.Kind)
		}
	}
	if err := in.Err(); err != nil {
		return err
	}
	if engine != nil {
		return out.Encode(engine.Snapshot())
	}
	return nil
}
