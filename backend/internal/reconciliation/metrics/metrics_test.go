package metrics_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"controle/internal/testutil"
	"controle/internal/user"
)

// TEST-021c [11-observability §7, REQ-022, REQ-036, AC-034]: o painel de métricas, os playbooks
// por banco e o resumo da competência na ficha, conferidos número a número sobre dados montados
// no banco.

var ctx = context.Background()

type env struct {
	t    *testing.T
	pool *pgxpool.Pool
	srv  *testutil.Server
	seq  int
}

func (e *env) scalar(sql string, args ...any) string {
	e.t.Helper()
	var v string
	if err := e.pool.QueryRow(ctx, sql, args...).Scan(&v); err != nil {
		e.t.Fatalf("%v\n%s", err, sql)
	}
	return v
}

func (e *env) exec(sql string, args ...any) {
	e.t.Helper()
	if _, err := e.pool.Exec(ctx, sql, args...); err != nil {
		e.t.Fatalf("%v\n%s", err, sql)
	}
}

func (e *env) next() int {
	e.seq++
	return e.seq
}

func (e *env) company(code string) string {
	e.t.Helper()
	return e.scalar(`INSERT INTO companies (internal_code, legal_name, cnpj, service_start_date, payroll_type, created_by, updated_by)
		VALUES ($1, 'Empresa ' || $1, lpad($1, 14, '1'), '2025-01-01', 'PAYROLL', $2, $2) RETURNING id`, code, user.AdminID)
}

// doc cria um documento da Conciliação com o tipo, o status e a competência.
func (e *env) doc(company, kind, status string, competence *string) string {
	e.t.Helper()
	n := e.next()
	return e.scalar(`INSERT INTO reconciliation_documents (company_id, competence, bucket, object_key, sha256, file_name, content_type, kind, status)
		VALUES ($1, $2::date, 'originais', 'originais/' || $3::text, lpad(to_hex($4::int), 64, '0'), 'doc-' || $3::text || '.pdf',
		'application/pdf', $5, $6)
		RETURNING id`, company, competence, fmt.Sprint(n), n, kind, status)
}

// statementImport importa o extrato do documento no período, com as tentativas do agente.
func (e *env) statementImport(company, document, bank, branch, account, start, end string, attempts int) string {
	e.t.Helper()
	e.exec(`UPDATE reconciliation_documents SET period_start = $2, period_end = $3 WHERE id = $1`, document, start, end)
	return e.scalar(`INSERT INTO statement_imports (company_id, document_id, bank, branch, account, period_start, period_end,
		opening_balance, closing_balance, validation_ok, validation, attempts)
		VALUES ($1, $2, $3, $4, $5, $6, $7, 100.00, 250.50, true, '{"ok": true}', $8) RETURNING id`,
		company, document, bank, branch, account, start, end, attempts)
}

func (e *env) log(company, document, alias string, attempt int, cost *string) {
	e.t.Helper()
	e.exec(`INSERT INTO extraction_logs (company_id, document_id, alias, attempt, cost_usd) VALUES ($1, $2, $3, $4, $5::numeric)`,
		company, document, alias, attempt, cost)
}

func (e *env) humanLog(company, document, action string) {
	e.t.Helper()
	e.exec(`INSERT INTO extraction_logs (company_id, document_id, alias, attempt, actor_id, change) VALUES ($1, $2, 'humano', 0, $3, $4)`,
		company, document, user.AdminID, fmt.Sprintf(`{"acao":%q}`, action))
}

func (e *env) entry(company string, document *string, date, status, fieldOrigin string) string {
	e.t.Helper()
	return e.scalar(`INSERT INTO client_entries (company_id, document_id, entry_date, amount, direction, counterparty, status, field_origin)
		VALUES ($1, $2, $3, 10.00, 'D', 'Fornecedor', $4, $5::jsonb) RETURNING id`, company, document, date, status, fieldOrigin)
}

func (e *env) line(company, imp, date string) string {
	e.t.Helper()
	n := e.next()
	return e.scalar(`INSERT INTO statement_transactions (company_id, statement_import_id, position, txn_date, description, amount, direction, dedupe_key)
		VALUES ($1, $2, $3::int, $4, 'LINHA ' || $5::text, 10.00, 'D', lpad(to_hex($3::int), 64, 'f')) RETURNING id`, company, imp, n, date,
		fmt.Sprint(n))
}

