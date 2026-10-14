package pipeline_test

import (
	"bytes"
	"net/http"
	"os"
	"strings"
	"testing"

	"controle/internal/ai/litellm"
	"controle/internal/ai/prompts"
	"controle/internal/reconciliation/pipeline"
)

// --- Leitor determinístico do PDF (processar_local da origem, agora no job) ---

// O extrato do cenário (leiaute do Banco A Fonte) é lido sem LLM e importado com tentativas = 0;
// o playbook do banco nasce da leitura aprovada, como na origem.
func TestLocalPDF_ImportsScenarioStatementWithoutLLM(t *testing.T) {
	e := setup(t)
	company := e.company(t, "301", "12345678000190")
	id := e.upload(t, "extrato_construtora_horizonte_2026-08.pdf", fixture(t, "extrato_construtora_horizonte_2026-08.pdf"), map[string]string{"company_id": company})
	e.process(t)
	d := e.doc(t, id)
	if d.Status != pipeline.StatusImported || d.Import == nil || d.Import.Attempts != 0 || d.Import.Transactions != 40 ||
		d.Import.OpeningBalance != "85430.12" || d.Import.ClosingBalance != "213142.37" || d.Import.TraceID != nil ||
		d.CompanyID == nil || *d.CompanyID != company || *d.Bank != "a_fonte" || *d.Account != "56789-0" {
		t.Fatalf("documento = %+v, importação = %+v", d, d.Import)
	}
	if len(e.gw.calls()) != 0 || e.count(t, `SELECT count(*) FROM ai_runs`) != 0 {
		t.Fatal("o leitor determinístico chamou o LLM ou abriu corrida")
	}
	if len(d.Attempts) != 1 || d.Attempts[0].Alias != "leitor_local" || d.Attempts[0].Attempt != 0 || !strings.Contains(string(d.Attempts[0].Validation), `"ok":true`) {
		t.Fatalf("log de extração = %+v", d.Attempts)
	}
	if n := e.count(t, `SELECT count(*) FROM bank_playbooks WHERE bank = 'a_fonte' AND version = 1 AND valid AND successes = 1 AND winning_strategy = 'pdf_texto_nativo'`); n != 1 {
		t.Fatal("playbook do banco não nasceu da leitura aprovada")
	}
	// As 40 transações com a chave de dedupe da origem e a confiança 1.
	if n := e.count(t, `SELECT count(*) FROM statement_transactions WHERE company_id = $1 AND confidence = 1 AND length(dedupe_key) = 64`, company); n != 40 {
		t.Fatalf("transações = %d", n)
	}
}

// Sem empresa no envio, a conta bancária cadastrada decide a empresa (D-020).
func TestLocalPDF_CompanyFromBankAccount(t *testing.T) {
	e := setup(t)
	company := e.company(t, "302", "12345678000190")
	account := e.bankAccount(t, company, "beta", "0021", "33412-7")
	id := e.upload(t, "beta.pdf", fixture(t, "extrato_banco_beta_2026-08.pdf"), nil)
	e.process(t)
	d := e.doc(t, id)
	if d.Status != pipeline.StatusImported || d.Import == nil || d.Import.CompanyID != company || d.Import.BankAccountID == nil ||
		*d.Import.BankAccountID != account || d.Import.Transactions != 3 || d.Import.ClosingBalance != "22325.00" {
		t.Fatalf("documento = %+v, importação = %+v", d, d.Import)
	}
}

// Sem empresa no envio e sem conta cadastrada, o extrato que fecha vai para a revisão: sem
// empresa, nada é importado (D-020).
func TestLocalPDF_WithoutCompanyGoesToReview(t *testing.T) {
	e := setup(t)
	id := e.upload(t, "beta.pdf", fixture(t, "extrato_banco_beta_2026-08.pdf"), nil)
	e.process(t)
	d := e.doc(t, id)
	if d.Status != pipeline.StatusExtractReview || d.Diagnosis["motivo"] != pipeline.ReasonNoCompany || d.Import != nil {
		t.Fatalf("documento = %+v", d)
	}
	if e.count(t, `SELECT count(*) FROM statement_transactions`) != 0 {
		t.Fatal("transação gravada sem empresa")
	}
}

