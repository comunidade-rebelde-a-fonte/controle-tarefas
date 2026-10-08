package inbox_test

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
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"controle/internal/ai/tracer"
	"controle/internal/company"
	"controle/internal/factorr"
	"controle/internal/factorr/inbox"
	"controle/internal/jobs"
	"controle/internal/platform/apperr"
	"controle/internal/platform/clock"
	"controle/internal/platform/config"
	"controle/internal/platform/pdftext"
	"controle/internal/testutil"
	"controle/internal/testutil/seeded"
)

// TEST-012: Inbox PGDAS-D ponta a ponta contra Postgres real — upload (API), job pgdas_parse
// (worker), decisão do parser_pgdas, gravação segura da série, revisão, vínculo, cadastro pelo
// extrato e nota ouro. Porte de tests/agents/test_parser_pgdas.py, tests/api/test_inbox_upload.py
// e tests/api/test_inbox_cadastro.py da origem, com as fixtures da origem.

const (
	cnpjPadrao   = "11222333000181" // txt_padrao: PA 09/2026, RPA 50.000,00
	cnpjFormat   = "11.222.333/0001-81"
	fixturesDir  = "../pgdas/testdata/fixtures"
	disclaimerPt = "O PGDAS-D da Receita Federal prevalece."
)

var (
	extractor = pdftext.New(0)
	quiet     = slog.New(slog.NewTextHandler(io.Discard, nil))
	codeSeq   atomic.Int32
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	docs, _ := filepath.Glob(filepath.Join(fixturesDir, name, "documento.*"))
	if len(docs) != 1 {
		t.Fatalf("fixture %s ausente", name)
	}
	b, err := os.ReadFile(docs[0])
	if err != nil {
		t.Fatal(err)
	}
	return b
}

type env struct {
	srv    *testutil.Server
	dir    string
	runner *jobs.Runner
	tr     *tracer.Tracer
	queue  *jobs.Queue
	sys    clock.Clock
}

func setup(t *testing.T) env {
	t.Helper()
	pool := seeded.NewCompaniesDB(t)
	dir := t.TempDir()
	srv := testutil.NewServer(t, pool, testutil.FixedClock(t, "2026-10-08T12:00:00Z"), func(c *config.Config) { c.FileStorageDir = dir })
	// A fila e o tracer usam o relógio real (tempo operacional), como na API e no worker.
	sys := clock.NewSystem(time.UTC)
	q := jobs.NewQueue(sys, 3)
	tr := tracer.New(sys, q)
	agent := inbox.NewAgent(sys, tr, inbox.NewStorage(dir), extractor)
	runner := jobs.NewRunner(pool, sys, quiet, jobs.RunnerConfig{WorkerID: "w-pgdas", Concurrency: 1, PollInterval: time.Millisecond, Lease: time.Minute},
		map[string]jobs.Handler{inbox.KindParse: agent.HandleParse})
	return env{srv: srv, dir: dir, runner: runner, tr: tr, queue: q, sys: sys}
}

type uploadOut struct {
	DocumentID string  `json:"document_id"`
	JobID      *string `json:"job_id"`
	Status     string  `json:"status"`
	Duplicate  bool    `json:"duplicate"`
}

type docOut struct {
	ID           string             `json:"id"`
	Status       string             `json:"status"`
	Motivo       *string            `json:"motivo"`
	CompanyID    *string            `json:"company_id"`
	NomeOriginal *string            `json:"nome_original"`
	Mime         string             `json:"mime"`
	Tamanho      int                `json:"tamanho_bytes"`
	Campos       map[string]*string `json:"campos"`
	Motivos      []string           `json:"motivos"`
	Confianca    *string            `json:"confianca"`
	TraceID      *string            `json:"trace_id"`
	TextoAgente  *string            `json:"texto_agente"`
	Job          *struct {
		Status        string  `json:"status"`
		Attempts      int     `json:"attempts"`
		LastErrorCode *string `json:"last_error_code"`
	} `json:"job"`
	Sugestao map[string]any `json:"sugestao_cadastro"`
	Version  int            `json:"version"`
}

func (e env) upload(t *testing.T, name string, data []byte) testutil.Response {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
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
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, e.srv.URL+"/api/v1/factor-r/pgdas", &body)
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
	return testutil.Response{Status: resp.StatusCode, Header: resp.Header, Body: b}
}

// process executa os jobs pendentes (o worker).
func (e env) process(t *testing.T) {
	t.Helper()
	for {
		ok, err := e.runner.ProcessOne(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			return
		}
	}
}

func (e env) doc(t *testing.T, id string) docOut {
	t.Helper()
	var d docOut
	e.srv.Get("/api/v1/factor-r/pgdas/"+id).MustStatus(t, http.StatusOK).JSON(t, &d)
	return d
}

// send envia o arquivo, processa o job e devolve o documento.
func (e env) send(t *testing.T, name string, data []byte) docOut {
	t.Helper()
	var u uploadOut
	e.upload(t, name, data).MustStatus(t, http.StatusAccepted).JSON(t, &u)
	e.process(t)
	return e.doc(t, u.DocumentID)
}

func companyBody(cnpjValue, name string, factorR bool) map[string]any {
	return map[string]any{
		"internal_code": fmt.Sprintf("PG%03d", codeSeq.Add(1)), "legal_name": name, "cnpj": cnpjValue,
		"service_start_date": "2024-01-01", "payroll_type": "PAYROLL", "tax_regime": "SIMPLES_NACIONAL", "factor_r": factorR,
	}
}

func (e env) company(t *testing.T, cnpjValue string) string {
	t.Helper()
	var c struct {
		ID string `json:"id"`
	}
	e.srv.Post("/api/v1/companies", companyBody(cnpjValue, "Clínica Exemplo", true)).MustStatus(t, http.StatusCreated).JSON(t, &c)
	return c.ID
}

type movement struct {
	Competence string
	Receita    string
	Split      string // pró-labore|salários|CPP|FGTS
	Declarada  *string
	FolhaMes   string
	Origem     string
	Observacao *string
	DocID      *string
	Version    int
}

