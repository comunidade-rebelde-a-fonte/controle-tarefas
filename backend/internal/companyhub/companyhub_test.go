package companyhub_test

import (
	"context"
	"net/http"
	"testing"

	"controle/internal/testutil"
	"controle/internal/testutil/seeded"
)

// TEST-025 [REQ-002, REQ-036, AC-002, AC-034]: a ficha da empresa como hub. A Visão geral e os
// indicadores da listagem saem das mesmas fontes de cada módulo e são sempre da mesma empresa
// (companies.id); nenhum módulo tem cadastro paralelo de empresa.

var ctx = context.Background()

type link struct {
	Path  string            `json:"path"`
	Query map[string]string `json:"query"`
}

type overviewBody struct {
	Company struct {
		ID           string `json:"id"`
		InternalCode string `json:"internal_code"`
	} `json:"company"`
	Competence          string `json:"competence"`
	CompetenceGenerated bool   `json:"competence_generated"`
	Modules             []struct {
		Module     string `json:"module"`
		Applicable int    `json:"applicable"`
		Done       int    `json:"done"`
		Overdue    int    `json:"overdue"`
		Drilldown  *link  `json:"drilldown"`
	} `json:"modules"`
	NextDue []struct {
		InstanceID string `json:"instance_id"`
		DueDate    string `json:"due_date"`
		Overdue    bool   `json:"overdue"`
	} `json:"next_due"`
	FactorR *struct {
		Light      *string `json:"semaforo"`
		FactorR    *string `json:"fator_r"`
		Savings12m *string `json:"economia_12m"`
		Trend      []struct {
			PA string `json:"pa"`
		} `json:"trend"`
	} `json:"factor_r"`
	Reconciliation *struct {
		Company struct {
			ID string `json:"id"`
		} `json:"company"`
	} `json:"reconciliation"`
	Attention []struct {
		Key    string `json:"key"`
		Count  int    `json:"count"`
		Sample []struct {
			Company struct {
				ID string `json:"id"`
			} `json:"company"`
		} `json:"sample"`
		Drilldown *link `json:"drilldown"`
	} `json:"attention"`
}

func count(t *testing.T, srv *testutil.Server, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := srv.Pool.QueryRow(ctx, sql, args...).Scan(&n); err != nil {
		t.Fatalf("%v\n%s", err, sql)
	}
	return n
}

func scalar(t *testing.T, srv *testutil.Server, sql string, args ...any) string {
	t.Helper()
	var v string
	if err := srv.Pool.QueryRow(ctx, sql, args...).Scan(&v); err != nil {
		t.Fatalf("%v\n%s", err, sql)
	}
	return v
}

func get(t *testing.T, srv *testutil.Server, path string, v any) {
	t.Helper()
	srv.Get(path).MustStatus(t, http.StatusOK).JSON(t, v)
}

