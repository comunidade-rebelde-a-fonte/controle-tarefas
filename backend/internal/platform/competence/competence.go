// Package competence representa a competência mensal: armazenada como
// "AAAA-MM" (ou DATE no dia 1) e exibida como "MM/AAAA" [PRD §5.3].
package competence

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"time"

	"controle/internal/platform/clock"
)

var keyRe = regexp.MustCompile(`^([0-9]{4})-(0[1-9]|1[0-2])$`)

// Competence é um mês de referência.
type Competence struct {
	Year  int
	Month time.Month
}

// New cria uma competência (normaliza meses fora de 1..12).
func New(year int, month time.Month) Competence {
	t := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
	return Competence{Year: t.Year(), Month: t.Month()}
}

// Parse lê o formato de armazenamento "AAAA-MM".
func Parse(s string) (Competence, error) {
	m := keyRe.FindStringSubmatch(s)
	if m == nil {
		return Competence{}, fmt.Errorf("competência inválida %q: use AAAA-MM", s)
	}
	y, _ := strconv.Atoi(m[1])
	mo, _ := strconv.Atoi(m[2])
	return Competence{Year: y, Month: time.Month(mo)}, nil
}

// MustParse é Parse que entra em pânico em erro (uso em testes e constantes).
func MustParse(s string) Competence {
	c, err := Parse(s)
	if err != nil {
		panic(err)
	}
	return c
}

// FromDate devolve a competência da data civil informada.
func FromDate(t time.Time) Competence {
	return Competence{Year: t.Year(), Month: t.Month()}
}

// Current devolve a competência corrente no fuso de negócio do relógio.
func Current(c clock.Clock) Competence {
	return FromDate(clock.Today(c))
}

// IsZero indica competência não informada.
func (c Competence) IsZero() bool { return c.Year == 0 }

// String devolve "AAAA-MM".
func (c Competence) String() string { return fmt.Sprintf("%04d-%02d", c.Year, int(c.Month)) }

// Display devolve "MM/AAAA".
func (c Competence) Display() string { return fmt.Sprintf("%02d/%04d", int(c.Month), c.Year) }

// AddMonths desloca a competência em n meses.
func (c Competence) AddMonths(n int) Competence { return New(c.Year, c.Month+time.Month(n)) }

// Next e Prev navegam para a competência seguinte e anterior.
func (c Competence) Next() Competence { return c.AddMonths(1) }
func (c Competence) Prev() Competence { return c.AddMonths(-1) }

// FirstDay devolve o dia 1 da competência (meia-noite UTC), representação em DATE.
func (c Competence) FirstDay() time.Time { return time.Date(c.Year, c.Month, 1, 0, 0, 0, 0, time.UTC) }

// LastDay devolve o último dia do mês.
func (c Competence) LastDay() time.Time { return c.Next().FirstDay().AddDate(0, 0, -1) }

func (c Competence) index() int { return c.Year*12 + int(c.Month) - 1 }

// Compare devolve -1, 0 ou 1.
func (c Competence) Compare(o Competence) int {
	switch a, b := c.index(), o.index(); {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

func (c Competence) Before(o Competence) bool { return c.Compare(o) < 0 }
func (c Competence) After(o Competence) bool  { return c.Compare(o) > 0 }
func (c Competence) Equal(o Competence) bool  { return c.Compare(o) == 0 }

// MonthsUntil devolve quantos meses separam c de o (o - c).
func (c Competence) MonthsUntil(o Competence) int { return o.index() - c.index() }

// MarshalJSON serializa como "AAAA-MM".
func (c Competence) MarshalJSON() ([]byte, error) { return json.Marshal(c.String()) }

// UnmarshalJSON lê "AAAA-MM".
func (c *Competence) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	p, err := Parse(s)
	if err != nil {
		return err
	}
	*c = p
	return nil
}