func (e env) movements(t *testing.T, companyID string) map[string]movement {
	t.Helper()
	rows, err := e.srv.Pool.Query(context.Background(), `
		SELECT to_char(competence, 'YYYY-MM'), receita_bruta::text,
		       concat_ws('|', pro_labore, salarios_13_ferias, cpp_recolhida, fgts_recolhido),
		       folha_total_declarada_pgdas::text, folha_mes::text, origem, observacao, pgdas_document_id::text, version
		FROM factor_r_movements WHERE company_id = $1`, companyID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]movement{}
	for rows.Next() {
		var m movement
		if err := rows.Scan(&m.Competence, &m.Receita, &m.Split, &m.Declarada, &m.FolhaMes, &m.Origem, &m.Observacao, &m.DocID, &m.Version); err != nil {
			t.Fatal(err)
		}
		out[m.Competence] = m
	}
	return out
}

func (e env) count(t *testing.T, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := e.srv.Pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (e env) decision(t *testing.T, traceID string) (string, map[string]any) {
	t.Helper()
	var kind string
	var raw []byte
	if err := e.srv.Pool.QueryRow(context.Background(), `SELECT kind, data FROM ai_decisions WHERE trace_id = $1`, traceID).Scan(&kind, &raw); err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	return kind, m
}

func (e env) gold(t *testing.T, traceID string) map[string]string {
	t.Helper()
	rows, err := e.srv.Pool.Query(context.Background(), `SELECT field, status FROM ai_gold_evals WHERE trace_id = $1`, traceID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var f, s string
		if err := rows.Scan(&f, &s); err != nil {
			t.Fatal(err)
		}
		out[f] = s
	}
	return out
}

func str(p *string) string {
	if p == nil {
		return "<nil>"
	}
	return *p
}

// --- Upload e armazenamento [AC-014, 09 §5] ---

func TestUpload_AcceptedStoredAndProcessedByJob(t *testing.T) {
	e := setup(t)
	var u uploadOut
	e.upload(t, "extrato.txt", fixture(t, "txt_padrao")).MustStatus(t, http.StatusAccepted).JSON(t, &u)
	if u.Status != inbox.StatusReceived || u.JobID == nil || u.Duplicate {
		t.Fatalf("upload = %+v", u)
	}
	// A requisição não esperou o processamento: o documento está recebido, com o job pendente.
	d := e.doc(t, u.DocumentID)
	if d.Status != inbox.StatusReceived || d.Job == nil || d.Job.Status != jobs.StatusPending || d.TraceID != nil || d.Mime != pdftext.MimeText {
		t.Fatalf("antes do worker = %+v", d)
	}
	files, _ := filepath.Glob(filepath.Join(e.dir, "pgdas", "*"))
	if len(files) != 1 || len(filepath.Base(files[0])) != 64 {
		t.Fatalf("arquivo guardado = %v (só o sha256 como nome)", files)
	}
	e.process(t)
	d = e.doc(t, u.DocumentID)
	if d.Status != inbox.StatusNeedsReview || d.Job.Status != jobs.StatusDone || d.TraceID == nil || len(*d.TraceID) != 32 {
		t.Fatalf("depois do worker = %+v", d)
	}
	if !strings.HasSuffix(str(d.TextoAgente), disclaimerPt) {
		t.Errorf("texto do agente sem disclaimer: %q", str(d.TextoAgente))
	}
	// Trace local [AC-029] e envio ao Langfuse enfileirado [AC-030].
	if n := e.count(t, `SELECT count(*) FROM ai_runs WHERE trace_id = $1 AND agent = 'parser_pgdas' AND trigger = 'upload' AND status = 'needs_review'`, *d.TraceID); n != 1 {
		t.Fatalf("corrida = %d", n)
	}
	if n := e.count(t, `SELECT count(*) FROM jobs WHERE kind = 'langfuse_sync' AND dedupe_key = $1`, *d.TraceID); n != 1 {
		t.Fatalf("sincronização enfileirada = %d", n)
	}
}

func TestUpload_TypeByContentSizeAndName(t *testing.T) {
	e := setup(t)
	for _, bad := range [][]byte{[]byte("MZ\x90\x00\x03\x00\x00\x00executavel"), []byte("\x89PNG\r\n\x1a\n\x00\x00imagem"), {}} {
		if code := e.upload(t, "extrato.pdf", bad).MustStatus(t, http.StatusUnsupportedMediaType).ErrorCode(t); code != "UNSUPPORTED_FILE" {
			t.Errorf("tipo inválido = %s", code)
		}
	}
	// A extensão não importa: PDF com nome .txt é PDF.
	var u uploadOut
	e.upload(t, "extrato.txt", []byte("%PDF-1.4\n1 0 obj<<>>endobj\ntrailer<<>>\n%%EOF\n")).MustStatus(t, http.StatusAccepted).JSON(t, &u)
	if d := e.doc(t, u.DocumentID); d.Mime != pdftext.MimePDF {
		t.Errorf("mime = %s", d.Mime)
	}
	big := bytes.Repeat([]byte("a"), inbox.MaxFileSize+1)
	if code := e.upload(t, "grande.txt", big).MustStatus(t, http.StatusRequestEntityTooLarge).ErrorCode(t); code != "FILE_TOO_LARGE" {
		t.Errorf("arquivo grande = %s", code)
	}
	// Nome com caminho: só o nome fica como metadado; o arquivo fica só sob pgdas/.
	e.upload(t, "../../../etc/passwd.txt", []byte("Extrato\nCNPJ: x\n")).MustStatus(t, http.StatusAccepted).JSON(t, &u)
	if d := e.doc(t, u.DocumentID); str(d.NomeOriginal) != "passwd.txt" {
		t.Errorf("nome original = %s", str(d.NomeOriginal))
	}
	_ = filepath.WalkDir(e.dir, func(p string, de os.DirEntry, err error) error {
		if err == nil && !de.IsDir() && filepath.Dir(p) != filepath.Join(e.dir, "pgdas") {
			t.Errorf("arquivo fora de pgdas/: %s", p)
		}
		return nil
	})
}

func TestUpload_DuplicateReturnsExistingWithoutNewJob(t *testing.T) {
	e := setup(t)
	var first, second uploadOut
	e.upload(t, "a.txt", fixture(t, "txt_padrao")).MustStatus(t, http.StatusAccepted).JSON(t, &first)
	e.upload(t, "outro-nome.txt", fixture(t, "txt_padrao")).MustStatus(t, http.StatusOK).JSON(t, &second)
	if second.DocumentID != first.DocumentID || !second.Duplicate || str(second.JobID) != str(first.JobID) {
		t.Fatalf("duplicado = %+v; primeiro = %+v", second, first)
	}
	var list struct {
		Total int            `json:"total"`
		Por   map[string]int `json:"contagem_por_status"`
	}
	e.srv.Get("/api/v1/factor-r/pgdas").MustStatus(t, http.StatusOK).JSON(t, &list)
	if list.Total != 1 || list.Por[inbox.StatusReceived] != 1 || e.count(t, `SELECT count(*) FROM jobs WHERE kind = 'pgdas_parse'`) != 1 {
		t.Fatalf("lista = %+v", list)
	}
}

func TestDownload_OriginalAsAttachment(t *testing.T) {
	e := setup(t)
	data := fixture(t, "txt_padrao")
	var u uploadOut
	e.upload(t, "extrato.txt", data).MustStatus(t, http.StatusAccepted).JSON(t, &u)
	r := e.srv.Get("/api/v1/factor-r/pgdas/"+u.DocumentID+"/file").MustStatus(t, http.StatusOK)
	if !bytes.Equal(r.Body, data) || !strings.HasPrefix(r.Header.Get("Content-Disposition"), "attachment") ||
		r.Header.Get("X-Content-Type-Options") != "nosniff" || r.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("download: headers %v", r.Header)
	}
	e.srv.Get("/api/v1/factor-r/pgdas/00000000-0000-0000-0000-000000000000/file").MustStatus(t, http.StatusNotFound)
}

