package resolver

import (
	"bytes"
	"encoding/json"
	"sort"
	"time"

	"controle/internal/platform/decimal"
)

// O contexto que o resolvedor recebe (contexto_resolvedor da origem): a linha do extrato com o
// motivo da divergência e as marcas, os comprovantes candidatos (mesmo valor e sentido em D±3,
// mais os que o matching anotou), os comprovantes soltos próximos (mesmo sentido em D±10, até 15)
// e as outras linhas do extrato com o mesmo valor e sentido em D±10. Só da empresa da linha.

// Janelas e limites da origem.
const (
	candidateWindow = 3
	nearbyWindow    = 10
	nearbyMax       = 15
	noDateDistance  = 99
)

// Line é uma linha do extrato da empresa.
type Line struct {
	ID          string
	Date        time.Time
	Amount      decimal.Decimal
	Direction   string
	Description string
	// Reconciled indica conciliação ativa.
	Reconciled bool
	// Flags são as marcas da linha (statement_transactions.flags).
	Flags []byte
}

// EntryInfo é um lançamento da empresa.
type EntryInfo struct {
	ID            string
	Date          *time.Time
	Amount        *decimal.Decimal
	Direction     *string
	Counterparty  *string
	DocumentRef   *string
	OperationType *string
	Flags         []string
	Status        string
	// Free indica que o lançamento não está em conciliação ativa.
	Free bool
}

// ContextInput são as entradas do contexto.
type ContextInput struct {
	Line    Line
	Entries []EntryInfo // todos os lançamentos da empresa, em ordem
	Lines   []Line      // as linhas da empresa, de onde saem as irmãs
}

// Context é o texto que vai ao modelo e os ids que ele cita.
type Context struct {
	Payload    string
	Candidates int
	Nearby     int
	Siblings   int
	// Offered são os ids de linhas e lançamentos citados no contexto.
	Offered map[string]bool
}

type entryOut struct {
	ID            string           `json:"id"`
	Date          *string          `json:"data"`
	Amount        *decimal.Decimal `json:"valor"`
	Direction     *string          `json:"sentido"`
	Counterparty  *string          `json:"contraparte"`
	DocumentRef   *string          `json:"documento"`
	OperationType *string          `json:"tipo_operacao"`
	Alerts        []string         `json:"alertas"`
	Status        string           `json:"status"`
}

type siblingOut struct {
	ID          string          `json:"id"`
	Date        string          `json:"data"`
	Amount      decimal.Decimal `json:"valor"`
	Direction   string          `json:"sentido"`
	Description string          `json:"descricao"`
	Reconciled  bool            `json:"ja_conciliada"`
}

type lineOut struct {
	ID          string          `json:"id"`
	Date        string          `json:"data"`
	Amount      decimal.Decimal `json:"valor"`
	Direction   string          `json:"sentido"`
	Description string          `json:"descricao"`
	Divergence  *string         `json:"motivo_divergencia"`
	Marks       []any           `json:"marcas"`
}

type payloadOut struct {
	Line       lineOut      `json:"transacao_extrato"`
	Candidates []entryOut   `json:"candidatos_mesmo_valor"`
	Nearby     []entryOut   `json:"outros_comprovantes_soltos_proximos"`
	Siblings   []siblingOut `json:"outras_linhas_do_extrato_mesmo_valor"`
	Note       string       `json:"observacao"`
	Rules      string       `json:"regras"`
}

// Textos da origem.
const (
	contextNote = "Cada linha do extrato casa com no máximo um comprovante numa relação 1:1. " +
		"Duas linhas do extrato iguais com dois comprovantes iguais podem ser pagamento em duplicidade."
	contextRules = "Use somente os ids listados acima. Você não concilia: o contador decide. " +
		"Se nenhum comprovante explica a linha, proponha tipo extrato_sem_comprovante."
)

func days(a, b time.Time) int {
	d := int(a.Sub(b).Hours() / 24)
	if d < 0 {
		return -d
	}
	return d
}