// Leitor que não fecha o saldo não importa nem manda para a revisão: o documento segue para o
// triador e o extrator (na origem, ia direto para a revisão; D-027).
func TestLocalPDF_GateFailureFallsBackToTheAgent(t *testing.T) {
	e := setup(t)
	company := e.company(t, "303", "12345678000190")
	s := &scripted{triage: triageJSON("extrato", "0.9", ptr("beta")), attempts: []string{stmt("100.00", "130.00", "50.00 C", "20.00 D")}}
	e.gw.reply = s.reply
	// Leiaute do Banco Beta, mas o saldo final impresso não fecha com as linhas.
	beta := textPDF("Banco Beta", "Extrato de conta corrente - 01/08/2026 a 31/08/2026", "Agência 0021 - Conta 33412-7",
		"01/08/2026", "SALDO ANTERIOR", "15.000,00", "10/08/2026", "Boleto Madeireira", "880001", "12.640,00 D", "2.360,00",
		"Saldo final", "2.000,00", t.Name())
	id := e.upload(t, "beta.pdf", beta, map[string]string{"company_id": company})
	e.process(t)
	d := e.doc(t, id)
	if len(d.Attempts) == 0 || d.Attempts[0].Alias != "leitor_local" || !strings.Contains(string(d.Attempts[0].Validation), `"ok":false`) {
		t.Fatalf("log = %+v", d.Attempts)
	}
	if calls := e.gw.calls(); len(calls) == 0 || calls[0].JSONSchema.Name != "resultado_triagem" {
		t.Fatal("o documento não seguiu para o triador")
	}
	if d.Status != pipeline.StatusImported || d.Import.Attempts != 1 {
		t.Fatalf("documento = %+v", d)
	}
}

// --- Triador ---

// O triador classifica pelo texto das primeiras páginas, com o modelo do alias e o schema estrito;
// triado como extrato, o documento ganha o job de extração [REQ-019].
func TestTriage_ClassifiesAndEnqueuesExtraction(t *testing.T) {
	e := setup(t)
	e.gw.reply = (&scripted{triage: triageJSON("extrato", "0.92", ptr("itau"))}).reply
	id := e.upload(t, "itau.pdf", itauPDF(t), nil)
	ok, err := e.runner.ProcessOne(ctx) // só a triagem
	if err != nil || !ok {
		t.Fatal(ok, err)
	}
	d := e.doc(t, id)
	if d.Status != pipeline.StatusTriaged || *d.Kind != "extrato" || *d.Bank != "itau" || *d.TriageConfidence != "0.9200" ||
		d.TriageTraceID == nil || d.Job == nil || d.Job.Status != "pending" {
		t.Fatalf("documento = %+v", d)
	}
	if n := e.count(t, `SELECT count(*) FROM jobs WHERE id = $1 AND kind = 'statement_extract' AND dedupe_key = $2`, d.Job.ID, id); n != 1 {
		t.Fatal("job de extração não enfileirado")
	}
	calls := e.gw.calls()
	if len(calls) != 1 || calls[0].Model.String() != "openai/gpt-6-luna" || calls[0].JSONSchema.Name != "resultado_triagem" ||
		calls[0].Messages[0].Content != prompts.Get("conciliacao/triador").Text || !strings.Contains(calls[0].Messages[1].Content, "Itau Unibanco") ||
		len(calls[0].Messages[1].Images) != 0 {
		t.Fatalf("chamada ao triador = %+v", calls)
	}
	// Corrida do triador com a decisão, e o log da tentativa com a saída bruta e o modelo real.
	var status, alias, model string
	if err := e.pool.QueryRow(ctx, `SELECT status, alias, model FROM ai_runs WHERE trace_id = $1 AND agent = 'triador'`, *d.TriageTraceID).Scan(&status, &alias, &model); err != nil ||
		status != "ok" || alias != "triador" || model != "gpt-6-luna" {
		t.Fatalf("corrida = %s %s %s %v", status, alias, model, err)
	}
	if n := e.count(t, `SELECT count(*) FROM ai_decisions WHERE trace_id = $1 AND kind = 'triagem_documento'`, *d.TriageTraceID); n != 1 {
		t.Fatal("decisão do triador não gravada")
	}
	if len(d.Attempts) != 1 || d.Attempts[0].Alias != "triador" || d.Attempts[0].Attempt != 1 || *d.Attempts[0].Model != "gpt-6-luna-2026-06-01" ||
		*d.Attempts[0].TraceID != *d.TriageTraceID || *d.Attempts[0].CostUSD != "0.00012345" {
		t.Fatalf("log = %+v", d.Attempts)
	}
}

