// Package migrate aplica as migrations embutidas usando goose como biblioteca.
package migrate

import (
	"context"
	"database/sql"
	"fmt"
	"io"

	_ "github.com/jackc/pgx/v5/stdlib" // driver database/sql "pgx"
	"github.com/pressly/goose/v3"

	"controle/migrations"
)

func provider(dsn string) (*goose.Provider, *sql.DB, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, nil, fmt.Errorf("abrir banco: %w", err)
	}
	p, err := goose.NewProvider(goose.DialectPostgres, db, migrations.FS)
	if err != nil {
		_ = db.Close()
		return nil, nil, fmt.Errorf("carregar migrations: %w", err)
	}
	return p, db, nil
}

// Up aplica todas as migrations pendentes.
func Up(ctx context.Context, dsn string, w io.Writer) error {
	p, db, err := provider(dsn)
	if err != nil {
		return err
	}
	defer closeDB(db)
	results, err := p.Up(ctx)
	for _, r := range results {
		printf(w, "up   %s (%s)\n", r.Source.Path, r.Duration.Round(1e6))
	}
	if err != nil {
		return fmt.Errorf("migrate up: %w", err)
	}
	if len(results) == 0 {
		printf(w, "nenhuma migration pendente\n")
	}
	return nil
}

// Down reverte a última migration aplicada.
func Down(ctx context.Context, dsn string, w io.Writer) error {
	p, db, err := provider(dsn)
	if err != nil {
		return err
	}
	defer closeDB(db)
	r, err := p.Down(ctx)
	if r != nil {
		printf(w, "down %s (%s)\n", r.Source.Path, r.Duration.Round(1e6))
	}
	if err != nil {
		return fmt.Errorf("migrate down: %w", err)
	}
	return nil
}

// Reset reverte todas as migrations (usado para recriar os bancos de E2E e carga).
func Reset(ctx context.Context, dsn string, w io.Writer) error {
	if err := DownTo(ctx, dsn, 0, w); err != nil {
		return fmt.Errorf("migrate reset: %w", err)
	}
	return nil
}

// DownTo reverte as migrations posteriores a version (0 = todas).
func DownTo(ctx context.Context, dsn string, version int64, w io.Writer) error {
	p, db, err := provider(dsn)
	if err != nil {
		return err
	}
	defer closeDB(db)
	results, err := p.DownTo(ctx, version)
	for _, r := range results {
		printf(w, "down %s\n", r.Source.Path)
	}
	if err != nil {
		return fmt.Errorf("migrate down até %d: %w", version, err)
	}
	return nil
}

// Status lista as migrations e seu estado.
func Status(ctx context.Context, dsn string, w io.Writer) error {
	p, db, err := provider(dsn)
	if err != nil {
		return err
	}
	defer closeDB(db)
	st, err := p.Status(ctx)
	if err != nil {
		return fmt.Errorf("migrate status: %w", err)
	}
	for _, s := range st {
		applied := "pendente"
		if s.State == goose.StateApplied {
			applied = "aplicada em " + s.AppliedAt.UTC().Format("2006-01-02 15:04:05") + " UTC"
		}
		printf(w, "%-45s %s\n", s.Source.Path, applied)
	}
	return nil
}

func closeDB(db *sql.DB) { _ = db.Close() }

func printf(w io.Writer, format string, a ...any) { _, _ = fmt.Fprintf(w, format, a...) }
