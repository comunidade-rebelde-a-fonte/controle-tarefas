package matching

import (
	"strconv"

	"controle/internal/reconciliation"
)

func normalize(s string) string { return reconciliation.NormalizeDescription(s) }

func itoa(i int) string { return strconv.Itoa(i) }