// Abaixo do limiar (0,70) ou tipo desconhecido: revisao_triagem com o diagnóstico, sem job novo;
// comprovante triado ganha o job receipt_extract (o roteiro aqui não responde o extrator).
func TestTriage_ThresholdUnknownAndReceipt(t *testing.T) {
	cases := []struct {
		triage, status, reason, nextJob string
	}{
		{triageJSON("extrato", "0.69", ptr("itau")), pipeline.StatusTriageReview, pipeline.ReasonLowConfidence, ""},
		{triageJSON("desconhecido", "0.95", nil), pipeline.StatusTriageReview, pipeline.ReasonUnknownKind, ""},
		{triageJSON("comprovante", "0.70", nil), pipeline.StatusTriaged, "", "receipt_extract"},
	}
	for _, c := range cases {
		t.Run(c.status+c.reason+c.nextJob, func(t *testing.T) {
			e := setup(t)
			e.gw.reply = (&scripted{triage: c.triage}).reply
			id := e.upload(t, "doc.pdf", itauPDF(t), nil)
			e.process(t)
			d := e.doc(t, id)
			if d.Status != c.status {
				t.Fatalf("status = %s", d.Status)
			}
			if c.reason != "" && (d.Diagnosis["motivo"] != c.reason || d.Diagnosis["limiar"] != "0.70") {
				t.Fatalf("diagnóstico = %v", d.Diagnosis)
			}
			jobs := e.count(t, `SELECT count(*) FROM jobs WHERE kind IN ('statement_extract', 'receipt_extract')`)
			if (c.nextJob == "" && jobs != 0) || (c.nextJob != "" && e.count(t, `SELECT count(*) FROM jobs WHERE kind = $1`, c.nextJob) != 1) {
				t.Fatalf("jobs seguintes = %d", jobs)
			}
		})
	}
}

// Saída do triador fora do schema: a tentativa do job falha (como na origem) e a corrida de erro
// fica gravada; o documento continua pendente.
func TestTriage_InvalidOutputFailsTheJobAttempt(t *testing.T) {
	e := setup(t)
	e.gw.reply = (&scripted{triage: `{"tipo": "extrato"}`}).reply
	id := e.upload(t, "doc.pdf", itauPDF(t), nil)
	e.process(t)
	d := e.doc(t, id)
	if d.Status != pipeline.StatusPending || d.Job == nil || d.Job.Status != "retry" || d.Job.LastErrorCode == nil || *d.Job.LastErrorCode != pipeline.CodeTriageOutput {
		t.Fatalf("documento = %+v job = %+v", d, d.Job)
	}
	if n := e.count(t, `SELECT count(*) FROM ai_runs WHERE agent = 'triador' AND status = 'error' AND error_code = $1`, pipeline.CodeTriageOutput); n != 1 {
		t.Fatal("corrida de erro não gravada")
	}
}

// Gateway fora do ar: falha transitória do job (LLM_UNAVAILABLE), sem mexer no documento.
func TestTriage_GatewayDownIsTransient(t *testing.T) {
	e := setup(t)
	e.gw.reply = func(int, litellm.Request) (string, error) {
		return "", &litellm.Error{Status: 503, Message: "indisponível", Retryable: true}
	}
	id := e.upload(t, "doc.pdf", itauPDF(t), nil)
	e.process(t)
	d := e.doc(t, id)
	if d.Status != pipeline.StatusPending || d.Job.Status != "retry" || *d.Job.LastErrorCode != "LLM_UNAVAILABLE" {
		t.Fatalf("documento = %+v job = %+v", d, d.Job)
	}
}

// Alias desligado na tela de IA: o documento vai para a revisão, sem gastar as tentativas do job.
func TestTriage_DisabledAliasGoesToReview(t *testing.T) {
	e := setup(t)
	if _, err := e.pool.Exec(ctx, `UPDATE ai_model_aliases SET enabled = false WHERE alias = 'triador'`); err != nil {
		t.Fatal(err)
	}
	id := e.upload(t, "doc.pdf", itauPDF(t), nil)
	e.process(t)
	d := e.doc(t, id)
	if d.Status != pipeline.StatusTriageReview || d.Diagnosis["motivo"] != pipeline.ReasonAgentDisabled || len(e.gw.calls()) != 0 {
		t.Fatalf("documento = %+v", d)
	}
}

