package pytext

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Classes do re do Python como classes do regexp do Go: os mesmos conjuntos de IsSpace e
// IsDigit (um teste confere rune a rune).
const (
	// SpaceClass é o \s do Python.
	SpaceClass = `[\t\n\v\f\r \x{1c}-\x{1f}\x{85}\x{a0}\x{1680}\x{2000}-\x{200a}\x{2028}\x{2029}\x{202f}\x{205f}\x{3000}]`
	// NotSpaceClass é o \S do Python.
	NotSpaceClass = `[^\t\n\v\f\r \x{1c}-\x{1f}\x{85}\x{a0}\x{1680}\x{2000}-\x{200a}\x{2028}\x{2029}\x{202f}\x{205f}\x{3000}]`
	// DigitClass é o \d do Python (dígito decimal Unicode).
	DigitClass = `\p{Nd}`
	// NotDigitClass é o \D do Python.
	NotDigitClass = `\P{Nd}`
	// WordClass é o \w do Python (o mesmo conjunto de IsWord).
	WordClass = `[\p{L}\p{N}_]`
	// NotWordClass é o \W do Python.
	NotWordClass = `[^\p{L}\p{N}_]`
)

// foldI são as letras que o IGNORECASE do Python iguala ao i: além de i e I, o I com ponto
// (İ, cuja minúscula é i) e o i sem ponto (ı), que o re trata como equivalentes. O (?i) do Go
// só iguala i e I.
const foldI = "iIıİ"

// IsWordBoundary informa se há \b do Python na posição pos (em bytes) de s: de um lado um
// caractere de palavra (IsWord) e do outro não, contando o início e o fim do texto como
// não-palavra.
func IsWordBoundary(s string, pos int) bool {
	before, after := false, false
	if pos > 0 {
		r, _ := utf8.DecodeLastRuneInString(s[:pos])
		before = IsWord(r)
	}
	if pos < len(s) {
		r, _ := utf8.DecodeRuneInString(s[pos:])
		after = IsWord(r)
	}
	return before != after
}

// DigitValue é o valor de um dígito decimal Unicode (categoria Nd), como int() do Python o lê:
// int("٣") == 3. O Unicode garante que esses dígitos vêm em blocos contíguos de 0 a 9.
func DigitValue(r rune) (int, bool) {
	if r >= '0' && r <= '9' {
		return int(r - '0'), true
	}
	if !unicode.Is(unicode.Nd, r) {
		return 0, false
	}
	// O bloco começa no primeiro dígito da sequência contígua (blocos vizinhos têm 10 cada).
	start := r
	for start > 0 && unicode.Is(unicode.Nd, start-1) {
		start--
	}
	return int(r-start) % 10, true
}

// ASCIIDigits troca os dígitos decimais Unicode pelos ASCII de mesmo valor, como int() e
// Decimal() do Python fazem ao ler o texto. O resto fica igual.
func ASCIIDigits(s string) string {
	return strings.Map(func(r rune) rune {
		if v, ok := DigitValue(r); ok {
			return rune("0123456789"[v])
		}
		return r
	}, s)
}

// Atoi lê um inteiro só de dígitos decimais Unicode, como int() do Python (sem sinal nem
// espaços: só o que as regex portadas capturam).
func Atoi(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	n := 0
	for _, r := range s {
		v, ok := DigitValue(r)
		if !ok || n > (1<<31)/10 {
			return 0, false
		}
		n = n*10 + v
	}
	return n, true
}

// Regexp é uma expressão do re do Python (texto Unicode) traduzida para o regexp do Go.
//
// O regexp do Go trata \s, \d, \b e o IGNORECASE só em ASCII e não tem lookahead. A tradução
// troca \s, \S, \d e \D pelas classes Unicode do Python, iguala i, I, ı e İ no IGNORECASE e
// transforma \b e (?!X) em grupos vazios que marcam a posição da asserção, conferida depois de
// cada correspondência candidata. Isso só reproduz o Python quando, a partir de um mesmo início,
// o caminho da correspondência é único (o backtracking não acharia outro caminho que passasse
// nas asserções): vale para as expressões portadas, e o golden gerado pelo Python confirma.
type Regexp struct {
	re       *regexp.Regexp
	groups   []int // grupo do Python → grupo do Go (groups[0] = 0, a correspondência toda)
	asserts  []assertion
	anchored bool // começa com ^: só casa no início do texto
}

