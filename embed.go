// Package migrations embeds the forward-only SQL migrations so the dpd binary
// carries its own schema. Files are applied in lexical order: NNNN_name.sql.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
