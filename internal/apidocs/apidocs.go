// Package apidocs serves the server's OpenAPI document at /openapi.json and a
// Scalar API reference that renders it at /docs.
//
// The document is openapi.json in this directory, with the Event, Match and
// Overlay contracts from schema/ added as components when the server starts,
// so the API reference can never disagree with the published schemas.
package apidocs

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/ayobamy/touchline/schema"
)

//go:embed openapi.json
var base []byte

//go:embed reference.html
var reference []byte

// contracts are the published schemas added to the document, by component name.
var contracts = []struct{ file, name string }{
	{"event.schema.json", "Event"},
	{"match.schema.json", "Match"},
	{"overlay.schema.json", "Overlay"},
}

// Spec returns the OpenAPI document with the contracts added and info.version
// set to the server's version.
func Spec(version string) ([]byte, error) {
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(base, &doc); err != nil {
		return nil, fmt.Errorf("apidocs: parse openapi.json: %w", err)
	}
	var info map[string]any
	if err := json.Unmarshal(doc["info"], &info); err != nil {
		return nil, fmt.Errorf("apidocs: openapi.json info: %w", err)
	}
	info["version"] = version
	var components map[string]json.RawMessage
	if err := json.Unmarshal(doc["components"], &components); err != nil {
		return nil, fmt.Errorf("apidocs: openapi.json components: %w", err)
	}
	var schemas map[string]json.RawMessage
	if err := json.Unmarshal(components["schemas"], &schemas); err != nil {
		return nil, fmt.Errorf("apidocs: openapi.json components.schemas: %w", err)
	}

	for _, c := range contracts {
		raw, err := schema.FS.ReadFile(c.file)
		if err != nil {
			return nil, fmt.Errorf("apidocs: %w", err)
		}
		var s map[string]any
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, fmt.Errorf("apidocs: parse %s: %w", c.file, err)
		}
		if err := addContract(schemas, c.name, s); err != nil {
			return nil, err
		}
	}

	var err error
	if components["schemas"], err = json.Marshal(schemas); err != nil {
		return nil, err
	}
	if doc["components"], err = json.Marshal(components); err != nil {
		return nil, err
	}
	if doc["info"], err = json.Marshal(info); err != nil {
		return nil, err
	}
	return encode(doc)
}

func encode(doc map[string]json.RawMessage) ([]byte, error) {
	compact, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("apidocs: encode: %w", err)
	}
	var out bytes.Buffer
	if err := json.Indent(&out, compact, "", "  "); err != nil {
		return nil, fmt.Errorf("apidocs: indent: %w", err)
	}
	out.WriteByte('\n')
	return out.Bytes(), nil
}

// addContract adds one JSON Schema as a component. Its $defs become
// components of their own (named after the contract, e.g. MatchTeam) and its
// internal references are rewritten to point at them, since "#/$defs/..."
// would otherwise resolve against the OpenAPI document's root.
func addContract(schemas map[string]json.RawMessage, name string, s map[string]any) error {
	parts := map[string]any{}
	if defs, ok := s["$defs"].(map[string]any); ok {
		for def, body := range defs {
			parts[name+exported(def)] = rewriteRefs(body, name)
		}
	}
	delete(s, "$defs")
	delete(s, "$schema")
	delete(s, "$id")
	parts[name] = rewriteRefs(s, name)

	for key, body := range parts {
		if existing := schemas[key]; existing != nil {
			return fmt.Errorf("apidocs: component %s already exists", key)
		}
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("apidocs: encode %s: %w", key, err)
		}
		schemas[key] = encoded
	}
	return nil
}

func rewriteRefs(v any, prefix string) any {
	switch t := v.(type) {
	case map[string]any:
		for k, item := range t {
			if ref, ok := item.(string); ok && k == "$ref" && strings.HasPrefix(ref, "#/$defs/") {
				t[k] = "#/components/schemas/" + prefix + exported(strings.TrimPrefix(ref, "#/$defs/"))
				continue
			}
			t[k] = rewriteRefs(item, prefix)
		}
	case []any:
		for i, item := range t {
			t[i] = rewriteRefs(item, prefix)
		}
	}
	return v
}

func exported(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// Register mounts GET /openapi.json and GET /docs on mux. It fails at start-up,
// not on first request, if the document cannot be built.
func Register(mux *http.ServeMux, version string) error {
	spec, err := Spec(version)
	if err != nil {
		return err
	}
	mux.HandleFunc("GET /openapi.json", func(w http.ResponseWriter, _ *http.Request) {
		send(w, "application/json", spec)
	})
	mux.HandleFunc("GET /docs", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Referrer-Policy", "no-referrer")
		send(w, "text/html; charset=utf-8", reference)
	})
	return nil
}

func send(w http.ResponseWriter, contentType string, body []byte) {
	w.Header().Set("Content-Type", contentType)
	if n, err := w.Write(body); err != nil {
		slog.Debug("apidocs: response not fully written", "written", n, "err", err)
	}
}
