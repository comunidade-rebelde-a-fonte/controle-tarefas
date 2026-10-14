package matching

import (
	"reflect"
	"sort"
	"testing"
)

// O cenário Horizonte com a política aprovada (ADR-008): os mesmos 25 casamentos da origem, as
// mesmas 14 linhas tiradas pelo pré-filtro com o motivo, e a disputa da Escola Crescer (o título a
// receber e o comprovante) marcada para a fila.
func TestRun_ScenarioWithTheApprovedPolicy(t *testing.T) {
	g := loadGolden(t).Scenario
	stmts := statements(t, g.Statements)
	recs := records(t, g.Entries)
	var ents []Entry
	for i, r := range recs {
		if r.Date != nil && r.Amount != nil && r.Direction != "" {
			ents = append(ents, Entry{ID: r.ID, Date: *r.Date, Amount: *r.Amount, Direction: r.Direction, Counterparty: deref(g.Entries[i].Counterparty)})
		}
	}
	out := Run(RunInput{Statements: stmts, Neighbors: stmts, Records: recs, Entries: ents, Options: DefaultOptions()})
	got := map[string][]string{}
	for _, m := range out.Matches {
		got[m.StatementID] = m.EntryIDs
	}
	want := map[string][]string{}
	for _, m := range g.Result.Matches {
		want[m.Statement] = m.Entries
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("casamentos = %d; origem %d", len(got), len(want))
	}
	for id, reason := range g.SkipsReasons {
		marks := out.Marks[id]
		if len(marks) == 0 || marks[0].Type != MarkDivergence || marks[0].Reason != reason {
			t.Errorf("linha %s: marcas %+v; origem %s", id, marks, reason)
		}
	}
	var disputed []string
	for id, marks := range out.Marks {
		for _, m := range marks {
			if m.Type == MarkEdge {
				t.Errorf("borda inesperada no cenário: %s %+v", id, m)
			}
			if m.Reason == "ambiguo_passe_1" {
				disputed = append(disputed, id)
			}
		}
	}
	sort.Strings(disputed)
	if len(disputed) != 1 || len(out.Marks[disputed[0]][0].Candidates) != 2 {
		t.Errorf("disputa da Escola Crescer = %v", disputed)
	}
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// Estorno e transferência entre contas saem do automático, inclusive quando o par está fora da
// competência ou já conciliado; o bruto × líquido tira a linha e o comprovante.
func TestRun_EdgesLeaveTheAutomaticMatching(t *testing.T) {
	stmts := []Statement{
		st("e1", "500.00", 10, "PIX RECEBIDO CLIENTE"),
		st("e2", "95.00", 12, "TED RECEBIDA PREFEITURA"),
		st("e3", "70.00", 20, "PIX RECEBIDO OUTRO"),
	}
	reversal := Statement{ID: "e0", Date: day(14), Amount: stmts[0].Amount, Direction: "D", Description: "ESTORNO PIX"}
	ents := []Entry{en("l1", "500.00", 10, "Cliente"), en("l2", "100.00", 12, "Prefeitura"), en("l3", "70.00", 20, "Outro")}
	out := Run(RunInput{Statements: stmts, Neighbors: append([]Statement{reversal}, stmts...), Entries: ents, Options: DefaultOptions()})
	if !reflect.DeepEqual(keysOf(out.Excluded), []string{"e1", "e2"}) {
		t.Fatalf("fora do automático = %v", out.Excluded)
	}
	if m := out.Marks["e1"]; len(m) != 1 || m[0].Type != MarkEdge || m[0].Reason != EdgeReversal || !reflect.DeepEqual(m[0].Related, []string{"e0"}) {
		t.Fatalf("marcas de e1 = %+v", m)
	}
	if m := out.Marks["e2"]; len(m) != 1 || m[0].Reason != EdgeGrossNet || !reflect.DeepEqual(m[0].Related, []string{"l2"}) {
		t.Fatalf("marcas de e2 = %+v", m)
	}
	if len(out.Matches) != 1 || out.Matches[0].StatementID != "e3" {
		t.Fatalf("casamentos = %+v", out.Matches)
	}
	if out.Cascade.ReceiptsWithoutStatement[0] != "l1" || len(out.Cascade.ReceiptsWithoutStatement) != 1 {
		t.Fatalf("comprovantes sem extrato = %v", out.Cascade.ReceiptsWithoutStatement)
	}
}

func keysOf(m map[string]bool) []string {
	out := []string{}
	for k, v := range m {
		if v {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}
