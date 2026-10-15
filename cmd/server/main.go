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
// The API reference is at http://localhost:8080/docs and the OpenAPI document
// at /openapi.json.
//
// Serve over stdio for a local MCP client (events must then come from a file
// or -simulate, since stdin carries the protocol):
//
//	go run ./cmd/server -simulate -stdio
package main

import (
	"bufio"
	"bytes"
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

	"github.com/ayobamy/touchline/internal/apidocs"
	"github.com/ayobamy/touchline/internal/events"
	"github.com/ayobamy/touchline/internal/mcp"
	"github.com/ayobamy/touchline/internal/mcptools"
	"github.com/ayobamy/touchline/internal/moments"
	"github.com/ayobamy/touchline/internal/sim"
	"github.com/ayobamy/touchline/internal/store"
)

const version = "0.3.0"

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

	match, src, err := openSource(c)
	if err != nil {
		return err
	}
	st := store.New(match, moments.DefaultThresholds())

	go func() {
		done, err := replay(ctx, st, src, c.speed)
		if err != nil && !errors.Is(err, context.Canceled) {
			log.Error("replay failed", "err", err, "events", done.events, "moments", done.moments)
			return
		}
		log.Info("replay finished", "events", done.events, "moments", done.moments)
	}()

	srv := mcp.NewServer("touchline", version, mcptools.Instructions)
	mcptools.Register(srv, st)

	if c.stdio {
		log.Info("serving MCP over stdio", "match", match.ID)
		return srv.ServeStdio(ctx, os.Stdin, os.Stdout)
	}

	mux, err := newMux(st, srv)
	if err != nil {
		return err
	}
	httpSrv := &http.Server{Addr: c.addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := httpSrv.Shutdown(shutdown); err != nil {
			log.Warn("shutdown", "err", err)
		}
	}()
	log.Info("serving", "addr", c.addr, "mcp", "/mcp", "docs", "/docs", "match", match.ID, "speed", c.speed)
	if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// route is one HTTP endpoint. Every route is also documented in
// internal/apidocs/openapi.json; a test checks the two agree.
type route struct {
	pattern string
	handler http.Handler
}

func routes(st *store.Store, srv *mcp.Server) []route {
	return []route{
		{"/mcp", srv.HTTPHandler()},
		{"GET /healthz", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { writeText(w, "ok\n") })},
		{"GET /api/status", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, st.Status()) })},
		{"GET /api/match", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, st.Match()) })},
		{"GET /api/snapshot", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, st.Snapshot()) })},
		{"GET /api/moments", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { serveMoments(w, r, st) })},
	}
}

// newMux serves the routes plus the API documentation at /openapi.json and /docs.
func newMux(st *store.Store, srv *mcp.Server) (*http.ServeMux, error) {
	mux := http.NewServeMux()
	for _, rt := range routes(st, srv) {
		mux.Handle(rt.pattern, rt.handler)
	}
	if err := apidocs.Register(mux, version); err != nil {
		return nil, err
	}
	return mux, nil
}

func serveMoments(w http.ResponseWriter, r *http.Request, st *store.Store) {
	after := 0
	if raw := r.URL.Query().Get("after"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 {
			http.Error(w, "after must be a non-negative integer", http.StatusBadRequest)
			return
		}
		after = n
	}
	ms, next := st.Moments(after)
	if ms == nil {
		ms = []moments.Moment{}
	}
	writeJSON(w, map[string]any{"moments": ms, "next": next})
}

// source is a stream of events. Once events is closed, done holds the error
// that ended the stream, or nil if it ended cleanly.
type source struct {
	events <-chan events.Event
	done   <-chan error
}

