// Package classification é o domínio da classificação contábil da Conciliação [REQ-026, REQ-027;
// TASK-020, D-031]: porte de app/domain/classificacao.py do Conciliador (commit 2bc6e26), com a
// precedência fixa regra → histórico → agente, a regra derivada da correção humana e o schema da
// sugestão do agente (SugestaoClassificacao).
//
// Travado por golden gerado pelo próprio Python da origem (testdata/classification_gen.py). A
// regex da regra usa o re do Python pelo pytext.Compile; padrão fora do subconjunto traduzido é
// recusado ao cadastrar (ValidatePattern).
package classification

import (
	"errors"
	"fmt"
	"strings"

	"controle/internal/platform/pytext"
	"controle/internal/reconciliation"
)

// Tipos de regra (D-020).
const (
	RuleCNPJ         = "cnpj"
	RuleCounterparty = "contraparte"
	RuleRegex        = "regex_descritivo"
)

// RuleTypes são os tipos de regra, na ordem da origem.
var RuleTypes = []string{RuleCNPJ, RuleCounterparty, RuleRegex}

// Origens da classificação (D-020).
const (
	OriginRule    = "regra"
	OriginHistory = "historico"
	OriginAgent   = "agente"
	OriginManual  = "manual"
)

// Rule é a regra de classificação: o tipo, o padrão e a conta. ID é o da tabela (rastreio).
type Rule struct {
	ID          string
	Type        string
	Pattern     string
	AccountCode string
	re          *pytext.Regexp
}

// HistoryItem é uma classificação confirmada de meses anteriores: a chave (contraparte ou
// descritivo normalizados) e a conta. LineID é a linha que a originou (rastreio).
type HistoryItem struct {
	Key         string
	AccountCode string
	LineID      string
}

// Input é o que a classificação lê do item conciliado: o CNPJ e a contraparte dos comprovantes e
// o descritivo do extrato. Vazio conta como ausente, como no Python.
type Input struct {
	CNPJ         *string
	Counterparty *string
	Description  *string
}

// Result é a classificação: a conta e a origem. Pending é o item que vai ao agente (sem regra nem
// histórico): fica pendente até a confirmação humana. Rule e History apontam o que classificou.
type Result struct {
	AccountCode *string
	Origin      string
	Pending     bool
	Rule        *Rule
	History     *HistoryItem
}

func truthy(s *string) bool { return s != nil && *s != "" }

// Prepare compila as regex das regras, na ordem dada. Regra com padrão que não compila fica de
// fora (o cadastro já recusa; aqui só protege dado antigo) e volta em skipped.
func Prepare(rules []Rule) (ready []Rule, skipped []Rule) {
	for _, r := range rules {
		if r.Type == RuleRegex {
			re, err := pytext.Compile(r.Pattern, true)
			if err != nil {
				skipped = append(skipped, r)
				continue
			}
			r.re = re
		}
		ready = append(ready, r)
	}
	return ready, skipped
}

// ApplyRule é o aplicar_regra da origem: a primeira regra da lista que casa vale. CNPJ igual;
// contraparte igual depois de normalizada; regex do descritivo com IGNORECASE (re.search). As
// regras regex precisam vir do Prepare.
func ApplyRule(in Input, rules []Rule) (Rule, bool) {
	for _, r := range rules {
		switch r.Type {
		case RuleCNPJ:
			if truthy(in.CNPJ) && r.Pattern == *in.CNPJ {
				return r, true
			}
		case RuleCounterparty:
			if truthy(in.Counterparty) && reconciliation.NormalizeDescription(r.Pattern) == reconciliation.NormalizeDescription(*in.Counterparty) {
				return r, true
			}
		case RuleRegex:
			if truthy(in.Description) && r.re != nil {
				if _, ok := r.re.Search(*in.Description); ok {
					return r, true
				}
			}
		}
	}
	return Rule{}, false
}

