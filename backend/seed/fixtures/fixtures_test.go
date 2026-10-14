package fixtures_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"controle/internal/ai"
	"controle/internal/ai/aliases"
	"controle/internal/ai/litellm"
	"controle/internal/ai/tracer"
	"controle/internal/factorr/inbox"
	"controle/internal/jobs"
	"controle/internal/platform/clock"
	"controle/internal/platform/config"
	"controle/internal/platform/decimal"
	"controle/internal/platform/objectstore"
	"controle/internal/platform/pdftext"
	"controle/internal/reconciliation"
	"controle/internal/reconciliation/matching"
	"controle/internal/reconciliation/pipeline"
	"controle/internal/reconciliation/reconcile"
	"controle/internal/testutil"
	"controle/seed"
	"controle/seed/fixtures"
)

// TEST-027 [REQ-048, REQ-049, AC-032]: banco limpo → seed de referência e de demonstração →
// preparação dos fixtures → a jornada da demonstração (F-012), reproduzível. As obrigações e o
// histórico da Construtora Horizonte (111), com o Contábil do cenário aberto; a carteira do Fator
// R; o cenário da Conciliação carregado pelos serviços (MinIO real, o pipeline do worker com os
// leitores determinísticos e nenhuma chamada ao modelo), com o resultado do rodar_matching da
// origem e o fechamento do Contábil travado até a conciliação; os extratos do PGDAS-D fazendo no
// inbox o que o LEIA-ME diz; e a preparação de novo sem mudar nada.

var (
	ctx   = context.Background()
	quiet = slog.New(slog.NewTextHandler(io.Discard, nil))
	pdf   = pdftext.New(0)
)

const (
	scenarioDir    = "../../internal/reconciliation/testdata/conciliacao"
	pgdasTemplates = "../../internal/factorr/pgdas/testdata/fixtures"
	matchingGolden = "../../internal/reconciliation/matching/testdata/matching_golden.json"
)

// refuse é o modelo da preparação: nenhum documento do cenário pode precisar dele.
type refuse struct{ calls atomic.Int32 }

func (g *refuse) Chat(context.Context, litellm.Request) (litellm.Response, error) {
	g.calls.Add(1)
	return litellm.Response{}, errors.New("a preparação dos fixtures não chama o modelo")
}

type env struct {
	pool   *pgxpool.Pool
	srv    *testutil.Server
	gw     *refuse
	runner *jobs.Runner
	deps   fixtures.Deps
	opts   fixtures.Options
}

// setup sobe o banco limpo com o seed de referência e de demonstração (hoje = 12/10/2026, a
// competência do cenário dentro do histórico), a API e o worker da Conciliação e do inbox.
func setup(t *testing.T) env {
	t.Helper()
	pool := testutil.NewDB(t)
	m := testutil.NewMinIO(t)
	clk := testutil.FixedClock(t, "2026-10-12T12:00:00-03:00")
	if err := seed.Reference(ctx, pool, clk, io.Discard); err != nil {
		t.Fatal(err)
	}
	if err := seed.Demo(ctx, pool, clk, io.Discard); err != nil {
		t.Fatal(err)
	}
	storage := t.TempDir()
	srv := testutil.NewServer(t, pool, clk, func(c *config.Config) { c.FileStorageDir = storage })
	sys := clock.NewSystem(time.UTC)
	q := jobs.NewQueue(sys, 3)
	originals := objectstore.New(objectstore.Config{Endpoint: m.Endpoint, Region: "us-east-1", AccessKey: m.AccessKey, SecretKey: m.SecretKey, Bucket: m.Bucket})
	gw := &refuse{}
	models := ai.Defaults{
		ai.Triador:       {Provider: "openai", Name: "modelo-recusado"},
		ai.Extrator:      {Provider: "openai", Name: "modelo-recusado"},
		ai.ExtratorForte: {Provider: "openai", Name: "modelo-recusado"},
	}
	proc := pipeline.New(sys, tracer.New(sys, q), q, originals, pdf, gw, aliases.NewResolver(models),
		pipeline.Config{TriageThreshold: decimal.MustParse("0.70"), FieldThreshold: decimal.MustParse("0.70")}, quiet)
	rounds := reconcile.NewService(pool, sys, matching.DefaultOptions(), quiet)
	pgdasAgent := inbox.NewAgent(sys, tracer.New(sys, q), inbox.NewStorage(storage), pdf)
	runner := jobs.NewRunner(pool, sys, quiet, jobs.RunnerConfig{WorkerID: "w-demo", Concurrency: 1, PollInterval: time.Millisecond, Lease: 10 * time.Minute},
		map[string]jobs.Handler{pipeline.KindTriage: proc.HandleTriage, pipeline.KindStatementExtract: proc.HandleExtract,
			pipeline.KindReceiptExtract: proc.HandleReceipt, reconciliation.JobRun: rounds.HandleRun, inbox.KindParse: pgdasAgent.HandleParse})
	e := env{pool: pool, srv: srv, gw: gw, runner: runner}
	e.deps = fixtures.Deps{Pool: pool, Clock: clk, Ops: sys, Originals: originals, Queue: q, Matching: matching.DefaultOptions(), Logger: quiet}
	e.opts = fixtures.Options{ScenarioDir: scenarioDir, PgdasTemplates: pgdasTemplates, OutDir: t.TempDir(), Process: e.drain,
		Poll: 10 * time.Millisecond, Timeout: 3 * time.Minute}
	return e
}