// Foto ou print vai ao triador como imagem; PDF sem camada de texto, como a imagem da primeira
// página a 72 DPI (renderizar_pagina).
func TestTriage_ImagesForPhotosAndScannedPDFs(t *testing.T) {
	scanned, err := os.ReadFile("../../platform/pdftext/testdata/pdf_sem_camada_texto.pdf")
	if err != nil {
		t.Fatal(err)
	}
	photo := append([]byte("\x89PNG\r\n\x1a\n"), []byte("foto do comprovante")...)
	for name, data := range map[string][]byte{"scan.pdf": scanned, "foto.png": photo} {
		t.Run(name, func(t *testing.T) {
			e := setup(t)
			e.gw.reply = (&scripted{triage: triageJSON("comprovante", "0.9", nil)}).reply
			e.upload(t, name, data, nil)
			e.process(t)
			calls := e.gw.calls()
			if len(calls) == 0 || calls[0].JSONSchema.Name != "resultado_triagem" || len(calls[0].Messages[1].Images) != 1 {
				t.Fatalf("chamadas = %+v", calls)
			}
			img := calls[0].Messages[1].Images[0]
			if !bytes.HasPrefix(img.Data, []byte("\x89PNG\r\n\x1a\n")) || img.MIME != "image/png" {
				t.Fatalf("imagem = %s %d bytes", img.MIME, len(img.Data))
			}
		})
	}
}

// --- Extrator ---

func (e env) triaged(t *testing.T, attempts ...string) (*scripted, string) {
	t.Helper()
	s := &scripted{triage: triageJSON("extrato", "0.9", ptr("itau")), attempts: attempts}
	e.gw.reply = s.reply
	company := e.company(t, "401", "11222333000181")
	id := e.upload(t, "itau.pdf", itauPDF(t), map[string]string{"company_id": company})
	e.process(t)
	return s, id
}

// Primeira tentativa que fecha: importa com tentativas = 1, a corrida do extrator e o playbook
// novo do banco (o leiaute descoberto, só com o portão aprovado).
func TestExtract_FirstAttemptImports(t *testing.T) {
	e := setup(t)
	_, id := e.triaged(t, stmt("100.00", "130.00", "50.00 C", "20.00 D"))
	d := e.doc(t, id)
	if d.Status != pipeline.StatusImported || d.Import == nil || d.Import.Attempts != 1 || d.Import.Transactions != 2 || d.Import.TraceID == nil {
		t.Fatalf("documento = %+v importação = %+v", d, d.Import)
	}
	calls := e.extractorCalls()
	if len(calls) != 1 || calls[0].Model.String() != "openai/gpt-6-sol" ||
		!strings.HasPrefix(calls[0].Messages[1].Content, "estrategia=texto_nativo dpi=72\nTEXTO:\n") ||
		!strings.Contains(calls[0].Messages[1].Content, "PIX RECEBIDO") || len(calls[0].Messages[1].Images) != 0 ||
		calls[0].Messages[0].Content != prompts.Get("conciliacao/extrator_extrato").Text {
		t.Fatalf("chamada = %+v", calls)
	}
	var status string
	if err := e.pool.QueryRow(ctx, `SELECT status FROM ai_runs WHERE trace_id = $1 AND agent = 'extrator'`, *d.Import.TraceID).Scan(&status); err != nil || status != "ok" {
		t.Fatalf("corrida = %s %v", status, err)
	}
	if n := e.count(t, `SELECT count(*) FROM bank_playbooks WHERE bank = 'itau' AND version = 1 AND valid AND successes = 1 AND table_start_page = 1 AND sign_representation = 'sufixo C/D'`); n != 1 {
		t.Fatal("playbook novo não gravado")
	}
	if a := d.Attempts[len(d.Attempts)-1]; a.Alias != "extrator" || a.Attempt != 1 || !strings.Contains(string(a.Validation), `"ok":true`) {
		t.Fatalf("log = %+v", d.Attempts)
	}
}