// --- Decisão do agente no upload [AC-008, AC-009] ---

func TestKnownCNPJ_LinksAndCreatesRevenueInPACompetence(t *testing.T) {
	e := setup(t)
	id := e.company(t, cnpjFormat)
	d := e.send(t, "extrato.txt", fixture(t, "txt_padrao"))
	if d.Status != inbox.StatusLinked || str(d.CompanyID) != id || d.Motivo != nil {
		t.Fatalf("documento = %+v", d)
	}
	if !strings.Contains(str(d.TextoAgente), "competência 2026-09") || !strings.HasSuffix(str(d.TextoAgente), disclaimerPt) {
		t.Errorf("texto = %q", str(d.TextoAgente))
	}
	ms := e.movements(t, id)
	m, ok := ms["2026-09"]
	if !ok || len(ms) != 1 || m.Receita != "50000.00" || m.Split != "0.00|0.00|0.00|0.00" || m.Declarada != nil ||
		m.FolhaMes != "0.00" || m.Origem != "pgdas" || str(m.DocID) != d.ID {
		t.Fatalf("movimentos = %+v", ms)
	}
	kind, dec := e.decision(t, *d.TraceID)
	if kind != "pgdas_documento" || dec["movimento"] != "criado" || dec["competencia"] != "2026-09" || dec["limiar"] != "0.400000" {
		t.Fatalf("decisão = %s %v", kind, dec)
	}
	if n := e.count(t, `SELECT count(*) FROM ai_runs WHERE trace_id = $1 AND status = 'ok' AND company_id = $2`, *d.TraceID, id); n != 1 {
		t.Fatal("corrida ok ligada à empresa não encontrada")
	}
}

func TestExistingCompetenceIsNeverOverwritten(t *testing.T) {
	e := setup(t)
	id := e.company(t, cnpjFormat)
	e.srv.Put("/api/v1/companies/"+id+"/factor-r/movements/2026-09", map[string]any{
		"receita_bruta": "48000.00", "pro_labore": "7000.00", "salarios_13_ferias": "9000.00",
		"cpp_recolhida": "3200.00", "fgts_recolhido": "720.00", "observacao": "lançado pelo analista",
	}).MustStatus(t, http.StatusOK)
	before := e.movements(t, id)["2026-09"]
	d := e.send(t, "extrato.txt", fixture(t, "txt_padrao"))
	if d.Status != inbox.StatusLinked || !strings.Contains(str(d.TextoAgente), "já tinha lançamento") {
		t.Fatalf("documento = %+v", d)
	}
	after := e.movements(t, id)["2026-09"]
	if after.Receita != before.Receita || after.Split != before.Split || after.Origem != before.Origem ||
		str(after.Observacao) != str(before.Observacao) || after.Version != before.Version || after.DocID != nil {
		t.Fatalf("competência alterada: %+v → %+v", before, after)
	}
}

func TestUnknownCNPJ_NeedsReviewWithoutWriting(t *testing.T) {
	e := setup(t)
	other := e.company(t, "45.723.174/0001-10")
	d := e.send(t, "extrato.txt", fixture(t, "txt_padrao"))
	if d.Status != inbox.StatusNeedsReview || str(d.Motivo) != "cnpj_nao_encontrado" || d.CompanyID != nil {
		t.Fatalf("documento = %+v", d)
	}
	if len(e.movements(t, other)) != 0 || e.count(t, `SELECT count(*) FROM factor_r_movements WHERE origem = 'pgdas'`) != 0 {
		t.Fatal("o extrato em revisão gravou movimento")
	}
}

func TestLowConfidence_NeedsReview(t *testing.T) {
	e := setup(t)
	id := e.company(t, cnpjFormat)
	if _, err := e.srv.Pool.Exec(context.Background(), `UPDATE factor_r_policy SET limiar_confianca_parser = 0.99`); err != nil {
		t.Fatal(err)
	}
	d := e.send(t, "parcial.txt", fixture(t, "txt_parcial_sem_folha_e_fator")) // confiança 0,80
	if d.Status != inbox.StatusNeedsReview || str(d.Motivo) != "confianca_baixa (0.8000 < 0.990000)" || d.CompanyID != nil || len(e.movements(t, id)) != 0 {
		t.Fatalf("documento = %+v", d)
	}
	if _, dec := e.decision(t, *d.TraceID); dec["company_id"] != nil || dec["movimento"] != "nao_aplicavel" {
		t.Fatalf("decisão = %v", dec)
	}
}

