// Command server replays a match into the stats engine and serves it to
// agents as MCP tools, plus a small JSON API for the orchestrator and viewer.
//
// Simulate a match and serve it over HTTP at one match minute per second:
//
//	go run ./cmd/server -simulate -seed 7 -scenario scenarios/late-press.json -speed 60
//
// Replay a recorded stream instead:
//
//	go run ./cmd/simulator -seed 7 > match.ndjson
//	go run ./cmd/server -in match.ndjson -speed 60
//
// Serve over stdio for a local MCP client (events must then come from a file
// or -simulate, since stdin carries the protocol):
//
//	go run ./cmd/server -simulate -stdio
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/ayobamy/touchline/internal/events"
	"github.com/ayobamy/touchline/internal/mcp"
	"github.com/ayobamy/touchline/internal/mcptools"
	"github.com/ayobamy/touchline/internal/moments"
	"github.com/ayobamy/touchline/internal/sim"
	"github.com/ayobamy/touchline/internal/store"
)

const version = "0.2.0"

type config struct {
	in       string
	simulate bool
	seed     uint64
	scenario string
	matchID  string
	speed    float64
	addr     string
	stdio    bool
}

func main() {
	var c config
	flag.StringVar(&c.in, "in", "-", "match stream to replay (NDJSON file, or - for stdin)")
	flag.BoolVar(&c.simulate, "simulate", false, "generate the match instead of reading -in")
	flag.Uint64Var(&c.seed, "seed", 7, "seed for -simulate")
	flag.StringVar(&c.scenario, "scenario", "", "scenario file for -simulate")
	flag.StringVar(&c.matchID, "match", "m-0001", "match ID for -simulate")
	flag.Float64Var(&c.speed, "speed", 0, "replay speed: 0 = all at once, 1 = real time, 60 = one match minute per second")
	flag.StringVar(&c.addr, "addr", ":8080", "HTTP listen address")
	flag.BoolVar(&c.stdio, "stdio", false, "serve MCP over stdin/stdout instead of HTTP")
	flag.Parse()

	// In stdio mode stdout carries the protocol, so logs go to stderr only.
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := run(c, log); err != nil {
		log.Error("server stopped", "err", err)
		os.Exit(1)
	}
}

func run(c config, log *slog.Logger) error {
	if c.stdio && !c.simulate && c.in == "-" {
		return errors.New("-stdio needs -simulate or -in <file>: stdin carries the MCP protocol")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	match, source, err := openSource(c)
	if err != nil {
		return err
	}
	st := store.New(match, moments.DefaultThresholds())

	go func() {
		n, err := replay(ctx, st, source, c.speed)
		if err != nil && !errors.Is(err, context.Canceled) {
			log.Error("replay failed", "err", err, "events", n)
			return
		}
		log.Info("replay finished", "events", n, "moments", st.Status().Moments)
	}()

	srv := mcp.NewServer("touchline", version, mcptools.Instructions)
	mcptools.Register(srv, st)

	if c.stdio {
		log.Info("serving MCP over stdio", "match", match.ID)
		return srv.ServeStdio(ctx, os.Stdin, os.Stdout)
	}

	mux := http.NewServeMux()
	mux.Handle("/mcp", srv.HTTPHandler())
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "ok\n") })
	mux.HandleFunc("GET /api/status", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, st.Status()) })
	mux.HandleFunc("GET /api/match", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, st.Match()) })
	mux.HandleFunc("GET /api/snapshot", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, st.Snapshot()) })
	mux.HandleFunc("GET /api/moments", func(w http.ResponseWriter, r *http.Request) {
		after, _ := strconv.Atoi(r.URL.Query().Get("after"))
		ms, next := st.Moments(after)
		writeJSON(w, map[string]any{"moments": ms, "next": next})
	})

	httpSrv := &http.Server{Addr: c.addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(shutdown)
	}()
	log.Info("serving", "addr", c.addr, "mcp", "/mcp", "match", match.ID, "speed", c.speed)
	if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// openSource returns the match and a channel of events from a file, stdin or
// the simulator.
func openSource(c config) (events.Match, <-chan events.Event, error) {
	if c.simulate {
		sc := sim.Baseline()
		if c.scenario != "" {
			var err error
			if sc, err = sim.LoadScenario(c.scenario); err != nil {
				return events.Match{}, nil, err
			}
		}
		s, err := sim.New(sim.Config{MatchID: c.matchID, Seed: c.seed, Scenario: sc})
		if err != nil {
			return events.Match{}, nil, err
		}
		ch := make(chan events.Event, 256)
		go func() {
			defer close(ch)
			s.Run(func(ev events.Event) { ch <- ev })
		}()
		return s.Match(), ch, nil
	}

	var r io.Reader = os.Stdin
	if c.in != "-" {
		f, err := os.Open(c.in)
		if err != nil {
			return events.Match{}, nil, err
		}
		r = f
	}
	in := bufio.NewScanner(r)
	in.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	if !in.Scan() {
		return events.Match{}, nil, errors.New("empty stream: expected a match message first")
	}
	var first events.Message
	if err := json.Unmarshal(in.Bytes(), &first); err != nil || first.Kind != events.KindMatch || first.Match == nil {
		return events.Match{}, nil, errors.New("stream must start with a match message")
	}
	ch := make(chan events.Event, 256)
	go func() {
		defer close(ch)
		for in.Scan() {
			var msg events.Message
			if json.Unmarshal(in.Bytes(), &msg) == nil && msg.Kind == events.KindEvent && msg.Event != nil {
				ch <- *msg.Event
			}
		}
	}()
	return *first.Match, ch, nil
}

// replay applies events to the store, pacing them by match clock at the
// given speed.
func replay(ctx context.Context, st *store.Store, source <-chan events.Event, speed float64) (int, error) {
	n := 0
	var prev *events.Event
	for ev := range source {
		if speed > 0 && prev != nil && prev.Period == ev.Period && ev.ClockMs > prev.ClockMs {
			wait := time.Duration(float64(ev.ClockMs-prev.ClockMs)/speed) * time.Millisecond
			select {
			case <-time.After(wait):
			case <-ctx.Done():
				return n, ctx.Err()
			}
		}
		if _, err := st.Apply(ev); err != nil {
			return n, err
		}
		prev = &ev
		n++
	}
	return n, nil
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		http.Error(w, fmt.Sprint(err), http.StatusInternalServerError)
	}
}