// drain é o worker: processa os jobs prontos.
func (e env) drain(ctx context.Context) error {
	for range 500 {
		ok, err := e.runner.ProcessOne(ctx)
		if err != nil || !ok {
			return err
		}
	}
	return errors.New("a fila não esvaziou")
}

func (e env) count(t *testing.T, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := e.pool.QueryRow(ctx, sql, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (e env) scalar(t *testing.T, sql string, args ...any) string {
	t.Helper()
	var s string
	if err := e.pool.QueryRow(ctx, sql, args...).Scan(&s); err != nil {
		t.Fatal(err)
	}
	return s
}

// goldenCounts são as conciliações e as linhas tiradas do automático no rodar_matching da origem.
func goldenCounts(t *testing.T) (matches, reasons int) {
	t.Helper()
	raw, err := os.ReadFile(matchingGolden)
	if err != nil {
		t.Fatal(err)
	}
	var g struct {
		Scenario struct {
			Reasons map[string]string `json:"motivos"`
			Result  struct {
				Matches []json.RawMessage `json:"matches"`
			} `json:"resultado"`
		} `json:"cenario"`
	}
	if err := json.Unmarshal(raw, &g); err != nil {
		t.Fatal(err)
	}
	return len(g.Scenario.Result.Matches), len(g.Scenario.Reasons)
}

func TestTEST027_CleanDatabaseSeedAndFixturesGiveTheDemoJourney(t *testing.T) {
	e := setup(t)
	var log bytes.Buffer
	rep, err := fixtures.Load(ctx, e.deps, e.opts, &log)
	if err != nil {
		t.Fatalf("%v\n%s", err, log.String())
	}
	horizonte := e.scalar(t, `SELECT id::text FROM companies WHERE internal_code = '111'`)

	// Controle: as 21 empresas e a Horizonte com obrigações nas 4 competências e o Contábil de
	// 08/2026 aberto, com a obrigação Conciliação por fazer (o seed não a conclui).
	if n := e.count(t, `SELECT count(*) FROM companies`); n != seed.DemoCompanyCount || n != 21 {
		t.Fatalf("empresas = %d", n)
	}
	if n := e.count(t, `SELECT count(DISTINCT i.competence_id) FROM obligation_instances i WHERE i.company_id = $1`, horizonte); n != 4 {
		t.Fatalf("competências da 111 = %d, want 4", n)
	}
	if s := e.scalar(t, `SELECT p.state FROM company_module_periods p JOIN competencies k ON k.id = p.competence_id
		WHERE p.company_id = $1 AND p.module_code = 'ACCOUNTING' AND k.competence_key = '2026-08'`, horizonte); s != "OPEN" {
		t.Fatalf("Contábil 08/2026 da 111 = %s", s)
	}
	if n := e.count(t, `SELECT count(*) FROM obligation_instances i JOIN competencies k ON k.id = i.competence_id
		JOIN obligation_types t ON t.id = i.obligation_type_id
		JOIN status_definitions d ON d.status_set_id = t.status_set_id AND d.code = i.status_code
		WHERE i.company_id = $1 AND k.competence_key = '2026-08' AND t.code = 'ACCOUNTING_RECONCILIATION' AND i.applicable AND NOT d.is_final`,
		horizonte); n != 1 {
		t.Fatal("a obrigação Conciliação de 08/2026 da 111 deveria estar por fazer")
	}

	// Fator R: a carteira do seed com os quatro casos (vermelho, amarelo, verde e dados insuficientes).
	var portfolio struct {
		Linhas []struct {
			Company struct {
				InternalCode string `json:"internal_code"`
			} `json:"company"`
			Status   string  `json:"status"`
			Semaforo *string `json:"semaforo"`
		} `json:"linhas"`
	}
	e.srv.Get("/api/v1/factor-r/portfolio?pa=2026-10").MustStatus(t, http.StatusOK).JSON(t, &portfolio)
	lights := map[string]string{}
	for _, l := range portfolio.Linhas {
		light := l.Status
		if l.Semaforo != nil {
			light = *l.Semaforo
		}
		lights[l.Company.InternalCode] = light
	}
	if _, ok := lights["205"]; !ok || len(lights) != 5 || lights["208"] != "vermelho" || lights["206"] != "amarelo" || lights["202"] != "verde" ||
		lights["210"] != "dados_insuficientes" {
		t.Fatalf("carteira do Fator R = %v", lights)
	}

	// Conciliação: o que a preparação criou, pelos serviços, e o resultado da origem.
	if rep.Locked != "" || rep.BankAccounts != 2 || rep.ChartAccounts != 16 || rep.Entries != 35 || rep.Uploaded != 15 || rep.AlreadyUploaded != 0 {
		t.Fatalf("preparação = %+v\n%s", rep, log.String())
	}
	if fmt.Sprint(rep.Documents) != fmt.Sprint(map[string]int{reconciliation.DocImported: 15}) {
		t.Fatalf("documentos = %v", rep.Documents)
	}
	if n := e.gw.calls.Load(); n != 0 {
		t.Fatalf("a preparação chamou o modelo %d vezes", n)
	}
	matches, reasons := goldenCounts(t)
	s := rep.Summary
	if s == nil || s.Reconciled != matches || s.Divergent != reasons+1 || s.WithoutReceipt != 3 || s.AwaitingReview != 0 {
		t.Fatalf("rodada = %+v (origem: %d conciliações, %d tiradas do automático)", s, matches, reasons)
	}
	if n := e.count(t, `SELECT count(*) FROM statement_imports WHERE company_id = $1 AND bank_account_id IS NOT NULL`, horizonte); n != 2 {
		t.Fatalf("extratos ligados às contas cadastradas = %d", n)
	}
	if n := e.count(t, `SELECT count(*) FROM reconciliation_documents WHERE company_id = $1 AND competence = '2026-08-01' AND object_version_id IS NOT NULL`,
		horizonte); n != 15 {
		t.Fatalf("originais no MinIO versionado na competência = %d", n)
	}
	var printed bytes.Buffer
	rep.Write(&printed, "demo/pgdas")
	t.Log("\n" + printed.String())
	for _, want := range []string{"documentos do cenário: importado 15", "extratos do PGDAS-D em demo/pgdas: 1_iris_veterinaria"} {
		if !strings.Contains(printed.String(), want) {
			t.Fatalf("resumo sem %q:\n%s", want, printed.String())
		}
	}

	// A jornada pela API: a ficha mostra a conciliação em andamento, e o Contábil não fecha antes
	// dela (TASK-023).
	var overview struct {
		Reconciliation struct {
			State              string `json:"state"`
			Statements         int    `json:"statements"`
			UsesReconciliation bool   `json:"uses_reconciliation"`
			AccountingState    string `json:"accounting_state"`
			BlocksAccounting   bool   `json:"blocks_accounting"`
		} `json:"reconciliation"`
	}
	e.srv.Get("/api/v1/companies/"+horizonte+"/overview?competence=2026-08").MustStatus(t, http.StatusOK).JSON(t, &overview)
	if r := overview.Reconciliation; r.State != "em_andamento" || r.Statements != 2 || !r.UsesReconciliation || r.AccountingState != "OPEN" || !r.BlocksAccounting {
		t.Fatalf("Conciliação na ficha = %+v", r)
	}
	var periodID string
	var version int
	if err := e.pool.QueryRow(ctx, `SELECT p.id::text, p.version FROM company_module_periods p JOIN competencies k ON k.id = p.competence_id
		WHERE p.company_id = $1 AND p.module_code = 'ACCOUNTING' AND k.competence_key = '2026-08'`, horizonte).Scan(&periodID, &version); err != nil {
		t.Fatal(err)
	}
	if code := e.srv.Post("/api/v1/company-module-periods/"+periodID+"/close", map[string]any{"version": version}).
		MustStatus(t, http.StatusUnprocessableEntity).ErrorCode(t); code != "RECONCILIATION_NOT_FINAL" {
		t.Fatalf("fechar o Contábil = %s", code)
	}

	// Os extratos do PGDAS-D no inbox, como o LEIA-ME diz.
	pgdasJourney(t, e, rep.Pgdas)

	// De novo: nada duplica e o resultado é o mesmo.
	before := snapshot(t, e)
	firstFiles := readAll(t, e.opts.OutDir)
	log.Reset()
	again, err := fixtures.Load(ctx, e.deps, e.opts, &log)
	if err != nil {
		t.Fatalf("%v\n%s", err, log.String())
	}
	if again.BankAccounts != 0 || again.ChartAccounts != 0 || again.Entries != 0 || again.Uploaded != 0 || again.AlreadyUploaded != 15 ||
		again.Summary == nil || again.Summary.NewMatches != 0 || again.Summary.Reconciled != s.Reconciled || again.Summary.Divergent != s.Divergent {
		t.Fatalf("segunda preparação = %+v\n%s", again, log.String())
	}
	if after := snapshot(t, e); after != before {
		t.Fatalf("a segunda preparação mudou os dados:\n%s\n%s", before, after)
	}
	if files := readAll(t, e.opts.OutDir); fmt.Sprint(files) != fmt.Sprint(firstFiles) {
		t.Fatal("os extratos do PGDAS-D mudaram na segunda preparação")
	}
}

// snapshot resume o que a preparação grava.
func snapshot(t *testing.T, e env) string {
	t.Helper()
	parts := make([]string, 0, 6)
	for _, table := range []string{"bank_accounts", "chart_accounts", "client_entries", "reconciliation_documents", "statement_transactions", "reconciliations"} {
		parts = append(parts, fmt.Sprintf("%s=%d", table, e.count(t, "SELECT count(*) FROM "+table)))
	}
	return strings.Join(parts, " ")
}

func readAll(t *testing.T, dir string) map[string]string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, f := range entries {
		b, err := os.ReadFile(filepath.Join(dir, f.Name()))
		if err != nil {
			t.Fatal(err)
		}
		out[f.Name()] = string(b)
	}
	return out
}

type pgdasDoc struct {
	ID        string             `json:"id"`
	Status    string             `json:"status"`
	Motivo    *string            `json:"motivo"`
	CompanyID *string            `json:"company_id"`
	TraceID   *string            `json:"trace_id"`
	Campos    map[string]*string `json:"campos"`
	Sugestao  map[string]any     `json:"sugestao_cadastro"`
}

// send envia o extrato ao inbox e roda o worker.
func (e env) send(t *testing.T, path string) pgdasDoc {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile("file", filepath.Base(path))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.srv.URL+"/api/v1/factor-r/pgdas", &body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := e.srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var up struct {
		DocumentID string `json:"document_id"`
	}
	if resp.StatusCode != http.StatusAccepted {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("envio de %s = %d %s", path, resp.StatusCode, b)
	}
	if err := json.NewDecoder(resp.Body).Decode(&up); err != nil {
		t.Fatal(err)
	}
	if err := e.drain(ctx); err != nil {
		t.Fatal(err)
	}
	return e.doc(t, up.DocumentID)
}

func (e env) doc(t *testing.T, id string) pgdasDoc {
	t.Helper()
	var d pgdasDoc
	e.srv.Get("/api/v1/factor-r/pgdas/"+id).MustStatus(t, http.StatusOK).JSON(t, &d)
	return d
}

func (e env) gold(t *testing.T, traceID string) string {
	t.Helper()
	rows, err := e.pool.Query(ctx, `SELECT field || '=' || status FROM ai_gold_evals WHERE trace_id = $1`, traceID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	sort.Strings(out)
	return strings.Join(out, " ")
}

func str(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// pgdasJourney: (1) o extrato da Íris cai em revisão (CNPJ fictício do seed, sem dígito válido) e se
// vincula à mão, com a nota ouro conferindo os números com o motor; (2) o de fora do cadastro
// oferece o cadastro, que cria a empresa e vincula; (3) o PA seguinte da mesma empresa se vincula
// sozinho.
func pgdasJourney(t *testing.T, e env, files []fixtures.PgdasFile) {
	t.Helper()
	names := make([]string, 0, len(files))
	for _, f := range files {
		names = append(names, f.Name)
	}
	want := []string{"1_iris_veterinaria_pa_2026-09.txt", "2_cnpj_fora_do_cadastro_pa_2026-08.txt", "3_cnpj_cadastrado_pa_2026-09.txt"}
	if fmt.Sprint(names) != fmt.Sprint(want) {
		t.Fatalf("extratos = %v", names)
	}
	readme, err := os.ReadFile(filepath.Join(e.opts.OutDir, "LEIA-ME.txt"))
	if err != nil || !strings.Contains(string(readme), want[0]) {
		t.Fatalf("LEIA-ME: %v %s", err, readme)
	}
	iris := e.scalar(t, `SELECT id::text FROM companies WHERE internal_code = '208'`)

	first := e.send(t, filepath.Join(e.opts.OutDir, want[0]))
	if first.Status != inbox.StatusNeedsReview || str(first.Motivo) != "cnpj_nao_encontrado" || first.Sugestao != nil ||
		str(first.Campos["cnpj"]) != "00100008000108" || str(first.Campos["pa"]) != "2026-09" || str(first.Campos["anexo"]) != "V" {
		t.Fatalf("extrato da Íris = %+v", first)
	}
	var linked pgdasDoc
	e.srv.Post("/api/v1/factor-r/pgdas/"+first.ID+"/link", map[string]any{"company_id": iris}).MustStatus(t, http.StatusOK).JSON(t, &linked)
	if linked.Status != inbox.StatusLinked || str(linked.CompanyID) != iris {
		t.Fatalf("vínculo manual = %+v", linked)
	}
	if g := e.gold(t, str(linked.TraceID)); g != "anexo=ok cnpj=ok fator_r=ok fs12=ok pa=ouro_indisponivel rbt12=ok" {
		t.Fatalf("nota ouro do extrato da Íris = %s", g)
	}

	second := e.send(t, filepath.Join(e.opts.OutDir, want[1]))
	if second.Status != inbox.StatusNeedsReview || str(second.Motivo) != "cnpj_nao_encontrado" || second.Sugestao == nil {
		t.Fatalf("extrato de fora do cadastro = %+v", second)
	}
	var created struct {
		Documento pgdasDoc `json:"documento"`
		Empresa   struct {
			ID string `json:"id"`
		} `json:"empresa"`
	}
	e.srv.Post("/api/v1/factor-r/pgdas/"+second.ID+"/create-company", map[string]any{"empresa": map[string]any{
		"internal_code": "DEMO-PGDAS", "legal_name": "CLINICA EXEMPLO LTDA", "cnpj": "11.222.333/0001-81", "service_start_date": "2026-01-01",
		"payroll_type": "PAYROLL", "tax_regime": "SIMPLES_NACIONAL", "factor_r": true,
	}}).MustStatus(t, http.StatusCreated).JSON(t, &created)
	if created.Documento.Status != inbox.StatusLinked || str(created.Documento.CompanyID) != created.Empresa.ID {
		t.Fatalf("cadastro pelo extrato = %+v", created)
	}

	third := e.send(t, filepath.Join(e.opts.OutDir, want[2]))
	if third.Status != inbox.StatusLinked || str(third.CompanyID) != created.Empresa.ID || str(third.Campos["pa"]) != "2026-09" {
		t.Fatalf("extrato do PA seguinte = %+v", third)
	}
}