// AC-002: toda coluna company_id referencia companies(id), direto ou por chave composta
// (id, company_id) para uma tabela-mãe que referencia (o filho tem a empresa da mãe), e nenhuma
// outra tabela guarda um cadastro de empresa (CNPJ com razão social).
func TestTEST025_OneCanonicalCompany(t *testing.T) {
	pool := testutil.NewDB(t)
	rows, err := pool.Query(ctx, `
		SELECT r.relname::text, p.relname::text, pa.attname::text
		FROM pg_constraint k
		JOIN pg_class r ON r.oid = k.conrelid
		JOIN pg_class p ON p.oid = k.confrelid
		CROSS JOIN LATERAL unnest(k.conkey, k.confkey) AS u(ck, pk)
		JOIN pg_attribute ca ON ca.attrelid = k.conrelid AND ca.attnum = u.ck
		JOIN pg_attribute pa ON pa.attrelid = k.confrelid AND pa.attnum = u.pk
		WHERE k.contype = 'f' AND ca.attname = 'company_id' AND r.relnamespace = 'public'::regnamespace`)
	if err != nil {
		t.Fatal(err)
	}
	type edge struct{ child, parent, column string }
	var edges []edge
	for rows.Next() {
		var e edge
		if err := rows.Scan(&e.child, &e.parent, &e.column); err != nil {
			t.Fatal(err)
		}
		edges = append(edges, e)
	}
	rows.Close()
	// As tabelas cuja company_id chega a companies(id), direto ou pela mãe (ponto fixo).
	anchored := map[string]bool{}
	for changed := true; changed; {
		changed = false
		for _, e := range edges {
			ok := (e.parent == "companies" && e.column == "id") || (anchored[e.parent] && e.column == "company_id")
			if ok && !anchored[e.child] {
				anchored[e.child], changed = true, true
			}
		}
	}
	tables, err := pool.Query(ctx, `
		SELECT c.table_name::text FROM information_schema.columns c
		JOIN information_schema.tables t ON t.table_schema = c.table_schema AND t.table_name = c.table_name AND t.table_type = 'BASE TABLE'
		WHERE c.table_schema = 'public' AND c.column_name = 'company_id' ORDER BY 1`)
	if err != nil {
		t.Fatal(err)
	}
	var loose []string
	n := 0
	for tables.Next() {
		var name string
		if err := tables.Scan(&name); err != nil {
			t.Fatal(err)
		}
		n++
		if !anchored[name] {
			loose = append(loose, name)
		}
	}
	tables.Close()
	if len(loose) > 0 || n < 10 {
		t.Errorf("company_id sem chegar a companies(id): %v (de %d tabelas)", loose, n)
	}
	var parallel int
	if err := pool.QueryRow(ctx, `
		SELECT count(DISTINCT a.table_name) FROM information_schema.columns a
		JOIN information_schema.columns b ON b.table_schema = a.table_schema AND b.table_name = a.table_name
		JOIN information_schema.tables t ON t.table_schema = a.table_schema AND t.table_name = a.table_name AND t.table_type = 'BASE TABLE'
		WHERE a.table_schema = 'public' AND a.column_name = 'cnpj' AND b.column_name IN ('legal_name', 'razao_social', 'nome_empresarial')
		  AND a.table_name <> 'companies'`).Scan(&parallel); err != nil {
		t.Fatal(err)
	}
	if parallel != 0 {
		t.Errorf("%d tabela(s) com cadastro paralelo de empresa", parallel)
	}
}