// auto concilia a linha pelo passe; manual, pelo método e com autor.
func (e *env) auto(company, line string, pass int) string {
	e.t.Helper()
	return e.scalar(`INSERT INTO reconciliations (company_id, statement_transaction_id, pass, method, origin, score)
		VALUES ($1, $2, $3::smallint, $4, 'auto', 1) RETURNING id`, company, line, pass, fmt.Sprintf("passe_%d", pass))
}

func (e *env) manual(company, line, method string) string {
	e.t.Helper()
	return e.scalar(`INSERT INTO reconciliations (company_id, statement_transaction_id, method, origin, created_by)
		VALUES ($1, $2, $3, 'manual', $4) RETURNING id`, company, line, method, user.AdminID)
}

func (e *env) competenceID(day string) string {
	e.t.Helper()
	return e.scalar(`INSERT INTO competencies (competence_key, year, month, first_day)
		VALUES (to_char($1::date, 'YYYY-MM'), extract(year FROM $1::date), extract(month FROM $1::date), $1::date)
		ON CONFLICT (first_day) DO UPDATE SET first_day = EXCLUDED.first_day RETURNING id`, day)
}

// accounting cria o período do Contábil da empresa no estado dado, com os eventos.
func (e *env) accounting(company, day, state string, events ...string) {
	e.t.Helper()
	closed := state == "CLOSED"
	period := e.scalar(`INSERT INTO company_module_periods (company_id, competence_id, module_code, state, closed_at, closed_by)
		VALUES ($1, $2, 'ACCOUNTING', $3, CASE WHEN $4 THEN now() END, CASE WHEN $4 THEN $5::uuid END) RETURNING id`,
		company, e.competenceID(day), state, closed, user.AdminID)
	for _, ev := range events {
		e.exec(`INSERT INTO company_module_period_events (company_module_period_id, event_type, reason, user_id)
			VALUES ($1, $2, CASE WHEN $2 = 'REOPENED' THEN 'erro achado depois do fechamento' END, $3)`, period, ev, user.AdminID)
	}
}

func (e *env) run(company, agent, competence string, cost *string) {
	e.t.Helper()
	n := e.next()
	e.exec(`INSERT INTO ai_runs (trace_id, agent, trigger, company_id, competence, status, cost_usd, created_at, updated_at)
		VALUES (lpad(to_hex($1::int), 32, 'a'), $2, 'pedido', $3, $4::date, 'needs_review', $5::numeric, now(), now())`,
		n, agent, company, competence, cost)
}

func ptr(s string) *string { return &s }

type extraction struct {
	Documents           int     `json:"documents"`
	LocalReader         int     `json:"local_reader"`
	Agent               int     `json:"agent"`
	FirstAttempt        int     `json:"first_attempt"`
	FirstAttemptPercent *string `json:"first_attempt_percent"`
	AverageAttempts     *string `json:"average_attempts"`
	NotApproved         int     `json:"not_approved"`
	CostUSD             string  `json:"cost_usd"`
	CostPerDocumentUSD  *string `json:"cost_per_document_usd"`
}

type matching struct {
	Lines         int            `json:"lines"`
	Reconciled    int            `json:"reconciled"`
	Auto          int            `json:"auto"`
	Manual        int            `json:"manual"`
	Free          int            `json:"free"`
	AutoPercent   *string        `json:"auto_percent"`
	TargetPercent string         `json:"target_percent"`
	ByMethod      map[string]int `json:"by_method"`
	UndoneAuto    int            `json:"undone_auto"`
}

type agentCost struct {
	Runs          int     `json:"runs"`
	CostUSD       string  `json:"cost_usd"`
	CostPerRunUSD *string `json:"cost_per_run_usd"`
}

type metricsOut struct {
	CompanyID  *string        `json:"company_id"`
	Competence *string        `json:"competence"`
	Matching   matching       `json:"matching"`
	Buckets    map[string]int `json:"buckets"`
	Extraction struct {
		Statements extraction `json:"statements"`
		Receipts   extraction `json:"receipts"`
	} `json:"extraction"`
	Corrections struct {
		Actions     int     `json:"actions"`
		Documents   int     `json:"documents"`
		RatePercent *string `json:"rate_percent"`
	} `json:"corrections"`
	AfterClose struct {
		Reopenings    int `json:"reopenings"`
		LateDocuments int `json:"late_documents"`
	} `json:"after_close"`
	Agents struct {
		Resolver   agentCost `json:"resolver"`
		Classifier agentCost `json:"classifier"`
	} `json:"agents"`
	Playbooks []struct {
		Bank           string  `json:"bank"`
		Versions       int     `json:"versions"`
		ValidVersion   *int    `json:"valid_version"`
		Successes      int     `json:"successes"`
		Failures       int     `json:"failures"`
		SuccessPercent *string `json:"success_percent"`
	} `json:"playbooks"`
}

