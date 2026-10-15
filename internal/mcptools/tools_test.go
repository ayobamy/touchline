package mcptools

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/ayobamy/touchline/internal/events"
	"github.com/ayobamy/touchline/internal/mcp"
	"github.com/ayobamy/touchline/internal/moments"
	"github.com/ayobamy/touchline/internal/sim"
	"github.com/ayobamy/touchline/internal/stats"
	"github.com/ayobamy/touchline/internal/store"
)

// setup replays a full simulated match into a store and returns a server.
func setup(t *testing.T) (*mcp.Server, *store.Store, []events.Event) {
	t.Helper()
	sc, err := sim.LoadScenario("../../scenarios/late-press.json")
	if err != nil {
		t.Fatal(err)
	}
	s, err := sim.New(sim.Config{Seed: 7, Scenario: sc})
	if err != nil {
		t.Fatal(err)
	}
	st := store.New(s.Match(), moments.DefaultThresholds())
	var evs []events.Event
	s.Run(func(ev events.Event) {
		evs = append(evs, ev)
		if _, err := st.Apply(ev); err != nil {
			t.Fatal(err)
		}
	})
	srv := mcp.NewServer("touchline", "test", Instructions)
	Register(srv, st)
	return srv, st, evs
}

var nextID int

// callTool calls a tool through the full JSON-RPC path and decodes its
// structured result into out. It returns the tool error text, if any.
func callTool(t *testing.T, srv *mcp.Server, name string, args any, out any) string {
	t.Helper()
	nextID++
	rawArgs, _ := json.Marshal(args)
	msg := fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"tools/call","params":{"name":%q,"arguments":%s}}`, nextID, name, rawArgs)
	var resp struct {
		Result struct {
			IsError           bool            `json:"isError"`
			StructuredContent json.RawMessage `json:"structuredContent"`
			Content           []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
		Error *struct{ Message string } `json:"error"`
	}
	if err := json.Unmarshal(srv.Handle(context.Background(), []byte(msg)), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Error != nil {
		t.Fatalf("%s: protocol error %s", name, resp.Error.Message)
	}
	if resp.Result.IsError {
		return resp.Result.Content[0].Text
	}
	if out != nil {
		if err := json.Unmarshal(resp.Result.StructuredContent, out); err != nil {
			t.Fatalf("%s: decode: %v", name, err)
		}
	}
	return ""
}

func TestToolsAreListedWithValidSchemas(t *testing.T) {
	srv, _, _ := setup(t)
	var resp struct {
		Result struct {
			Tools []struct {
				Name        string          `json:"name"`
				InputSchema json.RawMessage `json:"inputSchema"`
			} `json:"tools"`
		} `json:"result"`
	}
	_ = json.Unmarshal(srv.Handle(context.Background(), []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)), &resp)
	want := []string{"get_match", "get_status", "get_snapshot", "get_player_stats", "get_window", "get_events", "get_event", "compare_periods", "list_moments"}
	if len(resp.Result.Tools) != len(want) {
		t.Fatalf("%d tools, want %d", len(resp.Result.Tools), len(want))
	}
	for i, tool := range resp.Result.Tools {
		if tool.Name != want[i] {
			t.Errorf("tool %d is %s, want %s", i, tool.Name, want[i])
		}
		var s map[string]any
		if err := json.Unmarshal(tool.InputSchema, &s); err != nil || s["type"] != "object" {
			t.Errorf("%s: bad input schema %s", tool.Name, tool.InputSchema)
		}
	}
}

func TestSnapshotMatchesTheStatsEngine(t *testing.T) {
	srv, _, evs := setup(t)
	s, _ := sim.New(sim.Config{Seed: 7})
	eng := stats.New(s.Match())
	for _, ev := range evs {
		eng.Apply(ev)
	}
	want := eng.Snapshot()

	var got stats.Snapshot
	if msg := callTool(t, srv, "get_snapshot", map[string]any{}, &got); msg != "" {
		t.Fatal(msg)
	}
	if got.Score != want.Score || got.Teams[0].Passes != want.Teams[0].Passes || got.Teams[1].ExpectedGoals != want.Teams[1].ExpectedGoals {
		t.Errorf("snapshot differs from engine: got %+v, want %+v", got.Teams, want.Teams)
	}
	if got.Players != nil {
		t.Error("players included without include_players")
	}
}

func TestEventToolsAgree(t *testing.T) {
	srv, _, _ := setup(t)
	var list struct {
		Events   []store.EventView `json:"events"`
		Returned int               `json:"returned"`
		Total    int               `json:"total"`
	}
	callTool(t, srv, "get_events", map[string]any{"types": []string{"shot"}, "team_id": "iro", "limit": 5}, &list)
	if list.Returned != 5 || list.Total < 5 {
		t.Fatalf("returned %d of %d shots", list.Returned, list.Total)
	}
	for _, ev := range list.Events {
		if ev.Type != events.Shot || ev.TeamID != "iro" || ev.Derived.ExpectedGoals <= 0 {
			t.Errorf("unexpected event %+v", ev)
		}
	}

	var one store.EventView
	callTool(t, srv, "get_event", map[string]any{"event_id": list.Events[0].ID}, &one)
	if one.ID != list.Events[0].ID || one.Derived != list.Events[0].Derived {
		t.Errorf("get_event %+v differs from get_events %+v", one, list.Events[0])
	}
	if msg := callTool(t, srv, "get_event", map[string]any{"event_id": "nope"}, nil); msg == "" {
		t.Error("unknown event should be a tool error")
	}
}

func TestComparePeriodsShowsTheLatePress(t *testing.T) {
	srv, _, _ := setup(t)
	var cmp store.Comparison
	msg := callTool(t, srv, "compare_periods", map[string]any{
		"metric": "pressure_index",
		"a":      map[string]any{"period": 2, "from_ms": 45 * 60_000, "to_ms": 60 * 60_000},
		"b":      map[string]any{"period": 2, "from_ms": 60 * 60_000, "to_ms": 90 * 60_000},
	}, &cmp)
	if msg != "" {
		t.Fatal(msg)
	}
	if !(cmp.B.Values["iro"] > cmp.A.Values["iro"]) {
		t.Errorf("Ironbridge's pressure index should rise after the hour: %v then %v", cmp.A.Values, cmp.B.Values)
	}
	if msg := callTool(t, srv, "compare_periods", map[string]any{"metric": "vibes", "a": map[string]any{"period": 1, "from_ms": 0, "to_ms": 1}, "b": map[string]any{"period": 1, "from_ms": 0, "to_ms": 1}}, nil); msg == "" {
		t.Error("unknown metric should be a tool error")
	}
}

func TestMomentsAndPaging(t *testing.T) {
	srv, st, _ := setup(t)
	var page struct {
		Moments []moments.Moment `json:"moments"`
		Next    int              `json:"next"`
	}
	callTool(t, srv, "list_moments", map[string]any{}, &page)
	if page.Next != st.Status().Moments || len(page.Moments) != page.Next || page.Next == 0 {
		t.Fatalf("first page: %d moments, next %d", len(page.Moments), page.Next)
	}
	callTool(t, srv, "list_moments", map[string]any{"after": page.Next}, &page)
	if len(page.Moments) != 0 {
		t.Errorf("second page should be empty, got %d", len(page.Moments))
	}
}

func TestBadArgumentsAreToolErrors(t *testing.T) {
	srv, _, _ := setup(t)
	cases := []struct {
		tool string
		args map[string]any
	}{
		{"get_window", map[string]any{"minutes": 0}},
		{"get_player_stats", map[string]any{"player_id": "riv-99"}},
		{"get_snapshot", map[string]any{"include_player": true}}, // typo
	}
	for _, c := range cases {
		if msg := callTool(t, srv, c.tool, c.args, nil); msg == "" {
			t.Errorf("%s %v: want a tool error", c.tool, c.args)
		}
	}
}
