package calendar

import (
	"testing"
	"time"

	"controle/internal/platform/competence"
)

func dates(ss ...string) []time.Time {
	out := make([]time.Time, len(ss))
	for i, s := range ss {
		out[i] = d(s)
	}
	return out
}

// nationalDates devolve as datas dos feriados nacionais do seed nos anos informados.
func nationalDates(years ...int) []time.Time {
	var out []time.Time
	for _, y := range years {
		for _, h := range NationalHolidays(y) {
			out = append(out, h.Date)
		}
	}
	return out
}

func TestIsBusinessDay(t *testing.T) {
	cal := NewBusinessCalendar(nationalDates(2026))
	cases := []struct {
		day  string
		want bool
	}{
		{"2026-09-28", true},  // segunda-feira
		{"2026-10-02", true},  // sexta-feira
		{"2026-10-03", false}, // sábado
		{"2026-10-04", false}, // domingo
		{"2026-10-12", false}, // feriado nacional fixo numa segunda-feira
		{"2026-04-03", false}, // Sexta-feira Santa (feriado móvel)
		{"2026-02-18", true},  // quarta-feira de Cinzas não é feriado no seed
	}
	for _, c := range cases {
		if got := cal.IsBusinessDay(d(c.day)); got != c.want {
			t.Errorf("IsBusinessDay(%s) = %v, want %v", c.day, got, c.want)
		}
	}
	// A hora do dia não interfere: 03/10/2026 23h em São Paulo continua sendo sábado.
	sp := time.FixedZone("BRT", -3*60*60)
	if cal.IsBusinessDay(time.Date(2026, 10, 3, 23, 0, 0, 0, sp)) {
		t.Error("sábado com horário deveria ser dia não útil")
	}
}

func TestBusinessCalendarUnion(t *testing.T) {
	// Datas sintéticas de feriados estadual e municipal, apenas para o teste da união.
	national := nationalDates(2026)
	state := dates("2026-07-09")
	city := dates("2026-07-10")

	onlyNational := NewBusinessCalendar(national)
	union := NewBusinessCalendar(national, state, city)

	if !onlyNational.IsBusinessDay(d("2026-07-09")) {
		t.Error("sem o calendário da UF, 09/07/2026 deveria ser dia útil")
	}
	for _, day := range []string{"2026-07-09", "2026-07-10", "2026-12-25"} {
		if union.IsBusinessDay(d(day)) {
			t.Errorf("união nacional + UF + município: %s deveria ser dia não útil", day)
		}
	}

	rule := Rule{MonthOffset: 0, BaseDay: 9, DayType: DayTypeCalendar, Adjustment: AdjustNext}
	comp := competence.MustParse("2026-07")
	if got := mustDue(t, rule, comp, onlyNational); got != "2026-07-09" {
		t.Errorf("só nacional: got %s, want 2026-07-09", got)
	}
	// 09 (UF) e 10 (município) caem; 11 e 12 são fim de semana → segunda-feira 13.
	if got := mustDue(t, rule, comp, union); got != "2026-07-13" {
		t.Errorf("união: got %s, want 2026-07-13", got)
	}
}

func mustDue(t *testing.T, r Rule, c competence.Competence, cal BusinessCalendar) string {
	t.Helper()
	got, err := DueDate(r, c, cal)
	if err != nil {
		t.Fatalf("DueDate(%+v, %s): erro inesperado: %v", r, c, err)
	}
	return got.Format(time.DateOnly)
}

