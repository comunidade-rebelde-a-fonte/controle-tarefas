package calendar

import (
	"fmt"
	"time"

	"controle/internal/platform/competence"
)

// Tipos de dia e ajustes do motor de vencimento [PRD §21.1, §38.16].
const (
	DayTypeCalendar = "CALENDAR" // dia N do mês, limitado ao último dia
	DayTypeBusiness = "BUSINESS" // N-ésimo dia útil do mês

	AdjustPrevious = "PREVIOUS" // dia não útil antecipa para o dia útil anterior
	AdjustNext     = "NEXT"     // dia não útil posterga para o próximo dia útil
	AdjustNone     = "NONE"     // mantém a data mesmo em dia não útil
)

// maxScanDays limita a busca por dia útil (um calendário mal configurado não trava o motor).
const maxScanDays = 366

// Rule são os parâmetros puros de uma regra de vencimento.
type Rule struct {
	MonthOffset int    // meses após a competência (0 = no próprio mês)
	BaseDay     int    // dia N (CALENDAR) ou N-ésimo dia útil (BUSINESS)
	DayType     string // CALENDAR | BUSINESS
	Adjustment  string // PREVIOUS | NEXT | NONE (só se aplica a CALENDAR)
}

// Rule converte a regra cadastrada nos parâmetros do motor.
func (r DueDateRule) Rule() Rule {
	return Rule{
		MonthOffset: int(r.MonthOffset),
		BaseDay:     int(r.BaseDay),
		DayType:     r.DayType,
		Adjustment:  r.NonBusinessDayAdjustment,
	}
}

func (r Rule) validate() error {
	if r.MonthOffset < 0 || r.MonthOffset > 12 {
		return fmt.Errorf("month_offset %d fora de 0..12", r.MonthOffset)
	}
	if r.BaseDay < 1 || r.BaseDay > 31 {
		return fmt.Errorf("base_day %d fora de 1..31", r.BaseDay)
	}
	if r.DayType != DayTypeCalendar && r.DayType != DayTypeBusiness {
		return fmt.Errorf("day_type inválido %q", r.DayType)
	}
	switch r.Adjustment {
	case AdjustPrevious, AdjustNext, AdjustNone:
	default:
		return fmt.Errorf("non_business_day_adjustment inválido %q", r.Adjustment)
	}
	return nil
}

// BusinessCalendar é o calendário efetivo de uma empresa: sábados, domingos e a união
// dos feriados nacionais, da UF e do município [PRD §21.2].
type BusinessCalendar struct {
	holidays map[time.Time]struct{}
}

// NewBusinessCalendar une conjuntos de feriados (ex.: nacional, UF e município).
func NewBusinessCalendar(sets ...[]time.Time) BusinessCalendar {
	c := BusinessCalendar{holidays: map[time.Time]struct{}{}}
	for _, set := range sets {
		for _, t := range set {
			c.holidays[dayOf(t)] = struct{}{}
		}
	}
	return c
}

// dayOf normaliza para a data civil (meia-noite UTC) do próprio fuso de t.
func dayOf(t time.Time) time.Time {
	y, m, dd := t.Date()
	return date(y, m, dd)
}

// IsHoliday indica se a data é feriado no calendário.
func (c BusinessCalendar) IsHoliday(t time.Time) bool {
	_, ok := c.holidays[dayOf(t)]
	return ok
}

// IsBusinessDay indica dia útil: nem sábado, nem domingo, nem feriado.
func (c BusinessCalendar) IsBusinessDay(t time.Time) bool {
	switch t.Weekday() {
	case time.Saturday, time.Sunday:
		return false
	}
	return !c.IsHoliday(t)
}

// DueDate calcula o vencimento original de uma obrigação na competência [PRD §21.1]:
// o mês de vencimento é a competência deslocada em MonthOffset; em CALENDAR vence no dia
// BaseDay (limitado ao último dia do mês) com o ajuste configurado se cair em dia não
// útil; em BUSINESS vence no BaseDay-ésimo dia útil do mês.
func DueDate(rule Rule, comp competence.Competence, cal BusinessCalendar) (time.Time, error) {
	if err := rule.validate(); err != nil {
		return time.Time{}, err
	}
	month := comp.AddMonths(rule.MonthOffset)

	if rule.DayType == DayTypeBusiness {
		n := 0
		for t := month.FirstDay(); t.Month() == month.Month; t = t.AddDate(0, 0, 1) {
			if cal.IsBusinessDay(t) {
				n++
				if n == rule.BaseDay {
					return t, nil
				}
			}
		}
		return time.Time{}, fmt.Errorf("%s tem só %d dias úteis; regra pede o %dº", month.Display(), n, rule.BaseDay)
	}

	due := month.FirstDay().AddDate(0, 0, rule.BaseDay-1)
	if last := month.LastDay(); due.After(last) {
		due = last
	}
	switch rule.Adjustment {
	case AdjustPrevious:
		return seekBusinessDay(due, -1, cal)
	case AdjustNext:
		return seekBusinessDay(due, 1, cal)
	default:
		return due, nil
	}
}

// seekBusinessDay devolve t, se for dia útil, ou o primeiro dia útil na direção step.
func seekBusinessDay(t time.Time, step int, cal BusinessCalendar) (time.Time, error) {
	for i := 0; i <= maxScanDays; i++ {
		if cal.IsBusinessDay(t) {
			return t, nil
		}
		t = t.AddDate(0, 0, step)
	}
	return time.Time{}, fmt.Errorf("nenhum dia útil em %d dias a partir de %s", maxScanDays, t.Format(time.DateOnly))
}
