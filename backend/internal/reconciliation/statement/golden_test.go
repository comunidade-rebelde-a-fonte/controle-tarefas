package statement

import (
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"controle/internal/platform/decimal"
)

// golden é a saída do próprio código do Conciliador (testdata/statement_gen.py).
type golden struct {
	ExtractJSON []struct {
		In  string `json:"entrada"`
		Out string `json:"saida"`
	} `json:"extrair_json"`
	Statement []outputCase `json:"extrato"`
	Triage    []outputCase `json:"triagem"`
	Validator []struct {
		In struct {
			Start        string `json:"periodo_inicio"`
			PeriodEnd    string `json:"periodo_fim"`
			Opening      string `json:"saldo_inicial"`
			Closing      string `json:"saldo_final"`
			Transactions []struct {
				Order     int    `json:"ordem"`
				Date      string `json:"data"`
				Amount    string `json:"valor"`
				Direction string `json:"sentido"`
			} `json:"transacoes"`
		} `json:"entrada"`
		JSON  string   `json:"json"`
		Hints []string `json:"dicas"`
	} `json:"validador"`
	CSV []struct {
		In      string          `json:"entrada"`
		Bank    string          `json:"banco"`
		Branch  string          `json:"agencia"`
		Account string          `json:"conta"`
		Start   string          `json:"inicio"`
		End     string          `json:"fim"`
		Result  string          `json:"resultado"`
		Out     json.RawMessage `json:"saida"`
	} `json:"csv"`
	TXT []struct {
		In      string `json:"entrada"`
		Result  string `json:"resultado"`
		Branch  string `json:"agencia"`
		Account string `json:"conta"`
	} `json:"txt"`
	PDF []struct {
		In          string          `json:"entrada"`
		IsStatement bool            `json:"eh_extrato"`
		Result      string          `json:"resultado"`
		Out         json.RawMessage `json:"saida"`
	} `json:"pdf"`
}

type outputCase struct {
	In     string          `json:"entrada"`
	Result string          `json:"resultado"`
	Out    json.RawMessage `json:"saida"`
	Guard  []string        `json:"guarda"`
}

func loadGolden(t *testing.T) golden {
	t.Helper()
	raw, err := os.ReadFile("testdata/statement_golden.json")
	if err != nil {
		t.Fatal(err)
	}
	var g golden
	if err := json.Unmarshal(raw, &g); err != nil {
		t.Fatal(err)
	}
	if len(g.Statement) < 100 || len(g.Triage) < 30 || len(g.Validator) < 20 || len(g.CSV) < 25 || len(g.PDF) < 20 {
		t.Fatal("golden incompleto; gere de novo com testdata/statement_gen.py")
	}
	return g
}

// canonical normaliza um JSON (chaves ordenadas) para comparar conteúdo.
func canonical(t *testing.T, b []byte) string {
	t.Helper()
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatalf("JSON inválido %s: %v", b, err)
	}
	out, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func TestExtractJSON_MatchesOrigin(t *testing.T) {
	for _, c := range loadGolden(t).ExtractJSON {
		if got := ExtractJSON(c.In); got != c.Out {
			t.Errorf("ExtractJSON(%q) = %q; origem %q", c.In, got, c.Out)
		}
	}
}

// A saída do extrator passa pela mesma validação do pydantic da origem: o que ele aceita sai
// igual; o que ele recusa (ou derrubava o job) é recusado; e os limites do banco ("guarda")
// também recusam.
func TestParseStatement_MatchesPydantic(t *testing.T) {
	for _, c := range loadGolden(t).Statement {
		got, err := ParseStatement(c.In)
		accept := c.Result == "ok" && len(c.Guard) == 0
		if !accept {
			if err == nil || !errors.Is(err, ErrInvalidOutput) {
				t.Errorf("aceitou o que a origem recusa (%s, guarda %v):\n%s", c.Result, c.Guard, c.In)
			}
			continue
		}
		if err != nil {
			t.Errorf("recusou o que a origem aceita: %v\n%s", err, c.In)
			continue
		}
		b, err := json.Marshal(got)
		if err != nil {
			t.Fatal(err)
		}
		if canonical(t, b) != canonical(t, c.Out) {
			t.Errorf("saída diverge:\n--- porte\n%s\n--- origem\n%s\n--- entrada\n%s", b, c.Out, c.In)
		}
	}
}

func TestParseTriage_MatchesPydantic(t *testing.T) {
	for _, c := range loadGolden(t).Triage {
		got, err := ParseTriage(c.In)
		accept := c.Result == "ok" && len(c.Guard) == 0
		if !accept {
			if err == nil || !errors.Is(err, ErrInvalidOutput) {
				t.Errorf("aceitou o que a origem recusa (%s, guarda %v): %s", c.Result, c.Guard, c.In)
			}
			continue
		}
		if err != nil {
			t.Errorf("recusou o que a origem aceita: %v\n%s", err, c.In)
			continue
		}
		b, err := json.Marshal(got)
		if err != nil {
			t.Fatal(err)
		}
		if canonical(t, b) != canonical(t, c.Out) {
			t.Errorf("triagem diverge:\n--- porte\n%s\n--- origem\n%s", b, c.Out)
		}
	}
}

