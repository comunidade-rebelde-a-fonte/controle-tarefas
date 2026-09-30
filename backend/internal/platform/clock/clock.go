// Package clock abstrai o tempo corrente para permitir testes determinísticos.
//
// Regras de negócio nunca chamam time.Now() diretamente: usam um Clock.
// "Hoje" e a competência corrente são calculados no fuso de negócio
// (APP_TIMEZONE, padrão America/Sao_Paulo) [plan D10].
package clock

import (
	"sync"
	"time"
)

// Clock fornece o instante atual e o fuso de negócio.
type Clock interface {
	Now() time.Time
	Location() *time.Location
}

// System é o relógio real.
type System struct {
	loc *time.Location
}

// NewSystem cria o relógio real no fuso informado.
func NewSystem(loc *time.Location) System {
	return System{loc: loc}
}

func (s System) Now() time.Time           { return time.Now() }
func (s System) Location() *time.Location { return s.loc }

// Fixed é um relógio congelado, ajustável, para testes.
type Fixed struct {
	mu  sync.RWMutex
	t   time.Time
	loc *time.Location
}

// NewFixed cria um relógio parado em t, no fuso loc.
func NewFixed(t time.Time, loc *time.Location) *Fixed {
	return &Fixed{t: t, loc: loc}
}

func (f *Fixed) Now() time.Time {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.t
}

func (f *Fixed) Location() *time.Location { return f.loc }

// Set move o relógio para t.
func (f *Fixed) Set(t time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.t = t
}

// Advance avança o relógio em d.
func (f *Fixed) Advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.t = f.t.Add(d)
}

// Today devolve a data civil corrente no fuso de negócio, representada como
// meia-noite UTC (mesma representação das colunas DATE do banco).
func Today(c Clock) time.Time {
	return DateOf(c.Now().In(c.Location()))
}

// DateOf converte um instante em data civil (meia-noite UTC), usando o fuso do próprio valor.
func DateOf(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}
