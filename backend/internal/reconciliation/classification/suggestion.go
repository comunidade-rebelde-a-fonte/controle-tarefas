package classification

import (
	"encoding/json"

	"controle/internal/platform/decimal"
	"controle/internal/platform/pymodel"
)

// ErrInvalidOutput é a saída do agente fora do schema.
var ErrInvalidOutput = pymodel.ErrInvalidOutput

// Suggestion é a SugestaoClassificacao da origem: a conta proposta (do plano da empresa), a
// justificativa, a regra que o contador pode confirmar e a confiança.
type Suggestion struct {
	AccountCode   *string
	AccountName   *string
	Justification string
	// SuggestedRule é o regra_sugerida como veio (dict ou None); ProposedRule o lê.
	SuggestedRule map[string]any
	Confidence    decimal.Decimal
}

type suggestionJSON struct {
	AccountCode   *string         `json:"conta_codigo"`
	AccountName   *string         `json:"conta_nome"`
	Justification string          `json:"justificativa"`
	SuggestedRule map[string]any  `json:"regra_sugerida"`
	Confidence    pymodel.Decimal `json:"confianca"`
}

var one = decimal.FromInt(1)

// ParseSuggestion valida a saída do agente como a SugestaoClassificacao da origem (extra
// ignorado; conta, nome e regra opcionais). Além do que o pydantic recusava, recusa a confiança
// fora de 0 a 1 (guarda do porte, como no resolvedor).
func ParseSuggestion(content string) (Suggestion, error) {
	raw, err := pymodel.LoadsObject(pymodel.ExtractJSON(content))
	if err != nil {
		return Suggestion{}, err
	}
	var v pymodel.Validator
	s := Suggestion{
		AccountCode:   v.OptStr(raw, "conta_codigo"),
		AccountName:   v.OptStr(raw, "conta_nome"),
		Justification: v.Str(raw, "justificativa"),
		Confidence:    v.Decimal(raw["confianca"], "confianca"),
	}
	switch r := raw["regra_sugerida"].(type) {
	case nil:
	case map[string]any:
		s.SuggestedRule = r
	default:
		v.Fail("regra_sugerida")
	}
	if err := v.Err(); err != nil {
		return Suggestion{}, err
	}
	if s.Confidence.Sign() < 0 || s.Confidence.Cmp(one) > 0 {
		return Suggestion{}, pymodel.Invalid("confiança fora de 0 a 1")
	}
	return s, nil
}

// MarshalJSON escreve a sugestão como o model_dump(mode="json") da origem: é o payload gravado.
func (s Suggestion) MarshalJSON() ([]byte, error) {
	return pymodel.Marshal(suggestionJSON{AccountCode: s.AccountCode, AccountName: s.AccountName, Justification: s.Justification,
		SuggestedRule: s.SuggestedRule, Confidence: pymodel.Decimal{Decimal: s.Confidence}})
}

// Payload é a sugestão como a tabela a grava.
func (s Suggestion) Payload() []byte {
	b, err := json.Marshal(s)
	if err != nil {
		panic(err) // só tipos do pacote
	}
	return b
}

// ProposedRule é a regra que a sugestão propõe ao contador (M5.5): a regra_sugerida, se tiver
// tipo e padrão válidos. Ela só vale depois de confirmada.
func (s Suggestion) ProposedRule(accountCode string) (Rule, bool) {
	t, _ := s.SuggestedRule["tipo"].(string)
	p, _ := s.SuggestedRule["padrao"].(string)
	pattern, err := NormalizePattern(t, p)
	if err != nil {
		return Rule{}, false
	}
	return Rule{Type: t, Pattern: pattern, AccountCode: accountCode}, true
}