func mustDate(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := time.Parse(time.DateOnly, s)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// O validador e as dicas saem iguais aos da origem, inclusive o JSON do feedback, byte a byte.
func TestValidate_MatchesOrigin(t *testing.T) {
	for i, c := range loadGolden(t).Validator {
		s := Statement{
			PeriodStart: mustDate(t, c.In.Start), PeriodEnd: mustDate(t, c.In.PeriodEnd),
			OpeningBalance: decimal.MustParse(c.In.Opening), ClosingBalance: decimal.MustParse(c.In.Closing),
		}
		for _, tx := range c.In.Transactions {
			s.Transactions = append(s.Transactions, Transaction{
				Order: tx.Order, Date: mustDate(t, tx.Date), Amount: decimal.MustParse(tx.Amount), Direction: tx.Direction,
			})
		}
		r := Validate(s)
		b, err := json.Marshal(r)
		if err != nil {
			t.Fatal(err)
		}
		if string(b) != c.JSON {
			t.Errorf("caso %d: resultado diverge:\n--- porte\n%s\n--- origem\n%s", i, b, c.JSON)
		}
		if got := Hints(s, r); !reflect.DeepEqual(got, c.Hints) {
			t.Errorf("caso %d: dicas = %v; origem %v", i, got, c.Hints)
		}
	}
}

func TestFromCSV_MatchesOrigin(t *testing.T) {
	for _, c := range loadGolden(t).CSV {
		got, err := FromCSV(c.In, c.Bank, c.Branch, c.Account, mustDate(t, c.Start), mustDate(t, c.End))
		if c.Result != "ok" {
			if err == nil {
				t.Errorf("leu o CSV que a origem recusa (%s):\n%s", c.Result, c.In)
			}
			continue
		}
		if err != nil {
			t.Errorf("recusou o CSV que a origem lê: %v\n%s", err, c.In)
			continue
		}
		b, err := json.Marshal(got)
		if err != nil {
			t.Fatal(err)
		}
		if canonical(t, b) != canonical(t, c.Out) {
			t.Errorf("CSV diverge:\n--- porte\n%s\n--- origem\n%s", b, c.Out)
		}
	}
}

func TestTXTMetadata_MatchesOrigin(t *testing.T) {
	for _, c := range loadGolden(t).TXT {
		branch, account, err := TXTMetadata(c.In)
		if c.Result != "ok" {
			if err == nil {
				t.Errorf("leu o TXT que a origem recusa: %q", c.In)
			}
			continue
		}
		if err != nil || branch != c.Branch || account != c.Account {
			t.Errorf("TXTMetadata(%q) = %q, %q, %v; origem %q, %q", c.In, branch, account, err, c.Branch, c.Account)
		}
	}
}

func TestFromPDFText_MatchesOrigin(t *testing.T) {
	for _, c := range loadGolden(t).PDF {
		head := c.In[:min(len(c.In), 60)]
		if got := IsStatement(c.In); got != c.IsStatement {
			t.Errorf("IsStatement = %v; origem %v (%q)", got, c.IsStatement, head)
		}
		got, err := FromPDFText(c.In)
		if c.Result != "ok" {
			if err == nil {
				t.Errorf("leu o texto que a origem recusa (%s): %q", c.Result, head)
			}
			continue
		}
		if err != nil {
			t.Errorf("recusou o texto que a origem lê: %v (%q)", err, head)
			continue
		}
		b, err := json.Marshal(got)
		if err != nil {
			t.Fatal(err)
		}
		if canonical(t, b) != canonical(t, c.Out) {
			t.Errorf("texto do PDF diverge:\n--- porte\n%s\n--- origem\n%s", b, c.Out)
		}
	}
}

// Os testes da origem sobre os arquivos do cenário (test_csv_extrato e test_pdf_extrato).
func TestReaders_ScenarioFilesClose(t *testing.T) {
	read := func(name string) string {
		b, err := os.ReadFile("../testdata/conciliacao/extratos/" + name)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	aug1, aug31 := mustDate(t, "2026-08-01"), mustDate(t, "2026-08-31")
	fonte, err := FromCSV(read("extrato_construtora_horizonte_2026-08.csv"), "a_fonte", "1234", "56789-0", aug1, aug31)
	if err != nil {
		t.Fatal(err)
	}
	if r := Validate(fonte); !r.OK || !r.Difference.IsZero() || fonte.OpeningBalance.String() != "85430.12" ||
		fonte.ClosingBalance.String() != "213142.37" || fonte.Transactions[0].Direction != Debit || fonte.Transactions[0].Amount.String() != "89.90" {
		t.Fatalf("CSV A Fonte: ok=%v diferença=%s saldos %s → %s", r.OK, r.Difference, fonte.OpeningBalance, fonte.ClosingBalance)
	}
	beta, err := FromCSV(read("extrato_banco_beta_2026-08.csv"), "beta", "0021", "33412-7", aug1, aug31)
	if err != nil {
		t.Fatal(err)
	}
	if r := Validate(beta); !r.OK || beta.OpeningBalance.String() != "15000.00" || beta.ClosingBalance.String() != "22325.00" || len(beta.Transactions) != 3 {
		t.Fatalf("CSV Beta: ok=%v saldos %s → %s, %d linhas", r.OK, beta.OpeningBalance, beta.ClosingBalance, len(beta.Transactions))
	}
	branch, account, err := TXTMetadata(read("extrato_construtora_horizonte_2026-08.txt"))
	if err != nil || branch != "1234" || account != "56789-0" {
		t.Fatalf("TXT = %q %q %v", branch, account, err)
	}
	if !strings.Contains(read("extrato_construtora_horizonte_2026-08.txt"), "SALDO ANTERIOR") {
		t.Fatal("TXT do cenário mudou")
	}
}
