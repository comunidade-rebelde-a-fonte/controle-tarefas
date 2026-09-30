package app_test

import (
	"context"
	"net/http"
	"testing"

	"controle/internal/store"
	"controle/internal/testutil"
)

func TestHealth_WithDatabase(t *testing.T) {
	pool := testutil.NewDB(t)
	srv := testutil.NewServer(t, pool, testutil.FixedClock(t, "2026-10-01T12:00:00Z"))

	var body struct {
		Status   string `json:"status"`
		Database string `json:"database"`
	}
	srv.Get("/api/v1/health").MustStatus(t, http.StatusOK).JSON(t, &body)
	if body.Status != "ok" || body.Database != "ok" {
		t.Fatalf("health = %+v", body)
	}
}

func TestDatabase_SessionIsUTCAndExtensionsInstalled(t *testing.T) {
	pool := testutil.NewDB(t)
	ctx := context.Background()

	tz, err := store.New(pool).SessionTimezone(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if tz != "UTC" {
		t.Fatalf("timezone da sessão = %q, want UTC", tz)
	}

	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM pg_extension WHERE extname IN ('pgcrypto','btree_gist','pg_trgm')`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("extensões instaladas = %d, want 3", n)
	}
}

func TestDatabase_IsolatedPerTest(t *testing.T) {
	a := testutil.NewDB(t)
	b := testutil.NewDB(t)
	ctx := context.Background()
	if _, err := a.Exec(ctx, `CREATE TABLE only_in_a (id int)`); err != nil {
		t.Fatal(err)
	}
	var exists bool
	if err := b.QueryRow(ctx, `SELECT to_regclass('only_in_a') IS NOT NULL`).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatal("bancos de teste não estão isolados")
	}
}