func str(p *string) string {
	if p == nil {
		return "<nil>"
	}
	return *p
}

func (e *env) metrics(query string) metricsOut {
	e.t.Helper()
	var out metricsOut
	e.srv.Get("/api/v1/reconciliation/metrics?"+query).MustStatus(e.t, http.StatusOK).JSON(e.t, &out)
	return out
}

func (e *env) queueCounts(query string) map[string]int {
	e.t.Helper()
	var out struct {
		Counts map[string]int `json:"counts"`
	}
	e.srv.Get("/api/v1/reconciliation/review-queue?"+query).MustStatus(e.t, http.StatusOK).JSON(e.t, &out)
	return out.Counts
}

// scenario monta duas empresas com documentos, linhas, conciliações, logs, corridas, períodos do
// Contábil e playbooks. Devolve as empresas A e B.
func scenario(t *testing.T) (*env, string, string) {
	pool := testutil.NewDB(t)
	e := &env{t: t, pool: pool, srv: testutil.NewServer(t, pool, testutil.FixedClock(t, "2026-09-15T12:00:00-03:00"))}
	a, b, c := e.company("900"), e.company("901"), e.company("902")
	aug := ptr("2026-08-01")

	// Extratos da A em agosto: lido sem agente (S1), aprovado na 1ª tentativa (S2), na 2ª (S3) e
	// reprovado no portão depois de 3 tentativas (S4).
	s1 := e.doc(a, "extrato", "importado", nil)
	imp := e.statementImport(a, s1, "a_fonte", "0001", "12345-6", "2026-08-01", "2026-08-31", 0)
	e.log(a, s1, "leitor_local", 0, nil)
	s2 := e.doc(a, "extrato", "importado", nil)
	e.statementImport(a, s2, "a_fonte", "0001", "12345-6", "2026-08-01", "2026-08-31", 1)
	e.log(a, s2, "triador", 1, ptr("0.00100000"))
	e.log(a, s2, "extrator", 1, ptr("0.01000000"))
	s3 := e.doc(a, "extrato", "importado", nil)
	e.statementImport(a, s3, "beta", "0021", "33412-7", "2026-07-20", "2026-08-10", 2)
	e.log(a, s3, "extrator", 1, ptr("0.01000000"))
	e.log(a, s3, "extrator", 2, ptr("0.02000000"))
	s4 := e.doc(a, "extrato", "revisao_extracao", aug)
	e.log(a, s4, "extrator", 1, ptr("0.01000000"))
	e.log(a, s4, "extrator", 2, ptr("0.01000000"))
	e.log(a, s4, "extrator_forte", 3, ptr("0.01000000"))

	// Comprovantes da A: lido sem agente (R1), aprovado na 1ª (R2), parcial depois de 2 (R3) e
	// completo só depois da correção humana (R4).
	r1 := e.doc(a, "comprovante", "importado", nil)
	e.log(a, r1, "leitor_local", 0, nil)
	e.entry(a, &r1, "2026-08-05", "extraido", `{}`)
	r2 := e.doc(a, "comprovante", "importado", nil)
	e.log(a, r2, "extrator", 1, ptr("0.00500000"))
	e.entry(a, &r2, "2026-08-06", "extraido", `{"valor": {"pagina": 1}}`)
	r3 := e.doc(a, "comprovante", "revisao_extracao", nil)
	e.log(a, r3, "extrator", 1, ptr("0.00500000"))
	e.log(a, r3, "extrator", 2, ptr("0.00500000"))
	e.entry(a, &r3, "2026-08-07", "parcial", `{}`)
	r4 := e.doc(a, "comprovante", "importado", nil)
	e.log(a, r4, "extrator", 1, ptr("0.00500000"))
	e.entry(a, &r4, "2026-08-08", "extraido", `{"valor": {"origem": "humano"}}`)
	e.humanLog(a, r4, "corrigir_campos")
	e.humanLog(a, r4, "corrigir_campos")
	// "Que documento é esse?" num documento que ainda não foi lido; conciliar não é correção.
	d5 := e.doc(a, "extrato", "triado", aug)
	e.humanLog(a, d5, "identificar")
	e.humanLog(a, s1, "conciliar")
	// Lançamento manual da competência e documento que chegou com o Contábil fechado.
	e.entry(a, nil, "2026-08-20", "manual", `{}`)
	e.doc(a, "comprovante", "pendencia_periodo_fechado", aug)

	// Linhas da A em agosto: 4 automáticas (passes 1, 1, 2 e 3), 2 manuais, 1 automática desfeita
	// e 3 livres; uma linha conciliada em setembro.
	lines := make([]string, 10)
	for i := range lines {
		lines[i] = e.line(a, imp, fmt.Sprintf("2026-08-%02d", i+1))
	}
	for i, pass := range []int{1, 1, 2, 3} {
		e.auto(a, lines[i], pass)
	}
	e.manual(a, lines[4], "manual")
	e.manual(a, lines[5], "sem_comprovante")
	undone := e.auto(a, lines[6], 1)
	e.exec(`UPDATE reconciliations SET undone_at = now(), undone_by = $2, undo_reason = 'teste' WHERE id = $1`, undone, user.AdminID)
	e.auto(a, e.line(a, imp, "2026-09-02"), 1)
	// Classificação das conciliadas de agosto: 2 confirmadas, 1 à espera do agente, 3 sem nada.
	e.exec(`INSERT INTO chart_accounts (company_id, code, name) VALUES ($1, '1.01', 'Caixa')`, a)
	for _, l := range lines[:2] {
		e.exec(`UPDATE statement_transactions SET account_code = '1.01', classification_origin = 'manual', classified_by = $2,
			classified_at = now() WHERE id = $1`, l, user.AdminID)
	}
	e.exec(`UPDATE statement_transactions SET classification_origin = 'agente', classification_pending = true WHERE id = $1`, lines[2])

	// A empresa B: uma linha automática e uma livre em agosto, e o Contábil fechado.
	sb := e.doc(b, "extrato", "importado", nil)
	impB := e.statementImport(b, sb, "c6", "0001", "999-9", "2026-08-01", "2026-08-31", 0)
	e.auto(b, e.line(b, impB, "2026-08-03"), 1)
	e.line(b, impB, "2026-08-04")

	// Reaberturas do Contábil: 2 na A em agosto, 1 na B, e 1 na C, que não usa a Conciliação.
	e.accounting(a, "2026-08-01", "OPEN", "CLOSED", "REOPENED", "CLOSED", "REOPENED")
	e.accounting(b, "2026-08-01", "CLOSED", "CLOSED", "REOPENED", "CLOSED")
	e.accounting(c, "2026-08-01", "OPEN", "CLOSED", "REOPENED")

	// O resolvedor (2 corridas) e o classificador (1, sem custo informado) na A em agosto.
	e.run(a, "resolvedor", "2026-08-01", ptr("0.00200000"))
	e.run(a, "resolvedor", "2026-08-01", ptr("0.00400000"))
	e.run(a, "classificador", "2026-08-01", nil)

	// Playbooks: a_fonte com a v1 invalidada e a v2 vigente; beta só com a v1.
	for _, p := range []struct {
		bank       string
		version    int
		valid      bool
		succ, fail int
	}{{"a_fonte", 1, false, 3, 1}, {"a_fonte", 2, true, 5, 0}, {"beta", 1, true, 0, 0}} {
		e.exec(`INSERT INTO bank_playbooks (bank, version, table_start_page, ignored_pages, date_format, decimal_separator,
			sign_representation, balance_location, winning_strategy, valid, successes, failures)
			VALUES ($1, $2, 1, '[2, 3]', 'DD/MM/YYYY', ',', 'coluna_tipo', 'rodape', 'texto', $3, $4, $5)`,
			p.bank, p.version, p.valid, p.succ, p.fail)
	}
	return e, a, b
}

