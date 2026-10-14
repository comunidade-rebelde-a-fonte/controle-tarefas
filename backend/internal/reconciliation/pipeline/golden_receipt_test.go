package pipeline_test

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"controle/internal/ai/litellm"
	"controle/internal/platform/decimal"
	"controle/internal/reconciliation"
	"controle/internal/reconciliation/pipeline"
)

// Relatório do extrator de comprovante contra o gabarito do cenário (M3.9 da origem: o
// avaliar_extrator estendido com a acurácia por campo). Os leitores determinísticos ficam de fora
// (AgentOnly): mede o agente nos comprovantes que o leitor leria.

const receiptGabaritoPath = "../testdata/golden/comprovantes/gabarito.json"

type receiptGabarito struct {
	Company struct {
		Name string `json:"razao_social"`
		CNPJ string `json:"cnpj"`
	} `json:"empresa"`
	Items []struct {
		File            string  `json:"arquivo"`
		Date            string  `json:"data"`
		Amount          string  `json:"valor"`
		Direction       string  `json:"sentido"`
		Counterparty    string  `json:"contraparte"`
		CounterpartyDoc *string `json:"contraparte_doc"`
		OperationType   string  `json:"tipo_operacao"`
	} `json:"comprovantes"`
}

func loadReceiptGabarito(t *testing.T) receiptGabarito {
	t.Helper()
	raw, err := os.ReadFile(receiptGabaritoPath)
	if err != nil {
		t.Fatal(err)
	}
	var g receiptGabarito
	if err := json.Unmarshal(raw, &g); err != nil {
		t.Fatal(err)
	}
	return g
}

// evalFields são os campos medidos: os essenciais, o documento da contraparte e o tipo.
var evalFields = []string{"data", "valor", "sentido", "contraparte", "contraparte_doc", "tipo_operacao"}

type receiptEvalDoc struct {
	Name        string   `json:"documento"`
	Status      string   `json:"status"`
	Entry       string   `json:"lancamento"`
	Calls       int      `json:"chamadas"`
	CostUSD     string   `json:"custo_usd"`
	LatencyMs   int64    `json:"latencia_ms"`
	Differences []string `json:"diferencas"`
}

type receiptEvalReport struct {
	Documents []receiptEvalDoc  `json:"documentos"`
	Total     int               `json:"total"`
	Extracted int               `json:"extraidos"`
	Partial   int               `json:"parciais"`
	Accuracy  map[string]string `json:"acuracia_por_campo"`
	MeanCalls string            `json:"chamadas_medias"`
	CostUSD   string            `json:"custo_usd"`
	LatencyMs int64             `json:"latencia_ms"`
}

func value(p *string) string {
	if p == nil {
		return "null"
	}
	return *p
}

// runReceiptGolden envia cada comprovante do gabarito com a empresa do cenário, processa só pelo
// agente e compara o lançamento campo a campo (contraparte normalizada como na dedupe).
func runReceiptGolden(t *testing.T, e env, g receiptGabarito) receiptEvalReport {
	t.Helper()
	company := e.namedCompany(t, "990", g.Company.Name, g.Company.CNPJ)
	rep := receiptEvalReport{Accuracy: map[string]string{}}
	correct := map[string]int{}
	totalCost := decimal.MustParse("0")
	calls := 0
	for _, it := range g.Items {
		id := e.upload(t, it.File, receiptFixture(t, it.File), map[string]string{"company_id": company})
		e.process(t)
		d := e.doc(t, id)
		doc := receiptEvalDoc{Name: it.File, Status: d.Status, Differences: []string{}}
		cost := decimal.MustParse("0")
		for _, a := range d.Attempts {
			if a.Alias != "extrator" {
				continue
			}
			doc.Calls++
			if a.CostUSD != nil {
				cost = cost.Add(decimal.MustParse(*a.CostUSD))
			}
			if a.LatencyMs != nil {
				doc.LatencyMs += int64(*a.LatencyMs)
			}
		}
		doc.CostUSD = cost.Text()
		totalCost = totalCost.Add(cost)
		calls += doc.Calls
		rep.LatencyMs += doc.LatencyMs
		if en := d.Entry; en == nil {
			doc.Differences = append(doc.Differences, "sem lançamento: "+d.Status)
		} else {
			doc.Entry = en.Status
			switch en.Status {
			case "extraido":
				rep.Extracted++
			case "parcial":
				rep.Partial++
			}
			amount := "null"
			if en.Amount != nil {
				amount = decimal.MustParse(*en.Amount).Quantize(-2).Text()
			}
			got := map[string]string{
				"data": value(en.Date), "valor": amount, "sentido": value(en.Direction),
				"contraparte":     reconciliation.NormalizeDescription(value(en.Counterparty)),
				"contraparte_doc": value(en.CounterpartyDoc), "tipo_operacao": value(en.OperationType),
			}
			want := map[string]string{
				"data": it.Date, "valor": decimal.MustParse(it.Amount).Quantize(-2).Text(), "sentido": it.Direction,
				"contraparte": reconciliation.NormalizeDescription(it.Counterparty), "contraparte_doc": value(it.CounterpartyDoc),
				"tipo_operacao": it.OperationType,
			}
			for _, f := range evalFields {
				if got[f] == want[f] {
					correct[f]++
				} else {
					doc.Differences = append(doc.Differences, fmt.Sprintf("%s: %s; gabarito %s", f, got[f], want[f]))
				}
			}
		}
		rep.Documents = append(rep.Documents, doc)
	}
	rep.Total = len(g.Items)
	rep.CostUSD = totalCost.Text()
	if rep.Total > 0 {
		n := decimal.FromInt(int64(rep.Total))
		for _, f := range evalFields {
			rep.Accuracy[f] = decimal.FromInt(int64(correct[f])).Quo(n).Quantize(-4).Text()
		}
		rep.MeanCalls = decimal.FromInt(int64(calls)).Quo(n).Quantize(-2).Text()
	}
	return rep
}

