package matching

import (
	"encoding/json"
	"os"
	"reflect"
	"sort"
	"testing"
	"time"

	"controle/internal/platform/decimal"
)

// golden é a saída do próprio código do Conciliador (testdata/matching_gen.py).
type golden struct {
	Jaro []struct {
		A    string  `json:"a"`
		B    string  `json:"b"`
		Jaro float64 `json:"jaro"`
		JW   float64 `json:"jw"`
	} `json:"jaro"`
	Cascade []struct {
		Statements []gStatement `json:"extratos"`
		Entries    []gEntry     `json:"lancamentos"`
		Threshold  float64      `json:"limiar"`
		Window     int          `json:"janela"`
		MaxN       int          `json:"max_n"`
		Result     gResult      `json:"resultado"`
	} `json:"cascata"`
	Prefilter []gPrefilter `json:"divergentes"`
	Edges     []struct {
		Statements []gStatement `json:"extratos"`
		Entries    []gEntry     `json:"lancamentos"`
		Transfers  []gMark      `json:"transferencia_interna"`
		Reversals  []gMark      `json:"estorno"`
		GrossNet   []gMark      `json:"bruto_liquido"`
	} `json:"bordas"`
	Scenario struct {
		gPrefilter
		Result gResult `json:"resultado"`
	} `json:"cenario"`
	Candidates []struct {
		Entries   []gEntry `json:"lancamentos"`
		Amount    string   `json:"valor"`
		Date      string   `json:"data"`
		Direction string   `json:"sentido"`
		Window    int      `json:"janela"`
		Want      []string `json:"candidatos"`
	} `json:"candidatos"`
}

type gStatement struct {
	ID          string `json:"id"`
	Date        string `json:"data"`
	Amount      string `json:"valor"`
	Direction   string `json:"sentido"`
	Description string `json:"descricao"`
}

type gEntry struct {
	ID           string `json:"id"`
	Date         string `json:"data"`
	Amount       string `json:"valor"`
	Direction    string `json:"sentido"`
	Counterparty string `json:"contraparte"`
}

type gRecord struct {
	ID           string  `json:"id"`
	Date         *string `json:"data"`
	Amount       *string `json:"valor"`
	Direction    string  `json:"sentido"`
	Counterparty *string `json:"contraparte"`
	DocumentRef  *string `json:"documento"`
	Flags        []any   `json:"marcas"`
	Origin       string  `json:"origem"`
}

type gPrefilter struct {
	Statements   []gStatement      `json:"extratos"`
	Entries      []gRecord         `json:"lancamentos"`
	SkipStmts    []string          `json:"pular_extratos"`
	SkipEntries  []string          `json:"pular_lancamentos"`
	SkipsReasons map[string]string `json:"motivos"`
}

type gResult struct {
	Matches []struct {
		Statement string   `json:"extrato"`
		Entries   []string `json:"lancamentos"`
		Pass      int      `json:"passe"`
		Score     string   `json:"score"`
		Method    string   `json:"metodo"`
	} `json:"matches"`
	Divergences []struct {
		Statement  string   `json:"extrato"`
		Candidates []string `json:"candidatos"`
		Reason     string   `json:"motivo"`
	} `json:"divergencias"`
	WithoutReceipt   []string `json:"sem_comprovante"`
	WithoutStatement []string `json:"sem_extrato"`
}

type gMark struct {
	Kind   string   `json:"tipo"`
	IDs    []string `json:"ids"`
	Detail string   `json:"detalhe"`
}

func loadGolden(t *testing.T) golden {
	t.Helper()
	raw, err := os.ReadFile("testdata/matching_golden.json")
	if err != nil {
		t.Fatal(err)
	}
	var g golden
	if err := json.Unmarshal(raw, &g); err != nil {
		t.Fatal(err)
	}
	if len(g.Jaro) < 100 || len(g.Cascade) < 400 || len(g.Prefilter) < 200 || len(g.Edges) < 100 || len(g.Scenario.Statements) != 43 {
		t.Fatal("golden incompleto; gere de novo com testdata/matching_gen.py")
	}
	return g
}

