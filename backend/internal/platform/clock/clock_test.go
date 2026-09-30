package clock

import (
	"testing"
	"time"
)

func TestToday_UsesBusinessTimezone(t *testing.T) {
	sp, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		now  time.Time
		want string
	}{
		// 01/10 02:30 UTC ainda é 30/09 em São Paulo (UTC-3).
		{time.Date(2026, 10, 1, 2, 30, 0, 0, time.UTC), "2026-09-30"},
		{time.Date(2026, 10, 1, 3, 0, 0, 0, time.UTC), "2026-10-01"},
		{time.Date(2026, 12, 31, 23, 59, 0, 0, time.UTC), "2026-12-31"},
	}
	for _, tc := range cases {
		c := NewFixed(tc.now, sp)
		got := Today(c)
		if got.Format(time.DateOnly) != tc.want || got.Location() != time.UTC {
			t.Errorf("Today(%s) = %s (%s), want %s", tc.now, got, got.Location(), tc.want)
		}
	}
}

func TestFixed_SetAndAdvance(t *testing.T) {
	c := NewFixed(time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC), time.UTC)
	c.Advance(24 * time.Hour)
	if Today(c).Format(time.DateOnly) != "2026-01-02" {
		t.Fatal(Today(c))
	}
	c.Set(time.Date(2027, 3, 5, 0, 0, 0, 0, time.UTC))
	if !c.Now().Equal(time.Date(2027, 3, 5, 0, 0, 0, 0, time.UTC)) {
		t.Fatal(c.Now())
	}
}
