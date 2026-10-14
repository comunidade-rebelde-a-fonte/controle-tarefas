package pipeline_test

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"controle/internal/platform/decimal"
	"controle/internal/reconciliation/statement"
)

// TEST-018 [REQ-024, REQ-030, AC-018]: o cenário Horizonte inteiro pelo fluxo real, como o
// carregar_horizonte.py da origem o carrega: contas a pagar e a receber como lançamentos manuais,
// os 2 extratos e os 13 comprovantes pelo envio (os leitores determinísticos, sem o modelo) e a
// rodada do matching que o worker enfileira. O resultado é o do rodar_matching da origem (golden
// do pacote matching).

const contabilDir = "../testdata/conciliacao/contabil/"

func readCSV(t *testing.T, name string) []map[string]string {
	t.Helper()
	f, err := os.Open(contabilDir + name)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	r := csv.NewReader(f)
	r.Comma = ';'
	rows, err := r.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	var out []map[string]string
	for _, row := range rows[1:] {
		m := map[string]string{}
		for i, h := range rows[0] {
			m[h] = row[i]
		}
		out = append(out, m)
	}
	return out
}

func brlText(t *testing.T, s string) string {
	t.Helper()
	d, err := statement.ParseBRL(s)
	if err != nil {
		t.Fatal(err)
	}
	return d.Text()
}

func isoDate(t *testing.T, s string) string {
	t.Helper()
	d, err := statement.ParseDateBR(s)
	if err != nil {
		t.Fatal(err)
	}
	return d.Format(time.DateOnly)
}

var anyDate = regexp.MustCompile(`([0-9]{2}/[0-9]{2}/[0-9]{4})`)

// manualEntries cria os lançamentos de contas a pagar (data e valor pagos, saída) e a receber
// (vencimento e valor, entrada), com o documento, pela API.
func (e env) manualEntries(t *testing.T, company string) int {
	t.Helper()
	n := 0
	post := func(body map[string]any) {
		e.srv.Post("/api/v1/reconciliation/entries", body).MustStatus(t, http.StatusCreated)
		n++
	}
	for _, row := range readCSV(t, "contas_a_pagar.csv") {
		m := anyDate.FindString(row["data_pagamento"])
		if m == "" {
			continue
		}
		post(map[string]any{"company_id": company, "date": isoDate(t, m), "amount": brlText(t, row["valor_pago"]), "direction": "D",
			"counterparty": row["fornecedor"], "document_ref": row["documento"]})
	}
	for _, row := range readCSV(t, "contas_a_receber.csv") {
		post(map[string]any{"company_id": company, "date": isoDate(t, row["vencimento"]), "amount": brlText(t, row["valor"]), "direction": "C",
			"counterparty": row["cliente"], "document_ref": row["nf"], "counterparty_doc": row["cpf_cnpj"]})
	}
	return n
}

// lineKey identifica a linha do extrato pelos dados (no cenário, data, valor e sentido não se
// repetem).
func lineKey(date, amount, direction string) string {
	return date + "|" + decimal.MustParse(amount).Quantize(-2).Text() + "|" + direction
}

type scenarioGolden struct {
	Statements []struct {
		ID, Date, Amount, Direction string
	}
	Entries map[string]struct{ Date, Amount, Direction, Counterparty, Origin string }
	Reasons map[string]string
	Matches []struct {
		Statement string
		Entries   []string
		Pass      int
	}
}

