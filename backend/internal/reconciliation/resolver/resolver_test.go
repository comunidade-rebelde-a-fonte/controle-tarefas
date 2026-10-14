package resolver

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"controle/internal/platform/decimal"
)

func id(prefix, n int) string { return fmt.Sprintf("00000000-0000-%04d-0000-%012d", prefix, n) }

func day(d int) time.Time { return time.Date(2026, 8, d, 0, 0, 0, 0, time.UTC) }

func ptr[T any](v T) *T { return &v }

func entry(n int, amount string, d int, dir string, free bool) EntryInfo {
	a := decimal.MustParse(amount)
	date := day(d)
	return EntryInfo{ID: id(2, n), Date: &date, Amount: &a, Direction: &dir, Counterparty: ptr(fmt.Sprintf("Fornecedor %d", n)),
		Status: "extraido", Free: free, Flags: []string{"juros"}}
}

// O contexto da origem: candidatos em D±3 (mais os anotados pelo matching), do mais perto ao mais
// longe; até 15 comprovantes soltos em D±10 fora os candidatos; as linhas irmãs em D±10.
func TestBuildContext_LikeTheOrigin(t *testing.T) {
	flags := []byte(fmt.Sprintf(`[{"tipo": "divergencia", "motivo": "ambiguo_passe_1", "candidatos": [%q]},
		{"tipo": "borda", "motivo": "estorno", "relacionados": [%q]}]`, id(2, 5), id(1, 9)))
	line := Line{ID: id(1, 1), Date: day(10), Amount: decimal.MustParse("300.00"), Direction: "D", Description: "BOLETO ALFA", Flags: flags}
	annotated := EntryInfo{ID: id(2, 5), Status: "parcial", Free: true}
	entries := []EntryInfo{
		entry(1, "300.00", 12, "D", true),  // candidato a 2 dias
		entry(2, "300.00", 9, "D", true),   // candidato a 1 dia
		entry(3, "300.00", 20, "D", true),  // próximo (10 dias)
		entry(4, "300.00", 10, "C", true),  // outro sentido
		annotated,                          // anotado pelo matching, sem data
		entry(6, "300.00", 10, "D", false), // já conciliado
	}
	for n := 10; n < 30; n++ {
		entries = append(entries, entry(n, "50.00", 15, "D", true))
	}
	lines := []Line{line,
		{ID: id(1, 2), Date: day(15), Amount: line.Amount, Direction: "D", Description: "BOLETO ALFA 2", Reconciled: true},
		{ID: id(1, 3), Date: day(25), Amount: line.Amount, Direction: "D", Description: "LONGE"},
		{ID: id(1, 4), Date: day(11), Amount: line.Amount, Direction: "C", Description: "OUTRO SENTIDO"},
	}
	ctx := BuildContext(ContextInput{Line: line, Entries: entries, Lines: lines})
	var p struct {
		Line struct {
			Reason *string `json:"motivo_divergencia"`
			Marks  []any   `json:"marcas"`
		} `json:"transacao_extrato"`
		Candidates []struct {
			ID     string   `json:"id"`
			Date   *string  `json:"data"`
			Alerts []string `json:"alertas"`
			Status string   `json:"status"`
		} `json:"candidatos_mesmo_valor"`
		Nearby   []struct{ ID string } `json:"outros_comprovantes_soltos_proximos"`
		Siblings []struct {
			ID         string `json:"id"`
			Reconciled bool   `json:"ja_conciliada"`
		} `json:"outras_linhas_do_extrato_mesmo_valor"`
		Rules string `json:"regras"`
	}
	if err := json.Unmarshal([]byte(ctx.Payload), &p); err != nil {
		t.Fatal(err)
	}
	var cands []string
	for _, c := range p.Candidates {
		cands = append(cands, c.ID)
	}
	if !reflect.DeepEqual(cands, []string{id(2, 2), id(2, 1), id(2, 5)}) || p.Candidates[2].Date != nil || p.Candidates[2].Status != "parcial" {
		t.Fatalf("candidatos = %v", p.Candidates)
	}
	if !reflect.DeepEqual(p.Candidates[0].Alerts, []string{"juros"}) {
		t.Fatalf("alertas = %v", p.Candidates[0].Alerts)
	}
	if len(p.Nearby) != 15 || p.Nearby[0].ID != id(2, 3) {
		t.Fatalf("próximos = %d, primeiro %v", len(p.Nearby), p.Nearby)
	}
	if len(p.Siblings) != 1 || p.Siblings[0].ID != id(1, 2) || !p.Siblings[0].Reconciled {
		t.Fatalf("irmãs = %v", p.Siblings)
	}
	if p.Line.Reason == nil || *p.Line.Reason != "ambiguo_passe_1" || len(p.Line.Marks) != 2 || !strings.Contains(p.Rules, "Você não concilia") {
		t.Fatalf("linha = %+v", p.Line)
	}
	if ctx.Candidates != 3 || ctx.Nearby != 15 || ctx.Siblings != 1 || !ctx.Offered[id(2, 5)] || ctx.Offered[id(2, 6)] || ctx.Offered[id(2, 4)] {
		t.Fatalf("contagens = %+v", ctx)
	}
}

