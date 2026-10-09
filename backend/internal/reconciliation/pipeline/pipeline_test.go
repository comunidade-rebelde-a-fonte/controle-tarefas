package pipeline_test

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
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/text/encoding/charmap"

	"controle/internal/ai"
	"controle/internal/ai/aliases"
	"controle/internal/ai/litellm"
	"controle/internal/ai/tracer"
	"controle/internal/jobs"
	"controle/internal/platform/clock"
	"controle/internal/platform/config"
	"controle/internal/platform/decimal"
	"controle/internal/platform/objectstore"
	"controle/internal/platform/pdftext"
	"controle/internal/reconciliation/pipeline"
	"controle/internal/testutil"
	"controle/internal/user"
)

var (
	ctx   = context.Background()
	pdf   = pdftext.New(0)
	quiet = slog.New(slog.NewTextHandler(io.Discard, nil))
)

// --- Gateway simulado: responde cada chamada pela função do teste e guarda o pedido ---

type fakeGateway struct {
	mu       sync.Mutex
	requests []litellm.Request
	reply    func(n int, req litellm.Request) (string, error)
}

func (g *fakeGateway) Chat(_ context.Context, req litellm.Request) (litellm.Response, error) {
	g.mu.Lock()
	g.requests = append(g.requests, req)
	n := len(g.requests)
	reply := g.reply
	g.mu.Unlock()
	if reply == nil {
		return litellm.Response{}, errors.New("gateway simulado sem resposta configurada")
	}
	content, err := reply(n, req)
	if err != nil {
		return litellm.Response{}, err
	}
	return litellm.Response{
		Content: content, Model: req.Model.Name + "-2026-06-01", Usage: litellm.Usage{PromptTokens: 100, CompletionTokens: 50},
		CostUSD: "0.00012345", Latency: 15 * time.Millisecond,
	}, nil
}

func (g *fakeGateway) calls() []litellm.Request {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]litellm.Request(nil), g.requests...)
}

// --- Ambiente: API e worker sobre Postgres e MinIO reais ---

type env struct {
	srv    *testutil.Server
	pool   *pgxpool.Pool
	gw     *fakeGateway
	runner *jobs.Runner
}

var models = ai.Defaults{
	ai.Triador:       {Provider: "openai", Name: "gpt-6-luna"},
	ai.Extrator:      {Provider: "openai", Name: "gpt-6-sol"},
	ai.ExtratorForte: {Provider: "openai", Name: "gpt-6-astra"},
}

func setup(t *testing.T) env {
	t.Helper()
	gw := &fakeGateway{}
	e := setupWith(t, gw, models, pipeline.Config{TriageThreshold: decimal.MustParse("0.70")})
	e.gw = gw
	return e
}

// setupWith monta o ambiente com o gateway, os modelos padrão e a configuração dados (a avaliação
// do golden usa o LiteLLM real e só o agente).
func setupWith(t *testing.T, gw pipeline.Gateway, defaults ai.Defaults, cfg pipeline.Config) env {
	t.Helper()
	m := testutil.NewMinIO(t)
	pool := testutil.NewDB(t)
	srv := testutil.NewServer(t, pool, testutil.FixedClock(t, "2026-09-15T12:00:00-03:00"), func(c *config.Config) {
		c.S3Endpoint, c.S3Region, c.S3AccessKey, c.S3SecretKey, c.S3Bucket = m.Endpoint, "us-east-1", m.AccessKey, config.Secret(m.SecretKey), m.Bucket
	})
	sys := clock.NewSystem(time.UTC)
	q := jobs.NewQueue(sys, 3)
	originals := objectstore.New(objectstore.Config{Endpoint: m.Endpoint, Region: "us-east-1", AccessKey: m.AccessKey, SecretKey: m.SecretKey, Bucket: m.Bucket})
	proc := pipeline.New(sys, tracer.New(sys, q), q, originals, pdf, gw, aliases.NewResolver(defaults), cfg, quiet)
	runner := jobs.NewRunner(pool, sys, quiet, jobs.RunnerConfig{WorkerID: "w-conciliacao", Concurrency: 1, PollInterval: time.Millisecond, Lease: 10 * time.Minute},
		map[string]jobs.Handler{pipeline.KindTriage: proc.HandleTriage, pipeline.KindStatementExtract: proc.HandleExtract})
	return env{srv: srv, pool: pool, runner: runner}
}

