package pipeline_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"

	"controle/internal/ai/litellm"
	"controle/internal/reconciliation/pipeline"
)

// TEST-017 [REQ-023, REQ-030, AC-013]: comprovantes pelo leitor determinístico e pelo extrator,
// campo de baixa confiança, roteamento texto → imagem, duplicata de leiaute diferente, trava do
// Contábil e reprocessamento.

const (
	horizonteName = "Construtora Horizonte Ltda"
	horizonteCNPJ = "12345678000190" // o CNPJ do cenário de aula
	receiptsDir   = "../testdata/conciliacao/comprovantes/"
)

func receiptFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(receiptsDir + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// receiptJSON monta a saída do extrator de comprovante: o PIX da Ana Lucia recebido pela empresa,
// com confiança 0,95 em cada campo, e as trocas dadas (nil apaga o valor do campo).
func receiptJSON(t *testing.T, changes map[string]any, conf map[string]float64) string {
	t.Helper()
	fields := []string{"data", "valor", "sentido", "contraparte", "contraparte_doc", "documento", "tipo_operacao"}
	out := map[string]any{
		"data": "2026-08-05", "valor": "2000.00", "sentido": "C", "contraparte": "Ana Lucia Ferreira", "contraparte_doc": nil,
		"documento": "E777", "tipo_operacao": "pix", "alertas": []string{},
	}
	confidence, origin := map[string]float64{}, map[string]any{}
	for _, f := range fields {
		confidence[f] = 0.95
		origin[f] = map[string]any{"pagina": 1, "trecho": "", "bbox": nil}
	}
	origin["valor"] = map[string]any{"pagina": 1, "trecho": "R$ 2.000,00", "bbox": []float64{10, 20.5, 110, 40}}
	for k, v := range changes {
		out[k] = v
	}
	for k, v := range conf {
		confidence[k] = v
	}
	out["confianca_por_campo"], out["origem_por_campo"] = confidence, origin
	b, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// reciboPDF é um comprovante fora do leiaute do leitor (não se diz "comprovante"): vai ao triador
// e ao extrator.
func reciboPDF(t *testing.T, extra ...string) []byte {
	lines := append([]string{"Recibo de transferencia PIX", "Data e hora 05/08/2026 09:12", "Valor R$ 2.000,00",
		"Pagador Ana Lucia Ferreira", "Recebedor " + horizonteName, "CNPJ 12.345.678/0001-90"}, extra...)
	return textPDF(append(lines, t.Name())...)
}

// receiptCalls são os pedidos ao extrator de comprovante.
func (e env) receiptCalls() []litellm.Request {
	var out []litellm.Request
	for _, r := range e.gw.calls() {
		if r.JSONSchema != nil && r.JSONSchema.Name == "comprovante_extraido" {
			out = append(out, r)
		}
	}
	return out
}

// receiptGolden é a leitura da origem (parse_comprovante_texto) dos 13 comprovantes do cenário, na
// ordem dos arquivos (testdata/receipt_gen.py do pacote receipt).
func receiptGolden(t *testing.T) []map[string]any {
	t.Helper()
	raw, err := os.ReadFile("../receipt/testdata/receipt_golden.json")
	if err != nil {
		t.Fatal(err)
	}
	var g struct {
		Reader []struct {
			Out map[string]any `json:"saida"`
		} `json:"leitor"`
	}
	if err := json.Unmarshal(raw, &g); err != nil {
		t.Fatal(err)
	}
	out := make([]map[string]any, 13)
	for i := range out {
		out[i] = g.Reader[i].Out
	}
	return out
}

func str(p *string) any {
	if p == nil {
		return nil
	}
	return *p
}

// Os 13 comprovantes do cenário saem pelo leitor determinístico, sem chamar o modelo, iguais à
// leitura da origem (o texto do PDF pelo PDFium no formato do PyMuPDF): data, valor, sentido,
// contraparte, documento, tipo, marcas de borda e confiança.
func TestTEST017_ScenarioReceiptsByTheLocalReader(t *testing.T) {
	e := setup(t)
	company := e.namedCompany(t, "500", horizonteName, horizonteCNPJ)
	files, err := os.ReadDir(receiptsDir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range files {
		names = append(names, f.Name())
	}
	sort.Strings(names)
	golden := receiptGolden(t)
	if len(names) != len(golden) {
		t.Fatalf("%d comprovantes; o golden tem %d", len(names), len(golden))
	}
	for i, name := range names {
		id := e.upload(t, name, receiptFixture(t, name), map[string]string{"company_id": company})
		e.process(t)
		d := e.doc(t, id)
		if d.Status != pipeline.StatusImported || d.Kind == nil || *d.Kind != "comprovante" || d.Entry == nil {
			t.Fatalf("%s: documento = %+v", name, d)
		}
		en, want := d.Entry, golden[i]
		got := map[string]any{
			"data": str(en.Date), "valor": str(en.Amount), "sentido": str(en.Direction), "contraparte": str(en.Counterparty),
			"documento": str(en.DocumentRef), "tipo_operacao": str(en.OperationType),
		}
		for k, v := range got {
			if !reflect.DeepEqual(v, want[k]) {
				t.Errorf("%s: %s = %v; origem %v", name, k, v, want[k])
			}
		}
		var wantFlags []string
		for _, a := range want["alertas"].([]any) {
			wantFlags = append(wantFlags, a.(string))
		}
		if !reflect.DeepEqual(en.Flags, append([]string{}, wantFlags...)) {
			t.Errorf("%s: marcas = %v; origem %v", name, en.Flags, wantFlags)
		}
		for k, v := range want["confianca_por_campo"].(map[string]any) {
			if en.FieldConfidence[k] != v {
				t.Errorf("%s: confiança de %s = %s; origem %v", name, k, en.FieldConfidence[k], v)
			}
		}
		if en.Status != "extraido" || en.CompanyID != company || en.TraceID != nil || en.DuplicateFlagged {
			t.Errorf("%s: lançamento = %+v", name, en)
		}
		if len(d.Attempts) != 1 || d.Attempts[0].Alias != "leitor_local" || !strings.Contains(string(d.Attempts[0].Validation), `"ok":true`) {
			t.Errorf("%s: log = %+v", name, d.Attempts)
		}
	}
	if n := len(e.gw.calls()); n != 0 {
		t.Fatalf("o leitor determinístico chamou o modelo %d vezes", n)
	}
	// As bordas do cenário ficam marcadas para a fila [REQ-030].
	if e.count(t, `SELECT count(*) FROM client_entries WHERE flags ? 'juros'`) != 2 ||
		e.count(t, `SELECT count(*) FROM client_entries WHERE flags ? 'iss_retido'`) != 1 ||
		e.count(t, `SELECT count(*) FROM client_entries WHERE flags ? 'parcelado'`) < 3 {
		t.Fatal("marcas de borda do cenário")
	}
}

// Sem empresa no envio, vale a empresa cadastrada cujo CNPJ está impresso no comprovante; sem
// nenhuma, o leitor não decide e o extrator também não grava: revisão humana.
func TestTEST017_CompanyFromThePrintedCNPJ(t *testing.T) {
	t.Run("cadastrada", func(t *testing.T) {
		e := setup(t)
		company := e.namedCompany(t, "501", horizonteName, horizonteCNPJ)
		e.namedCompany(t, "502", "Outra Empresa Ltda", "11222333000181")
		id := e.upload(t, "pix.pdf", receiptFixture(t, "05_pix_recebido_ana_lucia_1de4.pdf"), nil)
		e.process(t)
		d := e.doc(t, id)
		if d.Status != pipeline.StatusImported || d.CompanyID == nil || *d.CompanyID != company || d.Entry.CompanyID != company {
			t.Fatalf("documento = %+v", d)
		}
	})
	t.Run("não cadastrada", func(t *testing.T) {
		e := setup(t)
		e.gw.reply = (&scripted{triage: triageJSON("comprovante", "0.9", nil), attempts: []string{receiptJSON(t, nil, nil)}}).reply
		id := e.upload(t, "pix.pdf", receiptFixture(t, "05_pix_recebido_ana_lucia_1de4.pdf"), nil)
		e.process(t)
		d := e.doc(t, id)
		if d.Status != pipeline.StatusExtractReview || d.Diagnosis["motivo"] != pipeline.ReasonNoCompany || d.Entry != nil {
			t.Fatalf("documento = %+v", d)
		}
		calls := e.receiptCalls()
		if len(calls) != 1 || !strings.Contains(calls[0].Messages[1].Content, "EMPRESA DO DOCUMENTO: não identificada") {
			t.Fatalf("chamadas = %d", len(calls))
		}
	})
}

// O extrator lê primeiro o texto (M3.3): com os campos essenciais acima do limiar, o lançamento é
// gravado como extraído, ligado à corrida do agente, com a empresa e as regras no log.
func TestTEST017_AgentReadsTheTextFirst(t *testing.T) {
	e := setup(t)
	company := e.namedCompany(t, "503", horizonteName, horizonteCNPJ)
	out := receiptJSON(t, map[string]any{"contraparte_doc": "123.456.789-01", "alertas": []string{"juros", "inventado", "nf:NF 9"}}, nil)
	e.gw.reply = (&scripted{triage: triageJSON("comprovante", "0.9", nil), attempts: []string{out}}).reply
	id := e.upload(t, "recibo.pdf", reciboPDF(t, "Parcela 1 de 3"), map[string]string{"company_id": company})
	e.process(t)
	d := e.doc(t, id)
	en := d.Entry
	if d.Status != pipeline.StatusImported || en == nil || en.Status != "extraido" || *en.Amount != "2000.00" || *en.Direction != "C" ||
		*en.CounterpartyDoc != "12345678901" || en.TraceID == nil || en.FieldConfidence["valor"] != "0.95" {
		t.Fatalf("documento = %+v lançamento = %+v", d, en)
	}
	// As marcas do agente (só as do vocabulário) somam-se às que o código lê do texto.
	if !reflect.DeepEqual(en.Flags, []string{"juros", "nf:NF 9", "parcelado"}) {
		t.Fatalf("marcas = %v", en.Flags)
	}
	var origin map[string]struct {
		Page    int       `json:"pagina"`
		Excerpt string    `json:"trecho"`
		BBox    []float64 `json:"bbox"`
	}
	if err := json.Unmarshal(en.FieldOrigin, &origin); err != nil || origin["valor"].Excerpt != "R$ 2.000,00" || len(origin["valor"].BBox) != 4 {
		t.Fatalf("origem dos campos = %s", en.FieldOrigin)
	}
	calls := e.receiptCalls()
	if len(calls) != 1 {
		t.Fatalf("%d chamadas ao extrator", len(calls))
	}
	msg := calls[0].Messages[1]
	if len(msg.Images) != 0 || !strings.Contains(msg.Content, "estrategia=texto_nativo") ||
		!strings.Contains(msg.Content, "EMPRESA DO DOCUMENTO: "+horizonteName+", CNPJ 12.345.678/0001-90") ||
		!strings.Contains(msg.Content, "TEXTO:\nRecibo de transferencia PIX") || calls[0].Model.Name != "gpt-6-sol" {
		t.Fatalf("mensagem = %q (%d imagens)", msg.Content, len(msg.Images))
	}
	a := d.Attempts[len(d.Attempts)-1]
	if a.Alias != "extrator" || a.Attempt != 1 || *a.TraceID != *en.TraceID || !strings.Contains(string(a.Validation), `"ok":true`) {
		t.Fatalf("log = %+v", a)
	}
	if n := e.count(t, `SELECT count(*) FROM ai_decisions WHERE trace_id = $1 AND kind = 'extracao_comprovante'`, *en.TraceID); n != 1 {
		t.Fatal("decisão do extrator não gravada")
	}
	if n := e.count(t, `SELECT count(*) FROM ai_runs WHERE trace_id = $1 AND agent = 'extrator_comprovante' AND status = 'ok'`, *en.TraceID); n != 1 {
		t.Fatal("corrida do extrator")
	}
}

// Campo fraco na leitura do texto: a 2ª chamada vai com a página renderizada (render + visão). Vale
// a leitura com menos campos a revisar; restando campo fraco, o lançamento fica parcial, fora do
// razão, e o documento vai à revisão com os campos [REQ-023].
func TestTEST017_WeakFieldRoutesToVisionAndPartialEntry(t *testing.T) {
	cases := []struct {
		name       string
		second     string
		status     string
		entry      string
		weak       []any
		amount     string
		directionN bool
	}{
		{"a imagem resolve", "", pipeline.StatusImported, "extraido", nil, "2000.00", false},
		{"continua fraco", "sentido", pipeline.StatusExtractReview, "parcial", []any{"sentido"}, "2000.00", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := setup(t)
			company := e.namedCompany(t, "504", horizonteName, horizonteCNPJ)
			first := receiptJSON(t, map[string]any{"valor": "200.00"}, map[string]float64{"valor": 0.5})
			second := receiptJSON(t, nil, nil)
			if c.second == "sentido" {
				second = receiptJSON(t, map[string]any{"sentido": nil}, map[string]float64{"sentido": 0.2})
			}
			e.gw.reply = (&scripted{triage: triageJSON("comprovante", "0.9", nil), attempts: []string{first, second}}).reply
			id := e.upload(t, "recibo.pdf", reciboPDF(t), map[string]string{"company_id": company})
			e.process(t)
			d := e.doc(t, id)
			if d.Status != c.status || d.Entry == nil || d.Entry.Status != c.entry || str(d.Entry.Amount) != c.amount ||
				(d.Entry.Direction == nil) != c.directionN {
				t.Fatalf("documento = %+v lançamento = %+v", d, d.Entry)
			}
			if c.weak != nil && (d.Diagnosis["motivo"] != pipeline.ReasonWeakFields || !reflect.DeepEqual(d.Diagnosis["campos"], c.weak)) {
				t.Fatalf("diagnóstico = %v", d.Diagnosis)
			}
			calls := e.receiptCalls()
			if len(calls) != 2 || len(calls[0].Messages[1].Images) != 0 || len(calls[1].Messages[1].Images) != 1 ||
				!strings.Contains(calls[1].Messages[1].Content, "estrategia=render_150") || !strings.Contains(calls[1].Messages[1].Content, "TEXTO:") {
				t.Fatalf("chamadas = %d", len(calls))
			}
			if !bytes.HasPrefix(calls[1].Messages[1].Images[0].Data, []byte("\x89PNG\r\n\x1a\n")) {
				t.Fatal("a 2ª chamada não levou a página renderizada")
			}
		})
	}
}

// Saída fora do schema nas duas chamadas: nada é gravado e o documento vai à revisão; cada
// tentativa fica no log com o erro.
func TestTEST017_InvalidOutputsGoToReview(t *testing.T) {
	e := setup(t)
	company := e.namedCompany(t, "505", horizonteName, horizonteCNPJ)
	e.gw.reply = (&scripted{triage: triageJSON("comprovante", "0.9", nil), attempts: []string{"não é json", `{"valor": 10.5}`}}).reply
	id := e.upload(t, "recibo.pdf", reciboPDF(t), map[string]string{"company_id": company})
	e.process(t)
	d := e.doc(t, id)
	if d.Status != pipeline.StatusExtractReview || d.Diagnosis["motivo"] != pipeline.ReasonInvalidOutput || d.Entry != nil {
		t.Fatalf("documento = %+v", d)
	}
	var errs int
	for _, a := range d.Attempts {
		if a.Alias == "extrator" && a.Error != nil {
			errs++
		}
	}
	if errs != 2 || e.count(t, `SELECT count(*) FROM client_entries`) != 0 {
		t.Fatalf("log = %+v", d.Attempts)
	}
}

// Foto ou print vai ao extrator como está; PDF sem texto, renderizado. Uma chamada só: não há
// leitura melhor a tentar [M3.3].
func TestTEST017_ImagesAndScannedReceipts(t *testing.T) {
	scanned, err := os.ReadFile("../../platform/pdftext/testdata/pdf_sem_camada_texto.pdf")
	if err != nil {
		t.Fatal(err)
	}
	photo := append([]byte("\x89PNG\r\n\x1a\n"), []byte("foto do comprovante "+t.Name())...)
	for name, c := range map[string]struct {
		data     []byte
		strategy string
	}{"scan.pdf": {scanned, "render_150"}, "foto.png": {photo, "imagem"}} {
		t.Run(name, func(t *testing.T) {
			e := setup(t)
			company := e.namedCompany(t, "506", horizonteName, horizonteCNPJ)
			weak := receiptJSON(t, nil, map[string]float64{"data": 0.3})
			e.gw.reply = (&scripted{triage: triageJSON("comprovante", "0.9", nil), attempts: []string{weak}}).reply
			id := e.upload(t, name, c.data, map[string]string{"company_id": company})
			e.process(t)
			calls := e.receiptCalls()
			if len(calls) != 1 || len(calls[0].Messages[1].Images) != 1 || !strings.Contains(calls[0].Messages[1].Content, "estrategia="+c.strategy) {
				t.Fatalf("chamadas = %d", len(calls))
			}
			img := calls[0].Messages[1].Images[0]
			if img.MIME != "image/png" || (name == "foto.png" && !bytes.Equal(img.Data, photo)) {
				t.Fatalf("imagem = %s", img.MIME)
			}
			if d := e.doc(t, id); d.Status != pipeline.StatusExtractReview || d.Entry.Status != "parcial" {
				t.Fatalf("documento = %+v", d)
			}
		})
	}
}

// Duplicata de leiaute diferente (mesmo valor, data e contraparte normalizada, na empresa): o
// segundo lançamento é gravado e sinalizado, nunca descartado [M3.5].
func TestTEST017_DuplicateOfDifferentLayoutIsFlagged(t *testing.T) {
	e := setup(t)
	company := e.namedCompany(t, "507", horizonteName, horizonteCNPJ)
	first := e.upload(t, "pix.pdf", receiptFixture(t, "05_pix_recebido_ana_lucia_1de4.pdf"), map[string]string{"company_id": company})
	e.process(t)
	other := textPDF("Comprovante de PIX recebido", "Data e hora", "05/08/2026 09:12", "Valor", "R$ 2.000,00", "Pagador",
		"ANA LÚCIA FERREIRA", "Recebedor", horizonteName)
	second := e.upload(t, "pix-app.pdf", other, map[string]string{"company_id": company})
	e.process(t)
	a, b := e.doc(t, first), e.doc(t, second)
	if a.Entry.DuplicateFlagged || !b.Entry.DuplicateFlagged || b.Status != pipeline.StatusImported {
		t.Fatalf("1º = %+v 2º = %+v", a.Entry, b.Entry)
	}
	if e.count(t, `SELECT count(*) FROM client_entries WHERE company_id = $1`, company) != 2 {
		t.Fatal("a duplicata foi descartada")
	}
	// Em outra empresa não é duplicata (a origem comparava com todos os lançamentos).
	otherCompany := e.namedCompany(t, "508", "Ana Lucia Ferreira ME", "11222333000181")
	third := e.upload(t, "pix-outra.pdf", textPDF("Comprovante PIX enviado", "Data e hora", "05/08/2026", "Valor", "R$ 2.000,00",
		"Pagador", "Ana Lucia Ferreira", "Recebedor", horizonteName, t.Name()), map[string]string{"company_id": otherCompany})
	e.process(t)
	if c := e.doc(t, third); c.Entry == nil || c.Entry.DuplicateFlagged || *c.Entry.Direction != "D" {
		t.Fatalf("3º = %+v", c.Entry)
	}
}

// Contábil fechado na competência do comprovante: nada é gravado e o documento fica como
// pendencia_periodo_fechado (AC-025).
func TestTEST017_ClosedAccountingPeriod(t *testing.T) {
	e := setup(t)
	company := e.namedCompany(t, "509", horizonteName, horizonteCNPJ)
	e.closeAccounting(t, company, "2026-08")
	id := e.upload(t, "pix.pdf", receiptFixture(t, "05_pix_recebido_ana_lucia_1de4.pdf"), map[string]string{"company_id": company})
	e.process(t)
	d := e.doc(t, id)
	if d.Status != pipeline.StatusClosedPeriod || d.Diagnosis["motivo"] != pipeline.ReasonClosedPeriod || d.Entry != nil {
		t.Fatalf("documento = %+v", d)
	}
}

// Reprocessamento: o comprovante em revisão sem lançamento volta ao extrator; com lançamento, a
// correção é campo a campo (409).
func TestTEST017_RetryReceipt(t *testing.T) {
	e := setup(t)
	company := e.namedCompany(t, "510", horizonteName, horizonteCNPJ)
	s := &scripted{triage: triageJSON("comprovante", "0.9", nil), attempts: []string{"não é json", "[]"}}
	e.gw.reply = s.reply
	id := e.upload(t, "recibo.pdf", reciboPDF(t), map[string]string{"company_id": company})
	e.process(t)
	if d := e.doc(t, id); d.Status != pipeline.StatusExtractReview || d.Entry != nil {
		t.Fatalf("documento = %+v", d)
	}
	s.attempts = append(s.attempts, receiptJSON(t, nil, map[string]float64{"contraparte": 0.4}), receiptJSON(t, nil, map[string]float64{"contraparte": 0.4}))
	e.srv.Post("/api/v1/reconciliation/documents/"+id+"/retry", nil).MustStatus(t, http.StatusAccepted)
	e.process(t)
	d := e.doc(t, id)
	if d.Status != pipeline.StatusExtractReview || d.Entry == nil || d.Entry.Status != "parcial" {
		t.Fatalf("documento = %+v", d)
	}
	e.srv.Post("/api/v1/reconciliation/documents/"+id+"/retry", nil).MustStatus(t, http.StatusConflict)
}
