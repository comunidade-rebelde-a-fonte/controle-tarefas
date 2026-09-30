// Package db cria o pool de conexões PostgreSQL e oferece helpers de transação.
package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NewPool cria o pool (sem conectar imediatamente) com timezone=UTC em todas as conexões.
//
// plan_cache_mode=force_custom_plan: o pgx prepara e reutiliza as consultas, e o PostgreSQL
// passa a usar um plano genérico após algumas execuções. Nas consultas com filtros opcionais
// (`$n IS NULL OR ...`), como grades, dashboard e backlog, o plano genérico é de 2 a 4 vezes
// mais lento; o plano com os valores reais custa poucos milissegundos de planejamento [T-M7-02].
func NewPool(ctx context.Context, url string, maxConns int32) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("DATABASE_URL inválida: %w", err)
	}
	cfg.MaxConns = maxConns
	cfg.ConnConfig.RuntimeParams["timezone"] = "UTC"
	cfg.ConnConfig.RuntimeParams["application_name"] = "controle-api"
	cfg.ConnConfig.RuntimeParams["plan_cache_mode"] = "force_custom_plan"
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("criar pool: %w", err)
	}
	return pool, nil
}

// Beginner é satisfeito por *pgxpool.Pool e pgx.Tx.
type Beginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// WithTx executa fn em uma transação, com commit em sucesso e rollback em erro/panic.
func WithTx(ctx context.Context, b Beginner, fn func(tx pgx.Tx) error) (err error) {
	tx, err := b.Begin(ctx)
	if err != nil {
		return fmt.Errorf("iniciar transação: %w", err)
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback(ctx)
			panic(p)
		}
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()
	if err = fn(tx); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// Códigos SQLSTATE usados na tradução de erros.
const (
	UniqueViolation     = "23505"
	ForeignKeyViolation = "23503"
	CheckViolation      = "23514"
	ExclusionViolation  = "23P01"
	NotNullViolation    = "23502"
)

// PgError extrai o *pgconn.PgError de err.
func PgError(err error) (*pgconn.PgError, bool) {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr, true
	}
	return nil, false
}

// IsConstraint indica se err é violação do código e (opcionalmente) da constraint informada.
func IsConstraint(err error, code, constraint string) bool {
	pgErr, ok := PgError(err)
	if !ok || pgErr.Code != code {
		return false
	}
	return constraint == "" || pgErr.ConstraintName == constraint
}

// InvalidTextRepresentation (22P02): ex.: UUID malformado.
const InvalidTextRepresentation = "22P02"

// IsInvalidText indica erro de conversão de texto (ex.: UUID inválido).
func IsInvalidText(err error) bool {
	pgErr, ok := PgError(err)
	return ok && pgErr.Code == InvalidTextRepresentation
}