func date(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := time.Parse(time.DateOnly, s)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func statements(t *testing.T, in []gStatement) []Statement {
	out := make([]Statement, len(in))
	for i, s := range in {
		out[i] = Statement{ID: s.ID, Date: date(t, s.Date), Amount: decimal.MustParse(s.Amount), Direction: s.Direction, Description: s.Description}
	}
	return out
}

func entries(t *testing.T, in []gEntry) []Entry {
	out := make([]Entry, len(in))
	for i, e := range in {
		out[i] = Entry{ID: e.ID, Date: date(t, e.Date), Amount: decimal.MustParse(e.Amount), Direction: e.Direction, Counterparty: e.Counterparty}
	}
	return out
}

func records(t *testing.T, in []gRecord) []EntryRecord {
	out := make([]EntryRecord, len(in))
	for i, r := range in {
		rec := EntryRecord{ID: r.ID, Direction: r.Direction, DocumentRef: r.DocumentRef}
		if r.Date != nil {
			d := date(t, *r.Date)
			rec.Date = &d
		}
		if r.Amount != nil {
			a := decimal.MustParse(*r.Amount)
			rec.Amount = &a
		}
		for _, f := range r.Flags {
			if s, ok := f.(string); ok {
				rec.Flags = append(rec.Flags, s)
			}
		}
		out[i] = rec
	}
	return out
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// sameResult compara o resultado com o da origem (o score do passe 3 pelo valor).
func sameResult(t *testing.T, label string, got Result, want gResult) bool {
	t.Helper()
	ok := len(got.Matches) == len(want.Matches) && len(got.Divergences) == len(want.Divergences)
	if ok {
		for i, m := range want.Matches {
			g := got.Matches[i]
			if g.StatementID != m.Statement || !reflect.DeepEqual(g.EntryIDs, m.Entries) || g.Pass != m.Pass || g.Method != m.Method ||
				g.Score.Cmp(decimal.MustParse(m.Score)) != 0 {
				ok = false
			}
		}
		for i, d := range want.Divergences {
			g := got.Divergences[i]
			if g.StatementID != d.Statement || !reflect.DeepEqual(nonNil(g.Candidates), nonNil(d.Candidates)) || g.Reason != d.Reason {
				ok = false
			}
		}
	}
	ok = ok && reflect.DeepEqual(nonNil(got.StatementsWithoutReceipt), nonNil(want.WithoutReceipt)) &&
		reflect.DeepEqual(nonNil(got.ReceiptsWithoutStatement), nonNil(want.WithoutStatement))
	if !ok {
		gb, _ := json.MarshalIndent(got, "", " ")
		wb, _ := json.MarshalIndent(want, "", " ")
		t.Errorf("%s: resultado diverge\n--- porte\n%s\n--- origem\n%s", label, gb, wb)
	}
	return ok
}

// Os valores em ponto flutuante saem iguais, bit a bit.
func TestJaroWinkler_MatchesOrigin(t *testing.T) {
	for _, c := range loadGolden(t).Jaro {
		if got := Jaro(normalize(c.A), normalize(c.B)); got != c.Jaro {
			t.Errorf("Jaro(%q, %q) = %v; origem %v", c.A, c.B, got, c.Jaro)
		}
		if got := JaroWinkler(c.A, c.B); got != c.JW {
			t.Errorf("JaroWinkler(%q, %q) = %v; origem %v", c.A, c.B, got, c.JW)
		}
	}
}

// A cascata com a regra da origem (a ordem do extrato decide) sai igual à da origem em todos os
// casos: os quatro passes, os scores, as divergências e os baldes.
func TestCascade_OneSidedMatchesOrigin(t *testing.T) {
	for i, c := range loadGolden(t).Cascade {
		o := Options{FuzzyThreshold: c.Threshold, WindowDays: c.Window, MaxN: c.MaxN, OneSided: true}
		got := Cascade(statements(t, c.Statements), entries(t, c.Entries), o)
		sameResult(t, "caso "+itoa(i), got, c.Result)
	}
}

// O pré-filtro sai igual ao _ids_divergentes da origem.
func TestPrefilter_MatchesOrigin(t *testing.T) {
	for i, c := range loadGolden(t).Prefilter {
		checkPrefilter(t, "caso "+itoa(i), c)
	}
}

func checkPrefilter(t *testing.T, label string, c gPrefilter) Skips {
	t.Helper()
	sk := Prefilter(statements(t, c.Statements), records(t, c.Entries))
	if got := keys(sk.Statements); !reflect.DeepEqual(got, nonNil(c.SkipStmts)) {
		t.Errorf("%s: linhas fora = %v; origem %v", label, got, c.SkipStmts)
	}
	if got := keys(sk.Entries); !reflect.DeepEqual(got, nonNil(c.SkipEntries)) {
		t.Errorf("%s: lançamentos fora = %v; origem %v", label, got, c.SkipEntries)
	}
	if !reflect.DeepEqual(sk.Reasons, c.SkipsReasons) && (len(sk.Reasons) > 0 || len(c.SkipsReasons) > 0) {
		t.Errorf("%s: motivos = %v; origem %v", label, sk.Reasons, c.SkipsReasons)
	}
	return sk
}

func keys(m map[string]bool) []string {
	out := []string{}
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// As marcações de borda saem iguais às da origem (o bruto × líquido sem janela, como lá).
func TestEdges_MatchOrigin(t *testing.T) {
	conv := func(marks []Mark) []gMark {
		out := []gMark{}
		for _, m := range marks {
			out = append(out, gMark(m))
		}
		return out
	}
	for i, c := range loadGolden(t).Edges {
		stmts, ents := statements(t, c.Statements), entries(t, c.Entries)
		for name, pair := range map[string][2][]gMark{
			"transferência": {conv(InternalTransfers(stmts)), nonNilMarks(c.Transfers)},
			"estorno":       {conv(Reversals(stmts, ReversalWindow)), nonNilMarks(c.Reversals)},
			"bruto×líquido": {conv(GrossNet(stmts, ents, GrossNetMaxDiff, -1)), nonNilMarks(c.GrossNet)},
		} {
			if !reflect.DeepEqual(pair[0], pair[1]) {
				t.Errorf("caso %d, %s: %v; origem %v", i, name, pair[0], pair[1])
			}
		}
	}
}

func nonNilMarks(m []gMark) []gMark {
	if m == nil {
		return []gMark{}
	}
	return m
}

// O buscar_candidatos (a ferramenta do resolvedor) sai igual ao da origem.
func TestCandidates_MatchOrigin(t *testing.T) {
	g := loadGolden(t)
	if len(g.Candidates) < 100 {
		t.Fatal("golden sem os casos do buscar_candidatos")
	}
	for i, c := range g.Candidates {
		var got []string
		for _, e := range Candidates(entries(t, c.Entries), decimal.MustParse(c.Amount), date(t, c.Date), c.Direction, c.Window) {
			got = append(got, e.ID)
		}
		if !reflect.DeepEqual(nonNil(got), nonNil(c.Want)) {
			t.Errorf("caso %d: %v; origem %v", i, got, c.Want)
		}
	}
}

// O cenário Horizonte inteiro (os dois extratos, contas a pagar e a receber e os 13 comprovantes,
// como o carregar_horizonte da origem carrega) passa pelo rodar_matching igual ao da origem.
func TestScenario_MatchesOrigin(t *testing.T) {
	g := loadGolden(t).Scenario
	sk := checkPrefilter(t, "cenário", g.gPrefilter)
	stmts, ents := scenarioInputs(t, g.gPrefilter, sk)
	got := Cascade(stmts, ents, Options{FuzzyThreshold: 0.86, WindowDays: 3, MaxN: 8, OneSided: true})
	sameResult(t, "cenário", got, g.Result)
}

// scenarioInputs são as entradas da cascata no rodar_matching: as linhas fora do pré-filtro e os
// lançamentos completos fora dele.
func scenarioInputs(t *testing.T, g gPrefilter, sk Skips) ([]Statement, []Entry) {
	var stmts []Statement
	for _, s := range statements(t, g.Statements) {
		if !sk.Statements[s.ID] {
			stmts = append(stmts, s)
		}
	}
	var ents []Entry
	for _, r := range g.Entries {
		if r.Date == nil || r.Amount == nil || r.Direction == "" || sk.Entries[r.ID] {
			continue
		}
		cp := ""
		if r.Counterparty != nil {
			cp = *r.Counterparty
		}
		ents = append(ents, Entry{ID: r.ID, Date: date(t, *r.Date), Amount: decimal.MustParse(*r.Amount), Direction: r.Direction, Counterparty: cp})
	}
	return stmts, ents
}
