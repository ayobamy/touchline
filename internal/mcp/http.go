package mcp

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"io"
	"net/http"
)

func bytesReader(b []byte) io.Reader { return bytes.NewReader(b) }

// maxBody caps the size of a request body.
const maxBody = 1 << 20

// HTTPHandler serves the MCP streamable HTTP transport at whatever path it is
// mounted on. Every request is a POST of one JSON-RPC message; responses are
// plain JSON (the transport allows a server to answer without SSE). GET
// returns 405, since this server never initiates messages.
func (s *Server) HTTPHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent) // sessions hold no state
			return
		default:
			w.Header().Set("Allow", "POST, DELETE")
			http.Error(w, "this MCP server only accepts POST", http.StatusMethodNotAllowed)
			return
		}

		body, err := io.ReadAll(io.LimitReader(r.Body, maxBody+1))
		if err != nil || len(body) > maxBody {
			http.Error(w, "request body too large or unreadable", http.StatusRequestEntityTooLarge)
			return
		}

		resp := s.Handle(r.Context(), body)
		if resp == nil {
			w.WriteHeader(http.StatusAccepted) // notification
			return
		}
		if isInitialize(body) {
			w.Header().Set("Mcp-Session-Id", newSessionID())
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(resp)
	})
}

func isInitialize(body []byte) bool {
	return bytes.Contains(body, []byte(`"initialize"`)) && !bytes.Contains(body, []byte(`"notifications/initialized"`))
}

func newSessionID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
