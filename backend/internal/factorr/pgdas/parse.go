// Package pgdas é o parser do extrato PGDAS-D: porte fiel de fator_r/parsing/pgdas.py (Fator R,
// commit 9dd7a74), por regras e regex, sem LLM (ADR-006). Função pura sobre o texto: não decide
// vínculo nem grava nada.
//
// As regex são as da origem, traduzidas por pytext para manter a semântica do re do Python em
// texto Unicode (\s, \d e \b Unicode, IGNORECASE e lookahead). Os números seguem o decimal do
// Python (internal/platform/decimal). Um golden gerado pelo próprio parser da origem
// (testdata/golden_gen.py) trava o porte resultado a resultado.
package pgdas

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"controle/internal/platform/decimal"
	"controle/internal/platform/pytext"
)

// ParserVersion é a versão do parser da origem portada (PARSER_VERSION).
const ParserVersion = "2026.09.3"

// Fields são os campos extraídos, na ordem da origem (CAMPOS).
var Fields = []string{"cnpj", "pa", "rbt12", "rpa", "fs12", "fator_r", "das", "anexo"}

// Peso de cada campo na confiança do documento (anexo é informativo, peso 0).
var weights = map[string]decimal.Decimal{
	"cnpj":    decimal.MustParse("0.25"),
	"pa":      decimal.MustParse("0.25"),
	"rbt12":   decimal.MustParse("0.15"),
	"rpa":     decimal.MustParse("0.10"),
	"fs12":    decimal.MustParse("0.10"),
	"fator_r": decimal.MustParse("0.10"),
	"das":     decimal.MustParse("0.05"),
	"anexo":   decimal.MustParse("0"),
}

var (
	bonusConsistencia      = decimal.MustParse("0.10")
	toleranciaConsistencia = decimal.MustParse("0.005")
	limiteInconsistencia   = decimal.MustParse("0.02")
	confiancaCNPJDVInvalid = decimal.MustParse("0.3")
	toleranciaSerie        = decimal.MustParse("0.01") // centavos: a soma dos meses tem de bater com o total
	one                    = decimal.FromInt(1)
	hundred                = decimal.FromInt(100)
)

const (
	maxCharsLinha = 2000
	maxCharsNome  = 200 // limite do nome no cadastro
	nbsp          = " "
)

// py e pyIgnore compilam uma regex da origem sem flags e com re.IGNORECASE.
func py(pattern string) *pytext.Regexp       { return pytext.MustCompile(pattern, false) }
func pyIgnore(pattern string) *pytext.Regexp { return pytext.MustCompile(pattern, true) }

// Quantificadores limitados: sem backtracking quadrático em sequências longas de dígitos.
const numero = `(\d{1,3}(?:\.\d{3}){1,6},\d{2}|\d{1,15},\d{2}|\d{1,15}\.\d{2}(?!\d))`