func loadScenario(t *testing.T) scenarioGolden {
	t.Helper()
	raw, err := os.ReadFile("../matching/testdata/matching_golden.json")
	if err != nil {
		t.Fatal(err)
	}
	var g struct {
		Scenario struct {
			Statements []struct {
				ID        string `json:"id"`
				Date      string `json:"data"`
				Amount    string `json:"valor"`
				Direction string `json:"sentido"`
			} `json:"extratos"`
			Entries []struct {
				ID           string `json:"id"`
				Date         string `json:"data"`
				Amount       string `json:"valor"`
				Direction    string `json:"sentido"`
				Counterparty string `json:"contraparte"`
				Origin       string `json:"origem"`
			} `json:"lancamentos"`
			Reasons map[string]string `json:"motivos"`
			Result  struct {
				Matches []struct {
					Statement string   `json:"extrato"`
					Entries   []string `json:"lancamentos"`
					Pass      int      `json:"passe"`
				} `json:"matches"`
			} `json:"resultado"`
		} `json:"cenario"`
	}
	if err := json.Unmarshal(raw, &g); err != nil {
		t.Fatal(err)
	}
	out := scenarioGolden{Entries: map[string]struct{ Date, Amount, Direction, Counterparty, Origin string }{}, Reasons: g.Scenario.Reasons}
	for _, s := range g.Scenario.Statements {
		out.Statements = append(out.Statements, struct{ ID, Date, Amount, Direction string }{s.ID, s.Date, s.Amount, s.Direction})
	}
	for _, e := range g.Scenario.Entries {
		out.Entries[e.ID] = struct{ Date, Amount, Direction, Counterparty, Origin string }{e.Date, e.Amount, e.Direction, e.Counterparty, e.Origin}
	}
	for _, m := range g.Scenario.Result.Matches {
		out.Matches = append(out.Matches, struct {
			Statement string
			Entries   []string
			Pass      int
		}{m.Statement, m.Entries, m.Pass})
	}
	return out
}

type dbLine struct {
	key      string
	flags    []map[string]any
	pass     *int
	entries  []string // "data|valor|sentido|contraparte|manual ou comprovante"
	describe string
}

func (e env) lines(t *testing.T, company string) map[string]dbLine {
	t.Helper()
	rows, err := e.pool.Query(ctx, `
		SELECT t.txn_date, t.amount::text, t.direction, t.description, t.flags, r.pass,
		       COALESCE((SELECT array_agg(ce.entry_date::text || '|' || ce.amount::text || '|' || ce.direction || '|' || ce.counterparty || '|' ||
		                  CASE WHEN ce.document_id IS NULL THEN 'manual' ELSE 'comprovante' END ORDER BY ce.entry_date, ce.amount)
		                 FROM reconciliation_items i JOIN client_entries ce ON ce.id = i.client_entry_id
		                 WHERE i.reconciliation_id = r.id AND i.released_at IS NULL), '{}')
		FROM statement_transactions t
		LEFT JOIN reconciliations r ON r.statement_transaction_id = t.id AND r.undone_at IS NULL
		WHERE t.company_id = $1`, company)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]dbLine{}
	for rows.Next() {
		var d time.Time
		var amount, dir, desc string
		var flags []byte
		var pass *int
		var items []string
		if err := rows.Scan(&d, &amount, &dir, &desc, &flags, &pass, &items); err != nil {
			t.Fatal(err)
		}
		l := dbLine{key: lineKey(d.Format(time.DateOnly), amount, dir), pass: pass, entries: items, describe: desc}
		if err := json.Unmarshal(flags, &l.flags); err != nil {
			t.Fatal(err)
		}
		if _, dup := out[l.key]; dup {
			t.Fatalf("linha repetida no cenário: %s", l.key)
		}
		out[l.key] = l
	}
	return out
}

