package dashboard_test

import (
	"context"
	"net/http"
	"slices"
	"testing"

	"controle/internal/testutil"
)

type backlogItem struct {
	Kind           string  `json:"kind"`
	InstanceID     *string `json:"instance_id"`
	Competence     string  `json:"competence"`
	Module         string  `json:"module"`
	StatusCode     *string `json:"status_code"`
	StatusLabel    string  `json:"status_label"`
	DaysOverdue    int     `json:"days_overdue"`
	ObligationType *struct {
		Code string `json:"code"`
	} `json:"obligation_type"`
	Company struct {
		InternalCode string `json:"internal_code"`
	} `json:"company"`
	Drilldown struct {
		Path  string            `json:"path"`
		Query map[string]string `json:"query"`
	} `json:"drilldown"`
}

type backlogBody struct {
	Items    []backlogItem `json:"items"`
	Total    int           `json:"total"`
	Page     int           `json:"page"`
	PageSize int           `json:"page_size"`
}

func backlog(t *testing.T, srv *testutil.Server, query string) backlogBody {
	t.Helper()
	var b backlogBody
	srv.Get("/api/v1/backlog?"+query).MustStatus(t, http.StatusOK).JSON(t, &b)
	return b
}

func TestCA021_BacklogShowsPreviousOpen(t *testing.T) {
	srv := server(t)
	ctx := context.Background()
	// Uma obrigação de 09/2026 ainda aberta (aplicável, não final).
	var id, code string
	if err := srv.Pool.QueryRow(ctx, `SELECT v.id, c.internal_code FROM v_obligation_instances v
		JOIN competencies k ON k.id = v.competence_id JOIN companies c ON c.id = v.company_id
		WHERE k.competence_key = '2026-09' AND v.applicable AND NOT v.is_final ORDER BY c.internal_code, v.type_sort_order LIMIT 1`).Scan(&id, &code); err != nil {
		t.Fatal(err)
	}
	b := backlog(t, srv, "before=2026-10&q="+code+"&page_size=500")
	i := slices.IndexFunc(b.Items, func(it backlogItem) bool {
		return it.InstanceID != nil && *it.InstanceID == id
	})
	if i < 0 {
		t.Fatalf("obrigação aberta de 09/2026 fora do backlog de 10/2026 [CA-021] (%d itens)", len(b.Items))
	}
	if it := b.Items[i]; it.Competence != "2026-09" || it.Kind != "OBLIGATION" || it.Drilldown.Query["competence"] != "2026-09" || it.Drilldown.Query["q"] != code {
		t.Fatalf("item = %+v", it)
	}
	// Nada da competência atual no backlog.
	for _, it := range backlog(t, srv, "before=2026-10&page_size=500").Items {
		if it.Competence >= "2026-10" {
			t.Fatalf("item de %s no backlog anterior a 10/2026", it.Competence)
		}
	}
}

func TestBacklog_ModuleItemsFiltersSortAndPaging(t *testing.T) {
	srv := server(t)
	ctx := context.Background()
	all := backlog(t, srv, "before=2026-10&page_size=500")
	if all.Total == 0 || all.Total != len(all.Items) || all.Page != 1 {
		t.Fatalf("backlog = %d itens, total %d", len(all.Items), all.Total)
	}
	// Ordenado pelos dias em atraso (maior primeiro).
	for i := 1; i < len(all.Items); i++ {
		if all.Items[i].DaysOverdue > all.Items[i-1].DaysOverdue {
			t.Fatalf("ordem por atraso quebrada em %d", i)
		}
	}
	// Módulo aberto sem obrigação pendente vira item MODULE.
	var modules int
	if err := srv.Pool.QueryRow(ctx, `SELECT count(*) FROM company_module_periods p JOIN competencies k ON k.id = p.competence_id
		WHERE k.competence_key < '2026-10' AND p.state = 'OPEN' AND NOT EXISTS (SELECT 1 FROM v_obligation_instances o
		WHERE o.company_id = p.company_id AND o.competence_id = p.competence_id AND o.primary_module = p.module_code
		AND o.applicable AND NOT o.is_final)`).Scan(&modules); err != nil {
		t.Fatal(err)
	}
	var gotModules int
	for _, it := range all.Items {
		if it.Kind == "MODULE" {
			gotModules++
			if it.InstanceID != nil || it.ObligationType != nil || it.StatusLabel != "Módulo aberto" {
				t.Fatalf("item de módulo = %+v", it)
			}
		}
	}
	if gotModules != modules {
		t.Fatalf("itens de módulo = %d, SQL = %d", gotModules, modules)
	}

	// Filtros: módulo, CNPJ com máscara, faixa de atraso, paginação.
	for _, it := range backlog(t, srv, "before=2026-10&module=accounting&page_size=500").Items {
		if it.Module != "ACCOUNTING" {
			t.Fatalf("filtro de módulo devolveu %s", it.Module)
		}
	}
	for _, it := range backlog(t, srv, "before=2026-10&q=00.000.001/0001-01&page_size=500").Items {
		if it.Company.InternalCode != "101" {
			t.Fatalf("busca por CNPJ devolveu %s", it.Company.InternalCode)
		}
	}
	for _, it := range backlog(t, srv, "before=2026-10&overdue_min=10&overdue_max=40&page_size=500").Items {
		if it.DaysOverdue < 10 || it.DaysOverdue > 40 {
			t.Fatalf("faixa de atraso devolveu %d", it.DaysOverdue)
		}
	}
	page2 := backlog(t, srv, "before=2026-10&page=2&page_size=5")
	if page2.Total != all.Total || len(page2.Items) != min(5, max(0, all.Total-5)) || page2.Page != 2 {
		t.Fatalf("página 2 = %d itens, total %d", len(page2.Items), page2.Total)
	}
}

func TestList_MatchesDashboardKPIs(t *testing.T) {
	srv := server(t)
	d := get(t, srv, "competence=2026-10")
	for state, want := range map[string]int{
		"applicable": d.KPIs.Applicable.Value, "done": d.KPIs.Done.Value, "in_progress": d.KPIs.InProgress.Value,
		"not_started": d.KPIs.NotStarted.Value, "pending": d.KPIs.Pending.Value, "overdue": d.KPIs.Overdue.Value,
	} {
		if got := backlog(t, srv, "competence=2026-10&state="+state+"&page_size=1").Total; got != want {
			t.Errorf("lista %s = %d, KPI = %d", state, got, want)
		}
	}
	// Com filtro, o drill-down do card do módulo também bate.
	simples := get(t, srv, "competence=2026-10&regime_group=SIMPLES")
	fiscal := simples.Modules[0]
	if got := backlog(t, srv, "competence=2026-10&state=done&module=FISCAL&regime_group=SIMPLES&page_size=1").Total; got != fiscal.Done.Value {
		t.Fatalf("Fiscal Simples concluídas: lista %d, card %d", got, fiscal.Done.Value)
	}

	srv.Get("/api/v1/backlog").MustStatus(t, http.StatusBadRequest)
	srv.Get("/api/v1/backlog?before=2026-10&competence=2026-10").MustStatus(t, http.StatusBadRequest)
	srv.Get("/api/v1/backlog?before=2026-10&state=x").MustStatus(t, http.StatusBadRequest)
	srv.Get("/api/v1/backlog?before=2026-10&page_size=900").MustStatus(t, http.StatusBadRequest)
}
