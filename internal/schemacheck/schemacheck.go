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
	var schema struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		return "", fmt.Errorf("parse %s: %w", path, err)
	}

	goFields := map[string]bool{}
	for i := range typ.NumField() {
		f := typ.Field(i)
		if !f.IsExported() {
			continue
		}
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if name == "" || name == "-" {
			continue
		}
		goFields[name] = true
	}

	var missing, extra []string
	for name := range goFields {
		if _, ok := schema.Properties[name]; !ok {
			missing = append(missing, name)
		}
	}
	for name := range schema.Properties {
		if !goFields[name] {
			extra = append(extra, name)
		}
	}
	slices.Sort(missing)
	slices.Sort(extra)

	var b strings.Builder
	if len(missing) > 0 {
		fmt.Fprintf(&b, "in %s but not in %s: %v. ", typ.Name(), path, missing)
	}
	if len(extra) > 0 {
		fmt.Fprintf(&b, "in %s but not in %s: %v.", path, typ.Name(), extra)
	}
	return b.String(), nil
}