// HistoryKey é a chave do histórico: a contraparte ou, sem ela, o descritivo, normalizados.
func HistoryKey(in Input) string {
	switch {
	case truthy(in.Counterparty):
		return reconciliation.NormalizeDescription(*in.Counterparty)
	case truthy(in.Description):
		return reconciliation.NormalizeDescription(*in.Description)
	}
	return ""
}

// Classify é o classificar da origem, com a precedência fixa: regra, histórico (o primeiro da
// lista com a mesma chave; o chamador ordena do mais recente para o mais antigo) e, sem os dois,
// o agente, pendente de confirmação.
func Classify(in Input, rules []Rule, history []HistoryItem) Result {
	if r, ok := ApplyRule(in, rules); ok && r.AccountCode != "" {
		code := r.AccountCode
		return Result{AccountCode: &code, Origin: OriginRule, Rule: &r}
	}
	key := HistoryKey(in)
	for i := range history {
		if history[i].Key == key {
			code := history[i].AccountCode
			return Result{AccountCode: &code, Origin: OriginHistory, History: &history[i]}
		}
	}
	return Result{Origin: OriginAgent, Pending: true}
}

// DerivedRule é a regra_derivada_de_correcao da origem: o CNPJ, senão a contraparte, senão o
// descritivo escapado. Sem padrão (tudo vazio), não há regra a propor.
func DerivedRule(in Input, accountCode string) (Rule, bool) {
	switch {
	case truthy(in.CNPJ):
		return Rule{Type: RuleCNPJ, Pattern: *in.CNPJ, AccountCode: accountCode}, true
	case truthy(in.Counterparty):
		return Rule{Type: RuleCounterparty, Pattern: *in.Counterparty, AccountCode: accountCode}, true
	}
	d := ""
	if in.Description != nil {
		d = *in.Description
	}
	p := Escape(d)
	if strings.TrimSpace(p) == "" {
		return Rule{}, false
	}
	return Rule{Type: RuleRegex, Pattern: p, AccountCode: accountCode}, true
}

// ErrInvalidPattern é o padrão de regra recusado no cadastro.
var ErrInvalidPattern = errors.New("padrão de regra inválido")

// NormalizePattern valida e normaliza o padrão do cadastro: o CNPJ (ou CPF) só com os dígitos,
// como o documento da contraparte é gravado; a contraparte sem espaços nas pontas; a regex como
// veio, desde que o porte a traduza do re do Python.
func NormalizePattern(ruleType, pattern string) (string, error) {
	switch ruleType {
	case RuleCNPJ:
		digits := strings.Map(func(r rune) rune {
			if r >= '0' && r <= '9' {
				return r
			}
			return -1
		}, pattern)
		if len(digits) != 11 && len(digits) != 14 {
			return "", fmt.Errorf("%w: o CNPJ (ou CPF) precisa de 14 (ou 11) dígitos", ErrInvalidPattern)
		}
		return digits, nil
	case RuleCounterparty:
		p := strings.TrimSpace(pattern)
		if reconciliation.NormalizeDescription(p) == "" {
			return "", fmt.Errorf("%w: informe a contraparte", ErrInvalidPattern)
		}
		return p, nil
	case RuleRegex:
		if strings.TrimSpace(pattern) == "" {
			return "", fmt.Errorf("%w: informe a expressão", ErrInvalidPattern)
		}
		if _, err := pytext.Compile(pattern, true); err != nil {
			return "", fmt.Errorf("%w: a expressão usa recurso não suportado ou é inválida", ErrInvalidPattern)
		}
		return pattern, nil
	}
	return "", fmt.Errorf("%w: tipo de regra desconhecido", ErrInvalidPattern)
}

// SamePattern diz se duas regras do mesmo tipo têm o mesmo padrão: a regra confirmada substitui a
// antiga (a mais nova vale; D-031). A contraparte compara normalizada.
func SamePattern(ruleType, a, b string) bool {
	if ruleType == RuleCounterparty {
		return reconciliation.NormalizeDescription(a) == reconciliation.NormalizeDescription(b)
	}
	return a == b
}
