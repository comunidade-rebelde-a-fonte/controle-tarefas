// Package classify classifica as linhas conciliadas da Conciliação [REQ-026, REQ-027, AC-020;
// TASK-020, D-031]: a precedência regra → histórico → agente sobre o banco, a correção humana
// (que propõe a regra derivada), a sugestão do agente classificador e o aceite dela.
//
// A classificação fica na linha do extrato conciliada (D-031): uma conta por linha do razão. O
// CNPJ e a contraparte vêm dos comprovantes da conciliação; a linha sem comprovante é classificada
// pelo descritivo. A classificação automática só preenche o que não foi confirmado; a do contador
// nunca é trocada por ela.
package classify

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"controle/internal/platform/competence"
	"controle/internal/reconciliation"
	"controle/internal/reconciliation/classification"
	"controle/internal/store"
)

// Summary é a classificação de uma competência: por regra, por histórico e as linhas à espera do
// agente ou do contador.
type Summary struct {
	ByRule    int `json:"by_rule"`
	ByHistory int `json:"by_history"`
	Pending   int `json:"pending"`
}

// sinceForever é o início do histórico (todas as competências anteriores).
var sinceForever = time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC)

// inputOf monta a entrada da classificação: o único CNPJ e a única contraparte (normalizada) dos
// comprovantes conciliados, e o descritivo do extrato. Comprovantes de documentos ou contrapartes
// diferentes (N:1 misto) não dão CNPJ nem contraparte: vale o descritivo.
func inputOf(description string, counterparties, docs []string) classification.Input {
	in := classification.Input{Description: &description}
	if d, ok := single(docs, func(s string) string { return s }); ok {
		in.CNPJ = &d
	}
	if c, ok := single(counterparties, reconciliation.NormalizeDescription); ok {
		in.Counterparty = &c
	}
	return in
}

// single devolve o valor (o primeiro, como veio) se todos os não vazios forem iguais pela chave.
func single(values []string, key func(string) string) (string, bool) {
	first, firstKey := "", ""
	for _, v := range values {
		k := key(v)
		if k == "" {
			continue
		}
		if firstKey == "" {
			first, firstKey = v, k
			continue
		}
		if k != firstKey {
			return "", false
		}
	}
	return first, firstKey != ""
}

// material são as regras ativas (da mais nova para a mais antiga) e o histórico (da linha mais
// recente para a mais antiga) que valem para uma competência, só com contas ativas do plano.
type material struct {
	rules   []classification.Rule
	history []classification.HistoryItem
	active  map[string]bool
}

// load lê as regras e o histórico da empresa: as classificações confirmadas antes da competência
// ("meses anteriores").
func load(ctx context.Context, q *store.Queries, companyID string, comp competence.Competence) (material, error) {
	m := material{active: map[string]bool{}}
	codes, err := q.ListActiveAccountCodes(ctx, companyID)
	if err != nil {
		return m, err
	}
	for _, c := range codes {
		m.active[c] = true
	}
	rows, err := q.ListClassificationRules(ctx, store.ListClassificationRulesParams{CompanyID: companyID, OnlyActive: true})
	if err != nil {
		return m, err
	}
	var rules []classification.Rule
	for _, r := range rows {
		if m.active[r.AccountCode] {
			rules = append(rules, classification.Rule{ID: r.ID, Type: r.RuleType, Pattern: r.Pattern, AccountCode: r.AccountCode})
		}
	}
	m.rules, _ = classification.Prepare(rules)
	hist, err := q.ListClassificationLines(ctx, store.ListClassificationLinesParams{
		CompanyID: companyID, DateFrom: sinceForever, DateTo: comp.FirstDay().AddDate(0, 0, -1), OnlyConfirmed: true,
	})
	if err != nil {
		return m, err
	}
	for _, h := range hist {
		if h.AccountCode == nil || !m.active[*h.AccountCode] {
			continue
		}
		m.history = append(m.history, classification.HistoryItem{
			Key: classification.HistoryKey(inputOf(h.Description, h.Counterparties, h.Docs)), AccountCode: *h.AccountCode, LineID: h.ID,
		})
	}
	return m, nil
}

// apply classifica a linha e grava o resultado (só se ela continua sem classificação confirmada).
// Devolve a origem aplicada, ou "" se nada mudou.
func apply(ctx context.Context, q *store.Queries, companyID string, l store.ListClassificationLinesRow, m material, now time.Time) (string, error) {
	res := classification.Classify(inputOf(l.Description, l.Counterparties, l.Docs), m.rules, m.history)
	p := store.SetAutoLineClassificationParams{ID: l.ID, CompanyID: companyID, Origin: &res.Origin, Pending: res.Pending, ClassifiedAt: &now,
		AccountCode: res.AccountCode}
	switch res.Origin {
	case classification.OriginRule:
		p.RuleID = &res.Rule.ID
	case classification.OriginHistory:
		p.HistoryID = &res.History.LineID
	default:
		if l.ClassificationOrigin != nil && *l.ClassificationOrigin == classification.OriginAgent && l.ClassificationPending {
			return "", nil // já à espera do agente
		}
		p.ClassifiedAt = nil
	}
	n, err := q.SetAutoLineClassification(ctx, p)
	if err != nil || n == 0 {
		return "", err
	}
	return res.Origin, nil
}

// Competence classifica as linhas conciliadas da competência ainda sem classificação confirmada,
// na transação dada (a rodada do matching e a API).
func Competence(ctx context.Context, tx pgx.Tx, companyID string, comp competence.Competence, now time.Time) (Summary, error) {
	q := store.New(tx)
	var sum Summary
	lines, err := q.ListClassificationLines(ctx, store.ListClassificationLinesParams{
		CompanyID: companyID, DateFrom: comp.FirstDay(), DateTo: comp.FirstDay().AddDate(0, 1, -1), OnlyUnconfirmed: true,
	})
	if err != nil || len(lines) == 0 {
		return sum, err
	}
	m, err := load(ctx, q, companyID, comp)
	if err != nil {
		return sum, err
	}
	for _, l := range lines {
		origin, err := apply(ctx, q, companyID, l, m, now)
		if err != nil {
			return sum, err
		}
		switch origin {
		case classification.OriginRule:
			sum.ByRule++
		case classification.OriginHistory:
			sum.ByHistory++
		default:
			sum.Pending++
		}
	}
	return sum, nil
}

// Lines classifica as linhas dadas (o aceite do resolvedor e o pedido ao agente), cada uma com o
// histórico da sua competência. Devolve a origem aplicada por linha ("" quando nada mudou).
func Lines(ctx context.Context, tx pgx.Tx, companyID string, lineIDs []string, now time.Time) (map[string]string, error) {
	q := store.New(tx)
	out := map[string]string{}
	cache := map[string]material{}
	for _, id := range lineIDs {
		lines, err := q.ListClassificationLines(ctx, store.ListClassificationLinesParams{
			CompanyID: companyID, DateFrom: sinceForever, DateTo: farFuture, OnlyUnconfirmed: true, LineID: &id,
		})
		if err != nil {
			return nil, err
		}
		if len(lines) == 0 {
			continue // sem conciliação ativa ou já confirmada
		}
		l := lines[0]
		comp := competence.FromDate(l.TxnDate)
		m, ok := cache[comp.String()]
		if !ok {
			if m, err = load(ctx, q, companyID, comp); err != nil {
				return nil, err
			}
			cache[comp.String()] = m
		}
		origin, err := apply(ctx, q, companyID, l, m, now)
		if err != nil {
			return nil, err
		}
		out[id] = origin
	}
	return out, nil
}

var farFuture = time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC)
