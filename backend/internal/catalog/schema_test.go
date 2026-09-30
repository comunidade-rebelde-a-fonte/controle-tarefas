package catalog

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"controle/internal/platform/db"
	"controle/internal/testutil"
)

func mkSet(t *testing.T, pool *pgxpool.Pool, code string, statuses ...string) string {
	t.Helper()
	ctx := context.Background()
	var id string
	if err := pool.QueryRow(ctx, `INSERT INTO status_sets (code, name) VALUES ($1, $1) RETURNING id`, code).Scan(&id); err != nil {
		t.Fatal(err)
	}
	for i, s := range statuses {
		if _, err := pool.Exec(ctx, `INSERT INTO status_definitions
			(status_set_id, code, label, sort_order, counts_as_started, counts_as_operational_progress, is_final, is_blocking)
			VALUES ($1, $2, $2, $3, false, false, false, false)`, id, s, i); err != nil {
			t.Fatal(err)
		}
	}
	return id
}

const insType = `INSERT INTO obligation_types (code, name, primary_module, status_set_id, sort_order, initial_status_code, status_when_no_payroll)
                 VALUES ($1, $1, 'PAYROLL', $2, 1, $3, $4) RETURNING id`

func TestSchema_TypeRequiresExistingStatusSetAndStatus(t *testing.T) {
	pool := testutil.NewDB(t)
	ctx := context.Background()
	setID := mkSet(t, pool, "PAYROLL", "NOT_STARTED", "NO_MOVEMENT")

	_, err := pool.Exec(ctx, insType, "X", "7b0f8d7e-0000-4000-8000-000000000000", "NOT_STARTED", nil)
	if !db.IsConstraint(err, db.ForeignKeyViolation, "") {
		t.Fatalf("status set inexistente deveria falhar: %v", err)
	}
	_, err = pool.Exec(ctx, insType, "X", setID, "INVENTADO", nil)
	if !db.IsConstraint(err, db.ForeignKeyViolation, "obligation_types_initial_status_fk") {
		t.Fatalf("status inicial fora do conjunto deveria falhar: %v", err)
	}
	_, err = pool.Exec(ctx, insType, "X", setID, "NOT_STARTED", "NOT_APPLICABLE")
	if !db.IsConstraint(err, db.ForeignKeyViolation, "obligation_types_no_payroll_status_fk") {
		t.Fatalf("status 'sem folha' fora do conjunto deveria falhar: %v", err)
	}
	var id string
	if err := pool.QueryRow(ctx, insType, "PAYROLL_PROCESSING", setID, "NOT_STARTED", "NO_MOVEMENT").Scan(&id); err != nil {
		t.Fatalf("tipo válido deveria ser aceito: %v", err)
	}
	_, err = pool.Exec(ctx, insType, "PAYROLL_PROCESSING", setID, "NOT_STARTED", nil)
	if !db.IsConstraint(err, db.UniqueViolation, "obligation_types_code_uq") {
		t.Fatalf("código duplicado deveria falhar: %v", err)
	}
}

func TestSchema_DependencyAndStatusConstraints(t *testing.T) {
	pool := testutil.NewDB(t)
	ctx := context.Background()
	setID := mkSet(t, pool, "ACCOUNTING", "NOT_STARTED", "OK")
	var a, b string
	if err := pool.QueryRow(ctx, insType, "A", setID, "NOT_STARTED", nil).Scan(&a); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, insType, "B", setID, "NOT_STARTED", nil).Scan(&b); err != nil {
		t.Fatal(err)
	}
	ins := `INSERT INTO obligation_dependencies (obligation_type_id, prerequisite_obligation_type_id, dependency_mode) VALUES ($1, $2, $3)`
	if _, err := pool.Exec(ctx, ins, b, a, "HARD"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, ins, a, a, "HARD"); !db.IsConstraint(err, db.CheckViolation, "obligation_dependencies_self_ck") {
		t.Fatalf("auto-dependência deveria falhar: %v", err)
	}
	if _, err := pool.Exec(ctx, ins, b, a, "SOFT"); !db.IsConstraint(err, db.UniqueViolation, "obligation_dependencies_uq") {
		t.Fatalf("dependência duplicada deveria falhar: %v", err)
	}
	if _, err := pool.Exec(ctx, ins, a, b, "MAYBE"); !db.IsConstraint(err, db.CheckViolation, "") {
		t.Fatalf("modo inválido deveria falhar: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO status_definitions (status_set_id, code, label, sort_order, counts_as_started, counts_as_operational_progress, is_final, is_blocking)
		VALUES ($1, 'OK', 'OK', 9, true, true, true, false)`, setID); !db.IsConstraint(err, db.UniqueViolation, "status_definitions_set_code_uq") {
		t.Fatalf("status duplicado no conjunto deveria falhar: %v", err)
	}
}

func TestSchema_DefaultCredentialThresholds(t *testing.T) {
	pool := testutil.NewDB(t)
	var days []int
	if err := pool.QueryRow(context.Background(), `SELECT ARRAY(SELECT jsonb_array_elements_text(value->'days')::int) FROM settings WHERE key = 'credential_thresholds'`).Scan(&days); err != nil {
		t.Fatal(err)
	}
	if len(days) != 4 || days[0] != 7 || days[3] != 60 {
		t.Fatalf("faixas = %v", days)
	}
}
