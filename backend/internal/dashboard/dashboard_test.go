package dashboard_test

import (
	"context"
	"net/http"
	"testing"

	"controle/internal/testutil"
	"controle/internal/testutil/seeded"
)

type metric struct {
	Value     int `json:"value"`
	Drilldown *struct {
		Path  string            `json:"path"`
		Query map[string]string `json:"query"`
	} `json:"drilldown"`
}

type countsBody struct {
	Applicable          metric  `json:"applicable"`
	Done                metric  `json:"done"`
	InProgress          metric  `json:"in_progress"`
	NotStarted          metric  `json:"not_started"`
	Pending             metric  `json:"pending"`
	Overdue             metric  `json:"overdue"`
	OperationalProgress float64 `json:"operational_progress"`
	FinalCompletion     float64 `json:"final_completion"`
}

type dashboardBody struct {
	Competence string `json:"competence"`
	KPIs       struct {
		countsBody
		ReadyToClose metric `json:"ready_to_close"`
		PreviousOpen metric `json:"previous_open"`
	} `json:"kpis"`
	Modules []struct {
		Module string `json:"module"`
		countsBody
		Companies    int    `json:"companies"`
		ReadyToClose metric `json:"ready_to_close"`
		Closed       metric `json:"closed"`
	} `json:"modules"`
	Credentials []struct {
		Type    string `json:"type"`
		Expired metric `json:"expired"`
		DueSoon metric `json:"due_soon"`
		Current metric `json:"current"`
		NoDate  metric `json:"no_date"`
	} `json:"credentials"`
	Responsibles []struct {
		User *struct {
			ID string `json:"id"`
		} `json:"user"`
		Module     string `json:"module"`
		Applicable int    `json:"applicable"`
	} `json:"responsibles"`
}

func server(t *testing.T) *testutil.Server {
	t.Helper()
	return testutil.NewServer(t, seeded.NewDemoDB(t), seeded.DemoClock())
}

func get(t *testing.T, srv *testutil.Server, query string) dashboardBody {
	t.Helper()
	var d dashboardBody
	srv.Get("/api/v1/dashboard?"+query).MustStatus(t, http.StatusOK).JSON(t, &d)
	return d
}

func scalar(t *testing.T, srv *testutil.Server, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := srv.Pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
	return n
}

// Contagens independentes, sem a view, direto das tabelas.
const independent = `
SELECT count(*) FROM obligation_instances i
JOIN obligation_types t ON t.id = i.obligation_type_id
JOIN status_definitions d ON d.status_set_id = t.status_set_id AND d.code = i.status_code
JOIN competencies k ON k.id = i.competence_id
WHERE k.competence_key = $1 AND `

