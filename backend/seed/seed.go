// Package seed carrega os dados de referência (status, catálogo, feriados, tabelas do
// Simples do Fator R) e os dados de demonstração (21 empresas fictícias e histórico). Todos
// os seeds são idempotentes: podem rodar várias vezes sem duplicar registros.
package seed

import (
	"context"
	"embed"
	"fmt"
	"io"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"controle/internal/platform/clock"
	"controle/internal/platform/db"
)

//go:embed *.sql
var sqlFiles embed.FS

// Step é uma etapa de seed executada numa transação.
type Step struct {
	Name string
	Run  func(ctx context.Context, tx pgx.Tx, clk clock.Clock) error
}

func sqlStep(file string) Step {
	return Step{Name: file, Run: func(ctx context.Context, tx pgx.Tx, _ clock.Clock) error {
		b, err := sqlFiles.ReadFile(file)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, string(b))
		return err
	}}
}

// ReferenceSteps são os dados de referência necessários ao funcionamento do sistema.
func ReferenceSteps() []Step {
	return []Step{
		sqlStep("status.sql"),
		calendarStep(),
		sqlStep("catalog.sql"),
		simplesTablesStep(),
	}
}

// Run executa as etapas em uma única transação.
func Run(ctx context.Context, pool *pgxpool.Pool, clk clock.Clock, steps []Step, out io.Writer) error {
	return db.WithTx(ctx, pool, func(tx pgx.Tx) error {
		for _, s := range steps {
			if err := s.Run(ctx, tx, clk); err != nil {
				return fmt.Errorf("seed %s: %w", s.Name, err)
			}
			_, _ = fmt.Fprintf(out, "seed %s ok\n", s.Name)
		}
		return nil
	})
}

// Reference carrega os dados de referência.
func Reference(ctx context.Context, pool *pgxpool.Pool, clk clock.Clock, out io.Writer) error {
	return Run(ctx, pool, clk, ReferenceSteps(), out)
}

// DemoSteps são os dados fictícios de demonstração (empresas e histórico).
func DemoSteps() []Step {
	return []Step{
		companiesStep(),
	}
}

// DemoCompanies carrega só as empresas fictícias e os responsáveis (sem competências).
func DemoCompanies(ctx context.Context, pool *pgxpool.Pool, clk clock.Clock, out io.Writer) error {
	return Run(ctx, pool, clk, DemoSteps(), out)
}

// Demo carrega os dados de demonstração (supõe os dados de referência já carregados):
// empresas fictícias e, em seguida, competências históricas com status variados.
func Demo(ctx context.Context, pool *pgxpool.Pool, clk clock.Clock, out io.Writer) error {
	if err := DemoCompanies(ctx, pool, clk, out); err != nil {
		return err
	}
	if err := Run(ctx, pool, clk, []Step{credentialsStep(), factorRDemoStep()}, out); err != nil {
		return err
	}
	if err := demoHistory(ctx, pool, clk); err != nil {
		return fmt.Errorf("seed demo: histórico: %w", err)
	}
	_, _ = fmt.Fprintf(out, "seed demo: %d competências anteriores + corrente ok\n", HistoryMonths)
	return nil
}
