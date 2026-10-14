package matching

import (
	"reflect"
	"testing"
	"time"

	"controle/internal/platform/decimal"
)

// Regra dos dois lados (ADR-008): a linha concilia só com um único comprovante candidato que só
// tem essa linha como candidata.

func crit(o Options, pass int, s Statement, e Entry) bool {
	switch pass {
	case 1:
		return sameDay(e.Date, s.Date) && e.Amount.Cmp(s.Amount) == 0 && e.Direction == s.Direction
	case 2:
		return e.Amount.Cmp(s.Amount) == 0 && e.Direction == s.Direction && days(e.Date, s.Date) <= o.WindowDays
	default:
		return e.Amount.Cmp(s.Amount) == 0 && e.Direction == s.Direction && JaroWinkler(e.Counterparty, s.Description) >= o.FuzzyThreshold
	}
}

// freeAt são as linhas e os lançamentos livres no início do passe, pelos casamentos dos passes
// anteriores.
func freeAt(pass int, stmts []Statement, ents []Entry, matches []Match) ([]Statement, []Entry) {
	usedS, usedE := map[string]bool{}, map[string]bool{}
	for _, m := range matches {
		if m.Pass < pass {
			usedS[m.StatementID] = true
			for _, id := range m.EntryIDs {
				usedE[id] = true
			}
		}
	}
	var fs []Statement
	for _, s := range stmts {
		if !usedS[s.ID] {
			fs = append(fs, s)
		}
	}
	var fe []Entry
	for _, e := range ents {
		if !usedE[e.ID] {
			fe = append(fe, e)
		}
	}
	return fs, fe
}

// solutionsOf são as soluções do passe 4 da linha sobre os lançamentos dados (todas).
func solutionsOf(o Options, s Statement, ents []Entry) [][]string {
	c := &cascade{o: Options{WindowDays: o.WindowDays, MaxN: o.MaxN}, usedE: map[string]bool{}}
	pool, values, target, ok := c.pool(s, ents)
	if !ok {
		return nil
	}
	sols, _ := subsetSums(values, target, o.MaxN, 0, 0)
	out := make([][]string, len(sols))
	for i, sol := range sols {
		out[i] = entryIDs(pool, sol)
	}
	return out
}

// contested informa se algum casamento levou comprovante que outra linha livre no início do
// passe também tinha como candidato: a disputa que a regra da origem resolve pela ordem.
func contested(o Options, stmts []Statement, ents []Entry, matches []Match) bool {
	byID := map[string]Entry{}
	for _, e := range ents {
		byID[e.ID] = e
	}
	for _, m := range matches {
		fs, fe := freeAt(m.Pass, stmts, ents, matches)
		taken := map[string]bool{}
		for _, id := range m.EntryIDs {
			taken[id] = true
		}
		for _, s := range fs {
			if s.ID == m.StatementID {
				continue
			}
			if m.Pass <= 3 {
				if crit(o, m.Pass, s, byID[m.EntryIDs[0]]) {
					return true
				}
				continue
			}
			for _, sol := range solutionsOf(o, s, fe) {
				for _, id := range sol {
					if taken[id] {
						return true
					}
				}
			}
		}
	}
	return false
}

func toMatches(r gResult) []Match {
	out := make([]Match, len(r.Matches))
	for i, m := range r.Matches {
		out[i] = Match{StatementID: m.Statement, EntryIDs: m.Entries, Pass: m.Pass}
	}
	return out
}

// Sem disputa, a regra dos dois lados dá exatamente o resultado da origem; com disputa, todo
// casamento dela é mútuo e único no seu passe.
func TestCascade_TwoSidedEqualsOriginWithoutDispute(t *testing.T) {
	disputes := 0
	for i, c := range loadGolden(t).Cascade {
		o := Options{FuzzyThreshold: c.Threshold, WindowDays: c.Window, MaxN: c.MaxN}
		stmts, ents := statements(t, c.Statements), entries(t, c.Entries)
		got := Cascade(stmts, ents, o)
		if !contested(o, stmts, ents, toMatches(c.Result)) {
			sameResult(t, "caso "+itoa(i), got, c.Result)
			continue
		}
		disputes++
		checkMutual(t, "caso "+itoa(i), o, stmts, ents, got)
	}
	if disputes < 5 {
		t.Fatalf("só %d casos com disputa: o golden não exercita a regra", disputes)
	}
	t.Logf("%d casos com disputa resolvida pela ordem na origem", disputes)
}

// checkMutual confere que cada casamento é único dos dois lados no início do seu passe.
func checkMutual(t *testing.T, label string, o Options, stmts []Statement, ents []Entry, r Result) {
	t.Helper()
	used := map[string]bool{}
	for _, m := range r.Matches {
		for _, id := range m.EntryIDs {
			if used[id] {
				t.Errorf("%s: lançamento %s em dois casamentos", label, id)
			}
			used[id] = true
		}
		fs, fe := freeAt(m.Pass, stmts, ents, r.Matches)
		var self Statement
		for _, s := range fs {
			if s.ID == m.StatementID {
				self = s
			}
		}
		if m.Pass <= 3 {
			n := 0
			for _, e := range fe {
				if crit(o, m.Pass, self, e) {
					n++
				}
			}
			rivals := 0
			for _, s := range fs {
				for _, e := range fe {
					if e.ID == m.EntryIDs[0] && crit(o, m.Pass, s, e) {
						rivals++
					}
				}
			}
			if n != 1 || rivals != 1 {
				t.Errorf("%s: casamento do passe %d não é mútuo (%d candidatos, %d linhas)", label, m.Pass, n, rivals)
			}
			continue
		}
		if sols := solutionsOf(o, self, fe); len(sols) != 1 {
			t.Errorf("%s: passe 4 com %d soluções", label, len(sols))
		}
	}
}

