package competence_test

import (
	"context"
	"testing"

	"controle/internal/platform/db"
	"controle/internal/testutil/seeded"
	"controle/internal/user"
)

func TestSchema_InstanceUniquePreventsDuplication(t *testing.T) {
	pool := seeded.NewDB(t)
	ctx := context.Background()
	var companyID, compID, typeID string
	if err := pool.QueryRow(ctx, `INSERT INTO companies (internal_code, legal_name, cnpj, service_start_date, payroll_type, created_by, updated_by)
		VALUES ('101', 'Alfa', '00000001000101', '2025-01-01', 'PAYROLL', $1, $1) RETURNING id`, user.AdminID).Scan(&companyID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO competencies (competence_key, year, month, first_day) VALUES ('2026-09', 2026, 9, '2026-09-01') RETURNING id`).Scan(&compID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT id FROM obligation_types WHERE code = 'MIT'`).Scan(&typeID); err != nil {
		t.Fatal(err)
	}
	ins := `INSERT INTO obligation_instances (company_id, competence_id, obligation_type_id, status_code, applicable, counts_in_progress, regime_snapshot)
	        VALUES ($1, $2, $3, 'NOT_STARTED', true, true, 'LUCRO_PRESUMIDO')`
	if _, err := pool.Exec(ctx, ins, companyID, compID, typeID); err != nil {
		t.Fatal(err)
	}
	// A mesma obrigação para a mesma empresa e competência não pode existir duas vezes,
	// mesmo que exibida em duas telas (Declarações e DCTFWeb) [RN-009].
	_, err := pool.Exec(ctx, ins, companyID, compID, typeID)
	if !db.IsConstraint(err, db.UniqueViolation, "obligation_instances_uq") {
		t.Fatalf("instância duplicada deveria falhar em obligation_instances_uq: %v", err)
	}
	// ON CONFLICT DO NOTHING (usado na geração) não duplica nem falha.
	tag, err := pool.Exec(ctx, ins+` ON CONFLICT (company_id, competence_id, obligation_type_id) DO NOTHING`, companyID, compID, typeID)
	if err != nil || tag.RowsAffected() != 0 {
		t.Fatalf("ON CONFLICT DO NOTHING: rows=%d err=%v", tag.RowsAffected(), err)
	}
}

func TestSchema_CompetenceAndPeriodConstraints(t *testing.T) {
	pool := seeded.NewDB(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO competencies (competence_key, year, month, first_day) VALUES ('2026-10', 2026, 9, '2026-10-01')`); !db.IsConstraint(err, db.CheckViolation, "competencies_consistency_ck") {
		t.Fatalf("competência inconsistente deveria falhar: %v", err)
	}
	var compID, companyID string
	if err := pool.QueryRow(ctx, `INSERT INTO competencies (competence_key, year, month, first_day) VALUES ('2026-10', 2026, 10, '2026-10-01') RETURNING id`).Scan(&compID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO competencies (competence_key, year, month, first_day) VALUES ('2026-10', 2026, 10, '2026-10-01')`); !db.IsConstraint(err, db.UniqueViolation, "") {
		t.Fatalf("competência duplicada deveria falhar: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO companies (internal_code, legal_name, cnpj, service_start_date, payroll_type, created_by, updated_by)
		VALUES ('101', 'Alfa', '00000001000101', '2025-01-01', 'PAYROLL', $1, $1) RETURNING id`, user.AdminID).Scan(&companyID); err != nil {
		t.Fatal(err)
	}
	var periodID string
	if err := pool.QueryRow(ctx, `INSERT INTO company_module_periods (company_id, competence_id, module_code) VALUES ($1, $2, 'FISCAL') RETURNING id`, companyID, compID).Scan(&periodID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO company_module_periods (company_id, competence_id, module_code) VALUES ($1, $2, 'FISCAL')`, companyID, compID); !db.IsConstraint(err, db.UniqueViolation, "company_module_periods_uq") {
		t.Fatalf("período duplicado deveria falhar: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE company_module_periods SET state = 'CLOSED' WHERE id = $1`, periodID); !db.IsConstraint(err, db.CheckViolation, "company_module_periods_closed_ck") {
		t.Fatalf("fechar sem data/usuário deveria falhar: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO company_module_period_events (company_module_period_id, event_type, reason, user_id) VALUES ($1, 'REOPENED', '  ', $2)`, periodID, user.AdminID); !db.IsConstraint(err, db.CheckViolation, "company_module_period_events_reason_ck") {
		t.Fatalf("reabertura sem motivo deveria falhar: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO company_module_periods (company_id, competence_id, module_code) VALUES ($1, $2, 'CREDENTIALS')`, companyID, compID); !db.IsConstraint(err, db.CheckViolation, "") {
		t.Fatalf("Procurações/Certificados não tem período mensal: %v", err)
	}
}
