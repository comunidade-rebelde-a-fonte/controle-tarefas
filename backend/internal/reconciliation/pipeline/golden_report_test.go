package pipeline_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"controle/internal/ai/litellm"
	"controle/internal/platform/decimal"
	"controle/internal/reconciliation"
	"controle/internal/reconciliation/pipeline"
)

// Relatório do golden do extrator (scripts/avaliar_extrator.py da origem; CC-25, eval-plan §3): por
// documento e no total, aprovação na 1ª tentativa, tentativas, custo, latência e a diferença das
// transações importadas para o gabarito. Os leitores determinísticos ficam de fora (AgentOnly).

const goldenDir = "../testdata/golden/extratos"

type gabarito struct {
	Bank         string `json:"banco"`
	Branch       string `json:"agencia"`
	Account      string `json:"conta"`
	PeriodStart  string `json:"periodo_inicio"`
	PeriodEnd    string `json:"periodo_fim"`
	Opening      string `json:"saldo_inicial"`
	Closing      string `json:"saldo_final"`
	Transactions []struct {
		Order       int    `json:"ordem"`
		Date        string `json:"data"`
		Description string `json:"descricao"`
		Amount      string `json:"valor"`
		Direction   string `json:"sentido"`
	} `json:"transacoes"`
}

type goldenPair struct {
	name     string // <banco>/<AAAA-MM>
	pdf      []byte
	expected gabarito
}

