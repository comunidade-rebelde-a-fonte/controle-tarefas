// Package calendar trata calendários, feriados e o motor de vencimento [PRD §21].
package calendar

import (
	"sort"
	"time"
)

// Holiday é um feriado numa data civil (meia-noite UTC).
type Holiday struct {
	Date time.Time
	Name string
}

func date(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

// Easter calcula o domingo de Páscoa (algoritmo de Meeus/Jones/Butcher, calendário gregoriano).
func Easter(year int) time.Time {
	a := year % 19
	b := year / 100
	c := year % 100
	d := b / 4
	e := b % 4
	f := (b + 8) / 25
	g := (b - f + 1) / 3
	h := (19*a + b - d - g + 15) % 30
	i := c / 4
	k := c % 4
	l := (32 + 2*e + 2*i - h - k) % 7
	m := (a + 11*h + 22*l) / 451
	month := (h + l - 7*m + 114) / 31
	day := ((h + l - 7*m + 114) % 31) + 1
	return date(year, time.Month(month), day)
}

// NationalHolidays devolve os feriados nacionais do ano, em ordem: fixos e móveis
// (Carnaval segunda e terça, Sexta-feira Santa e Corpus Christi, calculados pela Páscoa).
// A lista é configurável pela tela de feriados; este é o conteúdo inicial do seed.
func NationalHolidays(year int) []Holiday {
	easter := Easter(year)
	hs := []Holiday{
		{date(year, time.January, 1), "Confraternização Universal"},
		{date(year, time.April, 21), "Tiradentes"},
		{date(year, time.May, 1), "Dia do Trabalho"},
		{date(year, time.September, 7), "Independência do Brasil"},
		{date(year, time.October, 12), "Nossa Senhora Aparecida"},
		{date(year, time.November, 2), "Finados"},
		{date(year, time.November, 15), "Proclamação da República"},
		{date(year, time.November, 20), "Dia Nacional de Zumbi e da Consciência Negra"},
		{date(year, time.December, 25), "Natal"},
		{easter.AddDate(0, 0, -48), "Carnaval (segunda-feira)"},
		{easter.AddDate(0, 0, -47), "Carnaval (terça-feira)"},
		{easter.AddDate(0, 0, -2), "Sexta-feira Santa"},
		{easter.AddDate(0, 0, 60), "Corpus Christi"},
	}
	sort.Slice(hs, func(i, j int) bool { return hs[i].Date.Before(hs[j].Date) })
	return hs
}
