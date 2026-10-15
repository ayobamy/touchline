package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func testServer() *Server {
	s := NewServer("test", "0.0.1", "Use the add tool.")
	s.AddTool(Tool{
		Name:        "add",
		Description: "Add two numbers.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"a":{"type":"number"},"b":{"type":"number"}},"required":["a","b"]}`),
		Handler: func(_ context.Context, args json.RawMessage) (any, error) {
			var in struct{ A, B float64 }
			if err := DecodeArgs(args, &in); err != nil {
				return nil, err
			}
			return map[string]float64{"sum": in.A + in.B}, nil
		},
	})
	s.AddTool(Tool{
		Name: "fail",
		Handler: func(context.Context, json.RawMessage) (any, error) {
			return nil, errors.New("nothing to see")
		},
	})
	return s
}

func call(t *testing.T, s *Server, msg string) map[string]any {
	t.Helper()
	out := s.Handle(context.Background(), []byte(msg))
	if out == nil {
		t.Fatalf("no response to %s", msg)
	}
	var resp map[string]any
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestInitializeNegotiatesVersion(t *testing.T) {
	s := testServer()
	resp := call(t, s, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"c","version":"1"}}}`)
	result := resp["result"].(map[string]any)
	if result["protocolVersion"] != "2025-03-26" {
		t.Errorf("version %v, want the client's 2025-03-26", result["protocolVersion"])
	}
	if result["instructions"] != "Use the add tool." {
		t.Errorf("instructions missing: %v", result)
	}

	resp = call(t, s, `{"jsonrpc":"2.0","id":2,"method":"initialize","params":{"protocolVersion":"1999-01-01"}}`)
	if v := resp["result"].(map[string]any)["protocolVersion"]; v != LatestProtocolVersion {
		t.Errorf("unknown version answered with %v, want %s", v, LatestProtocolVersion)
	}
}

func TestToolsListAndCall(t *testing.T) {
	s := testServer()
	resp := call(t, s, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	tools := resp["result"].(map[string]any)["tools"].([]any)
	if len(tools) != 2 {
		t.Fatalf("%d tools, want 2", len(tools))
	}

	resp = call(t, s, `{"jsonrpc":"2.0","id":"a","method":"tools/call","params":{"name":"add","arguments":{"a":2,"b":3}}}`)
	if resp["id"] != "a" {
		t.Errorf("id not echoed: %v", resp["id"])
	}
	result := resp["result"].(map[string]any)
	if result["isError"] != false || result["structuredContent"].(map[string]any)["sum"] != 5.0 {
		t.Errorf("unexpected result %v", result)
	}
}

func TestToolErrorsAreReportedToTheModel(t *testing.T) {
	s := testServer()
	for _, msg := range []string{
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"fail"}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"add","arguments":{"a":1,"c":2}}}`,
	} {
		result := call(t, s, msg)["result"].(map[string]any)
		if result["isError"] != true {
			t.Errorf("want isError for %s, got %v", msg, result)
		}
	}
}

func TestProtocolErrors(t *testing.T) {
	s := testServer()
	cases := map[string]float64{
		`not json`: codeParseError,
		`{"jsonrpc":"2.0","id":1,"method":"resources/list"}`:                   codeMethodNotFound,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"x"}}`: codeInvalidParams,
		`{"id":1,"method":"ping"}`:                                             codeInvalidRequest,
	}
	for msg, code := range cases {
		resp := call(t, s, msg)
		if got := resp["error"].(map[string]any)["code"]; got != code {
			t.Errorf("%s: code %v, want %v", msg, got, code)
		}
	}
	if out := s.Handle(context.Background(), []byte(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)); out != nil {
		t.Errorf("notification answered: %s", out)
	}
}

func TestStdio(t *testing.T) {
	s := testServer()
	in := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}` + "\n" +
		`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n" +
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}` + "\n")
	var out bytes.Buffer
	if err := s.ServeStdio(context.Background(), in, &out); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("%d responses, want 2: %q", len(lines), out.String())
	}
}

func TestHTTP(t *testing.T) {
	srv := httptest.NewServer(testServer().HTTPHandler())
	defer srv.Close()

	post := func(body string) *http.Response {
		resp, err := http.Post(srv.URL, "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}

	resp := post(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`)
	if resp.StatusCode != 200 || resp.Header.Get("Mcp-Session-Id") == "" {
		t.Errorf("initialize: status %d, session %q", resp.StatusCode, resp.Header.Get("Mcp-Session-Id"))
	}
	if resp := post(`{"jsonrpc":"2.0","method":"notifications/initialized"}`); resp.StatusCode != http.StatusAccepted {
		t.Errorf("notification: status %d, want 202", resp.StatusCode)
	}
	get, _ := http.Get(srv.URL)
	if get.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("GET: status %d, want 405", get.StatusCode)
	}
}