// loadGolden lê os pares PDF + JSON (sem gabarito, o PDF fica de fora, como na origem).
func loadGoldenPairs(t *testing.T) []goldenPair {
	t.Helper()
	pdfs, err := filepath.Glob(filepath.Join(goldenDir, "*", "*.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(pdfs)
	var out []goldenPair
	for _, p := range pdfs {
		raw, err := os.ReadFile(strings.TrimSuffix(p, ".pdf") + ".json")
		if err != nil {
			continue
		}
		var g gabarito
		if err := json.Unmarshal(raw, &g); err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		rel, _ := filepath.Rel(goldenDir, p)
		out = append(out, goldenPair{name: strings.TrimSuffix(filepath.ToSlash(rel), ".pdf"), pdf: data, expected: g})
	}
	return out
}

// goldenCNPJs são CNPJs válidos para as empresas da avaliação (uma por documento).
var goldenCNPJs = []string{"11222333000181", "12345678000190", "99888777000166"}

type goldenDoc struct {
	Name          string   `json:"documento"`
	Status        string   `json:"status"`
	Attempts      int      `json:"tentativas"`
	FirstAttempt  bool     `json:"aprovado_na_1a"`
	CostUSD       string   `json:"custo_usd"`
	LatencyMs     int64    `json:"latencia_ms"`
	Differences   []string `json:"diferencas"`
	Transactions  int      `json:"transacoes"`
	ExpectedLines int      `json:"transacoes_gabarito"`
}

type goldenReport struct {
	Documents    []goldenDoc `json:"documentos"`
	Total        int         `json:"total"`
	Approved     int         `json:"aprovados"`
	FirstAttempt int         `json:"aprovados_na_1a"`
	MeanAttempts string      `json:"tentativas_medias"`
	CostUSD      string      `json:"custo_usd"`
	LatencyMs    int64       `json:"latencia_ms"`
	WithoutDiff  int         `json:"sem_diferenca"`
}

// runGolden envia cada PDF do golden, processa só pelo agente e compara com o gabarito.
func runGolden(t *testing.T, e env, pairs []goldenPair) goldenReport {
	t.Helper()
	var rep goldenReport
	totalCost := decimal.MustParse("0")
	attemptsSum := 0
	for i, g := range pairs {
		company := e.company(t, fmt.Sprintf("9%02d", i), goldenCNPJs[i%len(goldenCNPJs)])
		id := e.upload(t, strings.ReplaceAll(g.name, "/", "_")+".pdf", g.pdf, map[string]string{"company_id": company})
		e.process(t)
		d := e.doc(t, id)
		doc := goldenDoc{Name: g.name, Status: d.Status, ExpectedLines: len(g.expected.Transactions), Differences: []string{}}
		cost := decimal.MustParse("0")
		for _, a := range d.Attempts {
			if a.Alias != "extrator" && a.Alias != "extrator_forte" {
				continue
			}
			doc.Attempts = max(doc.Attempts, a.Attempt)
			if a.CostUSD != nil {
				cost = cost.Add(decimal.MustParse(*a.CostUSD))
			}
			if a.LatencyMs != nil {
				doc.LatencyMs += int64(*a.LatencyMs)
			}
		}
		doc.CostUSD = cost.Text()
		totalCost = totalCost.Add(cost)
		if d.Status == pipeline.StatusImported && d.Import != nil {
			rep.Approved++
			doc.Attempts = d.Import.Attempts
			doc.FirstAttempt = d.Import.Attempts == 1
			doc.Transactions = d.Import.Transactions
			doc.Differences = e.diff(t, d.Import.ID, d.Import.OpeningBalance, d.Import.ClosingBalance, g.expected)
		} else {
			doc.Differences = append(doc.Differences, "não importado: "+d.Status)
		}
		if doc.FirstAttempt {
			rep.FirstAttempt++
		}
		if len(doc.Differences) == 0 {
			rep.WithoutDiff++
		}
		attemptsSum += doc.Attempts
		rep.LatencyMs += doc.LatencyMs
		rep.Documents = append(rep.Documents, doc)
	}
	rep.Total = len(pairs)
	rep.CostUSD = totalCost.Text()
	if rep.Total > 0 {
		rep.MeanAttempts = decimal.FromInt(int64(attemptsSum)).Quo(decimal.FromInt(int64(rep.Total))).Quantize(-2).Text()
	}
	return rep
}

// diff compara a importação com o gabarito: saldos, quantidade e, linha a linha, data, valor,
// sentido e descrição normalizada (a chave de dedupe usa a mesma normalização).
func (e env) diff(t *testing.T, importID, opening, closing string, g gabarito) []string {
	t.Helper()
	out := []string{}
	if decimal.MustParse(opening).Cmp(decimal.MustParse(g.Opening)) != 0 {
		out = append(out, fmt.Sprintf("saldo inicial %s; gabarito %s", opening, g.Opening))
	}
	if decimal.MustParse(closing).Cmp(decimal.MustParse(g.Closing)) != 0 {
		out = append(out, fmt.Sprintf("saldo final %s; gabarito %s", closing, g.Closing))
	}
	rows, err := e.pool.Query(ctx, `SELECT txn_date, description, amount::text, direction FROM statement_transactions
		WHERE statement_import_id = $1 ORDER BY position`, importID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	i := 0
	for rows.Next() {
		var date time.Time
		var desc, amount, dir string
		if err := rows.Scan(&date, &desc, &amount, &dir); err != nil {
			t.Fatal(err)
		}
		if i >= len(g.Transactions) {
			out = append(out, fmt.Sprintf("linha %d a mais: %s %s %s", i+1, date.Format(time.DateOnly), amount, dir))
			i++
			continue
		}
		w := g.Transactions[i]
		if date.Format(time.DateOnly) != w.Date || decimal.MustParse(amount).Cmp(decimal.MustParse(w.Amount)) != 0 || dir != w.Direction ||
			reconciliation.NormalizeDescription(desc) != reconciliation.NormalizeDescription(w.Description) {
			out = append(out, fmt.Sprintf("linha %d: %s %s %s %q; gabarito %s %s %s %q", i+1, date.Format(time.DateOnly), amount, dir, desc,
				w.Date, w.Amount, w.Direction, w.Description))
		}
		i++
	}
	if i < len(g.Transactions) {
		out = append(out, fmt.Sprintf("faltam %d linhas do gabarito", len(g.Transactions)-i))
	}
	return out
}

// statementFrom monta a saída do extrator a partir do gabarito (o "modelo perfeito"), com a
// linha drop removida (0 = nenhuma).
func statementFrom(g gabarito, drop int) string {
	type tx struct {
		Order       int     `json:"ordem"`
		Date        string  `json:"data"`
		Description string  `json:"descricao"`
		Document    *string `json:"documento"`
		Amount      string  `json:"valor"`
		Direction   string  `json:"sentido"`
		Balance     *string `json:"saldo_apos"`
		Confidence  float64 `json:"confianca"`
	}
	out := map[string]any{
		"banco": g.Bank, "agencia": g.Branch, "conta": g.Account, "periodo_inicio": g.PeriodStart, "periodo_fim": g.PeriodEnd,
		"saldo_inicial": g.Opening, "saldo_final": g.Closing, "leiaute": nil,
	}
	var txs []tx
	for _, t := range g.Transactions {
		if t.Order == drop {
			continue
		}
		txs = append(txs, tx{Order: len(txs) + 1, Date: t.Date, Description: t.Description, Amount: t.Amount, Direction: t.Direction, Confidence: 0.95})
	}
	out["transacoes"] = txs
	b, _ := json.Marshal(out)
	return string(b)
}

// O relatório com o gateway simulado: o "modelo" devolve o gabarito (1ª tentativa) ou pula uma linha
// na 1ª e acerta na 2ª; as contas do relatório saem certas e o agente lê o PDF do cenário (o leitor
// determinístico fica de fora).
func TestGoldenReport_WithScriptedGateway(t *testing.T) {
	pairs := loadGoldenPairs(t)
	if len(pairs) != 2 {
		t.Fatalf("golden com %d pares; gere com testdata/golden/gabarito_gen.py", len(pairs))
	}
	gw := &fakeGateway{}
	e := setupWith(t, gw, models, pipeline.Config{TriageThreshold: decimal.MustParse("0.70"), AgentOnly: true})
	byBank := map[string]gabarito{}
	for _, p := range pairs {
		byBank[p.expected.Bank] = p.expected
	}
	gw.reply = func(n int, req litellm.Request) (string, error) {
		text := req.Messages[1].Content
		bank := "a_fonte"
		if strings.Contains(text, "Banco Beta") {
			bank = "beta"
		}
		if req.JSONSchema.Name == "resultado_triagem" {
			return triageJSON("extrato", "0.97", ptr(bank)), nil
		}
		// A Fonte: o modelo pula a 1ª linha na 1ª tentativa (vê o texto) e acerta na 2ª (vê o render).
		if bank == "a_fonte" && strings.Contains(text, "TEXTO:") {
			return statementFrom(byBank[bank], 1), nil
		}
		if strings.Contains(text, "estrategia=render_150") {
			bank = "a_fonte"
		}
		return statementFrom(byBank[bank], 0), nil
	}
	rep := runGolden(t, e, pairs)
	b, _ := json.MarshalIndent(rep, "", " ")
	t.Logf("relatório:\n%s", b)
	if rep.Total != 2 || rep.Approved != 2 || rep.FirstAttempt != 1 || rep.MeanAttempts != "1.50" || rep.WithoutDiff != 2 ||
		rep.CostUSD != "0.00037035" {
		t.Fatalf("relatório = %s", b)
	}
	if e.count(t, `SELECT count(*) FROM extraction_logs WHERE alias = 'leitor_local'`) != 0 {
		t.Fatal("a avaliação passou pelo leitor determinístico")
	}
	fonte := rep.Documents[0]
	if fonte.Name != "a_fonte/2026-08" || fonte.Attempts != 2 || fonte.FirstAttempt || fonte.Transactions != 40 {
		t.Fatalf("A Fonte = %+v", fonte)
	}
}

// Diferenças: saldo e linha que não batem com o gabarito aparecem no relatório.
func TestGoldenReport_ShowsDifferences(t *testing.T) {
	pairs := loadGoldenPairs(t)
	var beta goldenPair
	for _, p := range pairs {
		if p.expected.Bank == "beta" {
			beta = p
		}
	}
	gw := &fakeGateway{}
	e := setupWith(t, gw, models, pipeline.Config{TriageThreshold: decimal.MustParse("0.70"), AgentOnly: true})
	wrong := beta.expected
	wrong.Transactions = append(wrong.Transactions[:0:0], beta.expected.Transactions...)
	wrong.Transactions[1].Description = "PIX DE OUTRA PESSOA"
	gw.reply = func(n int, req litellm.Request) (string, error) {
		if req.JSONSchema.Name == "resultado_triagem" {
			return triageJSON("extrato", "0.97", ptr("beta")), nil
		}
		return statementFrom(wrong, 0), nil
	}
	rep := runGolden(t, e, []goldenPair{beta})
	if rep.Approved != 1 || rep.WithoutDiff != 0 || len(rep.Documents[0].Differences) != 1 ||
		!strings.Contains(rep.Documents[0].Differences[0], "linha 2") {
		t.Fatalf("relatório = %+v", rep)
	}
}
