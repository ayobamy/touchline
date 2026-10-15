package apidocs

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/ayobamy/touchline/internal/events"
	"github.com/ayobamy/touchline/internal/moments"
	"github.com/ayobamy/touchline/internal/schemacheck"
	"github.com/ayobamy/touchline/internal/stats"
	"github.com/ayobamy/touchline/internal/store"
	"github.com/ayobamy/touchline/schema"
)

type document struct {
	OpenAPI string `json:"openapi"`
	Info    struct {
		Version string `json:"version"`
	} `json:"info"`
	Paths      map[string]map[string]json.RawMessage `json:"paths"`
	Components struct {
		Schemas map[string]json.RawMessage `json:"schemas"`
	} `json:"components"`
}

func spec(t *testing.T) []byte {
	t.Helper()
	raw, err := Spec("9.9.9")
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func parse(t *testing.T, raw []byte) document {
	t.Helper()
	var doc document
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("spec is not JSON: %v", err)
	}
	return doc
}

func refs(v any, out *[]string) {
	switch t := v.(type) {
	case map[string]any:
		for k, item := range t {
			if ref, ok := item.(string); ok && k == "$ref" {
				*out = append(*out, ref)
			}
			refs(item, out)
		}
	case []any:
		for _, item := range t {
			refs(item, out)
		}
	}
}

func TestSpec(t *testing.T) {
	raw := spec(t)
	doc := parse(t, raw)

	t.Run("is OpenAPI 3.1 with the server version", func(t *testing.T) {
		if !strings.HasPrefix(doc.OpenAPI, "3.1.") {
			t.Errorf("openapi = %q, want 3.1.x", doc.OpenAPI)
		}
		if doc.Info.Version != "9.9.9" {
			t.Errorf("info.version = %q, want the server version", doc.Info.Version)
		}
	})

	t.Run("every reference resolves to a component", func(t *testing.T) {
		var tree any
		if err := json.Unmarshal(raw, &tree); err != nil {
			t.Fatal(err)
		}
		var all []string
		refs(tree, &all)
		if len(all) == 0 {
			t.Fatal("no references found")
		}
		for _, ref := range all {
			name, ok := strings.CutPrefix(ref, "#/components/schemas/")
			if !ok || doc.Components.Schemas[name] == nil {
				t.Errorf("unresolved reference %s", ref)
			}
		}
	})

	t.Run("embeds the published contracts and their parts", func(t *testing.T) {
		for _, name := range []string{"Event", "EventPoint", "EventScore", "Match", "MatchTeam", "MatchPlayer", "Overlay"} {
			if doc.Components.Schemas[name] == nil {
				t.Errorf("component %s missing", name)
			}
		}
	})

	t.Run("overlay component is the published schema", func(t *testing.T) {
		published, err := schema.FS.ReadFile("overlay.schema.json")
		if err != nil {
			t.Fatal(err)
		}
		var want, got struct {
			Required   []string                   `json:"required"`
			Properties map[string]json.RawMessage `json:"properties"`
		}
		if err := json.Unmarshal(published, &want); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(doc.Components.Schemas["Overlay"], &got); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(want.Required, got.Required) || len(want.Properties) != len(got.Properties) {
			t.Errorf("Overlay component differs from schema/overlay.schema.json")
		}
	})
}

func TestComponentsMatchGoTypes(t *testing.T) {
	doc := parse(t, spec(t))
	cases := map[string]reflect.Type{
		"Status":      reflect.TypeOf(store.Status{}),
		"Score":       reflect.TypeOf(events.Score{}),
		"Snapshot":    reflect.TypeOf(stats.Snapshot{}),
		"TeamStats":   reflect.TypeOf(stats.TeamStats{}),
		"PlayerStats": reflect.TypeOf(stats.PlayerStats{}),
		"Window":      reflect.TypeOf(stats.Window{}),
		"Moment":      reflect.TypeOf(moments.Moment{}),
	}
	for name, typ := range cases {
		t.Run(name, func(t *testing.T) {
			diff, err := schemacheck.DiffSchema(doc.Components.Schemas[name], name, typ)
			if err != nil {
				t.Fatal(err)
			}
			if diff != "" {
				t.Errorf("drift: %s", diff)
			}
		})
	}
}

func TestRegister(t *testing.T) {
	mux := http.NewServeMux()
	if err := Register(mux, "1.0.0"); err != nil {
		t.Fatal(err)
	}

	t.Run("serves the document", func(t *testing.T) {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/openapi.json", nil))
		if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "application/json" {
			t.Fatalf("status %d, content type %q", rec.Code, rec.Header().Get("Content-Type"))
		}
		if !json.Valid(rec.Body.Bytes()) {
			t.Error("body is not JSON")
		}
	})

	t.Run("serves the reference page pinned and private", func(t *testing.T) {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/docs", nil))
		body := rec.Body.String()
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d", rec.Code)
		}
		for _, want := range []string{
			"@scalar/api-reference@1.72.2/", `integrity="sha384-`, "url: '/openapi.json'",
			"telemetry: false", "withDefaultFonts: false", "agent: { disabled: true }",
		} {
			if !strings.Contains(body, want) {
				t.Errorf("reference page lacks %q", want)
			}
		}
	})
}
