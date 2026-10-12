package dashboard_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"controle/internal/testutil"
	"controle/internal/testutil/seeded"
	"controle/internal/user"
)

// TEST-024 [REQ-017, REQ-035, AC-012]: as seções Fator R e Conciliação do Dashboard batem com as
// fontes (a carteira do motor, os documentos, a fila e a Conciliação por empresa) e com as listas
// do drill-down, com e sem os filtros de empresa (responsável do Fiscal no Fator R e do Contábil
// na Conciliação; decisões do usuário, D-038).

type link struct {
	Path  string            `json:"path"`
	Query map[string]string `json:"query"`
}

type num struct {
	Value     int   `json:"value"`
	Drilldown *link `json:"drilldown"`
}

type sectionsBody struct {
	FactorR struct {
		PA            string `json:"pa"`
		Unavailable   *string
		Monitored     num `json:"monitored"`
		Red           num `json:"red"`
		Yellow        num `json:"yellow"`
		Green         num `json:"green"`
		Insufficient  num `json:"insufficient"`
		SavingsAtPlay struct {
			Value     string `json:"value"`
			Drilldown *link  `json:"drilldown"`
		} `json:"savings_at_play"`
		ActionRequired num `json:"action_required"`
		Priorities     []struct {
			Company struct {
				ID string `json:"id"`
			} `json:"company"`
			Acao      string `json:"acao"`
			Drilldown link   `json:"drilldown"`
		} `json:"priorities"`
	} `json:"factor_r"`
	Reconciliation struct {
		Processing       num `json:"processing"`
		ExtractionReview num `json:"extraction_review"`
		ClosedPeriod     num `json:"closed_period"`
		Queue            num `json:"queue"`
		Divergent        num `json:"divergent"`
		WithoutReceipt   num `json:"without_receipt"`
		WithoutStatement num `json:"without_statement"`
		Ready            num `json:"ready"`
		BlocksAccounting num `json:"blocks_accounting"`
	} `json:"reconciliation"`
}

type portfolioBody struct {
	KPIs struct {
		Monitoradas        int    `json:"monitoradas"`
		NoV                int    `json:"no_v"`
		NoLimite           int    `json:"no_limite"`
		Seguras            int    `json:"seguras"`
		DadosInsuficientes int    `json:"dados_insuficientes"`
		EconomiaEmJogo     string `json:"economia_em_jogo"`
	} `json:"kpis"`
	Linhas []struct {
		Company struct {
			ID string `json:"id"`
		} `json:"company"`
		Acao     string  `json:"acao"`
		Semaforo *string `json:"semaforo"`
	} `json:"linhas"`
}

func sections(t *testing.T, srv *testutil.Server, query string) sectionsBody {
	t.Helper()
	var b sectionsBody
	srv.Get("/api/v1/dashboard?"+query).MustStatus(t, http.StatusOK).JSON(t, &b)
	return b
}

func portfolio(t *testing.T, srv *testutil.Server, query string) portfolioBody {
	t.Helper()
	var p portfolioBody
	srv.Get("/api/v1/factor-r/portfolio?"+query).MustStatus(t, http.StatusOK).JSON(t, &p)
	return p
}

