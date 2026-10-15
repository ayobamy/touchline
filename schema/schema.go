// Package schema embeds the published JSON Schemas so the server can serve
// them inside its OpenAPI document. The .json files in this directory are the
// contracts; this file only makes them available to Go code.
package schema

import "embed"

// FS holds event.schema.json, match.schema.json and overlay.schema.json.
//
//go:embed *.schema.json
var FS embed.FS
