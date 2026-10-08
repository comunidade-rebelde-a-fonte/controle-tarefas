// Package reconciliation é o módulo Conciliação [REQ-018 a REQ-030, TASK-007 em diante].
//
// Nesta etapa (TASK-007) traz o schema, a chave de dedupe das transações do extrato (contrato
// congelado da origem) e a trava pelo fechamento do módulo Contábil, a única autoridade de
// fechamento [REQ-033, 05 §6].
package reconciliation

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"

	"controle/internal/platform/decimal"
)

var folder = cases.Fold()

// pyIsSpace segue o str.isspace() do Python, usado pelo str.split() da origem: os espaços do
// unicode.IsSpace mais os separadores de arquivo, grupo, registro e unidade (U+001C a U+001F).
func pyIsSpace(r rune) bool { return unicode.IsSpace(r) || (r >= 0x1C && r <= 0x1F) }

// NormalizeDescription normaliza a descrição como normalizar_descricao da origem: decomposição
// NFKD, sem marcas combinantes (classe combinante canônica ≠ 0), case folding e espaços
// colapsados.
func NormalizeDescription(s string) string {
	d := norm.NFKD.String(s)
	var b strings.Builder
	b.Grow(len(d))
	for i := 0; i < len(d); {
		r, size := utf8.DecodeRuneInString(d[i:])
		if norm.NFKD.PropertiesString(d[i:]).CCC() == 0 {
			b.WriteRune(r)
		}
		i += size
	}
	return strings.Join(strings.FieldsFunc(folder.String(b.String()), pyIsSpace), " ")
}

// DedupeKey é a chave de dedupe de uma transação do extrato, contrato congelado da origem
// (chave_dedupe): sha256(conta|data|valor|sentido|descrição normalizada|ordem no dia), em hex. O
// valor entra com 2 casas, como o Money da origem.
func DedupeKey(account string, date time.Time, amount decimal.Decimal, direction, description string, orderInDay int) string {
	payload := fmt.Sprintf("%s|%s|%s|%s|%s|%d", account, date.Format(time.DateOnly), amount.Quantize(-2).Text(),
		direction, NormalizeDescription(description), orderInDay)
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}

// OrdersInDay numera as transações dentro de cada dia, na ordem do extrato (ordens_no_dia).
func OrdersInDay(dates []time.Time) []int {
	count := map[string]int{}
	out := make([]int, len(dates))
	for i, d := range dates {
		k := d.Format(time.DateOnly)
		count[k]++
		out[i] = count[k]
	}
	return out
}
