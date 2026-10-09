package statement

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"golang.org/x/text/cases"

	"controle/internal/platform/decimal"
	"controle/internal/platform/pytext"
)

// Leitores determinísticos, caminhos paralelos ao agente (CC-08): o CSV do banco
// (app/domain/csv_extrato.py) e o texto nativo do PDF no leiaute do cenário da origem
// (app/domain/pdf_extrato.py). Saem com a confiança 1 em cada linha e o leiaute da estratégia, e
// passam pelo mesmo validador do extrator.

// ErrUnreadable é o arquivo que o leitor determinístico não consegue ler (a origem levantava a
// exceção do ponto em que parou).
var ErrUnreadable = errors.New("o leitor determinístico não leu o extrato")

func unreadable(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrUnreadable, fmt.Sprintf(format, args...))
}

// ParseBRL lê um valor no formato brasileiro (parse_brl): sem "R$", ponto de milhar e vírgula
// decimal, arredondado a centavos; vazio ou "-" é zero.
func ParseBRL(text string) (decimal.Decimal, error) {
	t := pyStrip(strings.ReplaceAll(pyStrip(text), "R$", ""))
	t = strings.ReplaceAll(strings.ReplaceAll(t, ".", ""), ",", ".")
	if t == "" || t == "-" {
		return decimal.MustParse("0.00"), nil
	}
	d, special, ok := decimal.ParsePython(t)
	if !ok || special {
		return decimal.Decimal{}, unreadable("valor %q", text)
	}
	q, ok := d.TryQuantize(-2)
	if !ok {
		return decimal.Decimal{}, unreadable("valor %q", text)
	}
	return q, nil
}

// reInt é o texto que o int() do Python aceita depois de tirar os espaços: sinal opcional e
// dígitos Unicode, com sublinhado só entre dígitos.
var reInt = regexp.MustCompile(`^[+-]?\p{Nd}+(?:_\p{Nd}+)*$`)

func pyInt(s string) (int, bool) {
	s = pyStrip(s)
	if !reInt.MatchString(s) {
		return 0, false
	}
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimLeft(s, "+-")
	n, ok := pytext.Atoi(strings.ReplaceAll(s, "_", ""))
	if neg {
		n = -n
	}
	return n, ok
}

// ParseDateBR lê DD/MM/AAAA (parse_data_br): três partes separadas por "/", cada uma pelo int()
// do Python, e a data tem de existir.
func ParseDateBR(text string) (time.Time, error) {
	parts := strings.Split(pyStrip(text), "/")
	if len(parts) != 3 {
		return time.Time{}, unreadable("data %q", text)
	}
	d, ok1 := pyInt(parts[0])
	m, ok2 := pyInt(parts[1])
	y, ok3 := pyInt(parts[2])
	if !ok1 || !ok2 || !ok3 || y < 1 || y > 9999 || m < 1 || m > 12 || d < 1 {
		return time.Time{}, unreadable("data %q", text)
	}
	t := time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC)
	if t.Day() != d {
		return time.Time{}, unreadable("data %q", text)
	}
	return t, nil
}

// --- CSV ---

