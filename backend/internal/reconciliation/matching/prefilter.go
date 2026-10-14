package matching

import (
	"strings"
	"time"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"controle/internal/platform/decimal"
	"controle/internal/reconciliation"
)

// Pré-filtro do rodar_matching da origem (_ids_divergentes): antes da cascata, tira do automático
// as linhas que a origem sabe que vão dar errado e as manda à fila com o motivo [REQ-030]:
//   - perto (mesmo valor e sentido, D±3) de um comprovante marcado como juros, ISS retido ou
//     parcelado, ou com nota fiscal repetida em mais de um lançamento;
//   - com o mesmo valor e sentido de outra linha de descritivo parecido (24 primeiros caracteres);
//   - com a contraparte de outra linha (28 primeiros caracteres do descritivo, sem "pix recebido"
//     e "pix enviado").

// Motivos do pré-filtro (os da origem).
const (
	ReasonDuplicateValue     = "duplicidade_mesmo_valor"
	ReasonSameCounterparty   = "varios_lancamentos_mesma_contraparte"
	ReasonReceiptAlert       = "alerta_comprovante"
	ReasonInterestOnBoleto   = "juros_no_boleto"
	ReasonISSWithheld        = "iss_retido"
	ReasonInstallment        = "parcelado"
	ReasonSameInvoiceManyPDF = "mesmo_nf_varios_pdfs"
)

// Marcas do comprovante que tiram do automático (receipt.Flag*).
const (
	flagInterest    = "juros"
	flagISSWithheld = "iss_retido"
	flagInstallment = "parcelado"
)

// prefilterWindow é a janela fixa do pré-filtro da origem.
const prefilterWindow = 3

// EntryRecord é o lançamento como o pré-filtro o vê: todos os lançamentos da empresa, conciliados
// ou não, completos ou não (os LancamentoCliente da origem).
type EntryRecord struct {
	ID          string
	Date        *time.Time
	Amount      *decimal.Decimal
	Direction   string
	DocumentRef *string
	Flags       []string
}

// Skips é o resultado do pré-filtro: as linhas e os lançamentos fora do automático e o motivo de
// cada linha.
type Skips struct {
	Statements map[string]bool
	Entries    map[string]bool
	Reasons    map[string]string
}

var upper = cases.Upper(language.Und)

// invoice é a nota fiscal do documento do lançamento (documento em maiúsculas começando por "NF").
func invoice(doc *string) (string, bool) {
	if doc == nil || *doc == "" {
		return "", false
	}
	u := upper.String(*doc)
	return u, strings.HasPrefix(u, "NF")
}

func hasFlag(flags []string, f string) bool {
	for _, x := range flags {
		if x == f {
			return true
		}
	}
	return false
}

// firstRunes é o fatiamento [:n] do Python (em caracteres).
func firstRunes(s string, n int) string {
	i := 0
	for k := range s {
		if i == n {
			return s[:k]
		}
		i++
	}
	return s
}

// Prefilter é o _ids_divergentes da origem, na mesma ordem de regras e de motivos.
func Prefilter(stmts []Statement, entries []EntryRecord) Skips {
	sk := Skips{Statements: map[string]bool{}, Entries: map[string]bool{}, Reasons: map[string]string{}}
	var invoiceKeys []string
	byInvoice := map[string][]EntryRecord{}
	for _, e := range entries {
		if hasFlag(e.Flags, flagInterest) || hasFlag(e.Flags, flagISSWithheld) || hasFlag(e.Flags, flagInstallment) {
			sk.Entries[e.ID] = true
		}
		if nf, ok := invoice(e.DocumentRef); ok {
			if _, seen := byInvoice[nf]; !seen {
				invoiceKeys = append(invoiceKeys, nf)
			}
			byInvoice[nf] = append(byInvoice[nf], e)
		}
	}
	for _, k := range invoiceKeys {
		if group := byInvoice[k]; len(group) >= 2 {
			for _, e := range group {
				sk.Entries[e.ID] = true
			}
		}
	}
	// Mesmo valor e sentido com descritivo parecido: duplicidade.
	var valueKeys []string
	byValue := map[string][]Statement{}
	for _, s := range stmts {
		k := s.Amount.Key() + "|" + s.Direction
		if _, seen := byValue[k]; !seen {
			valueKeys = append(valueKeys, k)
		}
		byValue[k] = append(byValue[k], s)
	}
	for _, k := range valueKeys {
		group := byValue[k]
		if len(group) < 2 {
			continue
		}
		first := firstRunes(reconciliation.NormalizeDescription(group[0].Description), 24)
		similar := false
		for _, s := range group[1:] {
			if firstRunes(reconciliation.NormalizeDescription(s.Description), 24) == first {
				similar = true
				break
			}
		}
		if similar {
			for _, s := range group {
				sk.Statements[s.ID] = true
				sk.Reasons[s.ID] = ReasonDuplicateValue
			}
		}
	}
	// Mesma contraparte em mais de uma linha.
	var nameKeys []string
	byName := map[string][]Statement{}
	for _, s := range stmts {
		key := reconciliation.NormalizeDescription(s.Description)
		key = strings.ReplaceAll(strings.ReplaceAll(key, "pix recebido ", ""), "pix enviado ", "")
		key = firstRunes(key, 28)
		if _, seen := byName[key]; !seen {
			nameKeys = append(nameKeys, key)
		}
		byName[key] = append(byName[key], s)
	}
	for _, k := range nameKeys {
		if group := byName[k]; len(group) >= 2 {
			for _, s := range group {
				sk.Statements[s.ID] = true
				if _, ok := sk.Reasons[s.ID]; !ok {
					sk.Reasons[s.ID] = ReasonSameCounterparty
				}
			}
		}
	}
	// Linhas perto de um lançamento tirado do automático, com o motivo da marca dele.
	for _, e := range entries {
		if !sk.Entries[e.ID] || e.Date == nil || e.Amount == nil {
			continue
		}
		for _, s := range stmts {
			if e.Amount.Cmp(s.Amount) != 0 || e.Direction != s.Direction || days(*e.Date, s.Date) > prefilterWindow {
				continue
			}
			sk.Statements[s.ID] = true
			reason := ReasonReceiptAlert
			switch {
			case hasFlag(e.Flags, flagInterest):
				reason = ReasonInterestOnBoleto
			case hasFlag(e.Flags, flagISSWithheld):
				reason = ReasonISSWithheld
			case hasFlag(e.Flags, flagInstallment):
				reason = ReasonInstallment
			default:
				if _, ok := invoice(e.DocumentRef); ok {
					reason = ReasonSameInvoiceManyPDF
				}
			}
			sk.Reasons[s.ID] = reason
		}
	}
	return sk
}