func TestPDFWithoutTextLayer_NeedsReview(t *testing.T) {
	e := setup(t)
	d := e.send(t, "x.pdf", fixture(t, "pdf_sem_camada_texto"))
	if d.Status != inbox.StatusNeedsReview || str(d.Motivo) != pdftext.ReasonNoTextLayer || str(d.Confianca) != "0.0000" {
		t.Fatalf("documento = %+v", d)
	}
}

// --- Nota ouro [AC-031, eval-plan §2] ---

func (e env) manualSeries(t *testing.T, id, proLabore string) {
	t.Helper()
	for m := 0; m < 12; m++ {
		comp := fmt.Sprintf("%04d-%02d", 2025+(8+m)/12, (8+m)%12+1) // 2025-09 a 2026-08
		e.srv.Put("/api/v1/companies/"+id+"/factor-r/movements/"+comp, map[string]any{"receita_bruta": "50000", "pro_labore": proLabore}).
			MustStatus(t, http.StatusOK)
	}
}

func TestGold_ManualSeriesMatchesExtract(t *testing.T) {
	e := setup(t)
	id := e.company(t, cnpjFormat)
	e.manualSeries(t, id, "15000") // RBT12 600k, FS12 180k, fator 30%: o mesmo do extrato
	d := e.send(t, "extrato.txt", fixture(t, "txt_padrao"))
	g := e.gold(t, *d.TraceID)
	want := map[string]string{"cnpj": "ok", "pa": "ouro_indisponivel", "rbt12": "ok", "fs12": "ok", "fator_r": "ok", "anexo": "ok"}
	if fmt.Sprint(g) != fmt.Sprint(want) {
		t.Fatalf("nota ouro = %v", g)
	}
}

func TestGold_DetectsExtractionError(t *testing.T) {
	e := setup(t)
	id := e.company(t, cnpjFormat)
	e.manualSeries(t, id, "10000") // FS12 real 120k; o extrato diz 180k
	d := e.send(t, "extrato.txt", fixture(t, "txt_padrao"))
	g := e.gold(t, *d.TraceID)
	if g["fs12"] != "erro" || g["fator_r"] != "erro" || g["anexo"] != "erro" || g["rbt12"] != "ok" {
		t.Fatalf("nota ouro = %v", g)
	}
}

func TestGold_UnavailableWithPGDASMonthInWindow(t *testing.T) {
	e := setup(t)
	id := e.company(t, cnpjFormat)
	e.manualSeries(t, id, "15000")
	if _, err := e.srv.Pool.Exec(context.Background(), `UPDATE factor_r_movements SET origem = 'pgdas' WHERE company_id = $1 AND competence = '2026-03-01'`, id); err != nil {
		t.Fatal(err)
	}
	d := e.send(t, "extrato.txt", fixture(t, "txt_padrao"))
	g := e.gold(t, *d.TraceID)
	for _, f := range []string{"rbt12", "fs12", "fator_r", "anexo"} {
		if g[f] != "ouro_indisponivel" {
			t.Errorf("%s = %s", f, g[f])
		}
	}
	if g["cnpj"] != "ok" {
		t.Errorf("cnpj = %s", g["cnpj"])
	}
}

// --- Revisão: vínculo manual e rejeição ---

func TestManualLinkAndRejectionCreateNewTraces(t *testing.T) {
	e := setup(t)
	d := e.send(t, "extrato.txt", fixture(t, "txt_padrao")) // sem empresa: revisão
	if d.Status != inbox.StatusNeedsReview {
		t.Fatalf("documento = %+v", d)
	}
	id := e.company(t, cnpjFormat)
	var linked docOut
	e.srv.Post("/api/v1/factor-r/pgdas/"+d.ID+"/link", map[string]any{"company_id": id}).MustStatus(t, http.StatusOK).JSON(t, &linked)
	if linked.Status != inbox.StatusLinked || str(linked.TraceID) == str(d.TraceID) || e.movements(t, id)["2026-09"].Origem != "pgdas" {
		t.Fatalf("vínculo = %+v", linked)
	}
	if kind, dec := e.decision(t, *linked.TraceID); kind != "pgdas_vinculo_manual" || dec["origem_decisao"] != "analista" {
		t.Fatalf("decisão = %s %v", kind, dec)
	}
	if code := e.srv.Post("/api/v1/factor-r/pgdas/"+d.ID+"/link", map[string]any{"company_id": id}).MustStatus(t, http.StatusConflict).ErrorCode(t); code != "PGDAS_INVALID_STATE" {
		t.Errorf("vínculo repetido = %s", code)
	}
	other := e.send(t, "sem-relacao.txt", fixture(t, "txt_sem_relacao"))
	var rejected docOut
	e.srv.Post("/api/v1/factor-r/pgdas/"+other.ID+"/review", map[string]any{"decisao": "rejeitar", "motivo": "não é extrato"}).
		MustStatus(t, http.StatusOK).JSON(t, &rejected)
	if rejected.Status != inbox.StatusRejected || str(rejected.Motivo) != "não é extrato" || str(rejected.TraceID) == str(other.TraceID) ||
		!strings.HasPrefix(str(rejected.TextoAgente), "Extrato rejeitado pelo analista.") {
		t.Fatalf("rejeição = %+v", rejected)
	}
	if n := e.count(t, `SELECT count(DISTINCT trigger) FROM ai_runs WHERE agent = 'parser_pgdas' AND trigger IN ('upload', 'vinculo_manual', 'rejeicao')`); n != 3 {
		t.Fatalf("gatilhos = %d", n)
	}
	e.srv.Post("/api/v1/factor-r/pgdas/"+other.ID+"/review", map[string]any{"decisao": "rejeitar", "motivo": "de novo"}).MustStatus(t, http.StatusConflict)
	for _, bad := range []map[string]any{{"decisao": "aprovar", "motivo": "abc"}, {"decisao": "rejeitar", "motivo": "ab"}} {
		e.srv.Post("/api/v1/factor-r/pgdas/"+d.ID+"/review", bad).MustStatus(t, http.StatusBadRequest)
	}
}