// Diferença na 1ª tentativa: a 2ª vai a 150 DPI, com as páginas renderizadas, o extrato anterior,
// o resultado do validador (o mesmo JSON da origem) e as dicas; a 3ª escala para o
// extrator_forte a 300 DPI [REQ-020].
func TestExtract_RetriesWithFeedbackStrategiesAndEscalation(t *testing.T) {
	e := setup(t)
	skipped := stmt("100.00", "130.00", "50.00 C")      // pulou a tarifa de 20,00: diferença +20,00
	decimalErr := stmt("100.00", "130.00", "5000.00 C") // separador errado
	good := stmt("100.00", "130.00", "50.00 C", "20.00 D")
	_, id := e.triaged(t, skipped, decimalErr, good)
	d := e.doc(t, id)
	if d.Status != pipeline.StatusImported || d.Import.Attempts != 3 {
		t.Fatalf("documento = %+v importação = %+v", d, d.Import)
	}
	calls := e.extractorCalls()
	if len(calls) != 3 {
		t.Fatalf("tentativas = %d", len(calls))
	}
	second := calls[1].Messages[1]
	wantValidation := `validacao={"ok":false,"soma_c":"50.00","soma_d":"0.00","saldo_calculado":"150.00","saldo_esperado":"130.00","diferenca":"20.00","linhas_rejeitadas":[],"lacunas_ordem":[],"datas_fora_periodo":[]}`
	if !strings.HasPrefix(second.Content, "estrategia=render_150 dpi=150\npáginas=1\nanterior=") || !strings.Contains(second.Content, wantValidation) ||
		!strings.Contains(second.Content, "\ndicas=") || len(second.Images) != 1 || second.Images[0].Detail != "high" ||
		calls[1].Messages[0].Content != prompts.Get("conciliacao/extrator_extrato_correcao").Text || calls[1].Model.String() != "openai/gpt-6-sol" {
		t.Fatalf("2ª tentativa = %q (%d imagens), modelo %s", second.Content, len(second.Images), calls[1].Model)
	}
	third := calls[2].Messages[1]
	if !strings.HasPrefix(third.Content, "estrategia=render_300 dpi=300") || calls[2].Model.String() != "openai/gpt-6-astra" || len(third.Images) != 1 {
		t.Fatalf("3ª tentativa = %q, modelo %s", third.Content, calls[2].Model)
	}
	// O render a 300 DPI é maior que o de 150.
	if len(third.Images[0].Data) <= len(second.Images[0].Data) {
		t.Fatalf("render 300 DPI (%d bytes) não é maior que o de 150 (%d)", len(third.Images[0].Data), len(second.Images[0].Data))
	}
	aliases := []string{}
	for _, a := range d.Attempts {
		if a.Attempt > 0 && a.Alias != "triador" {
			aliases = append(aliases, a.Alias)
		}
	}
	if strings.Join(aliases, ",") != "extrator,extrator,extrator_forte" {
		t.Fatalf("aliases das tentativas = %v", aliases)
	}
}

// AC-015: depois das 3 tentativas sem fechar, nenhuma transação do lote é gravada; o documento vai
// para revisao_extracao com o diagnóstico da última tentativa.
func TestExtract_GateRejectsAfterThreeAttemptsAndPersistsNothing(t *testing.T) {
	e := setup(t)
	off := stmt("100.00", "130.01", "50.00 C", "20.00 D") // um centavo [AC-016]
	_, id := e.triaged(t, off, off, off)
	d := e.doc(t, id)
	if d.Status != pipeline.StatusExtractReview || d.Import != nil || d.Diagnosis["motivo"] != pipeline.ReasonBalanceMismatch {
		t.Fatalf("documento = %+v", d)
	}
	v, _ := d.Diagnosis["validacao"].(map[string]any)
	if v["ok"] != false || v["diferenca"] != "-0.01" {
		t.Fatalf("diagnóstico = %v", d.Diagnosis)
	}
	if e.count(t, `SELECT count(*) FROM statement_imports`) != 0 || e.count(t, `SELECT count(*) FROM statement_transactions`) != 0 {
		t.Fatal("o portão reprovou e algo foi gravado")
	}
	if n := e.count(t, `SELECT count(*) FROM extraction_logs WHERE alias IN ('extrator', 'extrator_forte') AND validator_result->>'ok' = 'false'`); n != 3 {
		t.Fatalf("log das tentativas = %d", n)
	}
	if n := e.count(t, `SELECT count(*) FROM bank_playbooks`); n != 0 {
		t.Fatal("playbook gravado sem o portão aprovado")
	}
	if n := e.count(t, `SELECT count(*) FROM ai_runs WHERE agent = 'extrator' AND status = 'needs_review'`); n != 1 {
		t.Fatal("corrida do extrator sem needs_review")
	}
}