func day(n int) time.Time { return time.Date(2026, 3, n, 0, 0, 0, 0, time.UTC) }

func st(id, amount string, d int, desc string) Statement {
	return Statement{ID: id, Date: day(d), Amount: decimal.MustParse(amount), Direction: "C", Description: desc}
}

func en(id, amount string, d int, name string) Entry {
	return Entry{ID: id, Date: day(d), Amount: decimal.MustParse(amount), Direction: "C", Counterparty: name}
}

func matchesOf(r Result) map[string][]string {
	out := map[string][]string{}
	for _, m := range r.Matches {
		out[m.StatementID] = m.EntryIDs
	}
	return out
}

// Um comprovante para duas linhas iguais: nenhum passe de data concilia; o passe 3 decide pelo
// nome da contraparte, se só uma linha o tiver. (Com o prefixo "PIX RECEBIDO" no descritivo, o
// Jaro-Winkler da origem fica abaixo de 0,86 e a disputa vai à fila.)
func TestTwoSided_DisputedReceiptGoesToTheNamePass(t *testing.T) {
	stmts := []Statement{st("e1", "100.00", 10, "ANA LUCIA FERREIRA"), st("e2", "100.00", 10, "BRUNO SOUZA")}
	if JaroWinkler("Bruno Souza", "PIX RECEBIDO BRUNO SOUZA") >= 0.86 {
		t.Fatal("o exemplo do comentário mudou")
	}
	r := Cascade(stmts, []Entry{en("l1", "100.00", 10, "Bruno Souza")}, DefaultOptions())
	if !reflect.DeepEqual(matchesOf(r), map[string][]string{"e2": {"l1"}}) || r.Matches[0].Pass != 3 {
		t.Fatalf("casamentos = %+v", r.Matches)
	}
	if len(r.Divergences) != 4 || r.Divergences[0].Reason != "ambiguo_passe_1" || !reflect.DeepEqual(r.Divergences[0].Rivals, []string{"e2"}) {
		t.Fatalf("divergências = %+v", r.Divergences)
	}
	// Na origem, a primeira linha levava o comprovante no passe 1.
	o := DefaultOptions()
	o.OneSided = true
	if got := matchesOf(Cascade(stmts, []Entry{en("l1", "100.00", 10, "Bruno Souza")}, o)); !reflect.DeepEqual(got, map[string][]string{"e1": {"l1"}}) {
		t.Fatalf("origem = %v", got)
	}
}

// Cadeia na janela de data: o comprovante do meio serve às duas linhas; ninguém concilia.
func TestTwoSided_ChainInTheWindowIsAmbiguous(t *testing.T) {
	r := Cascade([]Statement{st("e1", "100.00", 10, "X"), st("e2", "100.00", 12, "Y")}, []Entry{en("l1", "100.00", 11, "Z")}, DefaultOptions())
	if len(r.Matches) != 0 || len(r.StatementsWithoutReceipt) != 2 || len(r.ReceiptsWithoutStatement) != 1 {
		t.Fatalf("resultado = %+v", r)
	}
}

// Passe 4: a mesma soma serve a duas linhas; nenhuma concilia, e cada uma aponta a outra.
func TestTwoSided_Pass4SharedSolution(t *testing.T) {
	r := Cascade([]Statement{st("e1", "300.00", 10, "X"), st("e2", "300.00", 11, "Y")},
		[]Entry{en("l1", "100.00", 10, "A"), en("l2", "200.00", 10, "B")}, DefaultOptions())
	if len(r.Matches) != 0 || len(r.Divergences) != 2 || r.Divergences[0].Reason != "ambiguo_passe_4" ||
		!reflect.DeepEqual(r.Divergences[0].Rivals, []string{"e2"}) || !reflect.DeepEqual(r.Divergences[0].Candidates, []string{"l1", "l2"}) {
		t.Fatalf("resultado = %+v", r)
	}
	// Solução única e sem disputa concilia N:1.
	r = Cascade([]Statement{st("e1", "300.00", 10, "X")}, []Entry{en("l1", "100.00", 10, "A"), en("l2", "200.00", 12, "B")}, DefaultOptions())
	if !reflect.DeepEqual(matchesOf(r), map[string][]string{"e1": {"l1", "l2"}}) || r.Matches[0].Score.String() != "0.8000" {
		t.Fatalf("resultado = %+v", r)
	}
}

// O passe 4 tem orçamento: a linha que passa dele vai à revisão em vez de prender o job.
func TestPass4Budget(t *testing.T) {
	var ents []Entry
	for i := range 30 {
		ents = append(ents, en("l"+itoa(i), "1.00", 10, "A"))
	}
	o := DefaultOptions()
	o.Pass4Budget = 1000
	r := Cascade([]Statement{st("e1", "8.00", 10, "X")}, ents, o)
	if len(r.Matches) != 0 || len(r.Divergences) != 1 || r.Divergences[0].Reason != ReasonSubsetBudget {
		t.Fatalf("resultado = %+v", r.Divergences)
	}
}

// O score do passe 3 é o Decimal(str(round(score, 4))) da origem.
func TestPass3Score(t *testing.T) {
	for in, want := range map[float64]string{1.0: "1.0", 0.93333333: "0.9333", 0.86: "0.86", 0.912345: "0.9123", 0.99995: "1.0"} {
		if got := pass3Score(in); got.String() != want {
			t.Errorf("pass3Score(%v) = %s; esperado %s", in, got, want)
		}
	}
}
