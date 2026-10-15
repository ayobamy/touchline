package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/ayobamy/touchline/internal/apidocs"
	"github.com/ayobamy/touchline/internal/events"
	"github.com/ayobamy/touchline/internal/mcp"
	"github.com/ayobamy/touchline/internal/mcptools"
	"github.com/ayobamy/touchline/internal/moments"
	"github.com/ayobamy/touchline/internal/sim"
	"github.com/ayobamy/touchline/internal/store"
)

// fullMatch returns a store holding a full simulated match and a server with its tools.
func fullMatch(t *testing.T) (*store.Store, *mcp.Server) {
	t.Helper()
	s, err := sim.New(sim.Config{MatchID: "m-0001", Seed: 7, Scenario: sim.Baseline()})
	if err != nil {
		t.Fatal(err)
	}
	st := store.New(s.Match(), moments.DefaultThresholds())
	var applyErr error
	found := 0
	s.Run(func(ev events.Event) {
		triggered, err := st.Apply(ev)
		if err != nil && applyErr == nil {
			applyErr = err
		}
		found += len(triggered)
	})
	if applyErr != nil {
		t.Fatal(applyErr)
	}
	if found == 0 {
		t.Fatal("a full match should flag some moments")
	}
	srv := mcp.NewServer("touchline", version, mcptools.Instructions)
	mcptools.Register(srv, st)
	return st, srv
}

func documentedPaths(t *testing.T) map[string]map[string]json.RawMessage {
	t.Helper()
	raw, err := apidocs.Spec(version)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	return doc.Paths
}

func TestEveryRouteIsDocumented(t *testing.T) {
	st, srv := fullMatch(t)
	paths := documentedPaths(t)
	for _, rt := range routes(st, srv) {
		method, path, found := strings.Cut(rt.pattern, " ")
		if !found {
			method, path = "post", rt.pattern // /mcp accepts POST and DELETE
		}
		if paths[path][strings.ToLower(method)] == nil {
			t.Errorf("route %q is not in internal/apidocs/openapi.json", rt.pattern)
		}
	}
}

func TestEveryDocumentedOperationAnswers(t *testing.T) {
	st, srv := fullMatch(t)
	mux, err := newMux(st, srv)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]int{http.MethodGet: http.StatusOK, http.MethodPost: http.StatusOK, http.MethodDelete: http.StatusNoContent}
	for path, operations := range documentedPaths(t) {
		for method := range operations {
			method := strings.ToUpper(method)
			t.Run(method+" "+path, func(t *testing.T) {
				var body *strings.Reader
				if method == http.MethodPost {
					body = strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
				} else {
					body = strings.NewReader("")
				}
				rec := httptest.NewRecorder()
				mux.ServeHTTP(rec, httptest.NewRequest(method, path, body))
				if rec.Code != want[method] {
					t.Errorf("status %d, want %d", rec.Code, want[method])
				}
			})
		}
	}
}

func TestMoments(t *testing.T) {
	st, srv := fullMatch(t)
	mux, err := newMux(st, srv)
	if err != nil {
		t.Fatal(err)
	}
	get := func(target string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
		return rec
	}

	t.Run("pages with after", func(t *testing.T) {
		var page struct {
			Moments []json.RawMessage `json:"moments"`
			Next    int               `json:"next"`
		}
		if err := json.Unmarshal(get("/api/moments").Body.Bytes(), &page); err != nil {
			t.Fatal(err)
		}
		if len(page.Moments) == 0 || page.Next != len(page.Moments) {
			t.Fatalf("got %d moments, next %d", len(page.Moments), page.Next)
		}
		rest := get("/api/moments?after=" + strconv.Itoa(page.Next))
		if !strings.Contains(rest.Body.String(), `"moments":[]`) {
			t.Errorf("no new moments should be an empty list, got %s", rest.Body.String())
		}
	})

	t.Run("rejects a bad after", func(t *testing.T) {
		for _, bad := range []string{"x", "-1"} {
			if rec := get("/api/moments?after=" + bad); rec.Code != http.StatusBadRequest {
				t.Errorf("after=%s: status %d, want 400", bad, rec.Code)
			}
		}
	})
}

