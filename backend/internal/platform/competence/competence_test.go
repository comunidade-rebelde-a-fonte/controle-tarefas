package competence

import (
	"encoding/json"
	"testing"
	"time"

	"controle/internal/platform/clock"
)

func TestParseAndFormat(t *testing.T) {
	cases := []struct {
		in, str, display string
		ok               bool
	}{
		{"2026-09", "2026-09", "09/2026", true},
		{"2025-12", "2025-12", "12/2025", true},
		{"2026-13", "", "", false},
		{"2026-00", "", "", false},
		{"26-09", "", "", false},
		{"09/2026", "", "", false},
		{"", "", "", false},
	}
	for _, tc := range cases {
		c, err := Parse(tc.in)
		if (err == nil) != tc.ok {
			t.Fatalf("Parse(%q) err = %v, ok esperado %v", tc.in, err, tc.ok)
		}
		if tc.ok && (c.String() != tc.str || c.Display() != tc.display) {
			t.Fatalf("Parse(%q) = %s / %s", tc.in, c.String(), c.Display())
		}
	}
}

func TestNavigationAcrossYears(t *testing.T) {
	cases := []struct{ in, next, prev string }{
		{"2026-12", "2027-01", "2026-11"},
		{"2027-01", "2027-02", "2026-12"},
		{"2026-06", "2026-07", "2026-05"},
	}
	for _, tc := range cases {
		c := MustParse(tc.in)
		if c.Next().String() != tc.next || c.Prev().String() != tc.prev {
			t.Fatalf("%s: next=%s prev=%s", tc.in, c.Next(), c.Prev())
		}
	}
	if MustParse("2026-11").AddMonths(14).String() != "2028-01" {
		t.Fatal("AddMonths(14)")
	}
	if MustParse("2026-01").AddMonths(-13).String() != "2024-12" {
		t.Fatal("AddMonths(-13)")
	}
}

func TestDates(t *testing.T) {
	c := MustParse("2024-02")
	if c.FirstDay() != time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC) {
		t.Fatal(c.FirstDay())
	}
	if c.LastDay() != time.Date(2024, 2, 29, 0, 0, 0, 0, time.UTC) {
		t.Fatal(c.LastDay())
	}
	if FromDate(time.Date(2026, 9, 30, 23, 0, 0, 0, time.UTC)).String() != "2026-09" {
		t.Fatal("FromDate")
	}
}

func TestCurrentUsesBusinessTimezone(t *testing.T) {
	sp, _ := time.LoadLocation("America/Sao_Paulo")
	// 01/10 01:00 UTC ainda é 30/09 em São Paulo.
	if got := Current(clock.NewFixed(time.Date(2026, 10, 1, 1, 0, 0, 0, time.UTC), sp)); got.String() != "2026-09" {
		t.Fatalf("Current = %s, want 2026-09", got)
	}
	if got := Current(clock.NewFixed(time.Date(2026, 10, 1, 3, 0, 0, 0, time.UTC), sp)); got.String() != "2026-10" {
		t.Fatalf("Current = %s, want 2026-10", got)
	}
}

func TestCompareAndJSON(t *testing.T) {
	a, b := MustParse("2026-09"), MustParse("2026-10")
	if !a.Before(b) || !b.After(a) || a.Compare(b) != -1 || !a.Equal(MustParse("2026-09")) {
		t.Fatal("comparação")
	}
	raw, _ := json.Marshal(struct{ C Competence }{a})
	if string(raw) != `{"C":"2026-09"}` {
		t.Fatal(string(raw))
	}
	var out struct{ C Competence }
	if err := json.Unmarshal([]byte(`{"C":"2027-01"}`), &out); err != nil || out.C.String() != "2027-01" {
		t.Fatal(out, err)
	}
	if err := json.Unmarshal([]byte(`{"C":"2027-1"}`), &out); err == nil {
		t.Fatal("deveria rejeitar formato inválido")
	}
}