var (
	reNumero = py(numero)
	reCNPJ   = pyIgnore(`\bCNPJ(?:\s+Matriz)?\s*[:\-]?\s*(\d{2}\.?\d{3}\.?\d{3}/?\d{4}-?\d{2})\b`)
	rePA     = []*pytext.Regexp{
		pyIgnore(`Per[ií]odo\s+de\s+Apura[cç][aã]o(?:\s*\(PA\))?\s*[:\-]?\s*(\d{2})/(\d{4})`),
		py(`\bPA\s*[:\-]\s*(\d{2})/(\d{4})\b`),
	}
	// Layout declaratório: "Periodo de Apuracao: 01/08/2026 a 31/08/2026" (só vale dentro do mesmo mês).
	rePAIntervalo = pyIgnore(`Per[ií]odo\s+de\s+Apura[cç][aã]o(?:\s*\(PA\))?\s*[:\-]?\s*` +
		`\d{2}/(\d{2})/(\d{4})\s+a\s+\d{2}/(\d{2})/(\d{4})`)
	rotulos = map[string][]*pytext.Regexp{
		"rpa": {
			pyIgnore(`Receita\s+Bruta\s+do\s+PA\s*\(RPA\)`),
			py(`\bRPA\b\s*[:\-]`),
		},
		"rbt12": {
			pyIgnore(`doze\s+meses\s+anteriores\s+ao\s+PA\s*\(RBT12\)`),
			pyIgnore(`12\s+meses\s+anteriores(?:\s+ao\s+PA)?\s*\(RBT12\)`),
			py(`\bRBT12\b\s*[:\-]`),
		},
		"fs12": {
			pyIgnore(`Total\s+de\s+Folhas?\s+de\s+Sal[aá]rios\s+Anteriores`),
			pyIgnore(`Folha\s+de\s+sal[aá]rios\s*\(FS12\)`),
			pyIgnore(`^Total\s+FS12\b`),
			py(`\bFS12\b\s*[:\-]`),
		},
		"das": {
			pyIgnore(`Valor\s+Total\s+do\s+D[eé]bito\s+Declarado`),
			pyIgnore(`Total\s+do\s+DAS`),
			pyIgnore(`Valor\s+do\s+DAS`),
		},
	}
	reFator = pyIgnore(`Fator\s*r\s*[:=]?\s*(\d{1,6}(?:[.,]\d{1,6})?)\s*(%)?`)
	reAnexo = pyIgnore(`\bAnexo\s+(III|V)(?![IVX])`)
	// _RE_QUALQUER_ANEXO = \bAnexo\s+(I{1,3}|IV|V)(?![IVX]): o lookahead depois de I{1,3} exige
	// backtracking (I antes de V em "IV"), então só o prefixo é regex; o grupo é lido em anexoNumeral.
	reQualquerAnexoPrefixo = pyIgnore(`\bAnexo\s+`)
	// Linhas que declaram o enquadramento; parágrafos explicativos do extrato não contam.
	reLinhaEstruturada  = pyIgnore(`^(?:Fator\s*r|Enquadramento|Atividade)\b`)
	reNome              = pyIgnore(`^Nome\s+empresarial\b\s*:?\s*(.*)$`)
	reFatorNaoSeAplica  = pyIgnore(`Fator\s*r\s*[:=]?\s*N[aã]o\s+se\s+aplica`)
	reDataAbertura      = pyIgnore(`Data\s+de\s+abertura(?:\s+no\s+CNPJ)?\s*:?\s*\d{2}/(\d{2})/(\d{4})`)
	reSecao             = py(`^\d+(?:\.\d+)*\s+\S`)
	reSecaoReceitas     = pyIgnore(`^\d+(?:\.\d+)*\s+Receitas\s+Brutas\s+Anteriores`)
	reSecaoFolhas       = pyIgnore(`^\d+(?:\.\d+)*\s+Folhas?\s+de\s+Sal[aá]rios\s+Anteriores`)
	reMesValor          = py(`\b(\d{2})/(\d{4})\s+` + numero)
	reEspacosTabulacoes = regexp.MustCompile(`[ \t]+`)
)

// Identification são os dados de cadastro lidos do extrato: fora da confiança e da nota ouro.
type Identification struct {
	NomeEmpresarial *string
	SujeitaFatorR   *bool
	InicioAtividade *string // "YYYY-MM" da data de abertura no CNPJ
}

// Series é a receita e a folha mês a mês dos 12 meses anteriores ao PA, como declaradas. Os
// *Conferem só são verdadeiros quando os meses são exatamente a janela do PA (PA-12 a PA-1) e a
// soma bate com o total declarado (RBT12 / FS12). Série que não confere nunca é gravada.
type Series struct {
	Receitas         map[string]string
	Folhas           map[string]string
	ReceitasConferem bool
	FolhasConferem   bool
}

// Result é o resultado do parse (ResultadoParse).
type Result struct {
	Fields          map[string]*string
	FieldConfidence map[string]decimal.Decimal
	Confidence      decimal.Decimal
	Reasons         []string
	ParserVersion   string
	Identification  Identification
	Series          Series
}

// Field devolve o campo (nil = não encontrado).
func (r Result) Field(name string) *string { return r.Fields[name] }

// ErrParse é a exceção que o parser da origem levantaria (ex.: PA com ano anterior a 1900 e
// tabelas mensais). Na origem ela derrubava o processo isolado, e o extrato ia para revisão com
// o motivo extracao_excedeu_limites; quem chama faz o mesmo.
var ErrParse = errors.New("pgdas: o parser levantou exceção")