type assertion struct {
	group  int            // grupo vazio do Go que marca a posição
	ahead  *regexp.Regexp // (?!X): ^(?:X) aplicado ao resto do texto
	behind *regexp.Regexp // (?<!X): ^(?:X)$ aplicado ao caractere anterior
	// Nenhum dos dois: \b.
}

// Compile traduz e compila um padrão do Python. Fora do subconjunto usado pelos portes (\w,
// \B, lookbehind, grupos nomeados, flags inline, \s dentro de classe…) devolve erro.
func Compile(pattern string, ignoreCase bool) (*Regexp, error) {
	t, err := translate(pattern, ignoreCase)
	if err != nil {
		return nil, err
	}
	re, err := regexp.Compile(flags(ignoreCase) + t.pattern)
	if err != nil {
		return nil, fmt.Errorf("pytext: %q: %w", pattern, err)
	}
	return &Regexp{re: re, groups: t.groups, asserts: t.asserts, anchored: strings.HasPrefix(pattern, "^")}, nil
}

// MustCompile é Compile que entra em pânico em erro (padrões fixos do código).
func MustCompile(pattern string, ignoreCase bool) *Regexp {
	r, err := Compile(pattern, ignoreCase)
	if err != nil {
		panic(err)
	}
	return r
}

func flags(ignoreCase bool) string {
	if ignoreCase {
		return "(?i)"
	}
	return ""
}

type translation struct {
	pattern string
	groups  []int
	asserts []assertion
}

