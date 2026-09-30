package credential

import (
	"strconv"
	"testing"
	"time"

	"controle/internal/platform/civil"
)

func day(s string) *time.Time {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		panic(err)
	}
	return &t
}

func intp(v int) *int { return &v }

func TestSituationOf(t *testing.T) {
	today := *day("2026-09-30")
	thresholds := []int{7, 15, 30, 60}
	cases := []struct {
		name      string
		expiresAt *time.Time
		want      Situation
	}{
		{"sem data cadastrada", nil, Situation{Code: SituationNoDate, Label: "Sem data cadastrada"}},
		{"vencido há um mês", day("2026-08-30"), Situation{Code: SituationExpired, Label: "Vencido", DaysLeft: intp(-31)}},
		{"vencido ontem", day("2026-09-29"), Situation{Code: SituationExpired, Label: "Vencido", DaysLeft: intp(-1)}},
		{"vence hoje: ainda não venceu", day("2026-09-30"), Situation{Code: SituationDueSoon, Label: "Vence em até 7 dias", ThresholdDays: intp(7), DaysLeft: intp(0)}},
		{"limite da faixa de 7", day("2026-10-07"), Situation{Code: SituationDueSoon, Label: "Vence em até 7 dias", ThresholdDays: intp(7), DaysLeft: intp(7)}},
		{"início da faixa de 15", day("2026-10-08"), Situation{Code: SituationDueSoon, Label: "Vence em até 15 dias", ThresholdDays: intp(15), DaysLeft: intp(8)}},
		{"limite da faixa de 15", day("2026-10-15"), Situation{Code: SituationDueSoon, Label: "Vence em até 15 dias", ThresholdDays: intp(15), DaysLeft: intp(15)}},
		{"faixa de 30", day("2026-10-20"), Situation{Code: SituationDueSoon, Label: "Vence em até 30 dias", ThresholdDays: intp(30), DaysLeft: intp(20)}},
		{"limite da faixa de 60", day("2026-11-29"), Situation{Code: SituationDueSoon, Label: "Vence em até 60 dias", ThresholdDays: intp(60), DaysLeft: intp(60)}},
		{"vigente: além da maior faixa", day("2026-11-30"), Situation{Code: SituationCurrent, Label: "Vigente", DaysLeft: intp(61)}},
		{"vigente: próximo ano", day("2027-09-30"), Situation{Code: SituationCurrent, Label: "Vigente", DaysLeft: intp(365)}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := SituationOf(c.expiresAt, today, thresholds)
			if !equalSituation(got, c.want) {
				t.Fatalf("got %s, want %s", fmtSituation(got), fmtSituation(c.want))
			}
		})
	}
}

// As faixas são configuráveis [PRD §19.3]: outros limites mudam a classificação.
func TestSituationOf_ConfiguredThresholds(t *testing.T) {
	today := *day("2026-09-30")
	got := SituationOf(day("2026-10-20"), today, []int{10, 45})
	want := Situation{Code: SituationDueSoon, Label: "Vence em até 45 dias", ThresholdDays: intp(45), DaysLeft: intp(20)}
	if !equalSituation(got, want) {
		t.Fatalf("got %s, want %s", fmtSituation(got), fmtSituation(want))
	}
	// Faixas fora de ordem são tratadas em ordem crescente.
	got = SituationOf(day("2026-10-03"), today, []int{45, 10})
	if got.ThresholdDays == nil || *got.ThresholdDays != 10 {
		t.Fatalf("got %s, want faixa de 10", fmtSituation(got))
	}
	// Sem faixas: só vencido, vigente ou sem data.
	if got := SituationOf(day("2026-10-01"), today, nil); got.Code != SituationCurrent {
		t.Fatalf("sem faixas: got %s", fmtSituation(got))
	}
}

// A hora e o fuso da data de validade não alteram a contagem de dias.
func TestSituationOf_IgnoresTimeOfDay(t *testing.T) {
	today := time.Date(2026, 9, 30, 23, 30, 0, 0, time.FixedZone("BRT", -3*3600))
	exp := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	got := SituationOf(&exp, today, []int{7})
	if got.DaysLeft == nil || *got.DaysLeft != 1 {
		t.Fatalf("got %s, want 1 dia", fmtSituation(got))
	}
}

func TestMatchesFilter(t *testing.T) {
	today := *day("2026-09-30")
	th := []int{7, 15, 30, 60}
	item := func(exp *time.Time, renewal string) Control {
		return Control{ExpiresAt: civil.Ptr(exp), RenewalStatus: renewal, Situation: SituationOf(exp, today, th)}
	}
	expired := item(day("2026-09-01"), RenewalCurrent)
	in5 := item(day("2026-10-05"), RenewalCurrent)
	in20 := item(day("2026-10-20"), RenewalRequested)
	in90 := item(day("2026-12-29"), RenewalAwaitingClient)
	noDate := item(nil, RenewalCurrent)

	cases := []struct {
		filter string
		want   map[string]bool
	}{
		{FilterExpired, map[string]bool{"expired": true}},
		{"due_7", map[string]bool{"in5": true}},
		{"due_30", map[string]bool{"in5": true, "in20": true}}, // acumulado: não vencidos em até 30 dias
		{"due_60", map[string]bool{"in5": true, "in20": true}},
		{FilterAwaitingClient, map[string]bool{"in90": true}},
		{FilterNoDate, map[string]bool{"noDate": true}},
	}
	all := map[string]Control{"expired": expired, "in5": in5, "in20": in20, "in90": in90, "noDate": noDate}
	for _, c := range cases {
		f, err := ParseFilter(c.filter)
		if err != nil {
			t.Fatalf("%s: %v", c.filter, err)
		}
		for name, ctl := range all {
			if got := f.Match(ctl); got != c.want[name] {
				t.Errorf("filtro %s, item %s: got %v, want %v", c.filter, name, got, c.want[name])
			}
		}
	}
	for _, bad := range []string{"due_0", "due_366", "due_x", "vencidos", "due_"} {
		if _, err := ParseFilter(bad); err == nil {
			t.Errorf("ParseFilter(%q): esperava erro", bad)
		}
	}
	if f, err := ParseFilter(""); err != nil || !f.Match(noDate) || !f.Match(expired) {
		t.Errorf("filtro vazio deveria aceitar tudo (err=%v)", err)
	}
}

func equalSituation(a, b Situation) bool {
	eq := func(x, y *int) bool { return (x == nil && y == nil) || (x != nil && y != nil && *x == *y) }
	return a.Code == b.Code && a.Label == b.Label && eq(a.ThresholdDays, b.ThresholdDays) && eq(a.DaysLeft, b.DaysLeft)
}

func fmtSituation(s Situation) string {
	v := func(p *int) string {
		if p == nil {
			return "nil"
		}
		return strconv.Itoa(*p)
	}
	return s.Code + "/" + s.Label + " faixa=" + v(s.ThresholdDays) + " dias=" + v(s.DaysLeft)
}