func TestDueDate(t *testing.T) {
	cal := NewBusinessCalendar(nationalDates(2025, 2026, 2027, 2028))

	cases := []struct {
		name string
		rule Rule
		comp string
		want string
	}{
		// Dia 31 em fevereiro: limitado ao último dia do mês.
		{"dia 31 em fevereiro, sem ajuste (sábado 28)", Rule{1, 31, DayTypeCalendar, AdjustNone}, "2026-01", "2026-02-28"},
		{"dia 31 em fevereiro, dia útil anterior", Rule{1, 31, DayTypeCalendar, AdjustPrevious}, "2026-01", "2026-02-27"},
		{"dia 31 em fevereiro, próximo dia útil", Rule{1, 31, DayTypeCalendar, AdjustNext}, "2026-01", "2026-03-02"},
		{"dia 31 em fevereiro de ano bissexto", Rule{1, 31, DayTypeCalendar, AdjustNone}, "2028-01", "2028-02-29"},
		{"dia 31 em mês de 30 dias", Rule{1, 31, DayTypeCalendar, AdjustNone}, "2026-03", "2026-04-30"},

		// Sábado com PREVIOUS e NEXT (15/08/2026 é sábado).
		{"sábado, dia útil anterior", Rule{1, 15, DayTypeCalendar, AdjustPrevious}, "2026-07", "2026-08-14"},
		{"sábado, próximo dia útil", Rule{1, 15, DayTypeCalendar, AdjustNext}, "2026-07", "2026-08-17"},
		{"sábado, sem ajuste", Rule{1, 15, DayTypeCalendar, AdjustNone}, "2026-07", "2026-08-15"},

		// Feriados móveis: Carnaval 16 e 17/02/2026, Corpus Christi 04/06/2026.
		{"Carnaval, próximo dia útil pula segunda e terça", Rule{1, 16, DayTypeCalendar, AdjustNext}, "2026-01", "2026-02-18"},
		{"Carnaval, dia útil anterior", Rule{1, 17, DayTypeCalendar, AdjustPrevious}, "2026-01", "2026-02-13"},
		{"Corpus Christi, próximo dia útil", Rule{1, 4, DayTypeCalendar, AdjustNext}, "2026-05", "2026-06-05"},

		// NONE mantém a data mesmo em feriado.
		{"Natal, sem ajuste", Rule{1, 25, DayTypeCalendar, AdjustNone}, "2026-11", "2026-12-25"},
		{"dia útil não é ajustado", Rule{1, 20, DayTypeCalendar, AdjustPrevious}, "2026-09", "2026-10-20"},

		// Deslocamento zero: vence no próprio mês da competência.
		{"deslocamento zero", Rule{0, 10, DayTypeCalendar, AdjustNone}, "2026-09", "2026-09-10"},
		{"deslocamento de dois meses", Rule{2, 10, DayTypeCalendar, AdjustNone}, "2026-09", "2026-11-10"},

		// Virada de ano.
		{"virada de ano, dia corrido", Rule{1, 20, DayTypeCalendar, AdjustNone}, "2026-12", "2027-01-20"},
		{"virada de ano, 1º de janeiro antecipa para o ano anterior", Rule{1, 1, DayTypeCalendar, AdjustPrevious}, "2026-12", "2026-12-31"},
		{"virada de ano, 1º de janeiro posterga para segunda-feira", Rule{1, 1, DayTypeCalendar, AdjustNext}, "2026-12", "2027-01-04"},
		{"virada de ano, 1º dia útil de janeiro", Rule{1, 1, DayTypeBusiness, AdjustNone}, "2026-12", "2027-01-04"},
		{"deslocamento que atravessa o ano", Rule{3, 5, DayTypeCalendar, AdjustNone}, "2026-11", "2027-02-05"},

		// N-ésimo dia útil. Abril/2026: Sexta-feira Santa (03) e Tiradentes (21) não contam.
		{"15º dia útil", Rule{1, 15, DayTypeBusiness, AdjustNone}, "2026-03", "2026-04-23"},
		{"1º dia útil com o dia 1 em domingo", Rule{1, 1, DayTypeBusiness, AdjustNone}, "2026-01", "2026-02-02"},
		{"dia útil ignora o ajuste configurado", Rule{1, 15, DayTypeBusiness, AdjustPrevious}, "2026-03", "2026-04-23"},
		{"20º dia útil", Rule{1, 20, DayTypeBusiness, AdjustNone}, "2026-08", "2026-09-29"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := mustDue(t, c.rule, competence.MustParse(c.comp), cal); got != c.want {
				t.Errorf("got %s, want %s", got, c.want)
			}
		})
	}
}

func TestDueDateInvalid(t *testing.T) {
	cal := NewBusinessCalendar(nationalDates(2026))
	comp := competence.MustParse("2026-01")
	cases := []struct {
		name string
		rule Rule
	}{
		{"dia-base zero", Rule{1, 0, DayTypeCalendar, AdjustNone}},
		{"dia-base acima de 31", Rule{1, 32, DayTypeCalendar, AdjustNone}},
		{"deslocamento negativo", Rule{-1, 10, DayTypeCalendar, AdjustNone}},
		{"deslocamento acima de 12", Rule{13, 10, DayTypeCalendar, AdjustNone}},
		{"tipo de dia desconhecido", Rule{1, 10, "WORKING", AdjustNone}},
		{"ajuste desconhecido", Rule{1, 10, DayTypeCalendar, "LATER"}},
		// Nenhum mês tem 25 dias úteis.
		{"mês sem o N-ésimo dia útil", Rule{1, 25, DayTypeBusiness, AdjustNone}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got, err := DueDate(c.rule, comp, cal); err == nil {
				t.Fatalf("esperava erro, got %s", got.Format(time.DateOnly))
			}
		})
	}
}

func TestDueDateRuleToRule(t *testing.T) {
	r := DueDateRule{MonthOffset: 1, BaseDay: 20, DayType: DayTypeCalendar, NonBusinessDayAdjustment: AdjustPrevious}
	want := Rule{MonthOffset: 1, BaseDay: 20, DayType: DayTypeCalendar, Adjustment: AdjustPrevious}
	if got := r.Rule(); got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}
