// Package migrations embarca os arquivos .sql do goose no binario.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