func TestTEST018_ScenarioMatchesLikeTheOrigin(t *testing.T) {
	e := setup(t)
	company := e.namedCompany(t, "800", horizonteName, horizonteCNPJ)
	if n := e.manualEntries(t, company); n < 30 {
		t.Fatalf("só %d lançamentos manuais", n)
	}
	for _, name := range []string{"extrato_construtora_horizonte_2026-08.pdf", "extrato_banco_beta_2026-08.pdf"} {
		e.upload(t, name, fixture(t, name), map[string]string{"company_id": company})
	}
	files, err := os.ReadDir(receiptsDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		e.upload(t, f.Name(), receiptFixture(t, f.Name()), map[string]string{"company_id": company})
	}
	e.process(t)
	if n := len(e.gw.calls()); n != 0 {
		t.Fatalf("o cenário chamou o modelo %d vezes", n)
	}
	g := loadScenario(t)
	byID := map[string]string{}
	for _, s := range g.Statements {
		byID[s.ID] = lineKey(s.Date, s.Amount, s.Direction)
	}
	got := e.lines(t, company)
	if len(got) != len(g.Statements) {
		t.Fatalf("%d linhas importadas; a origem tem %d", len(got), len(g.Statements))
	}
	// Os mesmos casamentos, com os mesmos lançamentos.
	matched := map[string]bool{}
	for _, m := range g.Matches {
		key := byID[m.Statement]
		matched[key] = true
		var want []string
		for _, id := range m.Entries {
			x := g.Entries[id]
			kind := "manual"
			if strings.HasSuffix(x.Origin, ".pdf") {
				kind = "comprovante"
			}
			want = append(want, x.Date+"|"+decimal.MustParse(x.Amount).Quantize(-2).Text()+"|"+x.Direction+"|"+x.Counterparty+"|"+kind)
		}
		sort.Strings(want)
		l := got[key]
		if l.pass == nil || *l.pass != m.Pass || fmt.Sprint(l.entries) != fmt.Sprint(want) {
			t.Errorf("linha %s (%s): passe %v, lançamentos %v; origem passe %d, %v", key, l.describe, l.pass, l.entries, m.Pass, want)
		}
	}
	// As linhas tiradas do automático, com o motivo da origem.
	for id, reason := range g.Reasons {
		l := got[byID[id]]
		if l.pass != nil || len(l.flags) == 0 || l.flags[0]["tipo"] != "divergencia" || l.flags[0]["motivo"] != reason {
			t.Errorf("linha %s (%s): marcas %v; origem %s", byID[id], l.describe, l.flags, reason)
		}
	}
	// O resto: a disputa da Escola Crescer (o título a receber e o comprovante) e as tarifas.
	for key, l := range got {
		if matched[key] || l.pass != nil {
			if !matched[key] {
				t.Errorf("linha %s (%s) conciliada; na origem não", key, l.describe)
			}
			continue
		}
		if _, skipped := reasonsByKey(g, byID)[key]; skipped {
			continue
		}
		// O extrato lido do PDF traz o descritivo em caixa mista (o do CSV da origem, em maiúsculas).
		switch desc := strings.ToUpper(l.describe); {
		case strings.Contains(desc, "ESCOLA CRESCER"):
			if len(l.flags) == 0 || l.flags[0]["motivo"] != "ambiguo_passe_1" || len(l.flags[0]["candidatos"].([]any)) != 2 {
				t.Errorf("Escola Crescer: marcas %v", l.flags)
			}
		case strings.Contains(desc, "TARIFA"):
			if len(l.flags) != 0 {
				t.Errorf("tarifa %s com marcas %v", key, l.flags)
			}
		default:
			t.Errorf("linha inesperada sem conciliação: %s (%s) %v", key, l.describe, l.flags)
		}
	}
	// A rodada pela API é idempotente e devolve os baldes da competência.
	var sum struct {
		NewMatches       int            `json:"new_matches"`
		ByPass           map[string]int `json:"by_pass"`
		Reconciled       int            `json:"reconciled"`
		Divergent        int            `json:"divergent"`
		WithoutReceipt   int            `json:"without_receipt"`
		WithoutStatement int            `json:"without_statement"`
		AwaitingReview   int            `json:"awaiting_review"`
	}
	e.srv.Post("/api/v1/companies/"+company+"/reconciliation/run?competence=2026-08", nil).MustStatus(t, http.StatusOK).JSON(t, &sum)
	if sum.NewMatches != 0 || sum.Reconciled != len(g.Matches) || sum.Divergent != len(g.Reasons)+1 || sum.WithoutReceipt != 3 || sum.AwaitingReview != 0 {
		t.Fatalf("baldes = %+v", sum)
	}
	if n := e.count(t, `SELECT count(*) FROM reconciliations WHERE origin = 'auto' AND created_by IS NULL AND method = 'passe_1' AND score = 1`); n != len(g.Matches) {
		t.Fatalf("%d conciliações automáticas", n)
	}
}

func reasonsByKey(g scenarioGolden, byID map[string]string) map[string]string {
	out := map[string]string{}
	for id, r := range g.Reasons {
		out[byID[id]] = r
	}
	return out
}
