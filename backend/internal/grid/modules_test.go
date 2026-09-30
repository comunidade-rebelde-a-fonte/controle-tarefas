package grid_test

import (
	"context"
	"net/http"
	"slices"
	"testing"

	"controle/internal/testutil"
)

type moduleRow struct {
	Company struct {
		ID           string `json:"id"`
		InternalCode string `json:"internal_code"`
	} `json:"company"`
	Snapshot struct {
		RegimeGroup string `json:"regime_group"`
		PayrollType string `json:"payroll_type"`
		FactorR     *bool  `json:"factor_r"`
	} `json:"snapshot"`
	Period struct {
		ID    string `json:"id"`
		State string `json:"state"`
	} `json:"period"`
	Progress struct {
		Denominator int `json:"denominator"`
	} `json:"progress"`
	ReadyToClose bool `json:"ready_to_close"`
	Dependencies *struct {
		Ready   bool `json:"ready"`
		Pending []struct {
			Code string `json:"code"`
		} `json:"pending"`
	} `json:"dependencies"`
	Cells map[string]struct {
		ID        string `json:"id"`
		Locked    bool   `json:"locked"`
		BlockedBy []struct {
			Code string `json:"code"`
		} `json:"blocked_by"`
	} `json:"cells"`
}

type moduleGrid struct {
	gridBody
	PeriodModule string      `json:"period_module"`
	Closable     bool        `json:"closable"`
	Rows         []moduleRow `json:"rows"`
}

func fetchModule(t *testing.T, srv *testutil.Server, module string) moduleGrid {
	t.Helper()
	var g moduleGrid
	srv.Get("/api/v1/modules/"+module+"/grid?competence=2026-10").MustStatus(t, http.StatusOK).JSON(t, &g)
	return g
}

func (g moduleGrid) row(code string) int {
	return slices.IndexFunc(g.Rows, func(r moduleRow) bool { return r.Company.InternalCode == code })
}

func TestDeclarationsGrid_CatalogColumns(t *testing.T) {
	srv := server(t)
	g := fetchModule(t, srv, "declarations")
	want := []string{"REINF", "EFD_CONTRIBUICOES", "SPED_FISCAL", "SINTEGRA", "MIT", "DCTFWEB", "PGDAS", "DESTDA"}
	if got := g.columnCodes(); !slices.Equal(got, want) || g.Module != "DECLARATIONS" || g.PeriodModule != "DECLARATIONS" || !g.Closable || g.Group != nil {
		t.Fatalf("grade = %s/%s closable=%v colunas %v", g.Module, g.PeriodModule, g.Closable, got)
	}
	normal, simples := g.Rows[g.row("101")], g.Rows[g.row("201")]
	if _, ok := normal.Cells["EFD_CONTRIBUICOES"]; !ok {
		t.Fatal("Normal deveria ter EFD Contribuições")
	}
	if _, ok := normal.Cells["PGDAS"]; ok {
		t.Fatal("Normal não tem PGDAS")
	}
	if _, ok := simples.Cells["PGDAS"]; !ok {
		t.Fatal("Simples deveria ter PGDAS")
	}
	// O agregado da linha cobre as obrigações do módulo, inclusive as só exibidas na DCTFWeb
	// (REINF Retidos/Lucros), como o fechamento e o RowSummary (plan R1).
	var inModule int
	if err := srv.Pool.QueryRow(context.Background(), `SELECT count(*) FROM v_obligation_instances v JOIN companies c ON c.id = v.company_id
		WHERE c.internal_code = '101' AND v.primary_module = 'DECLARATIONS' AND v.in_denominator`).Scan(&inModule); err != nil {
		t.Fatal(err)
	}
	if normal.Progress.Denominator != inModule || inModule <= len(normal.Cells) {
		t.Fatalf("denominador = %d, instâncias do módulo = %d, células = %d", normal.Progress.Denominator, inModule, len(normal.Cells))
	}
}

func TestPayrollGrid_FactorROnlySimples(t *testing.T) {
	srv := server(t)
	g := fetchModule(t, srv, "payroll")
	if got := g.columnCodes(); !slices.Equal(got, []string{"PAYROLL_PROCESSING", "ESOCIAL", "FGTS", "INSS_IRRF"}) {
		t.Fatalf("colunas = %v", got)
	}
	var withFactor int
	for _, r := range g.Rows {
		if r.Snapshot.RegimeGroup != "SIMPLES" && r.Snapshot.FactorR != nil {
			t.Fatalf("Fator R exposto para %s (Normal)", r.Company.InternalCode)
		}
		if r.Snapshot.FactorR != nil {
			withFactor++
		}
		if r.Snapshot.PayrollType == "" {
			t.Fatalf("tipo de folha ausente para %s", r.Company.InternalCode)
		}
	}
	if withFactor == 0 {
		t.Fatal("deveria haver empresas do Simples com Fator R")
	}
}