// readCSV lê o texto como o csv.reader do Python com delimitador ";" (aspas duplas, aspas
// dobradas e o modo não estrito): uma lista de registros, com [] para a linha em branco.
func readCSV(content string) ([][]string, error) {
	lines := strings.SplitAfter(content, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	const (
		startRecord = iota
		startField
		inField
		inQuoted
		quoteInQuoted
		eatCRNL
	)
	var records [][]string
	var fields []string
	var field strings.Builder
	fieldStarted := false
	state := startRecord
	save := func() {
		fields = append(fields, field.String())
		field.Reset()
		fieldStarted = false
	}
	const eol = rune(-1)
	process := func(c rune) error {
		switch state {
		case startRecord:
			if c == eol {
				return nil
			}
			if c == '\n' || c == '\r' {
				state = eatCRNL
				return nil
			}
			state = startField
			fallthrough
		case startField:
			switch c {
			case '\n', '\r', eol:
				save()
				if c == eol {
					state = startRecord
				} else {
					state = eatCRNL
				}
			case '"':
				state = inQuoted
				fieldStarted = true
			case ';':
				save()
			default:
				field.WriteRune(c)
				fieldStarted = true
				state = inField
			}
		case inField:
			switch c {
			case '\n', '\r', eol:
				save()
				if c == eol {
					state = startRecord
				} else {
					state = eatCRNL
				}
			case ';':
				save()
				state = startField
			default:
				field.WriteRune(c)
			}
		case inQuoted:
			switch c {
			case eol:
			case '"':
				state = quoteInQuoted
			default:
				field.WriteRune(c)
			}
		case quoteInQuoted:
			switch c {
			case '"':
				field.WriteRune('"')
				state = inQuoted
			case ';':
				save()
				state = startField
			case '\n', '\r', eol:
				save()
				if c == eol {
					state = startRecord
				} else {
					state = eatCRNL
				}
			default:
				field.WriteRune(c)
				state = inField
			}
		case eatCRNL:
			switch c {
			case '\n', '\r':
			case eol:
				state = startRecord
			default:
				return unreadable("quebra de linha dentro de campo sem aspas")
			}
		}
		return nil
	}
	i := 0
	for i < len(lines) {
		fields = nil
		for {
			if i == len(lines) {
				// Fim dos dados no meio de um registro: o modo não estrito fecha o campo.
				if field.Len() > 0 || fieldStarted || state == inQuoted {
					save()
				}
				break
			}
			line := lines[i]
			i++
			if strings.ContainsRune(line, 0) {
				return nil, unreadable("linha com NUL")
			}
			for _, c := range line {
				if err := process(c); err != nil {
					return nil, err
				}
			}
			if err := process(eol); err != nil {
				return nil, err
			}
			if state == startRecord {
				break
			}
		}
		if fields == nil {
			fields = []string{}
		}
		records = append(records, fields)
	}
	return records, nil
}

// FromCSV lê o CSV do banco (extrato_de_csv): cabeçalho data;descricao;documento;tipo;valor;saldo,
// tipo C/D, valor com sinal (o sentido vem do tipo) e o saldo após cada linha. O saldo inicial é
// o saldo da primeira linha desfeito dela; o final, o da última.
func FromCSV(content, bank, branch, account string, start, end time.Time) (Statement, error) {
	records, err := readCSV(content)
	if err != nil {
		return Statement{}, err
	}
	if len(records) == 0 {
		return Statement{}, unreadable("CSV sem linhas")
	}
	header := records[0]
	column := func(row []string, name string) (string, bool) {
		idx := -1
		for i, h := range header {
			if h == name {
				idx = i // como o dict(zip()): o último nome repetido vale
			}
		}
		if idx < 0 || idx >= len(row) {
			return "", false
		}
		return row[idx], true
	}
	s := Statement{Bank: bank, Branch: branch, Account: account, PeriodStart: start, PeriodEnd: end}
	var first *decimal.Decimal
	order := 0
	for _, row := range records[1:] {
		if len(row) == 0 {
			continue // o DictReader pula a linha em branco
		}
		order++
		tipo, ok1 := column(row, "tipo")
		valor, ok2 := column(row, "valor")
		saldo, ok3 := column(row, "saldo")
		data, ok4 := column(row, "data")
		desc, ok5 := column(row, "descricao")
		if !ok1 || !ok2 || !ok3 || !ok4 || !ok5 {
			return Statement{}, unreadable("linha %d sem as colunas do extrato", order)
		}
		direction := strings.ToUpper(pyStrip(tipo))
		raw, err := ParseBRL(valor)
		if err != nil {
			return Statement{}, err
		}
		amount := raw.Abs()
		balance, err := ParseBRL(saldo)
		if err != nil {
			return Statement{}, err
		}
		if first == nil {
			delta := amount.Neg()
			if direction == Credit {
				delta = amount
			}
			f := money(balance.Sub(delta))
			first = &f
		}
		date, err := ParseDateBR(data)
		if err != nil {
			return Statement{}, err
		}
		t := Transaction{
			Order: order, Date: date, Description: pyStrip(desc), Amount: amount, Direction: direction,
			BalanceAfter: &balance, Confidence: decimal.MustParse("1.0000"),
		}
		if doc, ok := column(row, "documento"); ok && doc != "" {
			t.DocumentRef = &doc
		}
		// O schema da linha (TransacaoExtraida): valor > 0 e sentido C/D.
		if amount.Sign() <= 0 || (direction != Credit && direction != Debit) {
			return Statement{}, unreadable("linha %d fora do schema", order)
		}
		s.Transactions = append(s.Transactions, t)
	}
	if first == nil {
		return Statement{}, unreadable("CSV sem linhas")
	}
	s.OpeningBalance = *first
	s.ClosingBalance = *s.Transactions[len(s.Transactions)-1].BalanceAfter
	s.Layout = &Layout{
		TableStartPage: 1, IgnoredPages: []int64{}, DateFormat: "DD/MM/YYYY", DecimalSeparator: ",",
		SignRepresentation: "coluna tipo C/D", Strategy: StrategyCSV,
	}
	return s, nil
}

// --- TXT ---

var reTXTAccount = pytext.MustCompile(`Agencia:\s*(\S+)\s+Conta:\s*(\S+)`, true)

// TXTMetadata lê a agência e a conta do cabeçalho do extrato em TXT (metadados_txt).
func TXTMetadata(content string) (branch, account string, err error) {
	m, ok := reTXTAccount.Search(content)
	if !ok {
		return "", "", unreadable("agência/conta não encontrados no TXT")
	}
	branch, _ = m.Group(1)
	account, _ = m.Group(2)
	return branch, account, nil
}

// --- Texto nativo do PDF ---

var (
	reTX        = pytext.MustCompile(`(\d{2}/\d{2}/\d{4})\n(.+)\n(\d+)\n([\d.]+,\d{2})\s+([CD])\n([\d.]+,\d{2})`, true)
	rePrevious  = pytext.MustCompile(`(\d{2}/\d{2}/\d{4})\nSALDO ANTERIOR\n([\d.]+,\d{2})`, true)
	reBeta      = pytext.MustCompile(`banco\s+beta`, true)
	reAFonte    = pytext.MustCompile(`banco\s+a\s+fonte`, true)
	reBranchNL  = pytext.MustCompile(`Agência\s*\n(\S+)`, false)
	reBranchIn  = pytext.MustCompile(`Agência\s+(\S+)`, false)
	reAccountNL = pytext.MustCompile(`Conta corrente\s*\n(\S+)`, false)
	reAccountIn = pytext.MustCompile(`Conta\s+(\S+)`, false)
	reBoth      = pytext.MustCompile(`Agência\s+(\S+)\s+-\s+Conta\s+(\S+)`, false)
	rePeriod    = pytext.MustCompile(`(\d{2}/\d{2}/\d{4})\s+a\s+(\d{2}/\d{2}/\d{4})`, false)
	reFinalNL   = pytext.MustCompile(`Saldo final[^\n]*\n([\d.]+,\d{2})`, true)
	reFinalIn   = pytext.MustCompile(`Saldo final[^\d]*([\d.]+,\d{2})`, true)
	folder      = cases.Fold()
)

// IsStatement diz se o texto parece extrato (eh_extrato): "saldo anterior" e "extrato" ou
// "conta corrente", sem diferenciar maiúsculas.
func IsStatement(text string) bool {
	t := folder.String(text)
	return strings.Contains(t, "saldo anterior") && (strings.Contains(t, "extrato") || strings.Contains(t, "conta corrente"))
}

// KnownBank diz se o texto nomeia um dos bancos do leiaute do leitor (Banco Beta ou Banco A
// Fonte). A origem assumia "a_fonte" para qualquer outro; o porte só confia no leitor com o banco
// nomeado (D-027).
func KnownBank(text string) bool { return reBeta.MatchString(text) || reAFonte.MatchString(text) }

func group(re *pytext.Regexp, text string, n int) (string, bool) {
	m, ok := re.Search(text)
	if !ok {
		return "", false
	}
	return m.Group(n)
}

// FromPDFText lê o extrato do texto nativo do PDF no leiaute do cenário da origem
// (extrato_de_pdf_texto): cabeçalho com agência, conta e período, cada célula da tabela numa
// linha (data, histórico, documento, "valor C|D", saldo) e o saldo final no rodapé.
func FromPDFText(text string) (Statement, error) {
	bank := "a_fonte"
	if reBeta.MatchString(text) {
		bank = "beta"
	} else if reAFonte.MatchString(text) {
		bank = "a_fonte"
	}
	branch, ok := group(reBranchNL, text, 1)
	if !ok || branch == "" {
		branch, _ = group(reBranchIn, text, 1)
	}
	account, ok := group(reAccountNL, text, 1)
	if !ok || account == "" {
		account, _ = group(reAccountIn, text, 1)
	}
	if branch == "" || account == "" {
		if m, ok := reBoth.Search(text); ok {
			branch, _ = m.Group(1)
			account, _ = m.Group(2)
		}
	}
	if branch == "" || account == "" {
		return Statement{}, unreadable("agência/conta não encontrados no PDF")
	}
	per, ok := rePeriod.Search(text)
	if !ok {
		return Statement{}, unreadable("período não encontrado no PDF")
	}
	from, _ := per.Group(1)
	to, _ := per.Group(2)
	start, err := ParseDateBR(from)
	if err != nil {
		return Statement{}, err
	}
	end, err := ParseDateBR(to)
	if err != nil {
		return Statement{}, err
	}
	s := Statement{
		Bank: bank, Branch: strings.TrimRight(pyStrip(branch), "-"), Account: pyStrip(account),
		PeriodStart: start, PeriodEnd: end,
	}
	for i, m := range reTX.FindAll(text) {
		g := func(n int) string { v, _ := m.Group(n); return v }
		date, err := ParseDateBR(g(1))
		if err != nil {
			return Statement{}, err
		}
		amount, err := ParseBRL(g(4))
		if err != nil {
			return Statement{}, err
		}
		balance, err := ParseBRL(g(6))
		if err != nil {
			return Statement{}, err
		}
		doc := g(3)
		t := Transaction{
			Order: i + 1, Date: date, Description: pyStrip(g(2)), DocumentRef: &doc, Amount: amount,
			Direction: strings.ToUpper(g(5)), BalanceAfter: &balance, Confidence: decimal.MustParse("1.0000"),
		}
		if amount.Sign() <= 0 {
			return Statement{}, unreadable("linha %d fora do schema", i+1)
		}
		s.Transactions = append(s.Transactions, t)
	}
	if len(s.Transactions) == 0 {
		return Statement{}, unreadable("nenhuma transação no PDF")
	}
	if v, ok := group(rePrevious, text, 2); ok {
		if s.OpeningBalance, err = ParseBRL(v); err != nil {
			return Statement{}, err
		}
	} else {
		s.OpeningBalance = inferOpening(s.Transactions[0])
	}
	s.ClosingBalance = *s.Transactions[len(s.Transactions)-1].BalanceAfter
	fin, ok := group(reFinalNL, text, 1)
	if !ok {
		fin, ok = group(reFinalIn, text, 1)
	}
	if ok {
		if s.ClosingBalance, err = ParseBRL(fin); err != nil {
			return Statement{}, err
		}
	}
	layout := DefaultLayout()
	layout.Strategy = StrategyPDFText
	s.Layout = &layout
	return s, nil
}

// inferOpening desfaz a primeira linha a partir do saldo após ela (_inferir_inicial).
func inferOpening(t Transaction) decimal.Decimal {
	if t.BalanceAfter == nil {
		return decimal.MustParse("0.00")
	}
	if t.Direction == Credit {
		return t.BalanceAfter.Sub(t.Amount)
	}
	return t.BalanceAfter.Add(t.Amount)
}

// csvColumns são as colunas que o leitor do CSV usa (o documento é opcional).
var csvColumns = []string{"data", "descricao", "tipo", "valor", "saldo"}

// LooksLikeCSV indica se o texto é o CSV de extrato que o leitor determinístico conhece: o
// cabeçalho tem as colunas data, descricao, tipo, valor e saldo, separadas por ";".
func LooksLikeCSV(content string) bool {
	records, err := readCSV(content)
	if err != nil || len(records) == 0 {
		return false
	}
	for _, want := range csvColumns {
		found := false
		for _, h := range records[0] {
			found = found || h == want
		}
		if !found {
			return false
		}
	}
	return true
}