// assertFactorR confere a seção contra a carteira com os mesmos filtros (a lista do drill-down).
func assertFactorR(t *testing.T, srv *testutil.Server, pa, filter string) sectionsBody {
	t.Helper()
	d := sections(t, srv, "competence="+pa+filter)
	p := portfolio(t, srv, "pa="+pa+filter)
	f := d.FactorR
	byLight := map[string]int{}
	var priorities []string
	action := 0
	for _, l := range p.Linhas {
		light := "insuficiente"
		if l.Semaforo != nil {
			light = *l.Semaforo
		}
		byLight[light]++
		if l.Acao != "manter" {
			action++
			if len(priorities) < 5 {
				priorities = append(priorities, l.Company.ID)
			}
		}
	}
	for name, got := range map[string][2]int{
		"monitoradas":  {f.Monitored.Value, p.KPIs.Monitoradas},
		"vermelho":     {f.Red.Value, byLight["vermelho"]},
		"anexo V":      {f.Red.Value, p.KPIs.NoV},
		"amarelo":      {f.Yellow.Value, byLight["amarelo"]},
		"no limite":    {f.Yellow.Value, p.KPIs.NoLimite},
		"verde":        {f.Green.Value, byLight["verde"]},
		"seguras":      {f.Green.Value, p.KPIs.Seguras},
		"insuficiente": {f.Insufficient.Value, byLight["insuficiente"]},
		"dados":        {f.Insufficient.Value, p.KPIs.DadosInsuficientes},
		"ação":         {f.ActionRequired.Value, action},
		"linhas":       {f.Monitored.Value, len(p.Linhas)},
	} {
		if got[0] != got[1] {
			t.Errorf("%s%s %s: Dashboard %d × carteira %d", pa, filter, name, got[0], got[1])
		}
	}
	if f.SavingsAtPlay.Value != p.KPIs.EconomiaEmJogo {
		t.Errorf("%s%s economia em jogo: %s × %s", pa, filter, f.SavingsAtPlay.Value, p.KPIs.EconomiaEmJogo)
	}
	if len(f.Priorities) != len(priorities) {
		t.Fatalf("%s%s prioridades: %d × %d", pa, filter, len(f.Priorities), len(priorities))
	}
	for i, pr := range f.Priorities {
		if pr.Company.ID != priorities[i] || pr.Acao == "manter" ||
			pr.Drilldown.Path != "/companies/"+pr.Company.ID || pr.Drilldown.Query["tab"] != "factor-r" || pr.Drilldown.Query["competence"] != pa {
			t.Errorf("%s%s prioridade %d = %+v (esperada %s)", pa, filter, i, pr, priorities[i])
		}
	}
	// O drill-down leva à carteira do PA com o semáforo e os mesmos filtros.
	for name, c := range map[string]struct {
		got  *link
		want string
	}{
		"monitoradas": {f.Monitored.Drilldown, ""},
		"vermelho":    {f.Red.Drilldown, "vermelho"},
		"amarelo":     {f.Yellow.Drilldown, "amarelo"},
		"verde":       {f.Green.Drilldown, "verde"},
		"sem dados":   {f.Insufficient.Drilldown, "insuficiente"},
		"economia":    {f.SavingsAtPlay.Drilldown, "vermelho,amarelo"},
		"ação":        {f.ActionRequired.Drilldown, "vermelho,amarelo,insuficiente"},
	} {
		if c.got == nil || c.got.Path != "/factor-r" || c.got.Query["pa"] != pa || c.got.Query["semaforo"] != c.want {
			t.Errorf("%s drill-down %s = %+v", pa, name, c.got)
		}
	}
	return d
}

func TestTEST024_FactorRSectionMatchesThePortfolio(t *testing.T) {
	srv := server(t) // a demonstração, com a carteira do Fator R
	d := assertFactorR(t, srv, "2026-10", "")
	if f := d.FactorR; f.Monitored.Value == 0 || f.Red.Value+f.Yellow.Value == 0 || f.ActionRequired.Value == 0 || len(f.Priorities) == 0 {
		t.Fatalf("a demonstração deveria ter empresas em risco: %+v", f)
	}
	assertFactorR(t, srv, "2026-09", "")

	// O filtro de responsável é o do Fiscal (decisão do usuário): duas empresas da carteira com
	// um responsável próprio no Fiscal e uma terceira com ele só no Contábil.
	ctx := context.Background()
	rows, err := srv.Pool.Query(ctx, `SELECT company_id FROM company_competence_snapshots s JOIN competencies k ON k.id = s.competence_id
		WHERE k.competence_key = '2026-10' AND s.factor_r AND s.tax_regime = 'SIMPLES_NACIONAL' ORDER BY s.company_id LIMIT 3`)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	rows.Close()
	if len(ids) < 3 {
		t.Fatalf("carteira pequena demais para o filtro: %v", ids)
	}
	owner := newUser(t, srv.Pool, "Responsável do Fator R")
	setSnapshot(t, srv.Pool, "fiscal_responsible_user_id", owner, "2026-10", ids[:2]...)
	setSnapshot(t, srv.Pool, "accounting_responsible_user_id", owner, "2026-10", ids[2])
	filtered := assertFactorR(t, srv, "2026-10", "&responsible_id="+owner)
	if filtered.FactorR.Monitored.Value != 2 || filtered.FactorR.Monitored.Drilldown.Query["responsible_id"] != owner {
		t.Fatalf("filtro do responsável do Fiscal = %+v", filtered.FactorR.Monitored)
	}
	// Agrupamento: a carteira é só do Simples.
	if n := sections(t, srv, "competence=2026-10&regime_group=NORMAL").FactorR.Monitored.Value; n != 0 {
		t.Fatalf("carteira nas normais = %d", n)
	}
	assertFactorR(t, srv, "2026-10", "&regime_group=SIMPLES")
}