func TestAccountingGrid_SequenceAndBlockedBy(t *testing.T) {
	srv := server(t)
	g := fetchModule(t, srv, "accounting")
	want := []string{"ACCOUNTING_MOVEMENT_RECEIVED", "ACCOUNTING_PAYROLL_POSTED", "ACCOUNTING_TAXES_POSTED",
		"ACCOUNTING_PROCESSED", "ACCOUNTING_RECONCILIATION", "ACCOUNTING_TRIAL_BALANCE_CLOSED"}
	if got := g.columnCodes(); !slices.Equal(got, want) {
		t.Fatalf("colunas = %v", got)
	}
	r := g.Rows[g.row("101")]
	if len(r.Cells["ACCOUNTING_MOVEMENT_RECEIVED"].BlockedBy) != 0 {
		t.Fatal("a primeira etapa não tem pré-requisito")
	}
	if b := r.Cells["ACCOUNTING_RECONCILIATION"].BlockedBy; len(b) != 4 || b[0].Code != "ACCOUNTING_MOVEMENT_RECEIVED" || b[3].Code != "ACCOUNTING_PROCESSED" {
		t.Fatalf("blocked_by da Conciliação = %+v", b)
	}
}

func TestDctfwebGrid_ViewWithoutClosing(t *testing.T) {
	srv := server(t)
	g := fetchModule(t, srv, "dctfweb")
	if got := g.columnCodes(); !slices.Equal(got, []string{"MIT", "REINF_RETIDOS", "REINF_LUCROS", "ESOCIAL", "DCTFWEB"}) {
		t.Fatalf("colunas = %v", got)
	}
	// Sem fechamento próprio (plan R1).
	if g.Closable || g.PeriodModule != "DECLARATIONS" {
		t.Fatalf("closable=%v period_module=%s", g.Closable, g.PeriodModule)
	}
	i := g.row("101")
	r := g.Rows[i]
	if r.ReadyToClose || r.Dependencies == nil || r.Dependencies.Ready || len(r.Dependencies.Pending) != 4 {
		t.Fatalf("linha 101 = ready %v dependências %+v", r.ReadyToClose, r.Dependencies)
	}

	// Componentes concluídos → apta.
	var ids []string
	for _, code := range []string{"MIT", "REINF_RETIDOS", "REINF_LUCROS", "ESOCIAL"} {
		ids = append(ids, r.Cells[code].ID)
	}
	srv.Post("/api/v1/obligation-instances/bulk-status", map[string]any{"instance_ids": ids[:3], "status_code": "TRANSMITTED"}).MustStatus(t, http.StatusOK)
	srv.Post("/api/v1/obligation-instances/bulk-status", map[string]any{"instance_ids": ids[3:], "status_code": "CLOSED"}).MustStatus(t, http.StatusOK)
	g = fetchModule(t, srv, "dctfweb")
	if d := g.Rows[g.row("101")].Dependencies; d == nil || !d.Ready || len(d.Pending) != 0 {
		t.Fatalf("dependências após concluir = %+v", d)
	}

	// Cada célula segue o período do seu módulo: Folha fechada trava só o eSocial.
	pay := fetchModule(t, srv, "payroll")
	period := pay.Rows[pay.row("101")].Period.ID
	if _, err := srv.Pool.Exec(context.Background(), `UPDATE company_module_periods SET state = 'CLOSED', closed_at = now(),
		closed_by = '00000000-0000-0000-0000-000000000001' WHERE id = $1`, period); err != nil {
		t.Fatal(err)
	}
	g = fetchModule(t, srv, "dctfweb")
	r = g.Rows[g.row("101")]
	if !r.Cells["ESOCIAL"].Locked || r.Cells["MIT"].Locked || r.Period.State != "OPEN" {
		t.Fatalf("locked eSocial=%v MIT=%v período=%s", r.Cells["ESOCIAL"].Locked, r.Cells["MIT"].Locked, r.Period.State)
	}
}