// JSON inválido, fora do schema ou com dinheiro em float conta como tentativa perdida; a 3ª, válida,
// importa. Três saídas inválidas: revisão sem diagnóstico do validador.
func TestExtract_InvalidOutputCountsAsAttempt(t *testing.T) {
	e := setup(t)
	floatMoney := strings.Replace(stmt("100.00", "130.00", "50.00 C", "20.00 D"), `"valor":"50.00"`, `"valor":50.5`, 1)
	_, id := e.triaged(t, "não é json", floatMoney, stmt("100.00", "130.00", "50.00 C", "20.00 D"))
	d := e.doc(t, id)
	if d.Status != pipeline.StatusImported || d.Import.Attempts != 3 {
		t.Fatalf("documento = %+v", d)
	}
	if n := e.count(t, `SELECT count(*) FROM extraction_logs WHERE error LIKE 'saída fora do schema%' AND validator_result IS NULL`); n != 2 {
		t.Fatalf("tentativas perdidas no log = %d", n)
	}
	e2 := setup(t)
	_, id2 := e2.triaged(t, "{}", "[]", "nada")
	d2 := e2.doc(t, id2)
	if d2.Status != pipeline.StatusExtractReview || d2.Diagnosis["motivo"] != pipeline.ReasonInvalidOutput || d2.Diagnosis["validacao"] != nil {
		t.Fatalf("documento = %+v", d2)
	}
}

func (e env) playbook(t *testing.T) {
	t.Helper()
	if _, err := e.pool.Exec(ctx, `INSERT INTO bank_playbooks (bank, version, table_start_page, ignored_pages, date_format, decimal_separator,
		sign_representation, balance_location, winning_strategy, validated_at, valid, successes)
		VALUES ('itau', 1, 2, '[3]', 'DD/MM', ',', 'coluna D/C', 'rodapé', 'texto_nativo', now(), true, 4)`); err != nil {
		t.Fatal(err)
	}
}

// AC-017: com playbook válido, a 1ª tentativa vai com ele (instruções no sistema e o playbook como
// dado); falhando, ele é invalidado e as tentativas seguintes redescobrem o leiaute, que vira a
// versão 2 com o portão aprovado.
func TestExtract_PlaybookInvalidatedOnFirstFailureAndRediscovered(t *testing.T) {
	e := setup(t)
	e.playbook(t)
	_, id := e.triaged(t, stmt("100.00", "130.00", "50.00 C"), stmt("100.00", "130.00", "50.00 C", "20.00 D"))
	d := e.doc(t, id)
	if d.Status != pipeline.StatusImported || d.Import.Attempts != 2 {
		t.Fatalf("documento = %+v", d)
	}
	calls := e.extractorCalls()
	withPlaybook := prompts.Get("conciliacao/extrator_extrato").Text + "\n\n" + prompts.Get("conciliacao/playbook").Text
	if calls[0].Messages[0].Content != withPlaybook || !strings.Contains(calls[0].Messages[1].Content, `PLAYBOOK VALIDADO={"banco":"itau"`) ||
		!strings.Contains(calls[0].Messages[1].Content, `"pagina_inicio_tabela":2`) {
		t.Fatalf("1ª tentativa sem o playbook: %q", calls[0].Messages[1].Content)
	}
	if strings.Contains(calls[1].Messages[0].Content, prompts.Get("conciliacao/playbook").Text) || strings.Contains(calls[1].Messages[1].Content, "PLAYBOOK") {
		t.Fatal("2ª tentativa ainda com o playbook invalidado")
	}
	if n := e.count(t, `SELECT count(*) FROM bank_playbooks WHERE bank = 'itau' AND version = 1 AND NOT valid AND failures = 1`); n != 1 {
		t.Fatal("playbook v1 não invalidado")
	}
	if n := e.count(t, `SELECT count(*) FROM bank_playbooks WHERE bank = 'itau' AND version = 2 AND valid AND successes = 1`); n != 1 {
		t.Fatal("redescoberta não virou a versão 2")
	}
}

// Playbook válido e 1ª tentativa aprovada: o playbook ganha um sucesso, sem versão nova.
func TestExtract_PlaybookSuccessCounts(t *testing.T) {
	e := setup(t)
	e.playbook(t)
	_, id := e.triaged(t, stmt("100.00", "130.00", "50.00 C", "20.00 D"))
	if d := e.doc(t, id); d.Status != pipeline.StatusImported {
		t.Fatalf("documento = %+v", d)
	}
	if n := e.count(t, `SELECT count(*) FROM bank_playbooks WHERE bank = 'itau' AND version = 1 AND valid AND successes = 5`); n != 1 ||
		e.count(t, `SELECT count(*) FROM bank_playbooks`) != 1 {
		t.Fatal("o sucesso do playbook não foi contado")
	}
}