// process executa os jobs prontos (o worker).
func (e env) process(t *testing.T) {
	t.Helper()
	for range 20 {
		ok, err := e.runner.ProcessOne(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			return
		}
	}
	t.Fatal("fila não esvaziou")
}

func (e env) upload(t *testing.T, name string, data []byte, fields map[string]string) string {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	for k, v := range fields {
		if err := mw.WriteField(k, v); err != nil {
			t.Fatal(err)
		}
	}
	fw, err := mw.CreateFormFile("file", name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.srv.URL+"/api/v1/reconciliation/documents", &body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := e.srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("envio = %d %s", resp.StatusCode, b)
	}
	var out struct {
		DocumentID string `json:"document_id"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out.DocumentID
}

type attemptOut struct {
	Alias         string          `json:"alias"`
	Attempt       int             `json:"attempt"`
	Model         *string         `json:"model"`
	PromptVersion *string         `json:"prompt_version"`
	Validation    json.RawMessage `json:"validation"`
	TokensIn      *int            `json:"tokens_in"`
	CostUSD       *string         `json:"cost_usd"`
	LatencyMs     *int            `json:"latency_ms"`
	TraceID       *string         `json:"trace_id"`
	ActorID       *string         `json:"actor_id"`
	Change        *string         `json:"change"`
	Error         *string         `json:"error"`
}

type docOut struct {
	ID               string         `json:"id"`
	Status           string         `json:"status"`
	Kind             *string        `json:"kind"`
	CompanyID        *string        `json:"company_id"`
	Bank             *string        `json:"bank"`
	Branch           *string        `json:"branch"`
	Account          *string        `json:"account"`
	PeriodStart      *string        `json:"period_start"`
	TriageConfidence *string        `json:"triage_confidence"`
	TriageTraceID    *string        `json:"triage_trace_id"`
	Diagnosis        map[string]any `json:"diagnosis"`
	Job              *struct {
		ID            string  `json:"id"`
		Status        string  `json:"status"`
		LastErrorCode *string `json:"last_error_code"`
	} `json:"job"`
	Import *struct {
		ID             string          `json:"id"`
		CompanyID      string          `json:"company_id"`
		BankAccountID  *string         `json:"bank_account_id"`
		OpeningBalance string          `json:"opening_balance"`
		ClosingBalance string          `json:"closing_balance"`
		Attempts       int             `json:"attempts"`
		Transactions   int             `json:"transactions"`
		Validation     json.RawMessage `json:"validation"`
		TraceID        *string         `json:"trace_id"`
	} `json:"import"`
	Attempts []attemptOut `json:"attempts"`
}

func (e env) doc(t *testing.T, id string) docOut {
	t.Helper()
	var d docOut
	e.srv.Get("/api/v1/reconciliation/documents/"+id).MustStatus(t, http.StatusOK).JSON(t, &d)
	return d
}

func (e env) count(t *testing.T, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := e.pool.QueryRow(ctx, sql, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (e env) company(t *testing.T, code, cnpj string) string {
	t.Helper()
	var id string
	if err := e.pool.QueryRow(ctx, `INSERT INTO companies (internal_code, legal_name, cnpj, service_start_date, payroll_type, created_by, updated_by)
		VALUES ($1, 'Empresa ' || $1, $2, '2025-01-01', 'PAYROLL', $3, $3) RETURNING id`, code, cnpj, user.AdminID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func (e env) bankAccount(t *testing.T, companyID, bank, branch, account string) string {
	t.Helper()
	var id string
	if err := e.pool.QueryRow(ctx, `INSERT INTO bank_accounts (company_id, bank, branch, account) VALUES ($1, $2, $3, $4) RETURNING id`,
		companyID, bank, branch, account).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func (e env) closeAccounting(t *testing.T, companyID, comp string) {
	t.Helper()
	var compID string
	y, m := comp[:4], comp[5:]
	if err := e.pool.QueryRow(ctx, `INSERT INTO competencies (competence_key, year, month, first_day) VALUES ($1, $2::int, $3::int, $4::date)
		ON CONFLICT (competence_key) DO UPDATE SET competence_key = EXCLUDED.competence_key RETURNING id`, comp, y, m, comp+"-01").Scan(&compID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.pool.Exec(ctx, `INSERT INTO company_module_periods (company_id, competence_id, module_code, state, closed_at, closed_by)
		VALUES ($1, $2, 'ACCOUNTING', 'CLOSED', now(), $3)`, companyID, compID, user.AdminID); err != nil {
		t.Fatal(err)
	}
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("../testdata/conciliacao/extratos/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// textPDF monta um PDF com o texto dado (uma linha por item), fora do leiaute do cenário.
func textPDF(lines ...string) []byte {
	var content strings.Builder
	content.WriteString("BT /F1 10 Tf 50 800 Td 14 TL")
	for _, l := range lines {
		cp, err := charmap.Windows1252.NewEncoder().String(l)
		if err != nil {
			panic(err)
		}
		fmt.Fprintf(&content, " (%s) Tj T*", strings.NewReplacer(`(`, `\(`, `)`, `\)`).Replace(cp))
	}
	content.WriteString(" ET")
	c := content.String()
	objs := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(c), c),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>",
	}
	var b bytes.Buffer
	b.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objs))
	for i, o := range objs {
		offsets[i] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(objs)+1)
	for _, off := range offsets {
		fmt.Fprintf(&b, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objs)+1, xref)
	return b.Bytes()
}

// itauPDF é um extrato fora do leiaute do leitor determinístico (vai ao triador e ao extrator).
func itauPDF(t *testing.T) []byte {
	return textPDF("Itau Unibanco - Extrato de conta corrente", "Agencia 0001 Conta 12345-6", "Periodo 01/03/2026 a 31/03/2026",
		"SALDO ANTERIOR 100,00", "02/03 PIX RECEBIDO 50,00 C", "03/03 TARIFA 20,00 D", "SALDO FINAL 130,00", t.Name())
}

func triageJSON(kind string, confidence string, bank *string) string {
	b := "null"
	if bank != nil {
		b = `"` + *bank + `"`
	}
	return `{"tipo":"` + kind + `","banco":` + b + `,"agencia":"0001","conta":"12345-6","periodo_inicio":"2026-03-01",` +
		`"periodo_fim":"2026-03-31","confianca":` + confidence + `,"justificativa":"cabeçalho do banco"}`
}

func ptr(s string) *string { return &s }

// stmt monta a saída do extrator: saldos e linhas (valor, sentido), com o leiaute descoberto.
func stmt(opening, closing string, lines ...string) string {
	var txs []string
	for i, l := range lines {
		amount, dir, _ := strings.Cut(l, " ")
		txs = append(txs, fmt.Sprintf(`{"ordem":%d,"data":"2026-03-%02d","descricao":"LINHA %d","documento":null,"valor":"%s","sentido":"%s","saldo_apos":null,"confianca":0.9}`,
			i+1, i+2, i+1, amount, dir))
	}
	return `{"banco":"itau","agencia":"0001","conta":"12345-6","periodo_inicio":"2026-03-01","periodo_fim":"2026-03-31",` +
		`"saldo_inicial":"` + opening + `","saldo_final":"` + closing + `","leiaute":{"pagina_inicio_tabela":1,"paginas_ignorar":[],` +
		`"formato_data":"DD/MM","separador_decimal":",","representacao_sinal":"sufixo C/D","localizacao_saldos":"topo e rodapé",` +
		`"estrategia":"texto_nativo"},"transacoes":[` + strings.Join(txs, ",") + `]}`
}

// scripted responde a triagem e, depois, as tentativas do extrator, na ordem.
type scripted struct {
	triage   string
	attempts []string
	n        int
}

func (s *scripted) reply(_ int, req litellm.Request) (string, error) {
	if req.JSONSchema != nil && req.JSONSchema.Name == "resultado_triagem" {
		return s.triage, nil
	}
	if s.n >= len(s.attempts) {
		return "", errors.New("roteiro do extrator acabou")
	}
	out := s.attempts[s.n]
	s.n++
	return out, nil
}

// extractorCalls são os pedidos ao extrator (sem a triagem).
func (e env) extractorCalls() []litellm.Request {
	var out []litellm.Request
	for _, r := range e.gw.calls() {
		if r.JSONSchema != nil && r.JSONSchema.Name == "extrato_extraido" {
			out = append(out, r)
		}
	}
	return out
}
