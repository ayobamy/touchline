// Package mcptools exposes a match store to agents as read-only MCP tools.
// Agents must fetch every number they state from these tools.
package mcptools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/ayobamy/touchline/internal/events"
	"github.com/ayobamy/touchline/internal/mcp"
	"github.com/ayobamy/touchline/internal/moments"
	"github.com/ayobamy/touchline/internal/store"
)

// Instructions are sent to the model when it connects.
const Instructions = `Touchline match tools. All data is synthetic and all clubs and players are fictional.
Every number you state must come from a tool result in this conversation; never estimate or compute statistics yourself.
Cite event IDs from get_events, get_event or list_moments as evidence.
The match clock is clock_ms in milliseconds; the second half starts at 2700000 (45:00) and events are ordered by period then clock.
The home team always attacks towards x = 105 on a 105 x 68 m pitch.`

// Register adds the Touchline tools to the server.
func Register(srv *mcp.Server, st *store.Store) {
	srv.AddTool(mcp.Tool{
		Name:        "get_match",
		Title:       "Match details",
		Description: "Teams, players (ID, name, shirt number, position), venue and scenario for the match.",
		Handler: func(context.Context, json.RawMessage) (any, error) {
			return st.Match(), nil
		},
	})

	srv.AddTool(mcp.Tool{
		Name:        "get_status",
		Title:       "Match status",
		Description: "Current period, clock, score, and how many events and moments have happened.",
		Handler: func(context.Context, json.RawMessage) (any, error) {
			return st.Status(), nil
		},
	})

	srv.AddTool(mcp.Tool{
		Name:        "get_snapshot",
		Title:       "Team statistics",
		Description: "Cumulative team statistics (passes, accuracy, shots, xG, tackles, pressures, possession, cards) plus the last-5-minutes window (momentum, pressure index, chaos, rhythm). Set include_players for every player's statistics too.",
		InputSchema: schema(`{"include_players":{"type":"boolean","description":"Also return per-player statistics (larger result). Default false."}}`),
		Handler: func(_ context.Context, args json.RawMessage) (any, error) {
			var in struct {
				IncludePlayers bool `json:"include_players"`
			}
			if err := mcp.DecodeArgs(args, &in); err != nil {
				return nil, err
			}
			snap := st.Snapshot()
			if !in.IncludePlayers {
				snap.Players = nil
			}
			return snap, nil
		},
	})

	srv.AddTool(mcp.Tool{
		Name:        "get_player_stats",
		Title:       "Player statistics",
		Description: "One player's cumulative statistics: touches, passes, pass accuracy, average pass difficulty, shots, goals, xG, tackles won, interceptions, pressures.",
		InputSchema: schema(`{"player_id":{"type":"string","description":"Player ID such as riv-10, from get_match."}}`, "player_id"),
		Handler: func(_ context.Context, args json.RawMessage) (any, error) {
			var in struct {
				PlayerID string `json:"player_id"`
			}
			if err := mcp.DecodeArgs(args, &in); err != nil {
				return nil, err
			}
			p, ok := st.Player(in.PlayerID)
			if !ok {
				return nil, fmt.Errorf("no player %q; call get_match for player IDs", in.PlayerID)
			}
			return p, nil
		},
	})

	srv.AddTool(mcp.Tool{
		Name:        "get_window",
		Title:       "Recent spell of play",
		Description: "Momentum (-1 away on top to +1 home on top), pressure index per team (pressures per 10 opponent passes), chaos (possession changes per minute) and rhythm (events per minute) over the last N minutes of the current half.",
		InputSchema: schema(`{"minutes":{"type":"integer","minimum":1,"maximum":45,"description":"Length of the window in minutes."}}`, "minutes"),
		Handler: func(_ context.Context, args json.RawMessage) (any, error) {
			var in struct {
				Minutes int `json:"minutes"`
			}
			if err := mcp.DecodeArgs(args, &in); err != nil {
				return nil, err
			}
			return st.Window(in.Minutes)
		},
	})

	srv.AddTool(mcp.Tool{
		Name:        "get_events",
		Title:       "Find events",
		Description: fmt.Sprintf("Events matching the filters, oldest first, each with derived numbers (pass distance and difficulty, xG, shot speed). Returns at most `limit` of the most recent matches (default 50, max %d) and the total that matched.", store.MaxEvents),
		InputSchema: schema(`{
			"period":{"type":"integer","enum":[1,2]},
			"from_ms":{"type":"integer","minimum":0,"description":"Match clock lower bound, inclusive."},
			"to_ms":{"type":"integer","minimum":0,"description":"Match clock upper bound, inclusive."},
			"team_id":{"type":"string"},
			"player_id":{"type":"string","description":"Matches the acting player or the pass recipient."},
			"types":{"type":"array","items":{"type":"string","enum":["kickoff","pass","shot","tackle","interception","pressure","possession_change","foul","card","goal","half_time","full_time"]}},
			"limit":{"type":"integer","minimum":1,"maximum":200}
		}`),
		Handler: func(_ context.Context, args json.RawMessage) (any, error) {
			var in struct {
				Period   int           `json:"period"`
				FromMs   int64         `json:"from_ms"`
				ToMs     int64         `json:"to_ms"`
				TeamID   string        `json:"team_id"`
				PlayerID string        `json:"player_id"`
				Types    []events.Type `json:"types"`
				Limit    int           `json:"limit"`
			}
			if err := mcp.DecodeArgs(args, &in); err != nil {
				return nil, err
			}
			evs, total := st.Events(store.Filter{
				Period: in.Period, FromMs: in.FromMs, ToMs: in.ToMs, TeamID: in.TeamID,
				PlayerID: in.PlayerID, Types: in.Types, Limit: in.Limit,
			})
			if evs == nil {
				evs = []store.EventView{}
			}
			return map[string]any{"events": evs, "returned": len(evs), "total": total}, nil
		},
	})

	srv.AddTool(mcp.Tool{
		Name:        "get_event",
		Title:       "One event",
		Description: "One event by ID with its derived numbers. Use it to check that a cited event says what a claim says.",
		InputSchema: schema(`{"event_id":{"type":"string","description":"Event ID such as m-0001-01234."}}`, "event_id"),
		Handler: func(_ context.Context, args json.RawMessage) (any, error) {
			var in struct {
				EventID string `json:"event_id"`
			}
			if err := mcp.DecodeArgs(args, &in); err != nil {
				return nil, err
			}
			ev, ok := st.Event(in.EventID)
			if !ok {
				return nil, fmt.Errorf("no event %q", in.EventID)
			}
			return ev, nil
		},
	})

	rangeSchema := `{"type":"object","properties":{"period":{"type":"integer","enum":[1,2]},"from_ms":{"type":"integer","minimum":0},"to_ms":{"type":"integer","minimum":0}},"required":["period","from_ms","to_ms"]}`
	metricsJSON, _ := json.Marshal(store.Metrics)
	srv.AddTool(mcp.Tool{
		Name:        "compare_periods",
		Title:       "Compare two spells",
		Description: "One metric for both teams over two stretches of the match, for claims like \"since the 60th minute\". Possession is not available here; use get_snapshot.",
		InputSchema: schema(fmt.Sprintf(`{"metric":{"type":"string","enum":%s},"a":%s,"b":%s}`, metricsJSON, rangeSchema, rangeSchema), "metric", "a", "b"),
		Handler: func(_ context.Context, args json.RawMessage) (any, error) {
			var in struct {
				Metric string      `json:"metric"`
				A      store.Range `json:"a"`
				B      store.Range `json:"b"`
			}
			if err := mcp.DecodeArgs(args, &in); err != nil {
				return nil, err
			}
			return st.Compare(in.Metric, in.A, in.B)
		},
	})

	srv.AddTool(mcp.Tool{
		Name:        "list_moments",
		Title:       "Candidate moments",
		Description: "Candidate moments the rules engine has flagged (goals, big chances, key passes, cards, momentum swings, press switches, chaotic spells, player milestones), each with a summary, metrics and evidence event IDs. Pass `after` = the `next` value from the previous call to get only new ones.",
		InputSchema: schema(`{"after":{"type":"integer","minimum":0,"description":"Number of moments already seen. Default 0."}}`),
		Handler: func(_ context.Context, args json.RawMessage) (any, error) {
			var in struct {
				After int `json:"after"`
			}
			if err := mcp.DecodeArgs(args, &in); err != nil {
				return nil, err
			}
			ms, next := st.Moments(in.After)
			if ms == nil {
				ms = []moments.Moment{}
			}
			return map[string]any{"moments": ms, "next": next}, nil
		},
	})
}

// schema builds an object schema from a properties object and required names.
func schema(properties string, required ...string) json.RawMessage {
	if required == nil {
		required = []string{}
	}
	req, _ := json.Marshal(required)
	raw := fmt.Sprintf(`{"type":"object","properties":%s,"required":%s,"additionalProperties":false}`, properties, req)
	var buf bytes.Buffer
	if err := json.Compact(&buf, []byte(raw)); err != nil {
		panic("mcptools: invalid schema: " + err.Error())
	}
	return buf.Bytes()
}