// Contábil fechado numa competência do extrato: o saldo fecha, mas nada é importado e o documento
// fica como pendência de período fechado (AC-025, F-009).
func TestExtract_ClosedAccountingPeriodBecomesPending(t *testing.T) {
	e := setup(t)
	s := &scripted{triage: triageJSON("extrato", "0.9", ptr("itau")), attempts: []string{stmt("100.00", "130.00", "50.00 C", "20.00 D")}}
	e.gw.reply = s.reply
	company := e.company(t, "402", "11222333000181")
	e.closeAccounting(t, company, "2026-03")
	id := e.upload(t, "itau.pdf", itauPDF(t), map[string]string{"company_id": company})
	e.process(t)
	d := e.doc(t, id)
	comps, _ := d.Diagnosis["competencias"].([]any)
	if d.Status != pipeline.StatusClosedPeriod || d.Import != nil || len(comps) != 1 || comps[0] != "2026-03" {
		t.Fatalf("documento = %+v", d)
	}
	if e.count(t, `SELECT count(*) FROM statement_transactions`) != 0 {
		t.Fatal("transação gravada com o Contábil fechado")
	}
}

// A conta lida pertence a outra empresa que a do envio: revisão, nada importado.
func TestExtract_AccountOfAnotherCompanyGoesToReview(t *testing.T) {
	e := setup(t)
	other := e.company(t, "403", "99888777000166")
	e.bankAccount(t, other, "itau", "0001", "12345-6")
	_, id := e.triaged(t, stmt("100.00", "130.00", "50.00 C", "20.00 D"))
	if d := e.doc(t, id); d.Status != pipeline.StatusExtractReview || d.Diagnosis["motivo"] != pipeline.ReasonAccountOtherFirm {
		t.Fatalf("documento = %+v", d)
	}
}

// Transação já importada (mesma chave de dedupe) não é gravada de novo (chave da origem).
func TestExtract_DedupeAcrossStatements(t *testing.T) {
	e := setup(t)
	company := e.company(t, "404", "11222333000181")
	first := &scripted{triage: triageJSON("extrato", "0.9", ptr("itau")), attempts: []string{stmt("100.00", "130.00", "50.00 C", "20.00 D")}}
	e.gw.reply = first.reply
	e.upload(t, "a.pdf", textPDF("extrato um", t.Name()), map[string]string{"company_id": company})
	e.process(t)
	second := &scripted{triage: triageJSON("extrato", "0.9", ptr("itau")), attempts: []string{stmt("100.00", "135.00", "50.00 C", "20.00 D", "5.00 C")}}
	e.gw.reply = second.reply
	id := e.upload(t, "b.pdf", textPDF("extrato dois", t.Name()), map[string]string{"company_id": company})
	e.process(t)
	d := e.doc(t, id)
	if d.Status != pipeline.StatusImported || d.Import.Transactions != 1 {
		t.Fatalf("documento = %+v importação = %+v", d, d.Import)
	}
	if n := e.count(t, `SELECT count(*) FROM statement_transactions WHERE company_id = $1`, company); n != 3 {
		t.Fatalf("transações = %d", n)
	}
}

// --- CSV: importador determinístico (pula o agente) ---

// O CSV do banco, triado como extrato, é lido pelo leitor determinístico com banco, agência e
// conta da triagem e o período da competência do envio; importa com tentativas = 0, sem extrator.
func TestCSV_ImportsWithoutTheExtractor(t *testing.T) {
	e := setup(t)
	company := e.company(t, "501", "12345678000190")
	e.gw.reply = (&scripted{triage: `{"tipo":"extrato","banco":"a_fonte","agencia":"1234","conta":"56789-0","periodo_inicio":null,"periodo_fim":null,"confianca":0.95,"justificativa":"CSV de extrato"}`}).reply
	id := e.upload(t, "extrato.csv", fixture(t, "extrato_construtora_horizonte_2026-08.csv"), map[string]string{"company_id": company, "competence": "2026-08"})
	e.process(t)
	d := e.doc(t, id)
	if d.Status != pipeline.StatusImported || d.Import == nil || d.Import.Attempts != 0 || d.Import.Transactions != 40 ||
		d.Import.OpeningBalance != "85430.12" || d.Import.ClosingBalance != "213142.37" || len(e.extractorCalls()) != 0 {
		t.Fatalf("documento = %+v importação = %+v", d, d.Import)
	}
	// O triador recebeu o conteúdo do CSV.
	if c := e.gw.calls(); len(c) != 1 || !strings.Contains(c[0].Messages[1].Content, "data;descricao;documento;tipo;valor;saldo") {
		t.Fatal("o triador não recebeu o conteúdo do arquivo de texto")
	}
}