// writeStream writes a match message, the first n events of seed 7 and then
// each extra line, and returns the file's path.
func writeStream(t *testing.T, n int, extra ...string) string {
	t.Helper()
	s, err := sim.New(sim.Config{MatchID: "m-0001", Seed: 7, Scenario: sim.Baseline()})
	if err != nil {
		t.Fatal(err)
	}
	match := s.Match()
	lines := []events.Message{{Kind: events.KindMatch, Match: &match}}
	s.Run(func(ev events.Event) {
		if len(lines) <= n {
			lines = append(lines, events.Message{Kind: events.KindEvent, Event: &ev})
		}
	})
	var b strings.Builder
	for _, msg := range lines {
		raw, err := json.Marshal(msg)
		if err != nil {
			t.Fatal(err)
		}
		b.Write(raw)
		b.WriteByte('\n')
	}
	for _, line := range extra {
		b.WriteString(line + "\n")
	}
	path := filepath.Join(t.TempDir(), "match.ndjson")
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func replayFile(t *testing.T, path string) (replayed, error) {
	t.Helper()
	match, src, err := openSource(config{in: path})
	if err != nil {
		t.Fatal(err)
	}
	return replay(context.Background(), store.New(match, moments.DefaultThresholds()), src, 0)
}

func TestReplayFromFile(t *testing.T) {
	t.Run("a clean stream replays every event", func(t *testing.T) {
		done, err := replayFile(t, writeStream(t, 50, ""))
		if err != nil {
			t.Fatal(err)
		}
		if done.events != 50 {
			t.Errorf("replayed %d events, want 50", done.events)
		}
	})

	t.Run("a malformed line is an error, not skipped", func(t *testing.T) {
		done, err := replayFile(t, writeStream(t, 5, "{not json"))
		if err == nil || !strings.Contains(err.Error(), "line 7") {
			t.Fatalf("err = %v, want a line 7 error", err)
		}
		if done.events != 5 {
			t.Errorf("replayed %d events before the bad line, want 5", done.events)
		}
	})

	t.Run("a read error is reported, not taken for the end of the match", func(t *testing.T) {
		tooLong := `{"kind":"event","pad":"` + strings.Repeat("x", maxLine) + `"}`
		done, err := replayFile(t, writeStream(t, 5, tooLong))
		if !errors.Is(err, bufio.ErrTooLong) {
			t.Fatalf("err = %v, want bufio.ErrTooLong", err)
		}
		if done.events != 5 {
			t.Errorf("replayed %d events before the error, want 5", done.events)
		}
	})

	t.Run("an empty stream and one without a match first are rejected", func(t *testing.T) {
		empty := filepath.Join(t.TempDir(), "empty.ndjson")
		if err := os.WriteFile(empty, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		match, src, err := openSource(config{in: empty})
		if err == nil || !strings.Contains(err.Error(), "empty stream") {
			t.Errorf("empty file: err = %v", err)
		}
		if match.ID != "" || src.events != nil {
			t.Error("a failed open must return no match and no events")
		}
		noMatch := writeStream(t, 0)
		lines, err := os.ReadFile(noMatch)
		if err != nil {
			t.Fatal(err)
		}
		event := `{"kind":"event","event":{}}` + "\n"
		if err := os.WriteFile(noMatch, append([]byte(event), lines...), 0o600); err != nil {
			t.Fatal(err)
		}
		got, src2, err := openSource(config{in: noMatch})
		if err == nil || !strings.Contains(err.Error(), "must start with a match") {
			t.Errorf("stream without a match first: err = %v", err)
		}
		if got.ID != "" || src2.events != nil {
			t.Error("a failed open must return no match and no events")
		}
	})
}
