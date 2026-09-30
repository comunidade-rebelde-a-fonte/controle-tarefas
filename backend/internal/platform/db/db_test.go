package db_test

import (
	"context"
	"testing"

	"controle/internal/testutil"
)

// As conexões usam planos com os valores reais (sem plano genérico de prepared statement) e
// fuso UTC [T-M7-02, PRD §45.2].
func TestNewPool_SessionSettings(t *testing.T) {
	pool := testutil.NewDB(t)
	for setting, want := range map[string]string{"plan_cache_mode": "force_custom_plan", "TimeZone": "UTC"} {
		var got string
		if err := pool.QueryRow(context.Background(), "SELECT current_setting($1)", setting).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("%s = %q, want %q", setting, got, want)
		}
	}
}