func TestTEST025_OverviewIsTheSameCompanyAsTheModules(t *testing.T) {
	srv := testutil.NewServer(t, seeded.NewDemoDB(t), seeded.DemoClock()) // hoje = 01/10/2026
	// Uma empresa da carteira do Fator R (com o Fiscal e as Declarações na competência).
	var portfolio struct {
		Linhas []struct {
			Company struct {
				ID string `json:"id"`
			} `json:"company"`
			Semaforo *string `json:"semaforo"`
		} `json:"linhas"`
	}
	get(t, srv, "/api/v1/factor-r/portfolio?pa=2026-10", &portfolio)
	if len(portfolio.Linhas) == 0 {
		t.Fatal("carteira vazia na demonstração")
	}
	id := portfolio.Linhas[0].Company.ID
	var o overviewBody
	get(t, srv, "/api/v1/companies/"+id+"/overview?competence=2026-10", &o)
	code := scalar(t, srv, `SELECT internal_code FROM companies WHERE id = $1`, id)
	if o.Company.ID != id || o.Company.InternalCode != code || o.Competence != "2026-10" || !o.CompetenceGenerated {
		t.Fatalf("identidade = %+v", o.Company)
	}

	// Módulos: as contagens batem com a lista de obrigações da empresa (o drill-down da ficha).
	if len(o.Modules) == 0 {
		t.Fatal("sem módulos")
	}
	applicable := 0
	for _, m := range o.Modules {
		applicable += m.Applicable
		want := count(t, srv, `SELECT count(*) FROM v_obligation_instances v JOIN competencies k ON k.id = v.competence_id
			WHERE v.company_id = $1 AND k.competence_key = '2026-10' AND v.primary_module = $2 AND v.in_denominator`, id, m.Module)
		if m.Applicable != want {
			t.Errorf("%s aplicáveis = %d × %d", m.Module, m.Applicable, want)
		}
		if m.Drilldown == nil || m.Drilldown.Query["q"] != code || m.Drilldown.Query["competence"] != "2026-10" {
			t.Errorf("%s drill-down = %+v", m.Module, m.Drilldown)
		}
	}
	var list struct {
		Total int `json:"total"`
		Items []struct {
			Company struct {
				ID string `json:"id"`
			} `json:"company"`
		} `json:"items"`
	}
	get(t, srv, "/api/v1/backlog?competence=2026-10&state=applicable&page_size=500&company_id="+id, &list)
	if list.Total != applicable {
		t.Errorf("lista da empresa = %d × módulos %d", list.Total, applicable)
	}
	for _, it := range list.Items {
		if it.Company.ID != id {
			t.Fatalf("a lista da empresa trouxe %s", it.Company.ID)
		}
	}
	srv.Get("/api/v1/backlog?competence=2026-10&company_id=x").MustStatus(t, http.StatusBadRequest)

	// Próximos vencimentos: só da empresa, abertos, do mais urgente para o menos.
	if len(o.NextDue) > 5 {
		t.Errorf("vencimentos = %d", len(o.NextDue))
	}
	for i, d := range o.NextDue {
		if n := count(t, srv, `SELECT count(*) FROM v_obligation_instances WHERE id = $1 AND company_id = $2 AND applicable AND NOT is_final`,
			d.InstanceID, id); n != 1 {
			t.Errorf("vencimento %s não é da empresa ou não está aberto", d.InstanceID)
		}
		if i > 0 && d.DueDate != "" && o.NextDue[i-1].DueDate > d.DueDate {
			t.Errorf("vencimentos fora de ordem: %s antes de %s", o.NextDue[i-1].DueDate, d.DueDate)
		}
		if d.Overdue != (d.DueDate != "" && d.DueDate < "2026-10-01") {
			t.Errorf("atraso de %s = %v", d.DueDate, d.Overdue)
		}
	}

	// Fator R: o mesmo cálculo da aba (e da carteira), com a tendência terminando no PA.
	var calc struct {
		Semaforo   *string `json:"semaforo"`
		FatorR     *string `json:"fator_r"`
		Economia   *string `json:"economia_12m"`
		Monitorada bool    `json:"monitorada"`
	}
	get(t, srv, "/api/v1/companies/"+id+"/factor-r/calc?pa=2026-10", &calc)
	if o.FactorR == nil || !calc.Monitorada || str(o.FactorR.Light) != str(calc.Semaforo) || str(o.FactorR.FactorR) != str(calc.FatorR) ||
		str(o.FactorR.Savings12m) != str(calc.Economia) || str(o.FactorR.Light) != str(portfolio.Linhas[0].Semaforo) {
		t.Fatalf("Fator R = %+v × cálculo %+v", o.FactorR, calc)
	}
	if n := len(o.FactorR.Trend); n == 0 || n > 6 || o.FactorR.Trend[n-1].PA != "2026-10" {
		t.Errorf("tendência = %+v", o.FactorR.Trend)
	}
	if o.Reconciliation != nil {
		t.Errorf("a demonstração não tem Conciliação: %+v", o.Reconciliation)
	}

	// Pendências: as categorias do "Exige atenção" só da empresa, com o "ver todos" na ficha.
	for _, c := range o.Attention {
		if c.Count == 0 {
			t.Errorf("categoria vazia na Visão geral: %s", c.Key)
		}
		for _, s := range c.Sample {
			if s.Company.ID != id {
				t.Errorf("%s com item de outra empresa (%s)", c.Key, s.Company.ID)
			}
		}
		if c.Key == "OVERDUE" {
			want := count(t, srv, `SELECT count(*) FROM v_obligation_instances v JOIN competencies k ON k.id = v.competence_id
				WHERE v.company_id = $1 AND k.competence_key = '2026-10' AND v.applicable AND NOT v.is_final AND v.due_date_effective < '2026-10-01'`, id)
			if c.Count != want || c.Drilldown == nil || c.Drilldown.Path != "/companies/"+id || c.Drilldown.Query["tab"] != "obligations" ||
				c.Drilldown.Query["state"] != "overdue" {
				t.Errorf("atrasadas = %+v (esperadas %d)", c, want)
			}
		}
	}

	// Uma empresa fora da carteira: sem Fator R na Visão geral.
	outside := scalar(t, srv, `SELECT id FROM companies WHERE NOT COALESCE(factor_r, false) ORDER BY internal_code LIMIT 1`)
	var other overviewBody
	get(t, srv, "/api/v1/companies/"+outside+"/overview?competence=2026-10", &other)
	if other.Company.ID != outside || other.FactorR != nil {
		t.Errorf("fora da carteira = %+v", other)
	}

	// Um PA futuro (competência não gerada): só o Fator R e a Conciliação.
	var future overviewBody
	get(t, srv, "/api/v1/companies/"+id+"/overview?competence=2026-12", &future)
	if future.CompetenceGenerated || len(future.Modules) != 0 || len(future.NextDue) != 0 || len(future.Attention) != 0 || future.FactorR == nil {
		t.Errorf("competência não gerada = %+v", future)
	}
	srv.Get("/api/v1/companies/00000000-0000-4000-8000-000000000000/overview?competence=2026-10").MustStatus(t, http.StatusNotFound)
	srv.Get("/api/v1/companies/"+id+"/overview?competence=2026").MustStatus(t, http.StatusBadRequest)
}

