package entries

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"

	"controle/internal/platform/decimal"
	"controle/internal/platform/pytext"
)

// Limites do lançamento manual.
const (
	maxText      = 500     // contraparte e documento
	MaxCSVBytes  = 2 << 20 // o arquivo CSV
	maxCSVLines  = 5000
	maxCSVErrors = 50 // erros listados na resposta
)

// manualLine é um lançamento manual validado.
type manualLine struct {
	date            time.Time
	amount          decimal.Decimal
	direction       string
	counterparty    *string
	counterpartyDoc *string
	documentRef     *string
}

// fieldErrors junta as mensagens por campo.
type fieldErrors map[string]string

func (f fieldErrors) add(field, msg string) {
	if _, ok := f[field]; !ok {
		f[field] = msg
	}
}

// parseDate lê a data em ISO (AAAA-MM-DD), como o date.fromisoformat da origem.
func parseDate(s string, errs fieldErrors) time.Time {
	d, err := time.Parse(time.DateOnly, strings.TrimSpace(s))
	if err != nil {
		errs.add("date", "Data inválida: use AAAA-MM-DD.")
	}
	return d
}

// parseAmount lê o valor com ponto ou vírgula decimal (o CSV da origem trocava a vírgula pelo
// ponto): positivo, com até 2 casas e dentro de numeric(14,2). Dinheiro nunca passa por float.
func parseAmount(s string, errs fieldErrors) decimal.Decimal {
	d, err := decimal.Parse(strings.ReplaceAll(strings.TrimSpace(s), ",", "."))
	switch {
	case err != nil:
		errs.add("amount", "Valor inválido: use 1234.56 ou 1234,56, sem separador de milhar.")
	case d.Sign() <= 0:
		errs.add("amount", "O valor deve ser maior que zero; o sentido (C ou D) diz se entra ou sai.")
	case !d.Fits(12, 2):
		errs.add("amount", "Valor com mais de 2 casas decimais ou acima de 999.999.999.999,99.")
	default:
		return d.Quantize(-2)
	}
	return decimal.Decimal{}
}

// parseDirection lê o sentido: C (entrada) ou D (saída), sem diferença de caixa.
func parseDirection(s string, errs fieldErrors) string {
	d := strings.ToUpper(strings.TrimSpace(s))
	if d != "C" && d != "D" {
		errs.add("direction", "Sentido inválido: use C (entrada) ou D (saída).")
	}
	return d
}

// optionalText é o texto opcional: vazio vira nulo.
func optionalText(s *string, field string, errs fieldErrors) *string {
	if s == nil {
		return nil
	}
	t := strings.TrimSpace(*s)
	if t == "" {
		return nil
	}
	if utf8.RuneCountInString(t) > maxText {
		errs.add(field, fmt.Sprintf("Máximo de %d caracteres.", maxText))
	}
	return &t
}

// parseDoc lê o CPF ou CNPJ da contraparte: só os dígitos, 11 ou 14.
func parseDoc(s *string, errs fieldErrors) *string {
	if s == nil || strings.TrimSpace(*s) == "" {
		return nil
	}
	var b strings.Builder
	for _, r := range *s {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '.' || r == '-' || r == '/' || r == ' ':
		default:
			errs.add("counterparty_doc", "CPF ou CNPJ inválido: use só os 11 ou 14 dígitos.")
			return nil
		}
	}
	d := b.String()
	if len(d) != 11 && len(d) != 14 {
		errs.add("counterparty_doc", "CPF ou CNPJ inválido: use só os 11 ou 14 dígitos.")
		return nil
	}
	return &d
}

// validate valida o lançamento do formulário.
func (in CreateInput) validate() (manualLine, fieldErrors) {
	errs := fieldErrors{}
	l := manualLine{
		date: parseDate(in.Date, errs), amount: parseAmount(in.Amount, errs), direction: parseDirection(in.Direction, errs),
		counterparty: optionalText(in.Counterparty, "counterparty", errs), counterpartyDoc: parseDoc(in.CounterpartyDoc, errs),
		documentRef: optionalText(in.DocumentRef, "document_ref", errs),
	}
	return l, errs
}

// LineError é o erro de uma linha do CSV.
type LineError struct {
	Line    int    `json:"line"`
	Message string `json:"message"`
}

// DecodeCSV lê o arquivo em UTF-8 (sem o BOM) ou, se não for UTF-8 válido, em Windows-1252 (o
// CSV do Excel); a origem só aceitava UTF-8.
func DecodeCSV(data []byte) string {
	if utf8.Valid(data) {
		return strings.TrimPrefix(string(data), "\xef\xbb\xbf")
	}
	s, err := charmap.Windows1252.NewDecoder().Bytes(data)
	if err != nil {
		return string(data)
	}
	return string(s)
}

// parseCSV lê o CSV de lançamentos da origem: data;valor;sentido;contraparte por linha (colunas
// a mais são ignoradas), com o cabeçalho opcional (a 1ª linha que contém "data") e linhas em
// branco puladas. Devolve as linhas válidas ou os erros de todas as inválidas.
func parseCSV(data []byte) ([]manualLine, []LineError) {
	var out []manualLine
	var errs []LineError
	for i, line := range pytext.SplitLines(DecodeCSV(data)) {
		n := i + 1
		if i == 0 && strings.Contains(strings.ToLower(line), "data") {
			continue
		}
		if strings.TrimSpace(line) == "" {
			continue
		}
		if len(out)+len(errs) >= maxCSVLines {
			errs = append(errs, LineError{Line: n, Message: fmt.Sprintf("O arquivo passa de %d lançamentos: divida em partes.", maxCSVLines)})
			break
		}
		parts := strings.Split(line, ";")
		if len(parts) < 4 {
			errs = append(errs, LineError{Line: n, Message: "Use data;valor;sentido;contraparte, separados por ponto e vírgula."})
			continue
		}
		fe := fieldErrors{}
		counterparty := parts[3]
		l := manualLine{
			date: parseDate(parts[0], fe), amount: parseAmount(parts[1], fe), direction: parseDirection(parts[2], fe),
			counterparty: optionalText(&counterparty, "counterparty", fe),
		}
		if len(fe) > 0 {
			msgs := make([]string, 0, len(fe))
			for _, field := range []string{"date", "amount", "direction", "counterparty"} {
				if m, ok := fe[field]; ok {
					msgs = append(msgs, m)
				}
			}
			errs = append(errs, LineError{Line: n, Message: strings.Join(msgs, " ")})
			continue
		}
		out = append(out, l)
	}
	return out, errs
}