func TestManualLink_UnknownCompany(t *testing.T) {
	e := setup(t)
	d := e.send(t, "extrato.txt", fixture(t, "txt_padrao"))
	e.srv.Post("/api/v1/factor-r/pgdas/"+d.ID+"/link", map[string]any{"company_id": "00000000-0000-0000-0000-000000000000"}).MustStatus(t, http.StatusNotFound)
	e.srv.Post("/api/v1/factor-r/pgdas/"+d.ID+"/link", map[string]any{"company_id": "x"}).MustStatus(t, http.StatusBadRequest)
}

// Rejeitado antes do processamento: o job encontra o documento fora de received e não faz nada.
func TestRejectBeforeProcessing(t *testing.T) {
	e := setup(t)
	var u uploadOut
	e.upload(t, "extrato.txt", fixture(t, "txt_padrao")).MustStatus(t, http.StatusAccepted).JSON(t, &u)
	e.srv.Post("/api/v1/factor-r/pgdas/"+u.DocumentID+"/review", map[string]any{"decisao": "rejeitar", "motivo": "enviado por engano"}).MustStatus(t, http.StatusOK)
	e.process(t)
	if d := e.doc(t, u.DocumentID); d.Status != inbox.StatusRejected || d.Job.Status != jobs.StatusDone {
		t.Fatalf("documento = %+v", d)
	}
}

// Falha do job visível no documento [AC-014]: sem o original, o job morre com código estável.
func TestJobFailureIsVisible(t *testing.T) {
	e := setup(t)
	var u uploadOut
	e.upload(t, "extrato.txt", fixture(t, "txt_padrao")).MustStatus(t, http.StatusAccepted).JSON(t, &u)
	files, _ := filepath.Glob(filepath.Join(e.dir, "pgdas", "*"))
	for _, f := range files {
		_ = os.Remove(f)
	}
	e.process(t)
	d := e.doc(t, u.DocumentID)
	if d.Status != inbox.StatusReceived || d.Job.Status != jobs.StatusDead || str(d.Job.LastErrorCode) != inbox.CodeFileMissing {
		t.Fatalf("documento = %+v", d)
	}
}

// --- Cadastro pelo extrato ---

func TestSuggestion_OnlyForValidCNPJOutsidePortfolio(t *testing.T) {
	e := setup(t)
	d := e.send(t, "extrato.txt", fixture(t, "txt_padrao"))
	want := map[string]any{"cnpj": cnpjPadrao, "cnpj_formatado": cnpjFormat, "nome_empresarial": "CLINICA EXEMPLO LTDA", "sujeita_fator_r": true, "inicio_atividade": "2019-03"}
	if fmt.Sprint(d.Sugestao) != fmt.Sprint(want) {
		t.Fatalf("sugestão = %v", d.Sugestao)
	}
	var list struct {
		Items []docOut `json:"items"`
	}
	e.srv.Get("/api/v1/factor-r/pgdas?status=needs_review").MustStatus(t, http.StatusOK).JSON(t, &list)
	if len(list.Items) != 1 || list.Items[0].Sugestao["cnpj"] != cnpjPadrao {
		t.Fatalf("lista = %+v", list.Items)
	}
	e.srv.Get("/api/v1/factor-r/pgdas?status=x").MustStatus(t, http.StatusBadRequest)
	e.srv.Get("/api/v1/factor-r/pgdas?limit=201").MustStatus(t, http.StatusBadRequest)
	for _, name := range []string{"txt_sem_cnpj", "txt_cnpj_dv_invalido"} {
		d := e.send(t, name+".txt", fixture(t, name))
		if d.Status != inbox.StatusNeedsReview || d.Sugestao != nil {
			t.Fatalf("%s = %+v", name, d)
		}
		code := e.srv.Post("/api/v1/factor-r/pgdas/"+d.ID+"/create-company", map[string]any{"empresa": companyBody(cnpjFormat, "X", true)}).
			MustStatus(t, http.StatusUnprocessableEntity).ErrorCode(t)
		if code != "PGDAS_NO_VALID_CNPJ" {
			t.Errorf("%s: %s", name, code)
		}
	}
}

type createOut struct {
	Documento docOut `json:"documento"`
	Empresa   struct {
		ID        string `json:"id"`
		Cnpj      string `json:"cnpj"`
		LegalName string `json:"legal_name"`
		FactorR   *bool  `json:"factor_r"`
	} `json:"empresa"`
}

func (e env) create(t *testing.T, docID string, body map[string]any, status int) testutil.Response {
	t.Helper()
	return e.srv.Post("/api/v1/factor-r/pgdas/"+docID+"/create-company", body).MustStatus(t, status)
}

func TestCreateCompany_CreatesLinksAndRecordsTrace(t *testing.T) {
	e := setup(t)
	d := e.send(t, "extrato.txt", fixture(t, "txt_padrao"))
	var out createOut
	e.create(t, d.ID, map[string]any{"empresa": companyBody(cnpjFormat, "CLINICA EXEMPLO LTDA", true)}, http.StatusCreated).JSON(t, &out)
	doc, co := out.Documento, out.Empresa
	if co.Cnpj != cnpjPadrao || co.LegalName != "CLINICA EXEMPLO LTDA" || doc.Status != inbox.StatusLinked || str(doc.CompanyID) != co.ID ||
		doc.Sugestao != nil || str(doc.TraceID) == str(d.TraceID) || !strings.Contains(str(doc.TextoAgente), "cadastrada a partir do extrato") ||
		!strings.HasSuffix(str(doc.TextoAgente), disclaimerPt) {
		t.Fatalf("cadastro = %+v", out)
	}
	ms := e.movements(t, co.ID)
	if len(ms) != 1 || ms["2026-09"].Receita != "50000.00" || ms["2026-09"].Origem != "pgdas" {
		t.Fatalf("movimentos = %+v", ms)
	}
	if n := e.count(t, `SELECT count(*) FROM ai_runs WHERE trace_id = $1 AND agent = 'parser_pgdas' AND trigger = 'cadastro_pelo_extrato' AND status = 'ok' AND company_id = $2`, *doc.TraceID, co.ID); n != 1 {
		t.Fatal("corrida do cadastro não encontrada")
	}
	kind, dec := e.decision(t, *doc.TraceID)
	if kind != "pgdas_cadastro_pelo_extrato" || dec["company_id"] != co.ID || dec["movimento"] != "criado" || dec["competencia"] != "2026-09" ||
		dec["empresa_criada"] != true || dec["nome_igual_extrato"] != true || dec["sujeita_igual_sugestao"] != true {
		t.Fatalf("decisão = %v", dec)
	}
	if _, ok := dec["nome"]; ok {
		t.Error("o nome entrou na decisão")
	}
	// Identificação não entra na nota ouro.
	for f := range e.gold(t, *doc.TraceID) {
		if !strings.Contains("cnpj pa rbt12 rpa fs12 fator_r das anexo", f) {
			t.Errorf("campo de ouro inesperado: %s", f)
		}
	}
	// Estado final: de novo → 409.
	if code := e.create(t, d.ID, map[string]any{"empresa": companyBody(cnpjFormat, "X", true)}, http.StatusConflict).ErrorCode(t); code != "PGDAS_INVALID_STATE" {
		t.Errorf("de novo = %s", code)
	}
}