// brl escreve o valor como o comprovante imprime (2.000,00).
func brl(v string) string {
	whole, frac, _ := strings.Cut(v, ".")
	var b strings.Builder
	for i, r := range whole {
		if i > 0 && (len(whole)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(r)
	}
	return b.String() + "," + frac
}

// O relatório com o gateway simulado: o "modelo" devolve o gabarito, exceto o valor do documento
// no lugar do valor pago (Cerâmica, erro com confiança alta: o relatório mostra) e o sentido em
// dúvida na leitura do texto da ordem bancária (resolvido na 2ª chamada, com a imagem).
func TestGoldenReceiptReport_WithScriptedGateway(t *testing.T) {
	g := loadReceiptGabarito(t)
	if len(g.Items) != 13 {
		t.Fatalf("gabarito com %d comprovantes", len(g.Items))
	}
	gw := &fakeGateway{}
	e := setupWith(t, gw, models, pipeline.Config{TriageThreshold: decimal.MustParse("0.70"), FieldThreshold: decimal.MustParse("0.70"), AgentOnly: true})
	docs := map[string]string{
		"89012345000167": "89.012.345/0001-67", "01234567000189": "01.234.567/0001-89", "90123456000178": "90.123.456/0001-78",
		"02345678000190": "02.345.678/0001-90", "56789012000134": "56.789.012/0001-34", "67890123000145": "67.890.123/0001-45",
	}
	gw.reply = func(n int, req litellm.Request) (string, error) {
		if req.JSONSchema.Name == "resultado_triagem" {
			return triageJSON("comprovante", "0.97", nil), nil
		}
		text := req.Messages[1].Content
		for _, it := range g.Items {
			y, m, d := it.Date[:4], it.Date[5:7], it.Date[8:]
			if !strings.Contains(text, d+"/"+m+"/"+y) || !strings.Contains(text, "R$ "+brl(it.Amount)) {
				continue
			}
			changes := map[string]any{"data": it.Date, "valor": it.Amount, "sentido": it.Direction, "contraparte": it.Counterparty,
				"contraparte_doc": nil, "tipo_operacao": it.OperationType}
			if it.CounterpartyDoc != nil {
				changes["contraparte_doc"] = docs[*it.CounterpartyDoc]
			}
			conf := map[string]float64{}
			switch {
			case strings.HasPrefix(it.File, "13_"):
				changes["valor"] = "18300.00"
			case strings.HasPrefix(it.File, "12_") && strings.Contains(text, "estrategia=texto_nativo"):
				changes["sentido"], conf["sentido"] = nil, 0.3
			}
			return receiptJSON(t, changes, conf), nil
		}
		return "", fmt.Errorf("comprovante fora do gabarito")
	}
	rep := runReceiptGolden(t, e, g)
	b, _ := json.MarshalIndent(rep, "", " ")
	t.Logf("relatório:\n%s", b)
	want := map[string]string{"data": "1.0000", "valor": "0.9231", "sentido": "1.0000", "contraparte": "1.0000",
		"contraparte_doc": "1.0000", "tipo_operacao": "1.0000"}
	for f, v := range want {
		if rep.Accuracy[f] != v {
			t.Errorf("acurácia de %s = %s; esperado %s", f, rep.Accuracy[f], v)
		}
	}
	if rep.Total != 13 || rep.Extracted != 13 || rep.Partial != 0 || rep.MeanCalls != "1.08" || rep.CostUSD != "0.00172830" {
		t.Fatalf("relatório = %s", b)
	}
	if e.count(t, `SELECT count(*) FROM extraction_logs WHERE alias = 'leitor_local'`) != 0 {
		t.Fatal("a avaliação passou pelo leitor determinístico")
	}
	for _, d := range rep.Documents {
		if strings.HasPrefix(d.Name, "13_") && (len(d.Differences) != 1 || !strings.HasPrefix(d.Differences[0], "valor: 18300.00")) {
			t.Fatalf("Cerâmica = %+v", d)
		}
		if strings.HasPrefix(d.Name, "12_") && d.Calls != 2 {
			t.Fatalf("ordem bancária = %+v", d)
		}
	}
}
