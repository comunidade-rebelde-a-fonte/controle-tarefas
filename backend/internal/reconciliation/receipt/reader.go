package receipt

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/cases"

	"controle/internal/platform/decimal"
	"controle/internal/platform/pymodel"
	"controle/internal/platform/pytext"
	"controle/internal/reconciliation"
	"controle/internal/reconciliation/statement"
)

// O leitor determinístico do texto nativo do comprovante (parse_comprovante_texto da origem): os
// rótulos do leiaute dos bancos do cenário, com confiança 1 em cada campo lido. É o caminho
// paralelo ao agente (CC-08); o pipeline decide quando o resultado vale (D-028).

// ErrUnreadable é o comprovante cujo texto o leitor reconhece mas não consegue ler (data ou valor
// ilegíveis): a origem deixava a exceção subir.
var ErrUnreadable = errors.New("o leitor determinístico não leu o comprovante")

var folder = cases.Fold()

// fold é o str.casefold() do Python.
func fold(s string) string { return folder.String(s) }

// Client é a empresa do documento. O leitor da origem procurava a palavra "horizonte" (a empresa
// do cenário de aula) no pagador e no recebedor para decidir o sentido; o porte procura o nome da
// empresa: a razão social e o nome fantasia sem a natureza jurídica (Ltda, S.A., ME, EPP, EIRELI),
// em palavras inteiras, sem acento nem caixa.
type Client struct {
	names [][]string
}

// legalSuffixes são as naturezas jurídicas que saem do fim do nome, em palavras normalizadas.
var legalSuffixes = [][]string{
	{"ltda"}, {"limitada"}, {"me"}, {"epp"}, {"eireli"}, {"mei"}, {"sa"}, {"s", "a"}, {"ss"}, {"s", "s"},
}

// NewClient monta a empresa pelos nomes (razão social e nome fantasia; vazios são ignorados).
func NewClient(names ...string) Client {
	var c Client
	for _, n := range names {
		w := words(n)
		for stripped := true; stripped; {
			stripped = false
			for _, suf := range legalSuffixes {
				if len(w) > len(suf) && equal(w[len(w)-len(suf):], suf) {
					w, stripped = w[:len(w)-len(suf)], true
				}
			}
		}
		if len(w) > 0 {
			c.names = append(c.names, w)
		}
	}
	return c
}

// Known informa se há algum nome para procurar.
func (c Client) Known() bool { return len(c.names) > 0 }

// In informa se o texto cita a empresa (algum nome, em palavras inteiras e seguidas).
func (c Client) In(s string) bool {
	w := words(s)
	for _, n := range c.names {
		for i := 0; i+len(n) <= len(w); i++ {
			if equal(w[i:i+len(n)], n) {
				return true
			}
		}
	}
	return false
}

