package statement

import (
	"controle/internal/platform/decimal"
	"controle/internal/platform/pymodel"
)

// Os campos do extrato que o pydantic da origem valida além dos tipos comuns (pymodel): a linha
// (TransacaoExtraida, extra="forbid") e o leiaute (LeiauteDescoberto, com os padrões).

// transactionKeys são os campos da TransacaoExtraida (extra="forbid": chave a mais recusa a linha).
var transactionKeys = map[string]bool{
	"ordem": true, "data": true, "descricao": true, "documento": true, "valor": true,
	"sentido": true, "saldo_apos": true, "confianca": true,
}

func parseTransaction(v *pymodel.Validator, item any) Transaction {
	raw, ok := item.(map[string]any)
	if !ok {
		v.Fail("transacoes")
		return Transaction{}
	}
	for k := range raw {
		if !transactionKeys[k] {
			v.Fail("transacoes." + k)
		}
	}
	t := Transaction{
		Order:       v.Int32(raw["ordem"], "ordem"),
		Date:        v.Date(raw, "data"),
		Description: v.Str(raw, "descricao"),
		DocumentRef: v.OptStr(raw, "documento"),
		Amount:      v.Money(raw, "valor"),
		Direction:   v.Literal(raw, "sentido", Credit, Debit),
		Confidence:  decimal.MustParse("0.0000"),
	}
	if t.Amount.Sign() <= 0 {
		v.Fail("valor deve ser > 0")
	}
	if b, present := raw["saldo_apos"]; present && b != nil {
		d := v.Money(raw, "saldo_apos")
		t.BalanceAfter = &d
	}
	if c, present := raw["confianca"]; present {
		t.Confidence = v.Decimal(c, "confianca")
	}
	return t
}

// parseLayout é o LeiauteDescoberto: campos opcionais com os padrões da origem; chaves a mais são
// ignoradas.
func parseLayout(v *pymodel.Validator, x any) Layout {
	raw, ok := x.(map[string]any)
	if !ok {
		v.Fail("leiaute")
		return Layout{}
	}
	l := DefaultLayout()
	if p, present := raw["pagina_inicio_tabela"]; present {
		l.TableStartPage = v.Int32(p, "leiaute.pagina_inicio_tabela")
	}
	if p, present := raw["paginas_ignorar"]; present {
		items, ok := p.([]any)
		if !ok {
			v.Fail("leiaute.paginas_ignorar")
		}
		l.IgnoredPages = make([]int64, 0, len(items))
		for _, it := range items {
			i, ok := pymodel.IntValue(it)
			if !ok {
				v.Fail("leiaute.paginas_ignorar")
			}
			l.IgnoredPages = append(l.IgnoredPages, i)
		}
	}
	for key, dst := range map[string]*string{
		"formato_data": &l.DateFormat, "separador_decimal": &l.DecimalSeparator,
		"representacao_sinal": &l.SignRepresentation, "localizacao_saldos": &l.BalanceLocation,
		"estrategia": &l.Strategy,
	} {
		if s, present := raw[key]; present {
			str, ok := s.(string)
			if !ok {
				v.Fail("leiaute." + key)
			}
			*dst = str
		}
	}
	return l
}