func TestTEST021c_MetricsPanel(t *testing.T) {
	e, a, b := scenario(t)

	m := e.metrics("company_id=" + a + "&competence=2026-08")
	if str(m.CompanyID) != a || str(m.Competence) != "2026-08" {
		t.Fatalf("filtros = %s, %s", str(m.CompanyID), str(m.Competence))
	}
	// % automático da origem: automáticas ÷ linhas (4 ÷ 10), contra a meta de 80%.
	wantMatching := matching{Lines: 10, Reconciled: 6, Auto: 4, Manual: 2, Free: 4, AutoPercent: ptr("40.0"), TargetPercent: "80",
		ByMethod: map[string]int{"passe_1": 2, "passe_2": 1, "passe_3": 1, "manual": 1, "sem_comprovante": 1}, UndoneAuto: 1}
	if !reflect.DeepEqual(m.Matching, wantMatching) {
		t.Errorf("conciliação = %+v (auto %s)\nesperado %+v", m.Matching, str(m.Matching.AutoPercent), wantMatching)
	}
	// Itens por balde: a mesma contagem da fila.
	if want := e.queueCounts("company_id=" + a + "&competence=2026-08"); !reflect.DeepEqual(m.Buckets, want) {
		t.Errorf("baldes = %v; fila %v", m.Buckets, want)
	}
	// Extratos: 4 lidos (1 sem agente, 3 pelo agente), 1 aprovado na 1ª, (1 + 2 + 3) ÷ 3 = 2,00
	// tentativas, 1 reprovado; custo 0,071 ÷ 4.
	st := m.Extraction.Statements
	if st.Documents != 4 || st.LocalReader != 1 || st.Agent != 3 || st.FirstAttempt != 1 || str(st.FirstAttemptPercent) != "33.3" ||
		str(st.AverageAttempts) != "2.00" || st.NotApproved != 1 || st.CostUSD != "0.07100000" || str(st.CostPerDocumentUSD) != "0.01775000" {
		t.Errorf("extratos = %+v (1ª %s, média %s, por documento %s)", st, str(st.FirstAttemptPercent), str(st.AverageAttempts),
			str(st.CostPerDocumentUSD))
	}
	// Comprovantes: 4 lidos (1 sem agente, 3 pelo agente); só o R2 aprovado na 1ª (o R4 foi
	// corrigido à mão); o R3 parcial e o R4 contam como não aprovados.
	rc := m.Extraction.Receipts
	if rc.Documents != 4 || rc.LocalReader != 1 || rc.Agent != 3 || rc.FirstAttempt != 1 || str(rc.AverageAttempts) != "1.33" ||
		rc.NotApproved != 2 || rc.CostUSD != "0.02000000" || str(rc.CostPerDocumentUSD) != "0.00500000" {
		t.Errorf("comprovantes = %+v (média %s, por documento %s)", rc, str(rc.AverageAttempts), str(rc.CostPerDocumentUSD))
	}
	// Correções: 2 no R4 e a identificação do D5 (3 ações, 2 documentos) ÷ 8 lidos; "conciliar" não conta.
	if c := m.Corrections; c.Actions != 3 || c.Documents != 2 || str(c.RatePercent) != "25.0" {
		t.Errorf("correções = %+v (%s)", c, str(c.RatePercent))
	}
	// Depois do fechamento: as 2 reaberturas da A e o documento tardio.
	if ac := m.AfterClose; ac.Reopenings != 2 || ac.LateDocuments != 1 {
		t.Errorf("depois do fechamento = %+v", ac)
	}
	// O resolvedor custa 0,006 em 2 corridas; o classificador não informou custo.
	if r := m.Agents.Resolver; r.Runs != 2 || r.CostUSD != "0.00600000" || str(r.CostPerRunUSD) != "0.00300000" {
		t.Errorf("resolvedor = %+v (%s)", r, str(r.CostPerRunUSD))
	}
	if c := m.Agents.Classifier; c.Runs != 1 || c.CostUSD != "0.00000000" || str(c.CostPerRunUSD) != "0.00000000" {
		t.Errorf("classificador = %+v (%s)", c, str(c.CostPerRunUSD))
	}
	// Playbooks por banco (sem filtro: são do banco, não da empresa).
	if len(m.Playbooks) != 2 {
		t.Fatalf("playbooks = %+v", m.Playbooks)
	}
	if p := m.Playbooks[0]; p.Bank != "a_fonte" || p.Versions != 2 || p.ValidVersion == nil || *p.ValidVersion != 2 || p.Successes != 8 ||
		p.Failures != 1 || str(p.SuccessPercent) != "88.9" {
		t.Errorf("a_fonte = %+v (%s)", p, str(p.SuccessPercent))
	}
	if p := m.Playbooks[1]; p.Bank != "beta" || p.Versions != 1 || p.Successes != 0 || p.SuccessPercent != nil {
		t.Errorf("beta = %+v", p)
	}

	// Sem filtro: as duas empresas, agosto e setembro; a reabertura da C (sem Conciliação) não conta.
	all := e.metrics("")
	if all.CompanyID != nil || all.Competence != nil {
		t.Errorf("filtros vazios = %s, %s", str(all.CompanyID), str(all.Competence))
	}
	if mm := all.Matching; mm.Lines != 13 || mm.Auto != 6 || mm.Manual != 2 || str(mm.AutoPercent) != "46.2" || mm.ByMethod["passe_1"] != 4 {
		t.Errorf("conciliação sem filtro = %+v (%s)", mm, str(mm.AutoPercent))
	}
	if ac := all.AfterClose; ac.Reopenings != 3 || ac.LateDocuments != 1 {
		t.Errorf("depois do fechamento sem filtro = %+v", ac)
	}
	if want := e.queueCounts(""); !reflect.DeepEqual(all.Buckets, want) {
		t.Errorf("baldes sem filtro = %v; fila %v", all.Buckets, want)
	}
	// Só a B: a linha automática e a livre.
	onlyB := e.metrics("company_id=" + b)
	if mm := onlyB.Matching; mm.Lines != 2 || mm.Auto != 1 || str(mm.AutoPercent) != "50.0" || onlyB.Extraction.Statements.LocalReader != 1 {
		t.Errorf("B = %+v, extratos %+v", mm, onlyB.Extraction.Statements)
	}
	// Competência sem nada: percentuais nulos, nunca divisão por zero.
	empty := e.metrics("competence=2025-01")
	if mm := empty.Matching; mm.Lines != 0 || mm.AutoPercent != nil || empty.Extraction.Statements.AverageAttempts != nil ||
		empty.Extraction.Statements.CostUSD != "0.00000000" || empty.Corrections.RatePercent != nil {
		t.Errorf("vazio = %+v, %+v, %+v", mm, empty.Extraction.Statements, empty.Corrections)
	}
	// Filtros inválidos.
	for _, q := range []string{"company_id=x", "competence=2026-13"} {
		if res := e.srv.Get("/api/v1/reconciliation/metrics?" + q); res.Status != http.StatusBadRequest || res.ErrorCode(t) != "VALIDATION_ERROR" {
			t.Errorf("%s: %d %s", q, res.Status, res.Body)
		}
	}
}

