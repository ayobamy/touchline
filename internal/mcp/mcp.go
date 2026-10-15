// Package mcp is a small, dependency-free server for the Model Context
// Protocol (MCP). It supports what Touchline needs: tools, over stdio and the
// streamable HTTP transport, with JSON responses.
//
// Supported methods: initialize, notifications/initialized, ping, tools/list,
// tools/call. Anything else returns JSON-RPC "method not found".
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"sync"
)

// LatestProtocolVersion is offered to clients that ask for a version this
// server does not know.
const LatestProtocolVersion = "2025-06-18"

var supportedVersions = []string{"2024-11-05", "2025-03-26", "2025-06-18", "2025-11-25"}

// JSON-RPC error codes.
const (
	codeParseError     = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
	codeInternalError  = -32603
)

// ToolFunc handles one tool call. args is the raw "arguments" object. The
// returned value must marshal to a JSON object; it is sent both as
// structured content and as text. A returned error becomes a tool error
// (isError: true) that the model can read and act on.
type ToolFunc func(ctx context.Context, args json.RawMessage) (any, error)

// Tool is one callable tool.
type Tool struct {
	Name        string
	Title       string
	Description string
	InputSchema json.RawMessage
	Handler     ToolFunc
}

// Server holds the tools and answers MCP requests.
type Server struct {
	name, version, instructions string

	mu    sync.RWMutex
	tools []Tool
}

// NewServer returns a server that introduces itself with name and version.
// instructions, if set, tells the model how to use the tools.
func NewServer(name, version, instructions string) *Server {
	return &Server{name: name, version: version, instructions: instructions}
}

// AddTool registers a tool. It panics on a duplicate name or a missing
// handler, since both are programming errors.
func (s *Server) AddTool(t Tool) {
	if t.Handler == nil {
		panic("mcp: tool " + t.Name + " has no handler")
	}
	if len(t.InputSchema) == 0 {
		t.InputSchema = json.RawMessage(`{"type":"object","properties":{}}`)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.tools {
		if existing.Name == t.Name {
			panic("mcp: duplicate tool " + t.Name)
		}
	}
	s.tools = append(s.tools, t)
}

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Handle processes one JSON-RPC message and returns the encoded response, or
// nil when the message was a notification that needs no reply.
func (s *Server) Handle(ctx context.Context, msg []byte) []byte {
	var req request
	if err := json.Unmarshal(msg, &req); err != nil {
		return encode(response{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{codeParseError, "parse error: " + err.Error()}})
	}
	isNotification := len(req.ID) == 0
	if req.JSONRPC != "2.0" || req.Method == "" {
		if isNotification {
			return nil
		}
		return encode(response{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{codeInvalidRequest, "invalid JSON-RPC 2.0 request"}})
	}

	result, rerr := s.dispatch(ctx, req)
	if isNotification {
		return nil
	}
	resp := response{JSONRPC: "2.0", ID: req.ID}
	if rerr != nil {
		resp.Error = rerr
	} else {
		resp.Result = result
	}
	return encode(resp)
}

func (s *Server) dispatch(ctx context.Context, req request) (any, *rpcError) {
	switch req.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(req.Params, &p)
		version := LatestProtocolVersion
		if slices.Contains(supportedVersions, p.ProtocolVersion) {
			version = p.ProtocolVersion
		}
		result := map[string]any{
			"protocolVersion": version,
			"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
			"serverInfo":      map[string]any{"name": s.name, "version": s.version},
		}
		if s.instructions != "" {
			result["instructions"] = s.instructions
		}
		return result, nil
	case "ping":
		return struct{}{}, nil
	case "tools/list":
		s.mu.RLock()
		defer s.mu.RUnlock()
		list := make([]map[string]any, 0, len(s.tools))
		for _, t := range s.tools {
			entry := map[string]any{"name": t.Name, "description": t.Description, "inputSchema": t.InputSchema}
			if t.Title != "" {
				entry["title"] = t.Title
			}
			list = append(list, entry)
		}
		return map[string]any{"tools": list}, nil
	case "tools/call":
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil || p.Name == "" {
			return nil, &rpcError{codeInvalidParams, "tools/call needs a tool name"}
		}
		tool, ok := s.lookup(p.Name)
		if !ok {
			return nil, &rpcError{codeInvalidParams, "unknown tool " + p.Name}
		}
		if len(p.Arguments) == 0 || string(p.Arguments) == "null" {
			p.Arguments = json.RawMessage("{}")
		}
		return s.call(ctx, tool, p.Arguments), nil
	default:
		if len(req.Method) > 14 && req.Method[:14] == "notifications/" {
			return nil, nil
		}
		return nil, &rpcError{codeMethodNotFound, "method not found: " + req.Method}
	}
}

func (s *Server) lookup(name string) (Tool, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, t := range s.tools {
		if t.Name == name {
			return t, true
		}
	}
	return Tool{}, false
}

func (s *Server) call(ctx context.Context, t Tool, args json.RawMessage) (result map[string]any) {
	defer func() {
		if r := recover(); r != nil {
			result = toolError(fmt.Sprintf("tool %s failed: %v", t.Name, r))
		}
	}()
	out, err := t.Handler(ctx, args)
	if err != nil {
		return toolError(err.Error())
	}
	text, err := json.Marshal(out)
	if err != nil {
		return toolError("could not encode result: " + err.Error())
	}
	return map[string]any{
		"content":           []map[string]any{{"type": "text", "text": string(text)}},
		"structuredContent": json.RawMessage(text),
		"isError":           false,
	}
}

func toolError(msg string) map[string]any {
	return map[string]any{
		"content": []map[string]any{{"type": "text", "text": msg}},
		"isError": true,
	}
}

func encode(r response) []byte {
	b, err := json.Marshal(r)
	if err != nil {
		b, _ = json.Marshal(response{JSONRPC: "2.0", ID: r.ID, Error: &rpcError{codeInternalError, "could not encode response"}})
	}
	return b
}

// ServeStdio reads newline-delimited JSON-RPC messages from r and writes
// responses to w until r is closed or ctx is cancelled.
func (s *Server) ServeStdio(ctx context.Context, r io.Reader, w io.Writer) error {
	in := bufio.NewScanner(r)
	in.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	out := bufio.NewWriter(w)
	for in.Scan() {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		line := in.Bytes()
		if len(line) == 0 {
			continue
		}
		if resp := s.Handle(ctx, line); resp != nil {
			if _, err := out.Write(append(resp, '\n')); err != nil {
				return err
			}
			if err := out.Flush(); err != nil {
				return err
			}
		}
	}
	if err := in.Err(); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

// DecodeArgs unmarshals tool arguments into v, rejecting unknown fields so a
// model's typo surfaces as an error it can fix.
func DecodeArgs(args json.RawMessage, v any) error {
	dec := json.NewDecoder(bytesReader(args))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("invalid arguments: %w", err)
	}
	return nil
}