func newUser(t *testing.T, pool *pgxpool.Pool, name string) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(context.Background(), `INSERT INTO users (name, role) VALUES ($1, 'OPERATOR') RETURNING id`, name).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

// setSnapshot põe o responsável no snapshot da competência (o que vale nos filtros do Dashboard).
func setSnapshot(t *testing.T, pool *pgxpool.Pool, column, userID, key string, companies ...string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), fmt.Sprintf(`UPDATE company_competence_snapshots s SET %s = $1 FROM competencies k
		WHERE k.id = s.competence_id AND k.competence_key = $2 AND s.company_id = ANY($3::uuid[])`, column), userID, key, companies); err != nil {
		t.Fatal(err)
	}
}

// reconEnv é o cenário da Conciliação em 10/2026, sobre as empresas da demonstração com a
// competência gerada.
type reconEnv struct {
	t   *testing.T
	srv *testutil.Server
	seq int
}

func (e *reconEnv) scalar(sql string, args ...any) string {
	e.t.Helper()
	var v string
	if err := e.srv.Pool.QueryRow(context.Background(), sql, args...).Scan(&v); err != nil {
		e.t.Fatalf("%v\n%s", err, sql)
	}
	return v
}

func (e *reconEnv) count(sql string, args ...any) int {
	e.t.Helper()
	var n int
	if err := e.srv.Pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		e.t.Fatalf("%v\n%s", err, sql)
	}
	return n
}

// document grava um documento no status dado (competência vazia: sem competência).
func (e *reconEnv) document(company, status, kind, comp string) string {
	e.t.Helper()
	e.seq++
	var c any
	if comp != "" {
		c = comp + "-01"
	}
	return e.scalar(`INSERT INTO reconciliation_documents (company_id, competence, bucket, object_key, sha256, file_name, content_type, kind, status)
		VALUES (NULLIF($1, '')::uuid, $2::date, 'originais', 'originais/' || $3::text, lpad(to_hex($4::int), 64, 'a'), 'doc.pdf', 'application/pdf',
		NULLIF($5, ''), $6)
		RETURNING id`, company, c, fmt.Sprint(e.seq), e.seq, kind, status)
}

// statement importa um extrato de 10/2026 com uma linha por marca ("" sem marca; "conciliada" e
// "classificada" resolvem a linha, a segunda com a conta).
func (e *reconEnv) statement(company string, lines ...string) {
	e.t.Helper()
	doc := e.document(company, "importado", "extrato", "")
	imp := e.scalar(`INSERT INTO statement_imports (company_id, document_id, bank, branch, account, period_start, period_end, opening_balance,
		closing_balance, validation_ok, validation, attempts)
		VALUES ($1, $2, 'beta', '0021', '33412-7', '2026-10-01', '2026-10-31', 0, 0, true, '{}', 0) RETURNING id`, company, doc)
	for _, kind := range lines {
		e.seq++
		flags := "[]"
		if kind == "divergencia" {
			flags = `[{"tipo": "divergencia", "motivo": "valor"}]`
		}
		line := e.scalar(`INSERT INTO statement_transactions (company_id, statement_import_id, position, txn_date, description, amount,
			direction, dedupe_key, flags)
			VALUES ($1, $2, $3::int, '2026-10-05', 'LINHA', 10.00, 'D', lpad(to_hex($3::int), 64, 'f'), $4::jsonb) RETURNING id`,
			company, imp, e.seq, flags)
		if kind == "conciliada" || kind == "classificada" {
			e.scalar(`INSERT INTO reconciliations (company_id, statement_transaction_id, method, origin, created_by)
				VALUES ($1, $2, 'sem_comprovante', 'manual', $3) RETURNING id`, company, line, user.AdminID)
		}
		if kind == "classificada" {
			e.scalar(`UPDATE statement_transactions SET account_code = '1.01', classification_origin = 'manual', classified_by = $2,
				classified_at = now() WHERE id = $1 RETURNING id`, line, user.AdminID)
		}
	}
}