func TestCreateCompany_EditedNameAndSubjectAreRecorded(t *testing.T) {
	e := setup(t)
	d := e.send(t, "extrato.txt", fixture(t, "txt_padrao"))
	var out createOut
	e.create(t, d.ID, map[string]any{"empresa": companyBody(cnpjFormat, "Clínica Exemplo (nome fantasia)", false)}, http.StatusCreated).JSON(t, &out)
	_, dec := e.decision(t, *out.Documento.TraceID)
	if dec["nome_igual_extrato"] != false || dec["sujeita_igual_sugestao"] != false || out.Empresa.LegalName != "Clínica Exemplo (nome fantasia)" {
		t.Fatalf("decisão = %v", dec)
	}
}

func TestCreateCompany_RefusalsWriteNothing(t *testing.T) {
	e := setup(t)
	d := e.send(t, "extrato.txt", fixture(t, "txt_padrao"))
	companies := e.count(t, `SELECT count(*) FROM companies`)
	if code := e.create(t, d.ID, map[string]any{"empresa": companyBody("04.252.011/0001-10", "X", true)}, http.StatusUnprocessableEntity).ErrorCode(t); code != "PGDAS_CNPJ_MISMATCH" {
		t.Errorf("CNPJ divergente = %s", code)
	}
	e.create(t, d.ID, map[string]any{"empresa": companyBody("11.222.333/0001-82", "X", true)}, http.StatusUnprocessableEntity)
	e.create(t, d.ID, map[string]any{"empresa": companyBody(cnpjFormat, "X", true), "inicio_atividade": "03/2019"}, http.StatusBadRequest)
	bad := companyBody(cnpjFormat, "", true)
	e.create(t, d.ID, map[string]any{"empresa": bad}, http.StatusBadRequest)
	if e.count(t, `SELECT count(*) FROM companies`) != companies || e.doc(t, d.ID).Status != inbox.StatusNeedsReview ||
		e.count(t, `SELECT count(*) FROM ai_runs WHERE trigger = 'cadastro_pelo_extrato'`) != 0 {
		t.Fatal("a recusa gravou algo")
	}
	// Documento rejeitado não vira cadastro.
	other := e.send(t, "outro.txt", bytes.Replace(fixture(t, "txt_padrao"), []byte(cnpjFormat), []byte("04.252.011/0001-10"), 1))
	e.srv.Post("/api/v1/factor-r/pgdas/"+other.ID+"/review", map[string]any{"decisao": "rejeitar", "motivo": "extrato duplicado"}).MustStatus(t, http.StatusOK)
	e.create(t, other.ID, map[string]any{"empresa": companyBody("04.252.011/0001-10", "Y", true)}, http.StatusConflict)
}

func TestCreateCompany_AlreadyRegisteredCNPJ(t *testing.T) {
	e := setup(t)
	d := e.send(t, "extrato.txt", fixture(t, "txt_padrao"))
	existing := e.company(t, cnpjFormat)
	var body struct {
		Code    string `json:"code"`
		Details struct {
			Conflicting struct {
				ID string `json:"id"`
			} `json:"conflicting_company"`
		} `json:"details"`
	}
	e.create(t, d.ID, map[string]any{"empresa": companyBody(cnpjFormat, "CLINICA EXEMPLO LTDA", true)}, http.StatusConflict).JSON(t, &body)
	if body.Code != "CNPJ_CONFLICT" || body.Details.Conflicting.ID != existing {
		t.Fatalf("conflito = %+v", body)
	}
	if e.doc(t, d.ID).Status != inbox.StatusNeedsReview || e.count(t, `SELECT count(*) FROM ai_runs WHERE trigger = 'cadastro_pelo_extrato'`) != 0 {
		t.Fatal("a recusa gravou algo")
	}
	e.srv.Post("/api/v1/factor-r/pgdas/"+d.ID+"/link", map[string]any{"company_id": existing}).MustStatus(t, http.StatusOK)
}

// Outro cadastro do mesmo CNPJ entre a checagem e a gravação: o UNIQUE barra, tudo é desfeito
// e fica a corrida de erro (como o teste de corrida da origem).
func TestCreateCompany_RaceRollsBackAndLeavesErrorRun(t *testing.T) {
	e := setup(t)
	d := e.send(t, "extrato.txt", fixture(t, "txt_padrao"))
	companies := company.NewService(e.srv.Pool, e.srv.Clock)
	svc := inbox.NewService(e.srv.Pool, e.sys, e.tr, e.queue, inbox.NewStorage(e.dir), companies, quiet)
	var winner string
	svc.SetBeforeCreateRun(func() { winner = e.company(t, cnpjFormat) })
	in := inbox.CreateCompanyInput{Empresa: company.CreateInput{
		InternalCode: "PG-RACE", LegalName: "CLINICA EXEMPLO LTDA", Cnpj: cnpjFormat, PayrollType: "PAYROLL", TaxRegime: "SIMPLES_NACIONAL",
	}}
	in.Empresa.ServiceStartDate.Time = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	_, err := svc.CreateCompany(context.Background(), d.ID, in)
	var ae *apperr.Error
	if !errors.As(err, &ae) || ae.Code != "CNPJ_CONFLICT" {
		t.Fatalf("corrida = %v", err)
	}
	if doc := e.doc(t, d.ID); doc.Status != inbox.StatusNeedsReview || doc.CompanyID != nil || len(e.movements(t, winner)) != 0 {
		t.Fatalf("documento = %+v", doc)
	}
	if n := e.count(t, `SELECT count(*) FROM ai_runs WHERE trigger = 'cadastro_pelo_extrato' AND status = 'error' AND error_code = 'CNPJ_CONFLICT' AND company_id IS NULL`); n != 1 {
		t.Fatalf("corridas de erro = %d", n)
	}
}

