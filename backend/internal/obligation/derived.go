// Package obligation trata as instâncias de obrigação: derivados (atraso, progresso,
// conclusão), alteração de status, dependências, detalhes e histórico [PRD §12, §23].
package obligation

import "time"

// IsOverdue define o atraso [RN-013, PRD §12.4]: aplicável, não final e com vencimento
// efetivo anterior a hoje (no fuso de negócio). Atraso nunca é status manual.
func IsOverdue(applicable, isFinal bool, dueEffective *time.Time, today time.Time) bool {
	if !applicable || isFinal || dueEffective == nil {
		return false
	}
	return dueEffective.Before(today)
}

// Item é o que o cálculo de progresso precisa de cada instância.
type Item struct {
	Applicable       bool
	CountsInProgress bool // snapshot da regra tipo × regime (D2)
	// Metadados do status atual [PRD §11].
	CountsAsProgress        bool
	IsFinal                 bool
	IsBlocking              bool
	ExcludedFromDenominator bool
}

// InDenominator indica se a instância entra nos percentuais [PRD §12]: aplicável, que
// conta no progresso e cujo status não a retira do denominador ("Não se aplica" no Fiscal).
//
// A VALIDAR (Q2, Q3): a coluna Fechamento do Fiscal e o ICMS do Simples ficam fora
// (counts_in_progress = false na regra de regime), portanto fora dos percentuais e do
// "pronto para fechar" (plan R2, R3, R4).
func (i Item) InDenominator() bool {
	return i.Applicable && i.CountsInProgress && !i.ExcludedFromDenominator
}

// Progress agrega empresa × módulo × competência [PRD §12, §10.5].
type Progress struct {
	Denominator     int     `json:"denominator"`
	OperationalDone int     `json:"operational_done"`
	FinalDone       int     `json:"final_done"`
	Blocking        int     `json:"blocking"`
	OperationalPct  float64 `json:"operational_progress"`
	FinalPct        float64 `json:"final_completion"`
	ReadyToClose    bool    `json:"ready_to_close"`
}

// Aggregate calcula progresso operacional (itens que contam / relevantes), conclusão final
// (finais / relevantes) e "pronto para fechar" (todos os relevantes finais, R2).
// Sem itens relevantes, os percentuais valem 1 e o módulo está pronto para fechar.
func Aggregate(items []Item) Progress {
	var p Progress
	for _, it := range items {
		if !it.InDenominator() {
			continue
		}
		p.Denominator++
		if it.CountsAsProgress {
			p.OperationalDone++
		}
		if it.IsFinal {
			p.FinalDone++
		}
		if it.IsBlocking {
			p.Blocking++
		}
	}
	return ProgressFromCounts(p.Denominator, p.OperationalDone, p.FinalDone, p.Blocking)
}

// ProgressFromCounts calcula os percentuais a partir das contagens (também usado pela
// agregação em SQL). Sem itens relevantes, percentuais = 1 e pronto para fechar.
func ProgressFromCounts(denominator, operationalDone, finalDone, blocking int) Progress {
	p := Progress{Denominator: denominator, OperationalDone: operationalDone, FinalDone: finalDone, Blocking: blocking}
	if denominator == 0 {
		p.OperationalPct, p.FinalPct, p.ReadyToClose = 1, 1, true
		return p
	}
	p.OperationalPct = float64(operationalDone) / float64(denominator)
	p.FinalPct = float64(finalDone) / float64(denominator)
	p.ReadyToClose = finalDone == denominator
	return p
}

// NearestDeadline devolve o menor vencimento efetivo entre as instâncias aplicáveis ainda
// não finais (prazo crítico mais próximo da linha).
func NearestDeadline(dues []*time.Time, applicable, final []bool) *time.Time {
	var best *time.Time
	for i, d := range dues {
		if d == nil || !applicable[i] || final[i] {
			continue
		}
		if best == nil || d.Before(*best) {
			best = d
		}
	}
	return best
}
