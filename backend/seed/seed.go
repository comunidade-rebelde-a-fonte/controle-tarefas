// Package seed carrega os dados de referência (status, catálogo, feriados) e os
// dados de demonstração (20 empresas fictícias e histórico). Todos os seeds são
// idempotentes: podem rodar várias vezes sem duplicar registros.
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
	return nil
}

// Demo carrega os dados de demonstração (supõe os dados de referência já carregados).
func Demo(ctx context.Context, pool *pgxpool.Pool, clk clock.Clock, out io.Writer) error {
	steps := DemoSteps()
	if len(steps) == 0 {
		_, _ = fmt.Fprintln(out, "seed demo: nenhum dado de demonstração disponível ainda")
		return nil
	}
	return Run(ctx, pool, clk, steps, out)
}
