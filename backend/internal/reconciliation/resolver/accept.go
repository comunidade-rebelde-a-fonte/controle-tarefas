package resolver

import (
	"regexp"
	"strings"
)

// Métodos da conciliação criada pelo aceite (D-020).
const (
	MethodResolver       = "resolvedor"
	MethodWithoutReceipt = "sem_comprovante"
)

// Creation é uma conciliação que o aceite cria: a linha e os lançamentos (nenhum: a linha
// confirmada sem comprovante).
type Creation struct {
	StatementID string
	EntryIDs    []string
	Method      string
}

// AcceptState é o que o aceite sabe da empresa: as linhas e os lançamentos livres (sem
// conciliação ativa). Id fora daqui é inventado, de outra empresa ou já conciliado.
type AcceptState struct {
	FreeLines   map[string]bool
	FreeEntries map[string]bool
}

var uuidRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// normalizeID devolve o UUID em minúsculas, ou vazio se não for um UUID (o _uuid_ou_none da origem,
// só na forma canônica que o contexto mostra).
func normalizeID(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if !uuidRe.MatchString(s) {
		return ""
	}
	return s
}

// CitedIDs são as linhas e os lançamentos que a hipótese cita (os UUIDs válidos, sem repetição),
// com a linha da proposta: o que o aceite consulta no banco.
func CitedIDs(h Hypothesis, proposalLine string) (lines, entries []string) {
	seenL, seenE := map[string]bool{}, map[string]bool{}
	add := func(list *[]string, seen map[string]bool, raw string) {
		if id := normalizeID(raw); id != "" && !seen[id] {
			seen[id] = true
			*list = append(*list, id)
		}
	}
	lines, entries = []string{}, []string{}
	add(&lines, seenL, proposalLine)
	for _, p := range h.Pairs {
		if p.StatementID != nil {
			add(&lines, seenL, *p.StatementID)
		}
		for _, e := range p.EntryIDs {
			add(&entries, seenE, e)
		}
	}
	return lines, entries
}

// OutsideContext são os ids que a hipótese cita sem que estivessem no contexto enviado ao modelo
// (os UUIDs, sem repetição) e quantos ids nem são UUID. Vão para a decisão da corrida, que segue
// para o Langfuse: o texto livre do modelo fica fora dela. O aceite ignora esses ids se não forem
// linhas ou lançamentos livres da empresa.
func OutsideContext(h Hypothesis, offered map[string]bool) (unknown []string, malformed int) {
	unknown = []string{}
	seen := map[string]bool{}
	check := func(raw string) {
		switch id := normalizeID(raw); {
		case id == "":
			malformed++
		case !offered[id] && !seen[id]:
			seen[id] = true
			unknown = append(unknown, id)
		}
	}
	for _, p := range h.Pairs {
		if p.StatementID != nil {
			check(*p.StatementID)
		}
		for _, e := range p.EntryIDs {
			check(e)
		}
	}
	return unknown, malformed
}

// PlanAccept é o aceitar_hip da origem: o contador aceitou, e cada par vira conciliação.
//   - A linha é a do par ou, sem ela, a da proposta; tem de estar livre (e não usada por um par
//     anterior).
//   - Os lançamentos do par que existem livres na empresa, sem repetição; com algum, conciliação
//     pelo método resolvedor.
//   - Sem lançamento e com o tipo extrato_sem_comprovante, a linha fica confirmada sem
//     comprovante.
//
// O resto do par é ignorado, como na origem ("ids inventados são ignorados").
func PlanAccept(h Hypothesis, proposalLine string, st AcceptState) []Creation {
	usedLines, usedEntries := map[string]bool{}, map[string]bool{}
	var out []Creation
	for _, p := range h.Pairs {
		line := proposalLine
		if p.StatementID != nil {
			if id := normalizeID(*p.StatementID); id != "" {
				line = id
			}
		}
		if line == "" || !st.FreeLines[line] || usedLines[line] {
			continue
		}
		var entries []string
		seen := map[string]bool{}
		for _, raw := range p.EntryIDs {
			id := normalizeID(raw)
			if id == "" || seen[id] || usedEntries[id] || !st.FreeEntries[id] {
				continue
			}
			seen[id] = true
			entries = append(entries, id)
		}
		switch {
		case len(entries) > 0:
			out = append(out, Creation{StatementID: line, EntryIDs: entries, Method: MethodResolver})
			for _, id := range entries {
				usedEntries[id] = true
			}
			usedLines[line] = true
		case p.Kind == KindWithoutReceipt:
			out = append(out, Creation{StatementID: line, Method: MethodWithoutReceipt})
			usedLines[line] = true
		}
	}
	return out
}