type overviewBody struct {
	Counts struct {
		Total            int `json:"total"`
		InProgress       int `json:"em_andamento"`
		Ready            int `json:"pronta"`
		Completed        int `json:"concluida"`
		BlocksAccounting int `json:"blocks_accounting"`
	} `json:"counts"`
	Items []struct {
		Company struct {
			ID string `json:"id"`
		} `json:"company"`
		State    string `json:"state"`
		Blocking struct {
			MissingStatement bool `json:"missing_statement"`
			Divergent        int  `json:"divergent"`
			WithoutReceipt   int  `json:"without_receipt"`
			Review           int  `json:"review"`
		} `json:"blocking"`
		WithoutStatement   int     `json:"without_statement"`
		UsesReconciliation bool    `json:"uses_reconciliation"`
		AccountingState    *string `json:"accounting_state"`
		Obligation         *struct {
			StatusCode string `json:"status_code"`
			IsFinal    bool   `json:"is_final"`
		} `json:"obligation"`
		BlocksAccounting bool `json:"blocks_accounting"`
	} `json:"items"`
}

func (e *reconEnv) overview(query string) overviewBody {
	e.t.Helper()
	var o overviewBody
	e.srv.Get("/api/v1/reconciliation/companies?competence=2026-10"+query).MustStatus(e.t, http.StatusOK).JSON(e.t, &o)
	return o
}

func (e *reconEnv) documentsTotal(query string) int {
	e.t.Helper()
	var l struct {
		Total int `json:"total"`
	}
	e.srv.Get("/api/v1/reconciliation/documents?"+query).MustStatus(e.t, http.StatusOK).JSON(e.t, &l)
	return l.Total
}

func (e *reconEnv) queue(query string) map[string]int {
	e.t.Helper()
	var q struct {
		Counts map[string]int `json:"counts"`
	}
	e.srv.Get("/api/v1/reconciliation/review-queue?competence=2026-10"+query).MustStatus(e.t, http.StatusOK).JSON(e.t, &q)
	return q.Counts
}

// assertReconciliation confere a seção contra as listas do drill-down, com o filtro dado.
func (e *reconEnv) assertReconciliation(filter string, filterCompetence string) sectionsBody {
	e.t.Helper()
	r := sections(e.t, e.srv, "competence=2026-10"+filter).Reconciliation
	docs := func(status string) int { return e.documentsTotal("status=" + status + filter + filterCompetence) }
	q := e.queue(filter)
	o := e.overview(filter)
	for name, got := range map[string][2]int{
		"processando":     {r.Processing.Value, docs("pendente,triado")},
		"em revisão":      {r.ExtractionReview.Value, docs("revisao_triagem,revisao_extracao")},
		"período fechado": {r.ClosedPeriod.Value, docs("pendencia_periodo_fechado")},
		"fila":            {r.Queue.Value, q["total"]},
		"divergências":    {r.Divergent.Value, q["divergencia"]},
		"extrato sem":     {r.WithoutReceipt.Value, q["extrato_sem"]},
		"comprovante sem": {r.WithoutStatement.Value, q["comprovante_sem"]},
		"prontas":         {r.Ready.Value, len(e.overview(filter + "&state=pronta").Items)},
		"prontas (total)": {r.Ready.Value, o.Counts.Ready},
		"bloqueadas":      {r.BlocksAccounting.Value, len(e.overview(filter + "&blocked=true").Items)},
	} {
		if got[0] != got[1] {
			e.t.Errorf("%s %s: Dashboard %d × lista %d", filter, name, got[0], got[1])
		}
	}
	return sectionsBody{Reconciliation: r}
}

