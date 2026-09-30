// Package migrations embute as migrations SQL (goose) no binário da API.
//
// Regra: migration commitada nunca é editada; correções entram em arquivo novo.
package migrations

import "embed"

// FS contém os arquivos NNNN_descricao.sql.
//
//go:embed *.sql
var FS embed.FS