// openSource returns the match and its events from a file, stdin or the
// simulator.
func openSource(c config) (events.Match, source, error) {
	if c.simulate {
		return simulate(c)
	}

	var r io.Reader = os.Stdin
	release := func() {}
	if c.in != "-" {
		f, err := os.Open(c.in)
		if err != nil {
			return events.Match{}, source{}, err
		}
		r = f
		release = func() {
			if err := f.Close(); err != nil {
				slog.Warn("closing the match stream", "file", c.in, "err", err)
			}
		}
	}
	in := bufio.NewScanner(r)
	in.Buffer(make([]byte, 0, 64*1024), maxLine)
	match, err := readMatch(in)
	if err != nil {
		release()
		return events.Match{}, source{}, err
	}
	ch := make(chan events.Event, 256)
	done := make(chan error, 1)
	go func() {
		defer close(ch)
		defer release()
		done <- scanEvents(in, ch)
	}()
	return match, source{events: ch, done: done}, nil
}

// maxLine is the longest line the stream may contain.
const maxLine = 1024 * 1024

func simulate(c config) (events.Match, source, error) {
	sc := sim.Baseline()
	if c.scenario != "" {
		var err error
		if sc, err = sim.LoadScenario(c.scenario); err != nil {
			return events.Match{}, source{}, err
		}
	}
	s, err := sim.New(sim.Config{MatchID: c.matchID, Seed: c.seed, Scenario: sc})
	if err != nil {
		return events.Match{}, source{}, err
	}
	ch := make(chan events.Event, 256)
	done := make(chan error, 1)
	go func() {
		defer close(ch)
		s.Run(func(ev events.Event) { ch <- ev })
		done <- nil
	}()
	return s.Match(), source{events: ch, done: done}, nil
}

// readMatch reads the first line, which must be the match message. A read
// error is reported as such, not mistaken for an empty stream.
func readMatch(in *bufio.Scanner) (events.Match, error) {
	if !in.Scan() {
		if err := in.Err(); err != nil {
			return events.Match{}, fmt.Errorf("reading the match message: %w", err)
		}
		return events.Match{}, errors.New("empty stream: expected a match message first")
	}
	var first events.Message
	if err := json.Unmarshal(in.Bytes(), &first); err != nil || first.Kind != events.KindMatch || first.Match == nil {
		return events.Match{}, errors.New("stream must start with a match message")
	}
	return *first.Match, nil
}

// scanEvents sends every event in the rest of the stream to ch. It stops at
// the first line that is not JSON and returns the scanner's error (a failed
// read, or a line longer than maxLine), so a corrupt or truncated stream ends
// the replay with an error instead of looking like a match that ended early.
// Blank lines and messages other than events are skipped.
func scanEvents(in *bufio.Scanner, ch chan<- events.Event) error {
	line := 1 // the match message
	for in.Scan() {
		line++
		raw := bytes.TrimSpace(in.Bytes())
		if len(raw) == 0 {
			continue
		}
		var msg events.Message
		if err := json.Unmarshal(raw, &msg); err != nil {
			return fmt.Errorf("line %d: %w", line, err)
		}
		if msg.Kind == events.KindEvent && msg.Event != nil {
			ch <- *msg.Event
		}
	}
	if err := in.Err(); err != nil {
		return fmt.Errorf("after line %d: %w", line, err)
	}
	return nil
}

// replayed counts what a replay applied.
type replayed struct {
	events, moments int
}

// replay applies events to the store, pacing them by match clock at the
// given speed. It returns the stream's error if the stream ended badly.
func replay(ctx context.Context, st *store.Store, src source, speed float64) (replayed, error) {
	var done replayed
	var prev *events.Event
	for ev := range src.events {
		if speed > 0 && prev != nil && prev.Period == ev.Period && ev.ClockMs > prev.ClockMs {
			wait := time.Duration(float64(ev.ClockMs-prev.ClockMs)/speed) * time.Millisecond
			select {
			case <-time.After(wait):
			case <-ctx.Done():
				return done, ctx.Err()
			}
		}
		triggered, err := st.Apply(ev)
		if err != nil {
			return done, err
		}
		prev = &ev
		done.events++
		done.moments += len(triggered)
	}
	if err := <-src.done; err != nil {
		return done, fmt.Errorf("event stream: %w", err)
	}
	return done, nil
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		http.Error(w, fmt.Sprint(err), http.StatusInternalServerError)
	}
}

func writeText(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if n, err := io.WriteString(w, body); err != nil {
		slog.Debug("response not fully written", "written", n, "err", err)
	}
}