func TestTEST024_ReconciliationSectionMatchesTheLists(t *testing.T) {
	pool := seeded.NewCompaniesDB(t)
	e := &reconEnv{t: t, srv: testutil.NewServer(t, pool, testutil.FixedClock(t, "2026-10-01T12:00:00Z"))}
	e.srv.Post("/api/v1/competencies/ensure-current", nil).MustStatus(t, http.StatusOK)
	code := func(c string) string { return e.scalar(`SELECT id FROM companies WHERE internal_code = $1`, c) }
	// Quatro empresas com o Contábil: A em andamento, B pronta, C concluída e D só com um
	// lançamento manual (não usa a Conciliação: o Contábil fecha como sempre).
	var withAccounting []string
	rows, err := pool.Query(context.Background(), `SELECT c.internal_code FROM company_module_periods p JOIN companies c ON c.id = p.company_id
		JOIN competencies k ON k.id = p.competence_id WHERE k.competence_key = '2026-10' AND p.module_code = 'ACCOUNTING'
		ORDER BY c.internal_code LIMIT 4`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			t.Fatal(err)
		}
		withAccounting = append(withAccounting, c)
	}
	rows.Close()
	if len(withAccounting) < 4 {
		t.Fatalf("empresas com o Contábil: %v", withAccounting)
	}
	a, b, c, d := code(withAccounting[0]), code(withAccounting[1]), code(withAccounting[2]), code(withAccounting[3])
	for _, id := range []string{a, b, c} {
		e.scalar(`INSERT INTO chart_accounts (company_id, code, name) VALUES ($1, '1.01', 'Caixa') RETURNING id`, id)
	}

	e.document(a, "pendente", "", "")
	e.document(a, "triado", "comprovante", "2026-10")
	e.document(a, "revisao_triagem", "", "")
	e.document(a, "revisao_extracao", "comprovante", "2026-10")
	e.document(a, "pendencia_periodo_fechado", "", "2026-10")
	e.document("", "pendente", "", "") // sem empresa: só entra sem filtro
	e.statement(a, "", "divergencia", "classificada")
	e.statement(b, "classificada")
	e.statement(c, "classificada")
	e.scalar(`INSERT INTO client_entries (company_id, entry_date, amount, direction, counterparty, status)
		VALUES ($1, '2026-10-03', 10.00, 'D', 'Fornecedor', 'manual') RETURNING id`, d)
	// C: o contador conclui (a obrigação Conciliação vai para OK com as etapas anteriores).
	for _, step := range []string{"ACCOUNTING_MOVEMENT_RECEIVED", "ACCOUNTING_PAYROLL_POSTED", "ACCOUNTING_TAXES_POSTED", "ACCOUNTING_PROCESSED"} {
		var id string
		var v int
		if err := pool.QueryRow(context.Background(), `SELECT i.id, i.version FROM obligation_instances i JOIN obligation_types t ON t.id = i.obligation_type_id
			JOIN competencies k ON k.id = i.competence_id WHERE i.company_id = $1 AND t.code = $2 AND k.competence_key = '2026-10' AND i.applicable`,
			c, step).Scan(&id, &v); err != nil {
			continue // etapa não aplicável à empresa
		}
		e.srv.Patch("/api/v1/obligation-instances/"+id+"/status", map[string]any{"status_code": "OK", "version": v}).MustStatus(t, http.StatusOK)
	}
	e.srv.Post("/api/v1/companies/"+c+"/reconciliation/complete?competence=2026-10", nil).MustStatus(t, http.StatusOK)

	r := e.assertReconciliation("", "").Reconciliation
	want := map[string][2]int{
		"processando":     {r.Processing.Value, 3},
		"em revisão":      {r.ExtractionReview.Value, 2},
		"período fechado": {r.ClosedPeriod.Value, 1},
		"fila":            {r.Queue.Value, 3 + 1 + 1 + 1},
		"divergências":    {r.Divergent.Value, 1},
		"extrato sem":     {r.WithoutReceipt.Value, 1},
		"comprovante sem": {r.WithoutStatement.Value, 1},
		"prontas":         {r.Ready.Value, 1},
		"bloqueadas":      {r.BlocksAccounting.Value, 2},
	}
	for name, got := range want {
		if got[0] != got[1] {
			t.Errorf("%s = %d, esperado %d", name, got[0], got[1])
		}
	}
	// As fontes, com SQL direto.
	if n := e.count(`SELECT count(*) FROM reconciliation_documents WHERE status IN ('pendente', 'triado')`); n != r.Processing.Value {
		t.Errorf("processando no banco = %d", n)
	}
	if n := e.count(`SELECT count(*) FROM statement_transactions t WHERE t.competence = '2026-10-01' AND NOT EXISTS (
		SELECT 1 FROM reconciliations r WHERE r.statement_transaction_id = t.id AND r.undone_at IS NULL)`); n != r.Divergent.Value+r.WithoutReceipt.Value {
		t.Errorf("linhas livres no banco = %d", n)
	}

	// A Conciliação por empresa: o estado, o que falta, o Contábil e a obrigação.
	o := e.overview("")
	if o.Counts.Total != 4 || o.Counts.InProgress != 2 || o.Counts.Ready != 1 || o.Counts.Completed != 1 || o.Counts.BlocksAccounting != 2 {
		t.Fatalf("contagens = %+v", o.Counts)
	}
	byID := map[string]int{}
	for i, it := range o.Items {
		byID[it.Company.ID] = i
	}
	ia, ib, ic, id := o.Items[byID[a]], o.Items[byID[b]], o.Items[byID[c]], o.Items[byID[d]]
	if ia.State != "em_andamento" || ia.Blocking.Divergent != 1 || ia.Blocking.WithoutReceipt != 1 || ia.Blocking.Review != 3 ||
		!ia.UsesReconciliation || !ia.BlocksAccounting || ia.AccountingState == nil || *ia.AccountingState != "OPEN" {
		t.Errorf("A = %+v", ia)
	}
	if ib.State != "pronta" || !ib.BlocksAccounting || ib.Obligation == nil || ib.Obligation.IsFinal {
		t.Errorf("B = %+v", ib)
	}
	if ic.State != "concluida" || ic.BlocksAccounting || ic.Obligation == nil || ic.Obligation.StatusCode != "OK" {
		t.Errorf("C = %+v", ic)
	}
	if id.State != "em_andamento" || !id.Blocking.MissingStatement || id.UsesReconciliation || id.BlocksAccounting || id.WithoutStatement != 1 {
		t.Errorf("D = %+v", id)
	}
	// O bloqueio é a guarda do fechamento do Contábil (D-037): a mesma resposta da API do Controle.
	for _, it := range o.Items {
		var closure struct {
			Period *struct {
				State string `json:"state"`
			} `json:"period"`
			RequiresReconciliation bool `json:"requires_reconciliation"`
		}
		e.srv.Get("/api/v1/companies/"+it.Company.ID+"/reconciliation/closure?competence=2026-10").MustStatus(t, http.StatusOK).JSON(t, &closure)
		final := it.Obligation != nil && it.Obligation.IsFinal
		if blocked := closure.Period != nil && closure.Period.State == "OPEN" && closure.RequiresReconciliation && !final; blocked != it.BlocksAccounting {
			t.Errorf("%s: bloqueio na lista %v × fechamento %v", it.Company.ID, it.BlocksAccounting, blocked)
		}
	}

	// Os filtros: o responsável é o do Contábil. A com um responsável próprio no Contábil e B com
	// ele só no Fiscal.
	owner := newUser(t, pool, "Responsável da Conciliação")
	setSnapshot(t, pool, "accounting_responsible_user_id", owner, "2026-10", a)
	setSnapshot(t, pool, "fiscal_responsible_user_id", owner, "2026-10", b)
	f := e.assertReconciliation("&responsible_id="+owner, "&filter_competence=2026-10").Reconciliation
	if f.Processing.Value != 2 || f.ExtractionReview.Value != 2 || f.ClosedPeriod.Value != 1 || f.Queue.Value != 5 ||
		f.Ready.Value != 0 || f.BlocksAccounting.Value != 1 {
		t.Errorf("com o responsável do Contábil = %+v", f)
	}
	if q := f.Processing.Drilldown.Query; f.Processing.Drilldown.Path != "/reconciliation/documents" || q["status"] != "pendente,triado" ||
		q["responsible_id"] != owner || q["filter_competence"] != "2026-10" {
		t.Errorf("drill-down de processando = %+v", f.Processing.Drilldown)
	}
	if q := f.Ready.Drilldown.Query; f.Ready.Drilldown.Path != "/reconciliation/companies" || q["state"] != "pronta" || q["competence"] != "2026-10" ||
		q["responsible_id"] != owner {
		t.Errorf("drill-down das prontas = %+v", f.Ready.Drilldown)
	}
	if q := f.Divergent.Drilldown.Query; f.Divergent.Drilldown.Path != "/reconciliation" || q["bucket"] != "divergencia" || q["responsible_id"] != owner {
		t.Errorf("drill-down das divergências = %+v", f.Divergent.Drilldown)
	}
	e.assertReconciliation("&regime_group=NORMAL", "&filter_competence=2026-10")
	e.assertReconciliation("&regime_group=SIMPLES", "&filter_competence=2026-10")

	// Os filtros de empresa exigem a competência de referência.
	e.srv.Get("/api/v1/reconciliation/review-queue?responsible_id="+owner).MustStatus(t, http.StatusBadRequest)
	e.srv.Get("/api/v1/reconciliation/documents?responsible_id="+owner).MustStatus(t, http.StatusBadRequest)
	e.srv.Get("/api/v1/reconciliation/companies").MustStatus(t, http.StatusBadRequest)
	e.srv.Get("/api/v1/reconciliation/companies?competence=2026-10&state=x").MustStatus(t, http.StatusBadRequest)
	e.srv.Get("/api/v1/reconciliation/companies?competence=2026-10&blocked=sim").MustStatus(t, http.StatusBadRequest)
	e.srv.Get("/api/v1/reconciliation/documents?status=pendente,x").MustStatus(t, http.StatusBadRequest)
}