func TestCreateCompany_WithoutPALinksWithoutRevenue(t *testing.T) {
	e := setup(t)
	var lines [][]byte
	for _, l := range bytes.Split(fixture(t, "txt_padrao"), []byte("\n")) {
		if !bytes.Contains(bytes.SplitN(l, []byte(":"), 2)[0], []byte("(PA)")) {
			lines = append(lines, l)
		}
	}
	d := e.send(t, "sem-pa.txt", bytes.Join(lines, []byte("\n")))
	if d.Campos["pa"] != nil || d.Sugestao == nil {
		t.Fatalf("documento = %+v", d)
	}
	var out createOut
	e.create(t, d.ID, map[string]any{"empresa": companyBody(cnpjFormat, "CLINICA EXEMPLO LTDA", true)}, http.StatusCreated).JSON(t, &out)
	if out.Documento.Status != inbox.StatusLinked || !strings.Contains(str(out.Documento.TextoAgente), "nenhum movimento foi criado") || len(e.movements(t, out.Empresa.ID)) != 0 {
		t.Fatalf("cadastro = %+v", out.Documento)
	}
}

// --- Layout declaratório (PDF com as tabelas dos 12 meses anteriores) ---

func janela2026_08() []string {
	out := make([]string, 12)
	for m := range 12 {
		out[m] = fmt.Sprintf("%04d-%02d", 2025+(7+m)/12, (7+m)%12+1) // 2025-08 a 2026-07
	}
	return out
}

func TestDeclaratoryPDF_EndToEnd(t *testing.T) {
	e := setup(t)
	d := e.send(t, "declaracao.pdf", fixture(t, "pdf_declaratorio_fator_r_abaixo_28"))
	want := map[string]any{"cnpj": "12345678000195", "cnpj_formatado": "12.345.678/0001-95", "nome_empresarial": "EMPRESA DE SERVICOS A - ANONIMIZADA", "sujeita_fator_r": true, "inicio_atividade": "2019-03"}
	if str(d.Campos["pa"]) != "2026-08" || fmt.Sprint(d.Sugestao) != fmt.Sprint(want) {
		t.Fatalf("documento = %+v", d)
	}
	var out createOut
	e.create(t, d.ID, map[string]any{
		"empresa":          companyBody("12.345.678/0001-95", "EMPRESA DE SERVICOS A - ANONIMIZADA", true),
		"inicio_atividade": "2019-03",
	}, http.StatusCreated).JSON(t, &out)
	text := str(out.Documento.TextoAgente)
	if !strings.Contains(text, "12 meses anteriores lançados") || !strings.Contains(text, "Nenhum lançamento existente foi alterado") {
		t.Fatalf("texto = %q", text)
	}
	var settings struct {
		InicioAtividade *string `json:"inicio_atividade"`
	}
	e.srv.Get("/api/v1/companies/"+out.Empresa.ID+"/factor-r/settings").MustStatus(t, http.StatusOK).JSON(t, &settings)
	if str(settings.InicioAtividade) != "2019-03" {
		t.Errorf("início da atividade = %s", str(settings.InicioAtividade))
	}
	ms := e.movements(t, out.Empresa.ID)
	if len(ms) != 13 {
		t.Fatalf("movimentos = %d", len(ms))
	}
	for _, c := range janela2026_08() {
		m := ms[c]
		if m.Receita != "10000.00" || str(m.Declarada) != "2000.00" || m.FolhaMes != "2000.00" || m.Split != "0.00|0.00|0.00|0.00" ||
			m.Origem != "pgdas" || !strings.Contains(str(m.Observacao), "total declarado no PGDAS-D") {
			t.Fatalf("%s = %+v", c, m)
		}
	}
	if pa := ms["2026-08"]; pa.Receita != "10000.00" || pa.Declarada != nil || pa.FolhaMes != "0.00" {
		t.Fatalf("PA = %+v", pa)
	}
	_, dec := e.decision(t, *out.Documento.TraceID)
	if fmt.Sprint(dec["meses_anteriores"]) != fmt.Sprint(map[string]any{"criados": 12.0, "existentes": 0.0, "com_folha": true, "motivo": nil}) {
		t.Fatalf("meses anteriores = %v", dec["meses_anteriores"])
	}
	// O motor sobre a série gravada: FS12 é a folha declarada.
	var calc struct {
		RBT12 string  `json:"rbt12"`
		FS12  string  `json:"fs12"`
		Anexo *string `json:"anexo"`
	}
	e.srv.Get("/api/v1/companies/"+out.Empresa.ID+"/factor-r/calc?pa=2026-08").MustStatus(t, http.StatusOK).JSON(t, &calc)
	if calc.RBT12 != "120000.00" || calc.FS12 != "24000.00" || str(calc.Anexo) != "V" {
		t.Fatalf("cálculo = %+v", calc)
	}
}

