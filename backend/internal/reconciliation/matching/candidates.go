package matching

import (
	"time"

	"controle/internal/platform/decimal"
)

// Candidates é o buscar_candidatos da origem (app/domain/conciliacao.py), a ferramenta de leitura
// do resolvedor: os lançamentos de mesmo valor e sentido com a data em D±window, na ordem dada.
func Candidates(entries []Entry, amount decimal.Decimal, date time.Time, direction string, window int) []Entry {
	out := []Entry{}
	for _, e := range entries {
		if e.Amount.Cmp(amount) == 0 && e.Direction == direction && days(e.Date, date) <= window {
			out = append(out, e)
		}
	}
	return out
}
