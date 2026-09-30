// Package civil representa datas sem hora ("AAAA-MM-DD"), como vencimentos e
// datas de atendimento. Internamente é meia-noite UTC (mesma representação do DATE).
package civil

import (
	"encoding/json"
	"fmt"
	"time"
)

// Date é uma data civil.
type Date struct{ time.Time }

// Of converte um time.Time na data civil do seu próprio fuso.
func Of(t time.Time) Date {
	y, m, d := t.Date()
	return Date{time.Date(y, m, d, 0, 0, 0, 0, time.UTC)}
}

// Parse lê "AAAA-MM-DD".
func Parse(s string) (Date, error) {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		return Date{}, fmt.Errorf("data inválida %q: use AAAA-MM-DD", s)
	}
	return Date{t}, nil
}

// MustParse é Parse que entra em pânico em erro.
func MustParse(s string) Date {
	d, err := Parse(s)
	if err != nil {
		panic(err)
	}
	return d
}

// Ptr converte *time.Time em *Date.
func Ptr(t *time.Time) *Date {
	if t == nil {
		return nil
	}
	d := Of(*t)
	return &d
}

// TimePtr converte *Date em *time.Time.
func TimePtr(d *Date) *time.Time {
	if d == nil {
		return nil
	}
	t := d.Time
	return &t
}

func (d Date) String() string { return d.Format(time.DateOnly) }

// MarshalJSON serializa como "AAAA-MM-DD".
func (d Date) MarshalJSON() ([]byte, error) { return json.Marshal(d.String()) }

// UnmarshalJSON lê "AAAA-MM-DD".
func (d *Date) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	p, err := Parse(s)
	if err != nil {
		return err
	}
	*d = p
	return nil
}
