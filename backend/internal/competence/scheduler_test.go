package competence

import (
	"testing"
	"time"
)

func TestNextRun(t *testing.T) {
	sp, _ := time.LoadLocation("America/Sao_Paulo")
	cases := []struct{ now, want string }{
		{"2026-10-01T02:00:00Z", "2026-10-01T00:05:00-03:00"}, // 23:00 de 30/09 em SP → 00:05 de 01/10
		{"2026-10-01T03:04:00Z", "2026-10-01T00:05:00-03:00"}, // 00:04 em SP → 00:05 do mesmo dia
		{"2026-10-01T03:05:00Z", "2026-10-02T00:05:00-03:00"}, // exatamente 00:05 → dia seguinte
		{"2026-12-31T15:00:00Z", "2027-01-01T00:05:00-03:00"}, // virada de ano
	}
	for _, tc := range cases {
		now, _ := time.Parse(time.RFC3339, tc.now)
		want, _ := time.Parse(time.RFC3339, tc.want)
		if got := NextRun(now, sp); !got.Equal(want) {
			t.Errorf("NextRun(%s) = %s, want %s", tc.now, got, want)
		}
	}
}