func translate(pattern string, ignoreCase bool) (translation, error) {
	rs := []rune(pattern)
	var b strings.Builder
	t := translation{groups: []int{0}}
	goGroup := 0
	inClass := false
	unsupported := func(what string) (translation, error) {
		return translation{}, fmt.Errorf("pytext: %q: %s não é suportado", pattern, what)
	}
	for i := 0; i < len(rs); i++ {
		c := rs[i]
		switch {
		case c == '\\':
			i++
			if i == len(rs) {
				return unsupported(`"\" no fim`)
			}
			e := rs[i]
			if inClass {
				// \s, \d, \D e \w entram na classe com os conjuntos do Python; \S, \W e \b não.
				switch e {
				case 's':
					b.WriteString(SpaceClass[1 : len(SpaceClass)-1])
				case 'd':
					b.WriteString(DigitClass)
				case 'D':
					b.WriteString(NotDigitClass)
				case 'w':
					b.WriteString(WordClass[1 : len(WordClass)-1])
				case 'S', 'W', 'b', 'B':
					return unsupported(`\` + string(e) + " dentro de classe")
				default:
					b.WriteRune('\\')
					b.WriteRune(e)
				}
				continue
			}
			switch e {
			case 's':
				b.WriteString(SpaceClass)
			case 'S':
				b.WriteString(NotSpaceClass)
			case 'd':
				b.WriteString(DigitClass)
			case 'D':
				b.WriteString(NotDigitClass)
			case 'b':
				goGroup++
				b.WriteString("()")
				t.asserts = append(t.asserts, assertion{group: goGroup})
			case 'w':
				b.WriteString(WordClass)
			case 'W':
				b.WriteString(NotWordClass)
			case 'B', 'A', 'Z', 'z', 'G':
				return unsupported(`\` + string(e))
			case 'n', 't', 'r', 'f', 'v', 'a':
				// Escapes de controle: os mesmos no Python e no Go.
				b.WriteRune('\\')
				b.WriteRune(e)
			default:
				if unicode.IsLetter(e) || unicode.IsDigit(e) {
					return unsupported(`\` + string(e))
				}
				b.WriteRune('\\')
				b.WriteRune(e)
			}
		case inClass:
			if c == ']' {
				inClass = false
				b.WriteRune(c)
				continue
			}
			if c == '-' && ignoreCase && rs[i-1] != '[' && i+1 < len(rs) && rs[i+1] != ']' {
				// O IGNORECASE do Python iguala ı, İ, K e ſ às letras ASCII de um intervalo; o
				// do Go não iguala ı e İ.
				return unsupported("intervalo em classe com IGNORECASE")
			}
			if ignoreCase && (c == 'i' || c == 'I') {
				b.WriteString(foldI)
				continue
			}
			b.WriteRune(c)
		case c == '[':
			if i+1 < len(rs) && rs[i+1] == ']' {
				return unsupported("classe começando com ]")
			}
			inClass = true
			b.WriteRune(c)
			if i+1 < len(rs) && rs[i+1] == '^' {
				// Classe negada: com IGNORECASE, só sem letras (o Python e o Go igualam letras
				// de forma diferente, ver foldI).
				if i+2 < len(rs) && rs[i+2] == ']' {
					return unsupported("classe negada começando com ]")
				}
				if ignoreCase && classHasLetter(rs, i+2) {
					return unsupported("classe negada com letras e IGNORECASE")
				}
				b.WriteRune('^')
				i++
			}
		case c == '(':
			if i+1 < len(rs) && rs[i+1] == '?' {
				switch {
				case i+2 < len(rs) && rs[i+2] == ':':
					b.WriteString("(?:")
					i += 2
				case i+2 < len(rs) && rs[i+2] == '!':
					end := closing(rs, i+3)
					if end < 0 {
						return unsupported("lookahead sem fechamento")
					}
					inner, err := translate(string(rs[i+3:end]), ignoreCase)
					if err != nil {
						return translation{}, err
					}
					if len(inner.groups) > 1 || len(inner.asserts) > 0 {
						return unsupported("grupo ou asserção dentro de lookahead")
					}
					ahead, err := regexp.Compile(flags(ignoreCase) + "^(?:" + inner.pattern + ")")
					if err != nil {
						return translation{}, fmt.Errorf("pytext: %q: %w", pattern, err)
					}
					goGroup++
					b.WriteString("()")
					t.asserts = append(t.asserts, assertion{group: goGroup, ahead: ahead})
					i = end
				case i+3 < len(rs) && rs[i+2] == '<' && rs[i+3] == '!':
					end := closing(rs, i+4)
					if end < 0 {
						return unsupported("lookbehind sem fechamento")
					}
					inner, err := translate(string(rs[i+4:end]), ignoreCase)
					if err != nil {
						return translation{}, err
					}
					if len(inner.groups) > 1 || len(inner.asserts) > 0 {
						return unsupported("grupo ou asserção dentro de lookbehind")
					}
					// Só lookbehind de um caractere: X é conferido contra o caractere anterior.
					behind, err := regexp.Compile(flags(ignoreCase) + "^(?:" + inner.pattern + ")$")
					if err != nil {
						return translation{}, fmt.Errorf("pytext: %q: %w", pattern, err)
					}
					goGroup++
					b.WriteString("()")
					t.asserts = append(t.asserts, assertion{group: goGroup, behind: behind})
					i = end
				default:
					return unsupported("grupo especial (?" + string(rs[i+2:min(i+3, len(rs))]))
				}
				continue
			}
			goGroup++
			t.groups = append(t.groups, goGroup)
			b.WriteRune(c)
		case ignoreCase && (c == 'i' || c == 'I'):
			b.WriteString("[" + foldI + "]")
		case c == '$':
			if i != len(rs)-1 {
				return unsupported("$ fora do fim do padrão")
			}
			// Sem MULTILINE, o $ do Python casa no fim ou antes de um \n final (que fica
			// dentro da correspondência aqui; os grupos não mudam).
			b.WriteString(`(?:\n?\z)`)
		default:
			b.WriteRune(c)
		}
	}
	if inClass {
		return unsupported("classe sem fechamento")
	}
	t.pattern = b.String()
	return t, nil
}

// closing acha o ")" que fecha um grupo aberto antes de start (sem grupos aninhados).
// classHasLetter informa se a classe que começa em start (depois do [ e do ^) tem letra literal
// (os escapes, como \d e \n, não contam).
func classHasLetter(rs []rune, start int) bool {
	for i := start; i < len(rs) && rs[i] != ']'; i++ {
		if rs[i] == '\\' {
			i++
			continue
		}
		if unicode.IsLetter(rs[i]) {
			return true
		}
	}
	return false
}

func closing(rs []rune, start int) int {
	inClass := false
	for i := start; i < len(rs); i++ {
		switch {
		case rs[i] == '\\':
			i++
		case inClass:
			inClass = rs[i] != ']'
		case rs[i] == '[':
			inClass = true
		case rs[i] == '(':
			return -1
		case rs[i] == ')':
			return i
		}
	}
	return -1
}

// Match é uma correspondência, com os grupos numerados como no Python.
type Match struct {
	s   string
	loc []int // pares início/fim por grupo do Python; -1 = o grupo não participou
}

// Group devolve o texto do grupo n e se ele participou (None no Python quando não).
func (m Match) Group(n int) (string, bool) {
	if m.loc[2*n] < 0 {
		return "", false
	}
	return m.s[m.loc[2*n]:m.loc[2*n+1]], true
}

// Start e End são as posições (em bytes) do grupo n.
func (m Match) Start(n int) int { return m.loc[2*n] }
func (m Match) End(n int) int   { return m.loc[2*n+1] }

// Search é o re.search do Python: a primeira correspondência no texto.
func (r *Regexp) Search(s string) (Match, bool) { return r.SearchFrom(s, 0) }

// SearchFrom procura a partir de pos (em bytes), com o texto inteiro como contexto dos \b.
func (r *Regexp) SearchFrom(s string, pos int) (Match, bool) {
	for pos <= len(s) {
		if r.anchored && pos > 0 {
			return Match{}, false
		}
		loc := r.re.FindStringSubmatchIndex(s[pos:])
		if loc == nil {
			return Match{}, false
		}
		for k := range loc {
			if loc[k] >= 0 {
				loc[k] += pos
			}
		}
		if r.holds(s, loc) {
			return r.match(s, loc), true
		}
		// Nesse início o Python também falha: tenta a partir do caractere seguinte.
		_, size := utf8.DecodeRuneInString(s[loc[0]:])
		pos = loc[0] + max(size, 1)
	}
	return Match{}, false
}

// FindAll é o re.finditer do Python: as correspondências sem sobreposição, da esquerda para a
// direita.
func (r *Regexp) FindAll(s string) []Match {
	var out []Match
	pos := 0
	for pos <= len(s) {
		m, ok := r.SearchFrom(s, pos)
		if !ok {
			break
		}
		out = append(out, m)
		if m.End(0) > m.Start(0) {
			pos = m.End(0)
		} else {
			_, size := utf8.DecodeRuneInString(s[m.End(0):])
			pos = m.End(0) + max(size, 1)
		}
	}
	return out
}

// ReplaceAllString é o re.sub do Python com substituição literal: troca cada correspondência de
// FindAll por repl.
func (r *Regexp) ReplaceAllString(s, repl string) string {
	var b strings.Builder
	last := 0
	for _, m := range r.FindAll(s) {
		b.WriteString(s[last:m.Start(0)])
		b.WriteString(repl)
		last = m.End(0)
	}
	b.WriteString(s[last:])
	return b.String()
}

// MatchString diz se há correspondência (re.search do Python como booleano).
func (r *Regexp) MatchString(s string) bool {
	_, ok := r.Search(s)
	return ok
}

func (r *Regexp) holds(s string, loc []int) bool {
	for _, a := range r.asserts {
		p := loc[2*a.group]
		if p < 0 {
			continue // a asserção está num ramo que não participou
		}
		switch {
		case a.ahead != nil:
			if a.ahead.MatchString(s[p:]) {
				return false
			}
		case a.behind != nil:
			if r, size := utf8.DecodeLastRuneInString(s[:p]); size > 0 && a.behind.MatchString(string(r)) {
				return false
			}
		default:
			if !IsWordBoundary(s, p) {
				return false
			}
		}
	}
	return true
}

func (r *Regexp) match(s string, loc []int) Match {
	out := make([]int, 2*len(r.groups))
	for n, g := range r.groups {
		out[2*n], out[2*n+1] = loc[2*g], loc[2*g+1]
	}
	return Match{s: s, loc: out}
}
