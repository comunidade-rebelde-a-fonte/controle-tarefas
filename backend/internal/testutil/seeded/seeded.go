// Package seeded fornece bancos de teste com os dados de referência carregados
// (status, catálogo, feriados). Fica fora de testutil para evitar ciclos de import.
package seeded

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"controle/internal/platform/clock"
	"controle/internal/testutil"
	"controle/seed"
)

const referenceTemplate = "tmpl_reference"

// NewDB devolve um banco isolado com migrations + seed de referência.
func NewDB(t testing.TB) *pgxpool.Pool {
	t.Helper()
	testutil.EnsureTemplate(t, referenceTemplate, func(ctx context.Context, pool *pgxpool.Pool) error {
		clk := clock.NewFixed(time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC), testutil.SaoPaulo)
		return seed.Reference(ctx, pool, clk, io.Discard)
	})
	return testutil.NewDBFromTemplate(t, referenceTemplate)
}

const demoTemplate = "tmpl_demo"

// NewDemoDB devolve um banco isolado com referência + as 20 empresas fictícias.
func NewDemoDB(t testing.TB) *pgxpool.Pool {
	t.Helper()
	NewDB(t) // garante o template de referência
	testutil.EnsureTemplate(t, demoTemplate, func(ctx context.Context, pool *pgxpool.Pool) error {
		clk := clock.NewFixed(time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC), testutil.SaoPaulo)
		if err := seed.Reference(ctx, pool, clk, io.Discard); err != nil {
			return err
		}
		return seed.Demo(ctx, pool, clk, io.Discard)
	})
	return testutil.NewDBFromTemplate(t, demoTemplate)
}