// Empty é o resultado sem texto (resultado_vazio): confiança zero e só o motivo.
func Empty(motivo string) Result {
	fields := make(map[string]*string, len(Fields))
	conf := make(map[string]decimal.Decimal, len(Fields))
	for _, f := range Fields {
		fields[f] = nil
		conf[f] = decimal.Zero
	}
	return Result{
		Fields: fields, FieldConfidence: conf, Confidence: decimal.MustParse("0.0000"),
		Reasons: []string{motivo}, ParserVersion: ParserVersion,
		Series: Series{Receitas: map[string]string{}, Folhas: map[string]string{}},
	}
}

// Normalize separa as linhas como a origem (normalizar): quebras unificadas, NBSP vira espaço,
// cada linha cortada em 2000 caracteres, espaços e tabulações colapsados e bordas aparadas.
func Normalize(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	text = strings.ReplaceAll(text, nbsp, " ")
	raw := strings.Split(text, "\n")
	lines := make([]string, len(raw))
	for i, l := range raw {
		l = truncate(l, maxCharsLinha)
		lines[i] = strings.TrimFunc(reEspacosTabulacoes.ReplaceAllString(l, " "), pytext.IsSpace)
	}
	return lines
}

// truncate corta em n caracteres (code points, como o fatiamento de str no Python).
func truncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	i := 0
	for k := range s {
		if i == n {
			return s[:k]
		}
		i++
	}
	return s
}

// NumeroBR lê um valor no formato brasileiro ("1.234,56") ou com ponto decimal ("1234.56")
// (numero_br). Nil quando não é número.
func NumeroBR(valor string) *decimal.Decimal {
	bruto := strings.TrimFunc(valor, pytext.IsSpace)
	normalizado := bruto
	if strings.Contains(bruto, ",") {
		normalizado = strings.ReplaceAll(strings.ReplaceAll(bruto, ".", ""), ",", ".")
	}
	d, err := decimal.Parse(pytext.ASCIIDigits(normalizado))
	if err != nil {
		return nil
	}
	return &d
}

func ptr[T any](v T) *T { return &v }

func onlyDigits(s string) string {
	return strings.Map(func(r rune) rune {
		if pytext.IsDigit(r) {
			return r
		}
		return -1
	}, s)
}

func cnpjField(lines []string) *string {
	for _, l := range lines {
		if m, ok := reCNPJ.Search(l); ok {
			g, _ := m.Group(1)
			return ptr(onlyDigits(g))
		}
	}
	return nil
}

func month(g string) (int, bool) {
	n, ok := pytext.Atoi(g)
	return n, ok && n >= 1 && n <= 12
}

func paField(lines []string) *string {
	for _, re := range rePA {
		for _, l := range lines {
			if m, ok := re.Search(l); ok {
				mes, _ := m.Group(1)
				ano, _ := m.Group(2)
				if _, valid := month(mes); valid {
					return ptr(ano + "-" + mes)
				}
			}
		}
	}
	for _, l := range lines {
		if m, ok := rePAIntervalo.Search(l); ok {
			mesIni, _ := m.Group(1)
			anoIni, _ := m.Group(2)
			mesFim, _ := m.Group(3)
			anoFim, _ := m.Group(4)
			if _, valid := month(mesIni); mesIni == mesFim && anoIni == anoFim && valid {
				return ptr(anoIni + "-" + mesIni)
			}
		}
	}
	return nil
}

// valorRotulado é a última quantia da linha do rótulo (nas tabelas do extrato, a coluna Total).
func valorRotulado(lines []string, campo string) *string {
	for _, re := range rotulos[campo] {
		for _, l := range lines {
			m, ok := re.Search(l)
			if !ok {
				continue
			}
			numeros := reNumero.FindAll(l[m.End(0):])
			if len(numeros) == 0 {
				continue
			}
			g, _ := numeros[len(numeros)-1].Group(1)
			if v := NumeroBR(g); v != nil {
				return ptr(v.Quantize(-2).String())
			}
		}
	}
	return nil
}

