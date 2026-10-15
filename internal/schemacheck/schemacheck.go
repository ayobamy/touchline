// Package schemacheck guards against drift between the Go types and the
// published JSON Schemas in schema/: every JSON field of the Go type must be
// a property in the schema, and the other way round.
package schemacheck

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"slices"
	"strings"
)

// Diff compares the top-level JSON field names of typ with the properties of
// the schema at path, and describes any mismatch. An empty string means they
// agree.
func Diff(path string, typ reflect.Type) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return DiffSchema(raw, path, typ)
}

// DiffSchema is Diff for a schema already in memory, such as a component of
// an OpenAPI document; name identifies the schema in the description.
func DiffSchema(raw []byte, name string, typ reflect.Type) (string, error) {
	var schema struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		return "", fmt.Errorf("parse %s: %w", name, err)
	}

	goFields := map[string]bool{}
	for i := range typ.NumField() {
		f := typ.Field(i)
		if !f.IsExported() {
			continue
		}
		field := strings.Split(f.Tag.Get("json"), ",")[0]
		if field == "" || field == "-" {
			continue
		}
		goFields[field] = true
	}

	var missing, extra []string
	for field := range goFields {
		if schema.Properties[field] == nil {
			missing = append(missing, field)
		}
	}
	for field := range schema.Properties {
		if !goFields[field] {
			extra = append(extra, field)
		}
	}
	slices.Sort(missing)
	slices.Sort(extra)

	var b strings.Builder
	if len(missing) > 0 {
		fmt.Fprintf(&b, "in %s but not in %s: %v. ", typ.Name(), name, missing)
	}
	if len(extra) > 0 {
		fmt.Fprintf(&b, "in %s but not in %s: %v.", name, typ.Name(), extra)
	}
	return b.String(), nil
}