func TestTEST025_IndicatorsMatchTheSources(t *testing.T) {
	srv := testutil.NewServer(t, seeded.NewDemoDB(t), seeded.DemoClock())
	var ind struct {
		Items []struct {
			CompanyID string `json:"company_id"`
			FactorR   *struct {
				Light *string `json:"semaforo"`
			} `json:"factor_r"`
			Reconciliation *struct {
				State string `json:"state"`
			} `json:"reconciliation"`
			Overdue            int `json:"overdue"`
			CredentialsExpired int `json:"credentials_expired"`
		} `json:"items"`
	}
	get(t, srv, "/api/v1/company-indicators?competence=2026-10", &ind)
	var portfolio struct {
		Linhas []struct {
			Company struct {
				ID string `json:"id"`
			} `json:"company"`
			Semaforo *string `json:"semaforo"`
		} `json:"linhas"`
	}
	get(t, srv, "/api/v1/factor-r/portfolio?pa=2026-10", &portfolio)
	lights := map[string]*string{}
	for _, l := range portfolio.Linhas {
		lights[l.Company.ID] = l.Semaforo
	}
	monitored, overdue := 0, 0
	for _, it := range ind.Items {
		if it.FactorR != nil {
			monitored++
			if light, ok := lights[it.CompanyID]; !ok || str(light) != str(it.FactorR.Light) {
				t.Errorf("%s: semáforo %s × carteira %s", it.CompanyID, str(it.FactorR.Light), str(light))
			}
		}
		want := count(t, srv, `SELECT count(*) FROM v_obligation_instances v JOIN competencies k ON k.id = v.competence_id
			WHERE v.company_id = $1 AND k.competence_key = '2026-10' AND v.applicable AND NOT v.is_final AND v.due_date_effective < '2026-10-01'`,
			it.CompanyID)
		if it.Overdue != want {
			t.Errorf("%s: atrasadas %d × %d", it.CompanyID, it.Overdue, want)
		}
		overdue += it.Overdue
		if it.FactorR == nil && it.Reconciliation == nil && it.Overdue == 0 && it.CredentialsExpired == 0 {
			t.Errorf("%s sem indicador na lista", it.CompanyID)
		}
	}
	if monitored != len(portfolio.Linhas) || monitored == 0 {
		t.Errorf("monitoradas: %d × carteira %d", monitored, len(portfolio.Linhas))
	}
	all := count(t, srv, `SELECT count(*) FROM v_obligation_instances v JOIN competencies k ON k.id = v.competence_id
		WHERE k.competence_key = '2026-10' AND v.applicable AND NOT v.is_final AND v.due_date_effective < '2026-10-01'`)
	if overdue != all {
		t.Errorf("atrasadas na lista %d × no banco %d", overdue, all)
	}
	srv.Get("/api/v1/company-indicators").MustStatus(t, http.StatusBadRequest)
}

func str(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}