// Sem banco ou conta na triagem, o CSV vai para a revisão com os campos que faltam.
func TestCSV_MissingMetadataGoesToReview(t *testing.T) {
	e := setup(t)
	e.gw.reply = (&scripted{triage: `{"tipo":"extrato","banco":null,"agencia":null,"conta":null,"periodo_inicio":null,"periodo_fim":null,"confianca":0.9,"justificativa":"CSV"}`}).reply
	id := e.upload(t, "extrato.csv", fixture(t, "extrato_banco_beta_2026-08.csv"), nil)
	e.process(t)
	d := e.doc(t, id)
	fields, _ := d.Diagnosis["campos"].([]any)
	if d.Status != pipeline.StatusExtractReview || d.Diagnosis["motivo"] != pipeline.ReasonMissingMetadata || len(fields) != 4 {
		t.Fatalf("documento = %+v", d)
	}
}

// --- Reprocessamento [06 §4] ---

func TestRetry_ReprocessesReviewAndRecordsTheHumanAction(t *testing.T) {
	e := setup(t)
	off := stmt("100.00", "130.01", "50.00 C", "20.00 D")
	s, id := e.triaged(t, off, off, off)
	if d := e.doc(t, id); d.Status != pipeline.StatusExtractReview {
		t.Fatalf("documento = %+v", d)
	}
	s.attempts, s.n = []string{stmt("100.00", "130.00", "50.00 C", "20.00 D")}, 0
	var out struct {
		JobID  *string `json:"job_id"`
		Status string  `json:"status"`
	}
	e.srv.Post("/api/v1/reconciliation/documents/"+id+"/retry", nil).MustStatus(t, http.StatusAccepted).JSON(t, &out)
	if out.Status != pipeline.StatusTriaged || out.JobID == nil {
		t.Fatalf("resposta = %+v", out)
	}
	e.process(t)
	d := e.doc(t, id)
	if d.Status != pipeline.StatusImported || d.Import.Attempts != 1 {
		t.Fatalf("documento = %+v", d)
	}
	human := 0
	for _, a := range d.Attempts {
		if a.Alias == "humano" && a.ActorID != nil && a.Change != nil && strings.Contains(*a.Change, `"de":"revisao_extracao"`) {
			human++
		}
	}
	if human != 1 {
		t.Fatalf("intervenção humana no log = %d", human)
	}
	// Importado não se reprocessa; documento inexistente, 404.
	if code := e.srv.Post("/api/v1/reconciliation/documents/"+id+"/retry", nil).MustStatus(t, http.StatusConflict).ErrorCode(t); code != "DOCUMENT_NOT_RETRYABLE" {
		t.Fatalf("código = %s", code)
	}
	e.srv.Post("/api/v1/reconciliation/documents/00000000-0000-0000-0000-000000000000/retry", nil).MustStatus(t, http.StatusNotFound)
}

// Pendência de período fechado só volta para a fila com o Contábil reaberto (AC-025).
func TestRetry_ClosedPeriodIsLocked(t *testing.T) {
	e := setup(t)
	company := e.company(t, "601", "11222333000181")
	e.closeAccounting(t, company, "2026-08")
	id := e.upload(t, "extrato.pdf", textPDF("extrato", t.Name()), map[string]string{"company_id": company, "competence": "2026-08"})
	if code := e.srv.Post("/api/v1/reconciliation/documents/"+id+"/retry", nil).MustStatus(t, http.StatusLocked).ErrorCode(t); code != "PERIOD_CLOSED" {
		t.Fatalf("código = %s", code)
	}
	if _, err := e.pool.Exec(ctx, `UPDATE company_module_periods SET state = 'OPEN', closed_at = NULL, closed_by = NULL WHERE company_id = $1`, company); err != nil {
		t.Fatal(err)
	}
	e.srv.Post("/api/v1/reconciliation/documents/"+id+"/retry", nil).MustStatus(t, http.StatusAccepted)
	if d := e.doc(t, id); d.Status != pipeline.StatusPending || d.Job == nil || d.Job.Status != "pending" {
		t.Fatalf("documento = %+v", d)
	}
}

// Documento ainda na fila não é reprocessado de novo.
func TestRetry_RefusesWhileQueued(t *testing.T) {
	e := setup(t)
	id := e.upload(t, "extrato.pdf", textPDF("extrato", t.Name()), nil)
	if code := e.srv.Post("/api/v1/reconciliation/documents/"+id+"/retry", nil).MustStatus(t, http.StatusConflict).ErrorCode(t); code != "DOCUMENT_NOT_RETRYABLE" {
		t.Fatalf("código = %s", code)
	}
}
