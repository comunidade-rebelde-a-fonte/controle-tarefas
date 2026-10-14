// Package engine é o motor determinístico do Fator R [REQ-010, TASK-010]: janela, RBT12,
// FS12, fator, anexo, semáforo, gaps, alíquotas efetivas, economia, carteira e simulador.
//
// É o porte fiel de fator_r/domain (Fator R de origem, commit 9dd7a74): mesmas regras, mesma
// ordem das contas e mesma aritmética decimal (pacote decimal, ADR-007). Não faz I/O, não
// arredonda no meio das contas (só na apresentação) e nunca passa por LLM. Cada tipo e função
// cita o nome correspondente na origem; os nomes dos campos JSON são os da origem.
package engine

import "controle/internal/platform/competence"

// WindowMonths é o tamanho da janela (MESES_JANELA).
const WindowMonths = 12

// Window é a janela móvel do PA: os 12 meses anteriores ao período de apuração (Janela).
type Window struct {
	PA    competence.Competence
	Start competence.Competence // PA-12
	End   competence.Competence // PA-1
}

// NewWindow devolve a janela do PA (janela). Ex.: PA 09/2026 → 09/2025 a 08/2026.
func NewWindow(pa competence.Competence) Window {
	return Window{PA: pa, Start: pa.AddMonths(-WindowMonths), End: pa.AddMonths(-1)}
}

// Months são os 12 meses da janela, em ordem (Janela.meses).
func (w Window) Months() []competence.Competence {
	out := make([]competence.Competence, WindowMonths)
	for i := range out {
		out[i] = w.Start.AddMonths(i)
	}
	return out
}

// ValidMonths são os meses da janela em que a empresa já existia; sem início de atividade
// informado, todos (meses_validos).
func ValidMonths(w Window, activityStart *competence.Competence) []competence.Competence {
	months := w.Months()
	if activityStart == nil {
		return months
	}
	out := make([]competence.Competence, 0, len(months))
	for _, m := range months {
		if !m.Before(*activityStart) {
			out = append(out, m)
		}
	}
	return out
}

// Classify separa os meses válidos em preenchidos e faltantes. Mês anterior ao início de
// atividade não é faltante (classificar).
func Classify(valid []competence.Competence, posted map[competence.Competence]bool) (filled, missing []competence.Competence) {
	filled, missing = []competence.Competence{}, []competence.Competence{}
	for _, m := range valid {
		if posted[m] {
			filled = append(filled, m)
		} else {
			missing = append(missing, m)
		}
	}
	return filled, missing
}

// NewCompanyInWindow indica empresa que começou a atividade depois do início da janela, ou
// seja, com menos de 12 meses no período (empresa_nova_na_janela).
func NewCompanyInWindow(w Window, activityStart *competence.Competence) bool {
	return activityStart != nil && activityStart.After(w.Start)
}
