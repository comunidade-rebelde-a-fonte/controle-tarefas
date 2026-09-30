package company

import (
	"context"
	"testing"

	"controle/internal/platform/db"
	"controle/internal/testutil"
	"controle/internal/user"
)

const insertCompany = `
INSERT INTO companies (internal_code, legal_name, cnpj, service_start_date, payroll_type, created_by, updated_by)
VALUES ($1, $2, $3, '2025-01-01', 'PAYROLL', $4, $4) RETURNING id`

func TestSchema_CompanyUniqueCnpjAndCode(t *testing.T) {
	pool := testutil.NewDB(t)
	ctx := context.Background()
	var id string
	if err := pool.QueryRow(ctx, insertCompany, "101", "Alfa", "00000001000101", user.AdminID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	_, err := pool.Exec(ctx, insertCompany, "999", "Outra", "00000001000101", user.AdminID)
	if !db.IsConstraint(err, db.UniqueViolation, "companies_cnpj_uq") {
		t.Fatalf("CNPJ duplicado deveria falhar em companies_cnpj_uq: %v", err)
	}
	_, err = pool.Exec(ctx, insertCompany, "101", "Outra", "00000002000102", user.AdminID)
	if !db.IsConstraint(err, db.UniqueViolation, "companies_internal_code_uq") {
		t.Fatalf("código duplicado deveria falhar em companies_internal_code_uq: %v", err)
	}
	_, err = pool.Exec(ctx, insertCompany, "102", "Beta", "0000000200010A", user.AdminID)
	if !db.IsConstraint(err, db.CheckViolation, "") {
		t.Fatalf("CNPJ com letra deveria falhar no CHECK de 14 dígitos: %v", err)
	}
}

func TestSchema_RegimeIntervalsCannotOverlap(t *testing.T) {
	pool := testutil.NewDB(t)
	ctx := context.Background()
	var id string
	if err := pool.QueryRow(ctx, insertCompany, "205", "Estação", "00100005000105", user.AdminID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	ins := `INSERT INTO company_tax_regime_history (company_id, tax_regime, effective_from_competence, effective_to_competence, created_by)
	        VALUES ($1, $2, $3, $4, $5)`
	if _, err := pool.Exec(ctx, ins, id, "SIMPLES_NACIONAL", "2025-01-01", "2026-06-01", user.AdminID); err != nil {
		t.Fatal(err)
	}
	// Vigência adjacente (07/2026 em diante) é permitida.
	if _, err := pool.Exec(ctx, ins, id, "LUCRO_PRESUMIDO", "2026-07-01", nil, user.AdminID); err != nil {
		t.Fatalf("vigência adjacente deveria ser aceita: %v", err)
	}
	// Sobreposição (06/2026 já coberta) é rejeitada pelo EXCLUDE.
	_, err := pool.Exec(ctx, ins, id, "LUCRO_REAL", "2026-06-01", "2026-06-01", user.AdminID)
	if !db.IsConstraint(err, db.ExclusionViolation, "company_tax_regime_no_overlap") {
		t.Fatalf("sobreposição deveria falhar: %v", err)
	}
	// Vigência aberta sobreposta a outra aberta também falha.
	_, err = pool.Exec(ctx, ins, id, "LUCRO_REAL", "2027-01-01", nil, user.AdminID)
	if !db.IsConstraint(err, db.ExclusionViolation, "company_tax_regime_no_overlap") {
		t.Fatalf("vigência aberta sobreposta deveria falhar: %v", err)
	}
	// Competência que não é dia 1 é rejeitada.
	_, err = pool.Exec(ctx, ins, id, "LUCRO_REAL", "2030-01-15", nil, user.AdminID)
	if !db.IsConstraint(err, db.CheckViolation, "") {
		t.Fatalf("competência fora do dia 1 deveria falhar: %v", err)
	}
}

func TestSchema_ModuleSettingsCannotOverlap(t *testing.T) {
	pool := testutil.NewDB(t)
	ctx := context.Background()
	var id string
	if err := pool.QueryRow(ctx, insertCompany, "101", "Alfa", "00000001000101", user.AdminID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	ins := `INSERT INTO company_module_settings (company_id, module_code, enabled, effective_from_competence, effective_to_competence, created_by)
	        VALUES ($1, $2, $3, $4, $5, $6)`
	if _, err := pool.Exec(ctx, ins, id, "FISCAL", true, "2025-01-01", nil, user.AdminID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, ins, id, "PAYROLL", true, "2025-01-01", nil, user.AdminID); err != nil {
		t.Fatalf("outro módulo no mesmo período deveria ser aceito: %v", err)
	}
	_, err := pool.Exec(ctx, ins, id, "FISCAL", false, "2026-10-01", nil, user.AdminID)
	if !db.IsConstraint(err, db.ExclusionViolation, "company_module_settings_no_overlap") {
		t.Fatalf("sobreposição de módulo deveria falhar: %v", err)
	}
}