func TestDeclaratoryPDF_CommerceImportsRevenueWithoutPayroll(t *testing.T) {
	e := setup(t)
	d := e.send(t, "declaracao.pdf", fixture(t, "pdf_declaratorio_comercio_sem_fator_r"))
	var out createOut
	e.create(t, d.ID, map[string]any{"empresa": companyBody("23.456.789/0001-95", "Comércio B", false)}, http.StatusCreated).JSON(t, &out)
	ms := e.movements(t, out.Empresa.ID)
	if len(ms) != 13 {
		t.Fatalf("movimentos = %d", len(ms))
	}
	for c, m := range ms {
		if m.Receita != "12000.00" || m.FolhaMes != "0.00" || m.Declarada != nil {
			t.Fatalf("%s = %+v", c, m)
		}
	}
	if !strings.Contains(str(ms["2025-08"].Observacao), "folha não se aplica") {
		t.Errorf("observação = %s", str(ms["2025-08"].Observacao))
	}
	_, dec := e.decision(t, *out.Documento.TraceID)
	if prior := dec["meses_anteriores"].(map[string]any); prior["com_folha"] != false || prior["criados"] != 12.0 {
		t.Fatalf("meses anteriores = %v", prior)
	}
}

func TestDeclaratoryPDF_KnownCompanyLinksAutomaticallyAndPreservesManualMonth(t *testing.T) {
	e := setup(t)
	id := e.company(t, "45.678.901/0001-75")
	e.srv.Put("/api/v1/companies/"+id+"/factor-r/movements/2026-03", map[string]any{"receita_bruta": "999.00", "pro_labore": "5000.00"}).MustStatus(t, http.StatusOK)
	d := e.send(t, "declaracao.pdf", fixture(t, "pdf_declaratorio_fator_r_acima_28"))
	if d.Status != inbox.StatusLinked || str(d.Confianca) != "1.0000" || d.Sugestao != nil {
		t.Fatalf("documento = %+v", d)
	}
	ms := e.movements(t, id)
	if len(ms) != 13 || ms["2026-03"].Origem != "manual" || ms["2026-03"].Receita != "999.00" || str(ms["2026-02"].Declarada) != "9200.00" {
		t.Fatalf("movimentos = %+v", ms)
	}
	_, dec := e.decision(t, *d.TraceID)
	if fmt.Sprint(dec["meses_anteriores"]) != fmt.Sprint(map[string]any{"criados": 11.0, "existentes": 1.0, "com_folha": true, "motivo": nil}) {
		t.Fatalf("meses anteriores = %v", dec["meses_anteriores"])
	}
}

func TestDeclaratory_TableNotSummingIsNotWritten(t *testing.T) {
	e := setup(t)
	text, err := extractor.Extract(context.Background(), fixture(t, "pdf_declaratorio_fator_r_abaixo_28"), pdftext.MimePDF)
	if err != nil {
		t.Fatal(err)
	}
	tampered := strings.Replace(text.Text, "07/2026 10.000,00", "07/2026 11.000,00", 1)
	if tampered == text.Text {
		t.Fatal("o texto do PDF mudou: ajuste o teste")
	}
	d := e.send(t, "declaracao.txt", []byte(tampered))
	var out createOut
	e.create(t, d.ID, map[string]any{"empresa": companyBody("12.345.678/0001-95", "Serviços A", true)}, http.StatusCreated).JSON(t, &out)
	ms := e.movements(t, out.Empresa.ID)
	if len(ms) != 1 || ms["2026-08"].Origem != "pgdas" || !strings.Contains(str(out.Documento.TextoAgente), "não somam o RBT12 declarado") {
		t.Fatalf("movimentos = %+v; texto = %q", ms, str(out.Documento.TextoAgente))
	}
	if _, dec := e.decision(t, *out.Documento.TraceID); dec["meses_anteriores"].(map[string]any)["motivo"] != "receitas_anteriores_nao_conferem" {
		t.Fatalf("decisão = %v", dec)
	}
}

func TestDisclaimerHelper(t *testing.T) {
	if got := factorr.WithDisclaimer(" texto \n"); got != "texto\n\n"+factorr.Disclaimer {
		t.Fatalf("WithDisclaimer = %q", got)
	}
	if got := factorr.WithDisclaimer("a\n\n" + factorr.Disclaimer); got != "a\n\n"+factorr.Disclaimer {
		t.Fatalf("disclaimer repetido = %q", got)
	}
}

// PA com ano anterior a 1900: a origem levantava exceção no vínculo. No job é falha permanente
// com código estável (e a corrida de erro); no vínculo manual, 422. Nada é gravado.
func TestInvalidPA_IsPermanentFailureAndUnprocessable(t *testing.T) {
	e := setup(t)
	id := e.company(t, cnpjFormat)
	text := []byte("CNPJ: " + cnpjFormat + "\nPA: 08/1899\nRPA: 1.000,00\n")
	var u uploadOut
	e.upload(t, "antigo.txt", text).MustStatus(t, http.StatusAccepted).JSON(t, &u)
	e.process(t)
	d := e.doc(t, u.DocumentID)
	if d.Status != inbox.StatusReceived || d.Job.Status != jobs.StatusDead || str(d.Job.LastErrorCode) != inbox.CodeInvalidPA || len(e.movements(t, id)) != 0 {
		t.Fatalf("documento = %+v", d)
	}
	if n := e.count(t, `SELECT count(*) FROM ai_runs WHERE trigger = 'upload' AND status = 'error' AND error_code = $1`, inbox.CodeInvalidPA); n != 1 {
		t.Fatalf("corrida de erro = %d", n)
	}
	// Em revisão (empresa ainda fora da carteira), o vínculo manual responde 422.
	other := e.send(t, "antigo2.txt", []byte("CNPJ: 11.444.777/0001-61\nPA: 08/1899\nRPA: 1.000,00\n"))
	if other.Status != inbox.StatusNeedsReview {
		t.Fatalf("documento = %+v", other)
	}
	target := e.company(t, "11.444.777/0001-61")
	if code := e.srv.Post("/api/v1/factor-r/pgdas/"+other.ID+"/link", map[string]any{"company_id": target}).MustStatus(t, http.StatusUnprocessableEntity).ErrorCode(t); code != inbox.CodeInvalidPA {
		t.Fatalf("vínculo = %s", code)
	}
	if len(e.movements(t, target)) != 0 || e.doc(t, other.ID).Status != inbox.StatusNeedsReview {
		t.Fatal("o vínculo recusado gravou algo")
	}
}