func TestTEST021c_Playbooks(t *testing.T) {
	e, _, _ := scenario(t)
	var out struct {
		Items []struct {
			Bank               string  `json:"bank"`
			Version            int     `json:"version"`
			Valid              bool    `json:"valid"`
			Successes          int     `json:"successes"`
			Failures           int     `json:"failures"`
			SuccessPercent     *string `json:"success_percent"`
			TableStartPage     int     `json:"table_start_page"`
			IgnoredPages       []int   `json:"ignored_pages"`
			DateFormat         string  `json:"date_format"`
			DecimalSeparator   string  `json:"decimal_separator"`
			SignRepresentation string  `json:"sign_representation"`
			BalanceLocation    string  `json:"balance_location"`
			WinningStrategy    string  `json:"winning_strategy"`
		} `json:"items"`
	}
	e.srv.Get("/api/v1/reconciliation/playbooks").MustStatus(t, http.StatusOK).JSON(t, &out)
	got := make([]string, 0, len(out.Items))
	for _, p := range out.Items {
		got = append(got, fmt.Sprintf("%s v%d %v %d/%d %s", p.Bank, p.Version, p.Valid, p.Successes, p.Failures, str(p.SuccessPercent)))
	}
	want := []string{"a_fonte v2 true 5/0 100.0", "a_fonte v1 false 3/1 75.0", "beta v1 true 0/0 <nil>"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("playbooks = %v; esperado %v", got, want)
	}
	if p := out.Items[0]; !reflect.DeepEqual(p.IgnoredPages, []int{2, 3}) || p.TableStartPage != 1 || p.DateFormat != "DD/MM/YYYY" ||
		p.DecimalSeparator != "," || p.SignRepresentation != "coluna_tipo" || p.BalanceLocation != "rodape" || p.WinningStrategy != "texto" {
		t.Errorf("receita = %+v", p)
	}
}