// O aceite da origem: cada par vira conciliação com os lançamentos livres que existem; id
// inventado, repetido ou já usado é ignorado; sem lançamento, o tipo extrato_sem_comprovante
// confirma a linha sem comprovante.
func TestPlanAccept_LikeTheOrigin(t *testing.T) {
	line1, line2, line3, taken := id(1, 1), id(1, 2), id(1, 3), id(1, 4)
	e1, e2, busy := id(2, 1), id(2, 2), id(2, 3)
	st := AcceptState{
		FreeLines:   map[string]bool{line1: true, line2: true, line3: true},
		FreeEntries: map[string]bool{e1: true, e2: true},
	}
	h := Hypothesis{Pairs: []Pair{
		{StatementID: ptr(line1), EntryIDs: []string{strings.ToUpper(e1)}, Kind: KindOneToOne},
		{StatementID: ptr(line2), EntryIDs: []string{e1}, Kind: KindOneToOne},                                  // e1 já usado
		{StatementID: ptr(line2), EntryIDs: []string{e2, e2, id(2, 99), "não é uuid", busy}, Kind: KindNToOne}, // só e2
		{StatementID: ptr(line3), Kind: KindWithoutReceipt},                                                    // sem comprovante
		{StatementID: ptr(taken), EntryIDs: []string{e2}, Kind: KindOneToOne},                                  // linha conciliada
		{StatementID: nil, Kind: KindWithoutReceipt},                                                           // a da proposta, já usada
		{StatementID: ptr("lixo"), Kind: KindOther},                                                            // nada a fazer
	}}
	got := PlanAccept(h, line1, st)
	want := []Creation{
		{StatementID: line1, EntryIDs: []string{e1}, Method: MethodResolver},
		{StatementID: line2, EntryIDs: []string{e2}, Method: MethodResolver},
		{StatementID: line3, Method: MethodWithoutReceipt},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("plano = %+v", got)
	}
	if got := PlanAccept(Hypothesis{Pairs: []Pair{{Kind: KindOther, EntryIDs: []string{busy}}}}, line1, st); len(got) != 0 {
		t.Fatalf("plano sem nada aceitável = %+v", got)
	}
}

// Os ids citados fora do contexto vão para a decisão só como UUID (o que nem é UUID é contado):
// o texto livre do modelo não sai da proposta.
func TestOutsideContext(t *testing.T) {
	line, e1, foreign := id(1, 1), id(2, 1), id(2, 9)
	offered := map[string]bool{line: true, e1: true}
	h := Hypothesis{Pairs: []Pair{
		{StatementID: ptr(line), EntryIDs: []string{strings.ToUpper(e1), foreign, "Fornecedor Alfa"}},
		{StatementID: ptr(strings.ToUpper(foreign)), EntryIDs: []string{""}},
		{StatementID: nil, EntryIDs: []string{e1}},
	}}
	unknown, malformed := OutsideContext(h, offered)
	if !reflect.DeepEqual(unknown, []string{foreign}) || malformed != 2 {
		t.Fatalf("fora do contexto = %v, inválidos = %d", unknown, malformed)
	}
	if unknown, malformed := OutsideContext(Hypothesis{}, offered); unknown == nil || len(unknown) != 0 || malformed != 0 {
		t.Fatalf("hipótese sem pares = %v, %d", unknown, malformed)
	}
}
