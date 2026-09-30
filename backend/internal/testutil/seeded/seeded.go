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

const (
	companiesTemplate = "tmpl_companies"
	demoTemplate      = "tmpl_demo"
)

// DemoClock é o relógio usado nos templates de demonstração (competência corrente 10/2026).
func DemoClock() *clock.Fixed {
	return clock.NewFixed(time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC), testutil.SaoPaulo)
}

// NewCompaniesDB devolve um banco com referência + as 20 empresas fictícias, sem nenhuma
// competência gerada (para testes do motor de geração).
func NewCompaniesDB(t testing.TB) *pgxpool.Pool {
	t.Helper()
	testutil.EnsureTemplate(t, companiesTemplate, func(ctx context.Context, pool *pgxpool.Pool) error {
		if err := seed.Reference(ctx, pool, DemoClock(), io.Discard); err != nil {
			return err
		}
		return seed.DemoCompanies(ctx, pool, DemoClock(), io.Discard)
	})
	return testutil.NewDBFromTemplate(t, companiesTemplate)
}

// NewDemoDB devolve um banco com a demonstração completa: 20 empresas e as competências
// 07/2026 a 10/2026 com status, fechamentos, reabertura e histórico (relógio DemoClock).
func NewDemoDB(t testing.TB) *pgxpool.Pool {
	t.Helper()
	testutil.EnsureTemplate(t, demoTemplate, func(ctx context.Context, pool *pgxpool.Pool) error {
		if err := seed.Reference(ctx, pool, DemoClock(), io.Discard); err != nil {
			return err
		}
		return seed.Demo(ctx, pool, DemoClock(), io.Discard)
	})
	return testutil.NewDBFromTemplate(t, demoTemplate)
}