func fatorField(lines []string) *string {
	for _, l := range lines {
		m, ok := reFator.Search(l)
		if !ok {
			continue
		}
		g, _ := m.Group(1)
		v := NumeroBR(g)
		if v == nil {
			continue
		}
		valor := *v
		if _, pct := m.Group(2); pct || valor.Cmp(one) > 0 {
			valor = valor.Quo(hundred).Quantize(-4)
		}
		return ptr(valor.String())
	}
	return nil
}

// Letras do numeral romano com o IGNORECASE do Python ([IVX] também aceita i, v, x, ı e İ).
func isI(r rune) bool   { return r == 'I' || r == 'i' || r == 'ı' || r == 'İ' }
func isV(r rune) bool   { return r == 'V' || r == 'v' }
func isIVX(r rune) bool { return isI(r) || isV(r) || r == 'X' || r == 'x' }

// anexoNumeral lê (I{1,3}|IV|V)(?![IVX]) no início de rest, na ordem de tentativa do re do
// Python: I{1,3} guloso (3, 2, 1), depois IV, depois V, cada um só se não vier I, V ou X depois.
func anexoNumeral(rest string) (string, bool) {
	rs := []rune(rest)
	at := func(k int) rune {
		if k < len(rs) {
			return rs[k]
		}
		return 0
	}
	n := 0
	for n < 3 && isI(at(n)) {
		n++
	}
	for k := n; k >= 1; k-- {
		if !isIVX(at(k)) {
			return string(rs[:k]), true
		}
	}
	if isI(at(0)) && isV(at(1)) && !isIVX(at(2)) {
		return string(rs[:2]), true
	}
	if isV(at(0)) && !isIVX(at(1)) {
		return string(rs[:1]), true
	}
	return "", false
}

// qualquerAnexo é _RE_QUALQUER_ANEXO.search(linha).group(1).
func qualquerAnexo(line string) (string, bool) {
	pos := 0
	for {
		m, ok := reQualquerAnexoPrefixo.SearchFrom(line, pos)
		if !ok {
			return "", false
		}
		if g, ok := anexoNumeral(line[m.End(0):]); ok {
			return g, true
		}
		_, size := utf8.DecodeRuneInString(line[m.Start(0):])
		pos = m.Start(0) + max(size, 1)
	}
}

// anexoField: Anexo III ou V. Linha estruturada manda; o texto livre só vale se não houver nenhuma.
func anexoField(lines []string) *string {
	for _, l := range lines {
		if _, ok := reLinhaEstruturada.Search(l); !ok {
			continue
		}
		if g, ok := qualquerAnexo(l); ok {
			if a := strings.ToUpper(g); a == "III" || a == "V" {
				return ptr(a)
			}
			return nil
		}
	}
	for _, l := range lines {
		if m, ok := reAnexo.Search(l); ok {
			g, _ := m.Group(1)
			return ptr(strings.ToUpper(g))
		}
	}
	return nil
}

func nomeEmpresarial(lines []string) *string {
	for _, l := range lines {
		m, ok := reNome.Search(l)
		if !ok {
			continue
		}
		g, _ := m.Group(1)
		if nome := truncate(strings.TrimFunc(g, pytext.IsSpace), maxCharsNome); nome != "" {
			return ptr(strings.TrimFunc(nome, pytext.IsSpace))
		}
	}
	return nil
}

func inicioAtividade(lines []string) *string {
	for _, l := range lines {
		if m, ok := reDataAbertura.Search(l); ok {
			mes, _ := m.Group(1)
			ano, _ := m.Group(2)
			if _, valid := month(mes); valid {
				return ptr(ano + "-" + mes)
			}
		}
	}
	return nil
}

// serie soma por mês os pares "MM/AAAA valor" das seções com o cabeçalho (ex.: interno e externo).
func serie(lines []string, secao *pytext.Regexp) (map[string]decimal.Decimal, []string) {
	valores := map[string]decimal.Decimal{}
	var ordem []string
	dentro := false
	for _, l := range lines {
		if _, ok := secao.Search(l); ok {
			dentro = true
			continue
		}
		if _, ok := reSecao.Search(l); dentro && ok {
			dentro = false
		}
		if !dentro {
			continue
		}
		for _, m := range reMesValor.FindAll(l) {
			mes, _ := m.Group(1)
			ano, _ := m.Group(2)
			num, _ := m.Group(3)
			v := NumeroBR(num)
			if _, valid := month(mes); v == nil || !valid {
				continue
			}
			chave := ano + "-" + mes
			atual, existe := valores[chave]
			if !existe {
				atual = decimal.Zero
				ordem = append(ordem, chave)
			}
			valores[chave] = atual.Add(*v)
		}
	}
	return valores, ordem
}