// marks são as marcas da linha (as da rodada do matching e as demais) e o motivo da primeira
// divergência (o _divergencia da origem) com os candidatos que ela anotou.
func marks(flags []byte) ([]any, *string, map[string]bool) {
	var list []any
	if len(flags) > 0 && json.Unmarshal(flags, &list) != nil {
		list = nil
	}
	if list == nil {
		list = []any{}
	}
	annotated := map[string]bool{}
	var reason *string
	for _, m := range list {
		obj, ok := m.(map[string]any)
		if !ok || obj["tipo"] != "divergencia" {
			continue
		}
		if r, ok := obj["motivo"].(string); ok {
			reason = &r
		}
		if cands, ok := obj["candidatos"].([]any); ok {
			for _, c := range cands {
				if s, ok := c.(string); ok {
					annotated[s] = true
				}
			}
		}
		break
	}
	return list, reason, annotated
}

func toEntry(e EntryInfo) entryOut {
	out := entryOut{ID: e.ID, Amount: e.Amount, Direction: e.Direction, Counterparty: e.Counterparty, DocumentRef: e.DocumentRef,
		OperationType: e.OperationType, Alerts: e.Flags, Status: e.Status}
	if out.Alerts == nil {
		out.Alerts = []string{}
	}
	if e.Date != nil {
		d := e.Date.Format(time.DateOnly)
		out.Date = &d
	}
	return out
}

// BuildContext monta o contexto do resolvedor para a linha.
func BuildContext(in ContextInput) Context {
	l := in.Line
	list, reason, annotated := marks(l.Flags)
	offered := map[string]bool{l.ID: true}
	// Candidatos: mesmo valor e sentido em D±3, mais os anotados pelo matching; do mais perto ao
	// mais longe (sem data, por último), estável.
	type cand struct {
		e    EntryInfo
		dist int
	}
	var cands []cand
	inCands := map[string]bool{}
	for _, e := range in.Entries {
		if !e.Free {
			continue
		}
		exact := e.Date != nil && e.Amount != nil && e.Direction != nil && e.Amount.Cmp(l.Amount) == 0 &&
			*e.Direction == l.Direction && days(*e.Date, l.Date) <= candidateWindow
		if exact || annotated[e.ID] {
			dist := noDateDistance
			if e.Date != nil {
				dist = days(*e.Date, l.Date)
			}
			cands = append(cands, cand{e, dist})
			inCands[e.ID] = true
		}
	}
	sort.SliceStable(cands, func(i, j int) bool { return cands[i].dist < cands[j].dist })
	p := payloadOut{
		Line: lineOut{ID: l.ID, Date: l.Date.Format(time.DateOnly), Amount: l.Amount, Direction: l.Direction, Description: l.Description,
			Divergence: reason, Marks: list},
		Candidates: []entryOut{}, Nearby: []entryOut{}, Siblings: []siblingOut{}, Note: contextNote, Rules: contextRules,
	}
	for _, c := range cands {
		p.Candidates = append(p.Candidates, toEntry(c.e))
		offered[c.e.ID] = true
	}
	// Comprovantes soltos próximos: mesmo sentido em D±10, fora os candidatos, até 15.
	for _, e := range in.Entries {
		if len(p.Nearby) == nearbyMax {
			break
		}
		if !e.Free || inCands[e.ID] || e.Date == nil || e.Direction == nil || *e.Direction != l.Direction || days(*e.Date, l.Date) > nearbyWindow {
			continue
		}
		p.Nearby = append(p.Nearby, toEntry(e))
		offered[e.ID] = true
	}
	// Outras linhas do extrato com o mesmo valor e sentido em D±10, conciliadas ou não.
	for _, s := range in.Lines {
		if s.ID == l.ID || s.Amount.Cmp(l.Amount) != 0 || s.Direction != l.Direction || days(s.Date, l.Date) > nearbyWindow {
			continue
		}
		p.Siblings = append(p.Siblings, siblingOut{ID: s.ID, Date: s.Date.Format(time.DateOnly), Amount: s.Amount, Direction: s.Direction,
			Description: s.Description, Reconciled: s.Reconciled})
		offered[s.ID] = true
	}
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(p); err != nil {
		panic(err) // só tipos do pacote
	}
	return Context{Payload: string(bytes.TrimRight(b.Bytes(), "\n")), Candidates: len(p.Candidates), Nearby: len(p.Nearby),
		Siblings: len(p.Siblings), Offered: offered}
}
