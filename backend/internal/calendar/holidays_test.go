package calendar

import (
	"testing"
	"time"
)

func d(s string) time.Time {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestEaster(t *testing.T) {
	cases := map[int]string{
		2025: "2025-04-20",
		2026: "2026-04-05",
		2027: "2027-03-28",
		2028: "2028-04-16",
		2029: "2029-04-01",
		2030: "2030-04-21",
	}
	for year, want := range cases {
		if got := Easter(year); !got.Equal(d(want)) {
			t.Errorf("Easter(%d) = %s, want %s", year, got.Format(time.DateOnly), want)
		}
	}
}

func TestNationalHolidays(t *testing.T) {
	for year := 2025; year <= 2030; year++ {
		hs := NationalHolidays(year)
		if len(hs) != 13 {
			t.Fatalf("%d: %d feriados, want 13 (9 fixos + 4 móveis)", year, len(hs))
		}
		seen := map[time.Time]bool{}
		for i, h := range hs {
			if seen[h.Date] {
				t.Fatalf("%d: data repetida %s", year, h.Date)
			}
			seen[h.Date] = true
			if i > 0 && !hs[i-1].Date.Before(h.Date) {
				t.Fatalf("%d: feriados fora de ordem", year)
			}
		}
	}
	// Móveis de 2026 (Páscoa em 05/04).
	want := map[string]string{
		"2026-02-16": "Carnaval (segunda-feira)",
		"2026-02-17": "Carnaval (terça-feira)",
		"2026-04-03": "Sexta-feira Santa",
		"2026-06-04": "Corpus Christi",
		"2026-11-20": "Dia Nacional de Zumbi e da Consciência Negra",
	}
	got := map[string]string{}
	for _, h := range NationalHolidays(2026) {
		got[h.Date.Format(time.DateOnly)] = h.Name
	}
	for date, name := range want {
		if got[date] != name {
			t.Errorf("2026 %s = %q, want %q", date, got[date], name)
		}
	}
}