func TestDashboard_KPIsMatchSeed(t *testing.T) {
	srv := server(t)
	d := get(t, srv, "competence=2026-10")
	k := d.KPIs
	const key = "2026-10"
	relevant := "i.applicable AND i.counts_in_progress AND NOT d.excluded_from_denominator"
	want := map[string]int{
		"applicable":  scalar(t, srv, independent+relevant, key),
		"done":        scalar(t, srv, independent+relevant+" AND d.is_final", key),
		"pending":     scalar(t, srv, independent+relevant+" AND d.is_blocking", key),
		"overdue":     scalar(t, srv, independent+"i.applicable AND NOT d.is_final AND i.due_date_effective < '2026-10-01'", key),
		"not_started": scalar(t, srv, independent+relevant+" AND NOT d.counts_as_started AND NOT d.is_final AND NOT d.is_blocking", key),
	}
	got := map[string]int{"applicable": k.Applicable.Value, "done": k.Done.Value, "pending": k.Pending.Value, "overdue": k.Overdue.Value, "not_started": k.NotStarted.Value}
	for name, w := range want {
		if got[name] != w {
			t.Errorf("%s = %d, SQL independente = %d", name, got[name], w)
		}
	}
	if k.Applicable.Value == 0 || k.Done.Value+k.InProgress.Value+k.NotStarted.Value+k.Pending.Value != k.Applicable.Value {
		t.Fatalf("as classes não particionam as aplicáveis: %+v", k.countsBody)
	}
	if k.FinalCompletion != float64(k.Done.Value)/float64(k.Applicable.Value) {
		t.Fatalf("conclusão = %v", k.FinalCompletion)
	}

	// Cards por módulo somam os KPIs.
	var sum countsBody
	for _, m := range d.Modules {
		sum.Applicable.Value += m.Applicable.Value
		sum.Done.Value += m.Done.Value
		sum.Overdue.Value += m.Overdue.Value
		sum.Pending.Value += m.Pending.Value
	}
	if len(d.Modules) != 4 || sum.Applicable.Value != k.Applicable.Value || sum.Done.Value != k.Done.Value ||
		sum.Overdue.Value != k.Overdue.Value || sum.Pending.Value != k.Pending.Value {
		t.Fatalf("cards %+v × KPIs %+v", sum, k.countsBody)
	}

	// Anteriores em aberto e prontas para fechar.
	prev := scalar(t, srv, `SELECT count(DISTINCT p.company_id) FROM company_module_periods p JOIN competencies k ON k.id = p.competence_id
		WHERE k.competence_key < '2026-10' AND p.state = 'OPEN'`)
	if k.PreviousOpen.Value != prev || k.PreviousOpen.Drilldown == nil || k.PreviousOpen.Drilldown.Path != "/backlog" ||
		k.PreviousOpen.Drilldown.Query["before"] != "2026-10" {
		t.Fatalf("anteriores em aberto = %+v (SQL %d)", k.PreviousOpen, prev)
	}
	var ready int
	for _, m := range d.Modules {
		ready += m.ReadyToClose.Value
		if m.ReadyToClose.Drilldown == nil || m.ReadyToClose.Drilldown.Query["ready_to_close"] != "true" {
			t.Fatalf("drill-down de prontas do %s = %+v", m.Module, m.ReadyToClose.Drilldown)
		}
	}
	if k.ReadyToClose.Value > ready {
		t.Fatalf("empresas prontas (%d) > pares empresa×módulo prontos (%d)", k.ReadyToClose.Value, ready)
	}

	// Drill-down dos números: lista filtrada com a classe e a competência.
	if dd := k.Overdue.Drilldown; dd == nil || dd.Path != "/obligations" || dd.Query["state"] != "overdue" || dd.Query["competence"] != "2026-10" {
		t.Fatalf("drill-down de atrasadas = %+v", dd)
	}

	// Responsáveis somam as aplicáveis.
	var byResp int
	for _, r := range d.Responsibles {
		byResp += r.Applicable
	}
	if byResp != k.Applicable.Value {
		t.Fatalf("responsáveis somam %d, KPIs %d", byResp, k.Applicable.Value)
	}

	// Credenciais: cada empresa com o módulo aparece numa situação por tipo.
	withModule := scalar(t, srv, `SELECT count(*) FROM (SELECT DISTINCT company_id FROM company_module_settings
		WHERE module_code = 'CREDENTIALS' AND enabled) x`)
	if len(d.Credentials) != 2 {
		t.Fatalf("cards de credenciais = %d", len(d.Credentials))
	}
	for _, c := range d.Credentials {
		if n := c.Expired.Value + c.DueSoon.Value + c.Current.Value + c.NoDate.Value; n == 0 || n > withModule {
			t.Fatalf("%s: %d itens (empresas com o módulo: %d)", c.Type, n, withModule)
		}
	}
}

func TestDashboard_Filters(t *testing.T) {
	srv := server(t)
	all := get(t, srv, "competence=2026-10")
	simples := get(t, srv, "competence=2026-10&regime_group=SIMPLES")
	normal := get(t, srv, "competence=2026-10&regime_group=normal")
	if simples.KPIs.Applicable.Value+normal.KPIs.Applicable.Value != all.KPIs.Applicable.Value || simples.KPIs.Applicable.Value == 0 {
		t.Fatalf("Simples %d + Normais %d ≠ total %d", simples.KPIs.Applicable.Value, normal.KPIs.Applicable.Value, all.KPIs.Applicable.Value)
	}
	if dd := simples.Modules[0].ReadyToClose.Drilldown; dd.Path != "/fiscal" || dd.Query["group"] != "simples" {
		t.Fatalf("drill-down do Fiscal no Simples = %+v", dd)
	}
	presumido := get(t, srv, "competence=2026-10&tax_regime=LUCRO_PRESUMIDO")
	if presumido.KPIs.Applicable.Value == 0 || presumido.KPIs.Applicable.Value >= normal.KPIs.Applicable.Value {
		t.Fatalf("Presumido = %d, Normais = %d", presumido.KPIs.Applicable.Value, normal.KPIs.Applicable.Value)
	}
	var someone string
	for _, r := range all.Responsibles {
		if r.User != nil {
			someone = r.User.ID
			break
		}
	}
	one := get(t, srv, "competence=2026-10&responsible_id="+someone)
	var own int
	for _, r := range all.Responsibles {
		if r.User != nil && r.User.ID == someone {
			own += r.Applicable
		}
	}
	if one.KPIs.Applicable.Value != own {
		t.Fatalf("filtro por responsável = %d, esperado %d", one.KPIs.Applicable.Value, own)
	}

	srv.Get("/api/v1/dashboard?competence=2027-01").MustStatus(t, http.StatusNotFound)
	srv.Get("/api/v1/dashboard?competence=2026-10&regime_group=MEI").MustStatus(t, http.StatusBadRequest)
	srv.Get("/api/v1/dashboard").MustStatus(t, http.StatusBadRequest)
}