func TestTEST021c_CompanySummary(t *testing.T) {
	e, a, b := scenario(t)
	var out struct {
		CompanyID        string `json:"company_id"`
		Competence       string `json:"competence"`
		AccountingClosed bool   `json:"accounting_closed"`
		Statements       []struct {
			DocumentID     string `json:"document_id"`
			FileName       string `json:"file_name"`
			BankAccount    string `json:"bank_account"`
			PeriodStart    string `json:"period_start"`
			PeriodEnd      string `json:"period_end"`
			OpeningBalance string `json:"opening_balance"`
			ClosingBalance string `json:"closing_balance"`
			Attempts       int    `json:"attempts"`
			Transactions   int    `json:"transactions"`
		} `json:"statements"`
		Matching matching       `json:"matching"`
		Buckets  map[string]int `json:"buckets"`
		Entries  struct {
			Total   int `json:"total"`
			Partial int `json:"partial"`
		} `json:"entries"`
		Classification struct {
			Classified int `json:"classified"`
			Pending    int `json:"pending"`
		} `json:"classification"`
	}
	e.srv.Get("/api/v1/companies/"+a+"/reconciliation?competence=2026-08").MustStatus(t, http.StatusOK).JSON(t, &out)
	if out.CompanyID != a || out.Competence != "2026-08" || out.AccountingClosed {
		t.Errorf("cabeçalho = %s %s fechado=%v", out.CompanyID, out.Competence, out.AccountingClosed)
	}
	// Os três extratos que cobrem agosto (o da beta começa em julho), por conta e período.
	got := make([]string, 0, len(out.Statements))
	for _, s := range out.Statements {
		got = append(got, fmt.Sprintf("%s %s..%s %s→%s %d tent. %d linhas", s.BankAccount, s.PeriodStart, s.PeriodEnd, s.OpeningBalance,
			s.ClosingBalance, s.Attempts, s.Transactions))
	}
	want := []string{
		"a_fonte 0001/12345-6 2026-08-01..2026-08-31 100.00→250.50 0 tent. 11 linhas",
		"a_fonte 0001/12345-6 2026-08-01..2026-08-31 100.00→250.50 1 tent. 0 linhas",
		"beta 0021/33412-7 2026-07-20..2026-08-10 100.00→250.50 2 tent. 0 linhas",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("extratos = %v\nesperado %v", got, want)
	}
	if out.Matching.Lines != 10 || out.Matching.Auto != 4 || str(out.Matching.AutoPercent) != "40.0" {
		t.Errorf("conciliação = %+v", out.Matching)
	}
	if want := e.queueCounts("company_id=" + a + "&competence=2026-08"); !reflect.DeepEqual(out.Buckets, want) {
		t.Errorf("baldes = %v; fila %v", out.Buckets, want)
	}
	// Lançamentos de agosto: os 4 dos comprovantes e o manual; o R3 parcial.
	if out.Entries.Total != 5 || out.Entries.Partial != 1 {
		t.Errorf("lançamentos = %+v", out.Entries)
	}
	// Das 6 conciliadas, 2 confirmadas; 1 à espera do agente e 3 sem classificação.
	if out.Classification.Classified != 2 || out.Classification.Pending != 4 {
		t.Errorf("classificação = %+v", out.Classification)
	}
	// O Contábil fechado da B aparece; competência obrigatória; empresa inexistente.
	var closed struct {
		AccountingClosed bool `json:"accounting_closed"`
	}
	e.srv.Get("/api/v1/companies/"+b+"/reconciliation?competence=2026-08").MustStatus(t, http.StatusOK).JSON(t, &closed)
	if !closed.AccountingClosed {
		t.Error("o Contábil da B está fechado em agosto")
	}
	if res := e.srv.Get("/api/v1/companies/" + a + "/reconciliation"); res.Status != http.StatusBadRequest {
		t.Errorf("sem competência = %d", res.Status)
	}
	for _, id := range []string{"00000000-0000-4000-8000-000000000000", "x"} {
		if res := e.srv.Get("/api/v1/companies/" + id + "/reconciliation?competence=2026-08"); res.Status != http.StatusNotFound ||
			res.ErrorCode(t) != "COMPANY_NOT_FOUND" {
			t.Errorf("%s: %d %s", id, res.Status, res.Body)
		}
	}
	var raw map[string]json.RawMessage
	e.srv.Get("/api/v1/companies/"+a+"/reconciliation?competence=2025-01").MustStatus(t, http.StatusOK).JSON(t, &raw)
	if string(raw["statements"]) != "[]" {
		t.Errorf("competência vazia: extratos = %s", raw["statements"])
	}
}
