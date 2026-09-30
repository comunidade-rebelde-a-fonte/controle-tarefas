package grid_test

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"controle/internal/grid"
	comp "controle/internal/platform/competence"
	"controle/internal/testutil"
	"controle/internal/testutil/seeded"
)

// server sobe a API sobre as 20 empresas com a competência 10/2026 gerada (hoje = 01/10/2026).
func server(t *testing.T) *testutil.Server {
	t.Helper()
	pool := seeded.NewCompaniesDB(t)
	srv := testutil.NewServer(t, pool, testutil.FixedClock(t, "2026-10-01T12:00:00Z"))
	srv.Post("/api/v1/competencies/ensure-current", nil).MustStatus(t, http.StatusOK)
	return srv
}

type cellBody struct {
	ID               string `json:"id"`
	StatusCode       string `json:"status_code"`
	Applicable       bool   `json:"applicable"`
	CountsInProgress bool   `json:"counts_in_progress"`
	IsOverdue        bool   `json:"is_overdue"`
	Version          int    `json:"version"`
	ObligationType   struct {
		Code string `json:"code"`
	} `json:"obligation_type"`
	BlockedBy        []any `json:"blocked_by"`
	DependencyStatus any   `json:"dependency_status"`
}

type gridBody struct {
	Module     string  `json:"module"`
	Competence string  `json:"competence"`
	Group      *string `json:"group"`
	Today      string  `json:"today"`
	Columns    []struct {
		ObligationType struct {
			ID   string `json:"id"`
			Code string `json:"code"`
			Name string `json:"name"`
		} `json:"obligation_type"`
		IsClosingStage bool `json:"is_closing_stage"`
		Statuses       []struct {
			Code  string `json:"code"`
			Label string `json:"label"`
		} `json:"statuses"`
	} `json:"columns"`
	Rows []struct {
		Company struct {
			ID           string `json:"id"`
			InternalCode string `json:"internal_code"`
			Cnpj         string `json:"cnpj"`
		} `json:"company"`
		Snapshot struct {
			TaxRegime   string `json:"tax_regime"`
			RegimeGroup string `json:"regime_group"`
			Responsible *struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"responsible"`
		} `json:"snapshot"`
		Period struct {
			ID          string  `json:"id"`
			State       string  `json:"state"`
			GeneralNote *string `json:"general_note"`
			Version     int     `json:"version"`
		} `json:"period"`
		Progress struct {
			Denominator     int     `json:"denominator"`
			FinalDone       int     `json:"final_done"`
			OperationalPct  float64 `json:"operational_progress"`
			FinalCompletion float64 `json:"final_completion"`
			ReadyToClose    bool    `json:"ready_to_close"`
		} `json:"progress"`
		Overdue         int                 `json:"overdue"`
		NearestDeadline *string             `json:"nearest_deadline"`
		ReadyToClose    bool                `json:"ready_to_close"`
		Cells           map[string]cellBody `json:"cells"`
	} `json:"rows"`
	Total int `json:"total"`
}

func (g gridBody) codes() []string {
	var out []string
	for _, r := range g.Rows {
		out = append(out, r.Company.InternalCode)
	}
	slices.Sort(out)
	return out
}

func (g gridBody) columnCodes() []string {
	var out []string
	for _, c := range g.Columns {
		out = append(out, c.ObligationType.Code)
	}
	return out
}

func fetch(t *testing.T, srv *testutil.Server, query string) gridBody {
	t.Helper()
	var g gridBody
	srv.Get("/api/v1/modules/fiscal/grid?competence=2026-10&"+query).MustStatus(t, http.StatusOK).JSON(t, &g)
	return g
}

func TestFiscalGrid_SeedGroups(t *testing.T) {
	srv := server(t)

	normal := fetch(t, srv, "group=normal")
	if normal.Module != "FISCAL" || normal.Competence != "2026-10" || normal.Group == nil || *normal.Group != "normal" || normal.Today != "2026-10-01" {
		t.Fatalf("cabeçalho = %+v", normal)
	}
	if len(normal.Rows) != 10 || normal.Total != 10 {
		t.Fatalf("Normais: %d linhas (%v)", len(normal.Rows), normal.codes())
	}
	wantNormal := []string{"FISCAL_FECHAMENTO", "FISCAL_ENTRADAS", "FISCAL_SAIDAS", "FISCAL_PRESTADOS", "FISCAL_TOMADOS", "FISCAL_PIS",
		"FISCAL_COFINS", "FISCAL_IRPJ", "FISCAL_CSLL", "FISCAL_ISSQN", "FISCAL_ICMS", "FISCAL_IPI", "FISCAL_RETIDOS"}
	if got := normal.columnCodes(); !slices.Equal(got, wantNormal) {
		t.Fatalf("colunas Normais = %v", got)
	}
	if !normal.Columns[0].IsClosingStage || len(normal.Columns[0].Statuses) != 7 || normal.Columns[0].Statuses[6].Label != "Encerrado" {
		t.Fatalf("coluna Fechamento = %+v", normal.Columns[0])
	}

	simples := fetch(t, srv, "group=SIMPLES")
	if len(simples.Rows) != 10 || !slices.Contains(simples.codes(), "205") {
		t.Fatalf("Simples: %v", simples.codes())
	}
	// Simples: 8 colunas de controle + Fechamento [PRD §14.1].
	wantSimples := []string{"FISCAL_FECHAMENTO", "FISCAL_ENTRADAS", "FISCAL_SAIDAS", "FISCAL_PRESTADOS", "FISCAL_TOMADOS",
		"FISCAL_SIMPLES_NACIONAL", "FISCAL_ISSQN", "FISCAL_ICMS", "FISCAL_RETIDOS"}
	if got := simples.columnCodes(); !slices.Equal(got, wantSimples) {
		t.Fatalf("colunas Simples = %v", got)
	}

	for _, r := range simples.Rows {
		if r.Snapshot.RegimeGroup != "SIMPLES" || len(r.Cells) != 9 || r.Period.State != "OPEN" || r.Period.Version != 1 {
			t.Fatalf("linha Simples %s = %+v", r.Company.InternalCode, r)
		}
		// ICMS visível e fora do progresso; Fechamento fora dos percentuais (RN-010, R3).
		if icms := r.Cells["FISCAL_ICMS"]; icms.ID == "" || icms.CountsInProgress {
			t.Fatalf("ICMS da %s = %+v", r.Company.InternalCode, icms)
		}
		if r.Progress.Denominator > 7 || r.ReadyToClose || r.NearestDeadline == nil {
			t.Fatalf("agregado da %s = %+v", r.Company.InternalCode, r)
		}
		if c := r.Cells["FISCAL_ENTRADAS"]; c.Version != 1 || c.BlockedBy == nil || c.DependencyStatus != nil || c.ObligationType.Code != "FISCAL_ENTRADAS" {
			t.Fatalf("célula = %+v", c)
		}
	}
	for _, r := range normal.Rows {
		if r.Snapshot.RegimeGroup != "NORMAL" || len(r.Cells) != 13 || r.Snapshot.Responsible == nil {
			t.Fatalf("linha Normal %s = %+v", r.Company.InternalCode, r)
		}
	}
}

func TestFiscalGrid_Gzip(t *testing.T) {
	srv := server(t)
	req, err := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/modules/fiscal/grid?competence=2026-10&group=normal", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Accept-Encoding", "gzip")
	res, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK || res.Header.Get("Content-Encoding") != "gzip" {
		t.Fatalf("status %d, Content-Encoding %q", res.StatusCode, res.Header.Get("Content-Encoding"))
	}
}

func setStatus(t *testing.T, srv *testutil.Server, company string, types []string, status string) {
	t.Helper()
	var ids []string
	rows, err := srv.Pool.Query(context.Background(), `SELECT i.id FROM obligation_instances i JOIN companies c ON c.id = i.company_id
		JOIN obligation_types t ON t.id = i.obligation_type_id WHERE c.internal_code = $1 AND t.code = ANY($2) AND i.applicable`, company, types)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	rows.Close()
	srv.Post("/api/v1/obligation-instances/bulk-status", map[string]any{"instance_ids": ids, "status_code": status}).MustStatus(t, http.StatusOK)
}

func TestFiscalGrid_FiltersAndDerived(t *testing.T) {
	srv := server(t)
	controls := []string{"FISCAL_ENTRADAS", "FISCAL_SAIDAS", "FISCAL_PRESTADOS", "FISCAL_TOMADOS", "FISCAL_PIS", "FISCAL_COFINS",
		"FISCAL_IRPJ", "FISCAL_CSLL", "FISCAL_ISSQN", "FISCAL_ICMS", "FISCAL_IPI", "FISCAL_RETIDOS"}

	// 101: todos os controles encerrados → pronta para fechar [CA-014].
	setStatus(t, srv, "101", controls, "CLOSED")
	if got := fetch(t, srv, "group=normal&ready_to_close=true").codes(); !slices.Equal(got, []string{"101"}) {
		t.Fatalf("ready_to_close = %v", got)
	}
	g := fetch(t, srv, "group=normal&ready_to_close=true")
	if r := g.Rows[0]; r.Progress.FinalCompletion != 1 || r.Progress.OperationalPct != 1 || r.NearestDeadline == nil {
		t.Fatalf("101 = %+v", r.Progress)
	}

	// 103: Entradas com pendência (Faltando documentos).
	setStatus(t, srv, "103", []string{"FISCAL_ENTRADAS"}, "MISSING_DOCUMENTS")
	if got := fetch(t, srv, "group=normal&pending=true").codes(); !slices.Equal(got, []string{"103"}) {
		t.Fatalf("pending = %v", got)
	}
	if got := fetch(t, srv, "group=normal&status=missing_documents").codes(); !slices.Equal(got, []string{"103"}) {
		t.Fatalf("status = %v", got)
	}
	g = fetch(t, srv, "group=normal")
	var saidas string
	for _, c := range g.Columns {
		if c.ObligationType.Code == "FISCAL_SAIDAS" {
			saidas = c.ObligationType.ID
		}
	}
	if got := fetch(t, srv, "group=normal&status=MISSING_DOCUMENTS&obligation_type_id="+saidas).codes(); len(got) != 0 {
		t.Fatalf("status + coluna = %v", got)
	}
	// Não iniciadas: a 101 não tem mais controles "A iniciar".
	if got := fetch(t, srv, "group=normal&not_started=true").codes(); len(got) != 9 || slices.Contains(got, "101") {
		t.Fatalf("not_started = %v", got)
	}

	// Fecha o Fiscal da 101 (Fechamento → Encerrado no mesmo ato).
	var periodID string
	var version int
	for _, r := range g.Rows {
		if r.Company.InternalCode == "101" {
			periodID, version = r.Period.ID, r.Period.Version
		}
	}
	srv.Post("/api/v1/company-module-periods/"+periodID+"/close", map[string]any{"version": version, "confirm_set_closing": true}).MustStatus(t, http.StatusOK)
	if got := fetch(t, srv, "group=normal&period_state=closed").codes(); !slices.Equal(got, []string{"101"}) {
		t.Fatalf("period_state = %v", got)
	}
	if got := fetch(t, srv, "group=normal&ready_to_close=true").codes(); len(got) != 0 {
		t.Fatalf("módulo fechado não está pronto para fechar: %v", got)
	}
	if got := fetch(t, srv, "group=normal&closing_status=CLOSED").codes(); !slices.Equal(got, []string{"101"}) {
		t.Fatalf("closing_status = %v", got)
	}
	if got := fetch(t, srv, "group=normal&period_state=OPEN").codes(); len(got) != 9 {
		t.Fatalf("abertas = %v", got)
	}

	// Observação geral.
	for _, r := range g.Rows {
		if r.Company.InternalCode == "102" {
			srv.Patch("/api/v1/company-module-periods/"+r.Period.ID+"/note", map[string]any{"version": r.Period.Version, "general_note": "Aguardando retenções"}).
				MustStatus(t, http.StatusOK)
		}
	}
	if got := fetch(t, srv, "group=normal&note=with").codes(); !slices.Equal(got, []string{"102"}) {
		t.Fatalf("note=with = %v", got)
	}
	if got := fetch(t, srv, "group=normal&note=without").codes(); len(got) != 9 {
		t.Fatalf("note=without = %v", got)
	}

	// Regime e responsável do snapshot.
	if got := fetch(t, srv, "group=normal&tax_regime=LUCRO_REAL"); len(got.Rows) == 0 || len(got.Rows) == 10 {
		t.Fatalf("tax_regime = %v", got.codes())
	}
	resp := *g.Rows[0].Snapshot.Responsible
	for _, r := range fetch(t, srv, "group=normal&responsible_id="+resp.ID).Rows {
		if r.Snapshot.Responsible == nil || r.Snapshot.Responsible.ID != resp.ID {
			t.Fatalf("responsible_id devolveu %s", r.Company.InternalCode)
		}
	}

	// Sem atrasos em 01/10; em 01/12 tudo o que não é final está atrasado.
	if got := fetch(t, srv, "group=normal&overdue=true").codes(); len(got) != 0 {
		t.Fatalf("overdue em 01/10 = %v", got)
	}
	srv.Clock.Set(time.Date(2026, 12, 1, 12, 0, 0, 0, time.UTC))
	late := fetch(t, srv, "group=normal&overdue=true")
	if slices.Contains(late.codes(), "101") || len(late.Rows) != 9 {
		t.Fatalf("overdue em 01/12 = %v", late.codes())
	}
	for _, r := range late.Rows {
		if r.Overdue == 0 || !r.Cells["FISCAL_SAIDAS"].IsOverdue {
			t.Fatalf("atraso da %s = %d", r.Company.InternalCode, r.Overdue)
		}
	}
}

func TestFiscalGrid_Validation(t *testing.T) {
	srv := server(t)
	for _, q := range []string{
		"competence=2026-10",                               // sem grupo
		"competence=2026-10&group=mei",                     // grupo inválido
		"group=normal",                                     // sem competência
		"competence=10/2026&group=normal",                  // formato
		"competence=2026-10&group=normal&tax_regime=MEI",   // regime
		"competence=2026-10&group=normal&responsible_id=x", // uuid
		"competence=2026-10&group=normal&period_state=X",
		"competence=2026-10&group=normal&note=talvez",
		"competence=2026-10&group=normal&overdue=sim",
		"competence=2026-10&group=normal&status=a-b",
		"competence=2026-10&group=normal&obligation_type_id=00000000-0000-0000-0000-000000000001",
	} {
		srv.Get("/api/v1/modules/fiscal/grid?"+q).MustStatus(t, http.StatusBadRequest)
	}
	res := srv.Get("/api/v1/modules/fiscal/grid?competence=2027-01&group=normal")
	res.MustStatus(t, http.StatusNotFound)
	if !strings.Contains(string(res.Body), "COMPETENCE_NOT_FOUND") {
		t.Fatalf("404 = %s", res.Body)
	}
}

// queryCounter conta as consultas enviadas ao banco (exceto controle de transação).
type queryCounter struct{ n atomic.Int32 }

func (c *queryCounter) TraceQueryStart(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryStartData) context.Context {
	sql := strings.ToLower(strings.TrimSpace(d.SQL))
	if !strings.HasPrefix(sql, "begin") && !strings.HasPrefix(sql, "rollback") && !strings.HasPrefix(sql, "commit") {
		c.n.Add(1)
	}
	return ctx
}

func (c *queryCounter) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func TestGrid_AtMostThreeQueries(t *testing.T) {
	srv := server(t)
	cfg := srv.Pool.Config()
	counter := &queryCounter{}
	cfg.ConnConfig.Tracer = counter
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	spec, err := grid.FiscalSpec("normal")
	if err != nil {
		t.Fatal(err)
	}
	g, err := grid.NewBuilder(pool, srv.Clock).BuildGrid(context.Background(), spec, comp.MustParse("2026-10"), grid.Filters{})
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Rows) != 10 {
		t.Fatalf("linhas = %d", len(g.Rows))
	}
	if n := counter.n.Load(); n > 3 {
		t.Fatalf("a grade fez %d consultas; o máximo é 3 (sem N+1)", n)
	}
}