// ParsePA lê a competência "YYYY-MM" como parse_competencia da origem (dígitos Unicode
// aceitos, mês de 1 a 12, ano a partir de 1900).
func ParsePA(s string) (year, month int, err error) {
	rs := []rune(s)
	if len(rs) != 7 || rs[4] != '-' {
		return 0, 0, fmt.Errorf("competência deve estar no formato YYYY-MM")
	}
	y, ok1 := pytext.Atoi(string(rs[:4]))
	m, ok2 := pytext.Atoi(string(rs[5:]))
	if !ok1 || !ok2 {
		return 0, 0, fmt.Errorf("competência deve estar no formato YYYY-MM")
	}
	if m < 1 || m > 12 || y < 1900 {
		return 0, 0, fmt.Errorf("competência inválida")
	}
	return y, m, nil
}

// confere: os meses da série são exatamente a janela do PA e a soma bate com o total. A soma
// segue a ordem em que os meses apareceram (a do dict da origem).
func confere(valores map[string]decimal.Decimal, ordem []string, pa, total *string) (bool, error) {
	if len(valores) == 0 || pa == nil || total == nil {
		return false, nil
	}
	y, m, err := ParsePA(*pa)
	if err != nil {
		return false, fmt.Errorf("%w: %w", ErrParse, err)
	}
	idx := y*12 + m - 1
	for i := 1; i <= 12; i++ {
		k := idx - i
		if _, ok := valores[fmt.Sprintf("%04d-%02d", k/12, k%12+1)]; !ok {
			return false, nil
		}
	}
	if len(valores) != 12 {
		return false, nil
	}
	soma := decimal.Zero
	for _, k := range ordem {
		soma = soma.Add(valores[k])
	}
	t := decimal.MustParse(*total)
	diff := soma.Sub(t)
	if diff.Sign() < 0 {
		diff = diff.Neg()
	}
	return diff.Cmp(toleranciaSerie) <= 0, nil
}

func seriesAnteriores(lines []string, fields map[string]*string) (Series, error) {
	receitas, ordemR := serie(lines, reSecaoReceitas)
	folhas, ordemF := serie(lines, reSecaoFolhas)
	s := Series{Receitas: centavos(receitas, ordemR), Folhas: centavos(folhas, ordemF)}
	var err error
	if s.ReceitasConferem, err = confere(receitas, ordemR, fields["pa"], fields["rbt12"]); err != nil {
		return Series{}, err
	}
	if s.FolhasConferem, err = confere(folhas, ordemF, fields["pa"], fields["fs12"]); err != nil {
		return Series{}, err
	}
	return s, nil
}

func centavos(valores map[string]decimal.Decimal, ordem []string) map[string]string {
	out := make(map[string]string, len(valores))
	chaves := append([]string(nil), ordem...)
	sort.Strings(chaves)
	for _, k := range chaves {
		out[k] = valores[k].Quantize(-2).String()
	}
	return out
}

// sujeitaFatorR: fator r numérico → sujeita; "não se aplica" → não sujeita; sem indício → nil.
func sujeitaFatorR(lines []string, fator *string) *bool {
	if fator != nil {
		return ptr(true)
	}
	for _, l := range lines {
		if _, ok := reFatorNaoSeAplica.Search(l); ok {
			return ptr(false)
		}
	}
	return nil
}

