package obligation

import (
	"testing"
	"time"
)

func day(s string) *time.Time {
	t, _ := time.Parse(time.DateOnly, s)
	return &t
}

func TestCA012_OverdueComputed(t *testing.T) {
	today := *day("2026-11-12")
	cases := []struct {
		name       string
		applicable bool
		final      bool
		due        *time.Time
		want       bool
	}{
		{"vencida e não concluída", true, false, day("2026-11-11"), true},
		{"vence hoje não está atrasada", true, false, day("2026-11-12"), false},
		{"vence amanhã", true, false, day("2026-11-13"), false},
		{"concluída não atrasa", true, true, day("2026-10-01"), false},
		{"não aplicável não atrasa", false, false, day("2026-10-01"), false},
		{"sem vencimento", true, false, nil, false},
	}
	for _, tc := range cases {
		if got := IsOverdue(tc.applicable, tc.final, tc.due, today); got != tc.want {
			t.Errorf("%s: IsOverdue = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func done() Item {
	return Item{Applicable: true, CountsInProgress: true, CountsAsProgress: true, IsFinal: true}
}
func started() Item { return Item{Applicable: true, CountsInProgress: true, CountsAsProgress: true} }
func pending() Item { return Item{Applicable: true, CountsInProgress: true, IsBlocking: true} }
func todo() Item    { return Item{Applicable: true, CountsInProgress: true} }

func TestAggregate_ProgressAndCompletion(t *testing.T) {
	p := Aggregate([]Item{done(), done(), started(), pending(), todo()})
	if p.Denominator != 5 || p.OperationalDone != 3 || p.FinalDone != 2 || p.Blocking != 1 {
		t.Fatalf("contagens = %+v", p)
	}
	if p.OperationalPct != 0.6 || p.FinalPct != 0.4 || p.ReadyToClose {
		t.Fatalf("percentuais = %+v", p)
	}
}

func TestCA007_NotApplicableKeepsPercent(t *testing.T) {
	// IPI configurado como não aplicável: fica fora do denominador e não reduz o percentual.
	ipi := Item{Applicable: false, CountsInProgress: true}
	items := []Item{done(), done(), done(), ipi}
	p := Aggregate(items)
	if p.Denominator != 3 || p.FinalPct != 1 || !p.ReadyToClose {
		t.Fatalf("com IPI não aplicável: %+v, want 3 itens, 100%%, pronto para fechar", p)
	}
	// "Não se aplica" como status no Fiscal também sai do denominador [PRD §11.2].
	naStatus := Item{Applicable: true, CountsInProgress: true, IsFinal: true, ExcludedFromDenominator: true}
	if p := Aggregate([]Item{done(), naStatus, todo()}); p.Denominator != 2 || p.FinalPct != 0.5 {
		t.Fatalf("com status Não se aplica: %+v", p)
	}
}

func TestCA014_ReadyToClose(t *testing.T) {
	// Fechamento (estágio) e ICMS do Simples não contam: não bloqueiam o "pronto para fechar" (R2, R4).
	closingStage := Item{Applicable: true, CountsInProgress: false} // A iniciar
	icmsSimples := Item{Applicable: true, CountsInProgress: false}  // A iniciar
	p := Aggregate([]Item{done(), done(), closingStage, icmsSimples})
	if !p.ReadyToClose || p.Denominator != 2 {
		t.Fatalf("todos os controles finais: %+v, want pronto para fechar", p)
	}
	if p := Aggregate([]Item{done(), started()}); p.ReadyToClose {
		t.Fatal("item em andamento não pode estar pronto para fechar")
	}
	if p := Aggregate(nil); !p.ReadyToClose || p.FinalPct != 1 {
		t.Fatalf("sem itens relevantes: %+v", p)
	}
}

func TestICMSSimplesOutOfPercentages(t *testing.T) {
	icms := Item{Applicable: true, CountsInProgress: false}
	with := Aggregate([]Item{done(), todo(), icms})
	without := Aggregate([]Item{done(), todo()})
	if with != without {
		t.Fatalf("ICMS no Simples alterou os percentuais: %+v vs %+v", with, without)
	}
}

func TestNearestDeadline(t *testing.T) {
	dues := []*time.Time{day("2026-11-20"), day("2026-11-10"), day("2026-11-05"), nil}
	got := NearestDeadline(dues, []bool{true, true, false, true}, []bool{false, false, false, false})
	if got == nil || got.Format(time.DateOnly) != "2026-11-10" {
		t.Fatalf("prazo crítico = %v, want 2026-11-10 (a de 05/11 não é aplicável)", got)
	}
	if NearestDeadline(dues[:1], []bool{true}, []bool{true}) != nil {
		t.Fatal("item final não tem prazo crítico")
	}
}
