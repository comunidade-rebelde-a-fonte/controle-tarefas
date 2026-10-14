package factorr_test

import (
	"testing"
	"time"

	"controle/internal/platform/decimal"
)

func dec(s string) decimal.Decimal { return decimal.MustParse(s) }

func mustDate(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := time.Parse(time.DateOnly, s)
	if err != nil {
		t.Fatal(err)
	}
	return d
}