// Parse extrai os campos do texto do extrato e calcula a confiança por regra (parse). Devolve
// ErrParse nos casos em que o parser da origem levantaria exceção.
func Parse(text string) (res Result, err error) {
	defer func() {
		// O decimal entra em pânico onde o Python levantaria InvalidOperation (ex.: quantize de
		// uma soma com mais de 28 dígitos); a origem perdia o parse do mesmo jeito.
		if p := recover(); p != nil {
			res, err = Result{}, fmt.Errorf("%w: %v", ErrParse, p)
		}
	}()
	lines := Normalize(text)
	fields := map[string]*string{
		"cnpj":    cnpjField(lines),
		"pa":      paField(lines),
		"rbt12":   valorRotulado(lines, "rbt12"),
		"rpa":     valorRotulado(lines, "rpa"),
		"fs12":    valorRotulado(lines, "fs12"),
		"fator_r": fatorField(lines),
		"das":     valorRotulado(lines, "das"),
		"anexo":   anexoField(lines),
	}
	var motivos []string
	conf := make(map[string]decimal.Decimal, len(Fields))
	for _, f := range Fields {
		conf[f] = decimal.Zero
		if fields[f] != nil {
			conf[f] = one
		}
	}
	if c := fields["cnpj"]; c != nil && !CNPJValido(*c) {
		conf["cnpj"] = confiancaCNPJDVInvalid
		motivos = append(motivos, "cnpj_digito_verificador_invalido")
	}
	for _, f := range []string{"cnpj", "pa"} {
		if fields[f] == nil {
			motivos = append(motivos, f+"_nao_encontrado")
		}
	}
	total := decimal.Zero
	for _, f := range Fields {
		total = total.Add(weights[f].Mul(conf[f]))
	}
	rbt12, fs12, fator := numeroOuNil(fields["rbt12"]), numeroOuNil(fields["fs12"]), numeroOuNil(fields["fator_r"])
	if rbt12 != nil && !rbt12.IsZero() && fs12 != nil && fator != nil {
		diferenca := fator.Sub(fs12.Quo(*rbt12))
		if diferenca.Sign() < 0 {
			diferenca = diferenca.Neg()
		}
		switch {
		case diferenca.Cmp(toleranciaConsistencia) <= 0:
			total = total.Add(bonusConsistencia)
		case diferenca.Cmp(limiteInconsistencia) > 0:
			total = total.Sub(bonusConsistencia)
			motivos = append(motivos, "fator_r_inconsistente_com_fs12_rbt12")
		}
	}
	confianca := decimal.Max(decimal.Zero, total)
	if confianca.Cmp(one) >= 0 {
		confianca = one
	}
	series, err := seriesAnteriores(lines, fields)
	if err != nil {
		return Result{}, err
	}
	return Result{
		Fields: fields, FieldConfidence: conf, Confidence: confianca.Quantize(-4), Reasons: motivos,
		ParserVersion: ParserVersion,
		Identification: Identification{
			NomeEmpresarial: nomeEmpresarial(lines),
			SujeitaFatorR:   sujeitaFatorR(lines, fields["fator_r"]),
			InicioAtividade: inicioAtividade(lines),
		},
		Series: series,
	}, nil
}

func numeroOuNil(v *string) *decimal.Decimal {
	if v == nil {
		return nil
	}
	return NumeroBR(*v)
}

// CNPJValido segue cnpj_valido da origem: 14 dígitos (Unicode, como int() lê), não todos iguais
// e dígitos verificadores certos. Os verificadores lidos são comparados como texto com os
// calculados ("0" a "9"), como na origem.
func CNPJValido(valor string) bool {
	d := []rune(onlyDigits(valor))
	if len(d) != 14 {
		return false
	}
	allSame := true
	for _, r := range d[1:] {
		allSame = allSame && r == d[0]
	}
	if allSame {
		return false
	}
	digito := func(nums []rune, pesos []int) (rune, bool) {
		soma := 0
		for i, r := range nums {
			v, ok := pytext.DigitValue(r)
			if !ok {
				return 0, false
			}
			soma += v * pesos[i]
		}
		resto := soma % 11
		if resto < 2 {
			return '0', true
		}
		return rune('0' + 11 - resto), true
	}
	dv1, ok1 := digito(d[:12], []int{5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2})
	dv2, ok2 := digito(append(append([]rune{}, d[:12]...), dv1), []int{6, 5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2})
	return ok1 && ok2 && d[12] == dv1 && d[13] == dv2
}
