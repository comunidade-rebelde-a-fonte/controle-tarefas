package seed_test

import "testing"

func TestCalendarSeed_NationalHolidaysPerYear(t *testing.T) {
	pool := seededPool(t) // roda o seed de referência 2x
	if n := count(t, pool, `SELECT count(*) FROM business_calendars WHERE scope = 'NATIONAL'`); n != 1 {
		t.Fatalf("calendários nacionais = %d, want 1", n)
	}
	for year := 2025; year <= 2030; year++ {
		if n := count(t, pool, `SELECT count(*) FROM holidays WHERE extract(year FROM date) = $1`, year); n != 13 {
			t.Fatalf("%d: %d feriados, want 13", year, n)
		}
	}
	if n := count(t, pool, `SELECT count(*) FROM holidays WHERE date = '2027-03-26' AND name = 'Sexta-feira Santa'`); n != 1 {
		t.Fatal("Sexta-feira Santa de 2027 ausente")
	}
}
