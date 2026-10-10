package matching

import "sort"

// A rodada de uma empresa numa competência (o rodar_matching da origem, ADR-008): o pré-filtro, as
// marcas de borda e a cascata dos dois lados. Tudo o que não concilia fica marcado para a fila com
// o motivo; nada é resolvido por chute.

// Tipos de marca gravados na linha do extrato.
const (
	MarkDivergence = "divergencia"
	MarkEdge       = "borda"
)

// StatementMark é uma marca da linha: divergência (o motivo, os candidatos e as linhas que os
// disputam) ou caso de borda (o motivo e as linhas ou lançamentos relacionados).
type StatementMark struct {
	Type       string   `json:"tipo"`
	Reason     string   `json:"motivo"`
	Candidates []string `json:"candidatos,omitempty"`
	Rivals     []string `json:"concorrentes,omitempty"`
	Related    []string `json:"relacionados,omitempty"`
}

// RunInput são as entradas da rodada.
type RunInput struct {
	// Statements são as linhas livres da competência, na ordem do extrato.
	Statements []Statement
	// Neighbors são as linhas da empresa a até 7 dias da competência, livres ou não: o par de um
	// estorno ou de uma transferência pode estar fora dela ou já conciliado.
	Neighbors []Statement
	// Records são todos os lançamentos da empresa (o pré-filtro olha as marcas e a nota fiscal).
	Records []EntryRecord
	// Entries são os lançamentos livres e completos que entram no automático (extraídos e
	// manuais; o parcial espera a revisão).
	Entries []Entry
	Options Options
}

// RunOutput é o resultado da rodada.
type RunOutput struct {
	Matches []Match
	// Marks são as marcas de cada linha livre da competência que não conciliou.
	Marks map[string][]StatementMark
	// Cascade é o resultado da cascata (os baldes).
	Cascade Result
	// Excluded são as linhas tiradas do automático antes da cascata (pré-filtro e bordas).
	Excluded map[string]bool
}

// Run executa a rodada sobre as entradas, sem efeito colateral.
func Run(in RunInput) RunOutput {
	out := RunOutput{Marks: map[string][]StatementMark{}, Excluded: map[string]bool{}}
	inScope := map[string]bool{}
	for _, s := range in.Statements {
		inScope[s.ID] = true
	}
	skips := Prefilter(in.Statements, in.Records)
	for _, s := range in.Statements {
		if skips.Statements[s.ID] {
			out.Excluded[s.ID] = true
			out.Marks[s.ID] = append(out.Marks[s.ID], StatementMark{Type: MarkDivergence, Reason: skips.Reasons[s.ID], Candidates: []string{}})
		}
	}
	excludedEntries := map[string]bool{}
	for id := range skips.Entries {
		excludedEntries[id] = true
	}
	// Bordas: a linha da competência envolvida sai do automático, com os relacionados.
	edge := func(m Mark, self string, related ...string) {
		out.Excluded[self] = true
		out.Marks[self] = append(out.Marks[self], StatementMark{Type: MarkEdge, Reason: m.Kind, Related: related})
	}
	neighbors := in.Neighbors
	for _, m := range append(InternalTransfers(neighbors), Reversals(neighbors, ReversalWindow)...) {
		a, b := m.IDs[0], m.IDs[1]
		if inScope[a] {
			edge(m, a, b)
		}
		if inScope[b] {
			edge(m, b, a)
		}
	}
	for _, m := range GrossNet(in.Statements, in.Entries, GrossNetMaxDiff, in.Options.WindowDays) {
		edge(m, m.IDs[0], m.IDs[1])
		excludedEntries[m.IDs[1]] = true
	}
	var stmts []Statement
	for _, s := range in.Statements {
		if !out.Excluded[s.ID] {
			stmts = append(stmts, s)
		}
	}
	var entries []Entry
	for _, e := range in.Entries {
		if !excludedEntries[e.ID] {
			entries = append(entries, e)
		}
	}
	out.Cascade = Cascade(stmts, entries, in.Options)
	out.Matches = out.Cascade.Matches
	matched := map[string]bool{}
	for _, m := range out.Matches {
		matched[m.StatementID] = true
	}
	for _, d := range out.Cascade.Divergences {
		if matched[d.StatementID] {
			continue // a disputa de um passe foi resolvida num passe seguinte
		}
		out.Marks[d.StatementID] = append(out.Marks[d.StatementID], StatementMark{Type: MarkDivergence, Reason: d.Reason,
			Candidates: d.Candidates, Rivals: d.Rivals})
	}
	for id, marks := range out.Marks {
		out.Marks[id] = dedupeMarks(marks)
	}
	return out
}

// dedupeMarks tira marcas repetidas (o mesmo par de borda visto de mais de um jeito), mantendo a
// ordem.
func dedupeMarks(marks []StatementMark) []StatementMark {
	seen := map[string]bool{}
	out := marks[:0]
	for _, m := range marks {
		related := append([]string(nil), m.Related...)
		sort.Strings(related)
		key := m.Type + "|" + m.Reason + "|" + join(m.Candidates) + "|" + join(related)
		if !seen[key] {
			seen[key] = true
			out = append(out, m)
		}
	}
	return out
}

func join(list []string) string {
	s := ""
	for _, x := range list {
		s += x + ","
	}
	return s
}
