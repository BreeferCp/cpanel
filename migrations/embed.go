// Package migrations embeds the SQL schema files.
package migrations

import "embed"

// FS contains every NNNN_name.sql migration in lexical order.
//
//go:embed *.sql
var FS embed.FS