// words normaliza como a descrição da dedupe (sem acento, casefold) e separa em palavras de letras
// e dígitos.
func words(s string) []string {
	n := reconciliation.NormalizeDescription(s)
	return strings.FieldsFunc(n, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// pyEscape é o re.escape do Python 3.7+: escapa só os caracteres especiais (o espaço também).
func pyEscape(s string) string {
	var b strings.Builder
	for _, r := range s {
		if strings.ContainsRune("()[]{}?*+-|^$\\.&~# \t\n\r\v\f", r) {
			b.WriteRune('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// Rótulos do leiaute, na ordem de preferência da origem.
var (
	dateLabels = []string{"Data do pagamento", "Data do processamento", "Data do lançamento", "Data do débito",
		"Data e hora", "Data"}
	amountLabels = []string{"Valor total pago", "Valor total do lote", "Valor pago", "Valor líquido creditado",
		"Valor debitado", "Valor total", "Valor principal", "Valor"}
	payerLabels    = []string{"Pagador", "Remetente"}
	receiverLabels = []string{"Recebedor", "Favorecido", "Beneficiário", "Órgão arrecadador", "Empresa conveniada"}
	kindLabels     = []string{"Tipo", "Tipo de documento"}
	refLabels      = []string{"ID da transação (E2E)", "Ordem bancária", "Nosso número"}
)

// labelRe é a expressão do _rotulo da origem para cada rótulo: o rótulo, espaços e a linha
// seguinte (IGNORECASE).
var labelRe = func() map[string]*pytext.Regexp {
	m := map[string]*pytext.Regexp{}
	for _, list := range [][]string{dateLabels, amountLabels, payerLabels, receiverLabels, kindLabels, refLabels} {
		for _, l := range list {
			m[l] = pytext.MustCompile(pyEscape(l)+`\s*\n([^\n]+)`, true)
		}
	}
	return m
}()

// label é o _rotulo da origem: a linha depois do rótulo, sem os espaços das pontas; nil se o
// rótulo não aparece.
func label(text, l string) *string {
	m, ok := labelRe[l].Search(text)
	if !ok {
		return nil
	}
	g, _ := m.Group(1)
	s := pymodel.Strip(g)
	return &s
}

// or é o "or" do Python entre str | None: o primeiro texto não vazio ou, sem nenhum, o último
// valor (que pode ser "" ou None).
func or(vals ...*string) *string {
	for _, v := range vals {
		if v != nil && *v != "" {
			return v
		}
	}
	return vals[len(vals)-1]
}

// firstLabel é o "or" dos rótulos da lista.
func firstLabel(text string, labels []string) *string {
	vals := make([]*string, len(labels))
	for i, l := range labels {
		vals[i] = label(text, l)
	}
	return or(vals...)
}

func str(s string) *string { return &s }

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// issuer é o _emissor da origem: a primeira linha do documento, sem o setor ("Prefeitura X -
// Tesouraria" vira "Prefeitura X").
func issuer(text string) *string {
	for _, line := range pytext.SplitLines(text) {
		line = pymodel.Strip(line)
		if line != "" {
			s := pymodel.Strip(strings.SplitN(line, " - ", 2)[0])
			if s == "" {
				return nil
			}
			return &s
		}
	}
	return nil
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

var (
	reAnyDate   = pytext.MustCompile(`(\d{2}/\d{2}/\d{4})`, false)
	reAnyAmount = pytext.MustCompile(`R\$\s*([\d.]+,\d{2})`, false)

	reInvoice     = pytext.MustCompile(`\bNF[S\-]*e?\s*\d+`, true)
	reInstallment = pytext.MustCompile(`parcela\s+\d|parte\s+\d+\s+de\s+\d`, true)
	reInterest    = pytext.MustCompile(`\(\+\)\s*juros[^\n]*\nR\$ (?!0,00)`, true)
	reInterestPct = pytext.MustCompile(`\(\+\)\s*juros 1%`, true)
	reISSWithheld = pytext.MustCompile(`iss\s+retido`, true)
)

// debitMarkers são os textos de débito em lote ou da conta do cliente (debito_lote da origem).
var debitMarkers = []string{
	"conta debitada", "débito automático", "debito automatico", "pagamento de salários", "pagamento de salarios",
	"folha de pagamento", "adiantamento salarial", "aviso de lançamento", "aviso de lancamento",
	"comprovante de arrecadação", "comprovante de arrecadacao",
}

// textFlags são as marcas da origem lidas do texto, na ordem dela (parcelado, juros, iss_retido e a
// nota fiscal citada), e o número da nota, que vira o documento.
func textFlags(text string) ([]string, *string) {
	flags := []string{}
	if reInstallment.MatchString(text) {
		flags = append(flags, FlagInstallment)
	}
	if reInterest.MatchString(text) || reInterestPct.MatchString(text) {
		flags = append(flags, FlagInterest)
	}
	if reISSWithheld.MatchString(text) {
		flags = append(flags, FlagISSWithheld)
	}
	if m, ok := reInvoice.Search(text); ok {
		g, _ := m.Group(0)
		nf := strings.ToUpper(g)
		flags = append(flags, FlagInvoice+nf)
		return flags, &nf
	}
	return flags, nil
}

// TextFlags são as marcas de borda que o código lê do texto do comprovante, as mesmas do leitor
// determinístico. No comprovante lido pelo agente, somam-se às marcas dele.
func TextFlags(text string) []string {
	flags, _ := textFlags(text)
	return flags
}

var four = decimal.MustParse("1.0000")

// FromText é o parse_comprovante_texto da origem. Sem data ou sem valor, não lê (ok = false);
// data ou valor ilegíveis são erro. O sentido sai da posição da empresa: recebedora é entrada;
// pagadora, débito em lote ou conta debitada, saída; sem a empresa no comprovante, o leitor supõe
// entrada com confiança 0,80.
func FromText(text string, client Client) (Receipt, bool, error) {
	dateRaw := deref(firstLabel(text, dateLabels))
	amountRaw := deref(firstLabel(text, amountLabels))
	if dateRaw == "" {
		if m, ok := reAnyDate.Search(text); ok {
			dateRaw, _ = m.Group(1)
		}
	}
	if amountRaw == "" {
		if m, ok := reAnyAmount.Search(text); ok {
			amountRaw, _ = m.Group(1)
		}
	}
	if dateRaw == "" || amountRaw == "" {
		return Receipt{}, false, nil
	}
	date, err := statement.ParseDateBR(firstRunes(dateRaw, 10))
	if err != nil {
		return Receipt{}, false, fmt.Errorf("%w: data %q", ErrUnreadable, dateRaw)
	}
	amount, err := statement.ParseBRL(strings.ReplaceAll(amountRaw, "R$", ""))
	if err != nil {
		return Receipt{}, false, fmt.Errorf("%w: valor %q", ErrUnreadable, amountRaw)
	}
	payer := deref(or(label(text, payerLabels[0]), label(text, payerLabels[1]), str("")))
	receiver := deref(or(append(labelValues(text, receiverLabels), str(""))...))
	t := fold(text)
	debitBatch := false
	for _, m := range debitMarkers {
		debitBatch = debitBatch || strings.Contains(t, m)
	}
	clientSide := client.In(payer) || client.In(receiver) || strings.Contains(t, "conta debitada")
	clientReceives := client.In(receiver) && !client.In(payer)
	var direction string
	var counterparty *string
	switch {
	case clientReceives:
		// A empresa é a favorecida: é entrada, mesmo que o comprovante mostre a "conta debitada"
		// do remetente. Sem rótulo de pagador, quem paga é o emissor.
		direction = Credit
		counterparty = or(str(payer), issuer(text), str(receiver))
	case debitBatch || client.In(payer):
		direction = Debit
		counterparty = or(str(receiver), label(text, kindLabels[0]), label(text, kindLabels[1]), str("lote/guia"))
	default:
		direction = Credit
		counterparty = or(str(payer), str(receiver))
	}
	kind := "outro"
	switch {
	case strings.Contains(t, "pix"):
		kind = "pix"
	case strings.Contains(t, "boleto"):
		kind = "boleto"
	case strings.Contains(t, "ted") || strings.Contains(t, "ordem banc"):
		kind = "ted"
	}
	flags, invoice := textFlags(text)
	documentRef := or(invoice, label(text, refLabels[0]), label(text, refLabels[1]), label(text, refLabels[2]))
	directionConf := four
	if !clientSide {
		directionConf = decimal.MustParse("0.8000")
	}
	return Receipt{
		Date: &date, Amount: &amount, Direction: &direction, Counterparty: counterparty, DocumentRef: documentRef,
		OperationType: &kind, Alerts: flags,
		Confidence: map[string]decimal.Decimal{FieldDate: four, FieldAmount: four, FieldDirection: directionConf, FieldCounterparty: four},
		Origin:     map[string]FieldOrigin{},
	}, true, nil
}

func labelValues(text string, labels []string) []*string {
	out := make([]*string, len(labels))
	for i, l := range labels {
		out[i] = label(text, l)
	}
	return out
}

// ClientNamed informa se o leitor achou a empresa no comprovante (no pagador, no recebedor ou pela
// conta debitada): sem isso, o sentido do leitor é suposição.
func ClientNamed(text string, client Client) bool {
	payer := deref(or(label(text, payerLabels[0]), label(text, payerLabels[1]), str("")))
	receiver := deref(or(append(labelValues(text, receiverLabels), str(""))...))
	return client.In(payer) || client.In(receiver) || strings.Contains(fold(text), "conta debitada")
}

// IsReceiptText informa se o texto se apresenta como comprovante: o porte só passa pelo leitor
// determinístico o PDF que diz "comprovante" (a origem tentava ler qualquer PDF com data e valor).
func IsReceiptText(text string) bool { return strings.Contains(fold(text), "comprovante") }

// reCNPJ é um CNPJ impresso, com ou sem pontuação.
var reCNPJ = regexp.MustCompile(`[0-9]{2}\.?[0-9]{3}\.?[0-9]{3}/?[0-9]{4}-?[0-9]{2}`)

// CNPJs são os CNPJs impressos no texto (14 dígitos, sem dígito colado antes ou depois), sem
// repetição, na ordem do texto: o pipeline identifica por eles a empresa cadastrada.
func CNPJs(text string) []string {
	ascii := pytext.ASCIIDigits(text)
	out := []string{}
	seen := map[string]bool{}
	for _, loc := range reCNPJ.FindAllStringIndex(ascii, -1) {
		if loc[0] > 0 && isASCIIDigit(ascii[loc[0]-1]) || loc[1] < len(ascii) && isASCIIDigit(ascii[loc[1]]) {
			continue
		}
		d := digits(ascii[loc[0]:loc[1]])
		if !seen[d] {
			seen[d] = true
			out = append(out, d)
		}
	}
	return out
}

func isASCIIDigit(b byte) bool { return b >= '0' && b <= '9' }
