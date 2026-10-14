// Package pytext reproduz classes de caractere do Python 3 usadas pelo código portado das
// origens: str.isspace() e as classes \s, \d e \w das expressões regulares em texto (Unicode).
// O regexp do Go trata \s, \d, \w e \b só em ASCII; quem porta uma regex da origem usa estas
// funções para dar o mesmo resultado (provado por goldens gerados pelo próprio Python).
package pytext

import (
	"unicode"
	"unicode/utf8"
)

// IsSpace segue str.isspace() e o \s do re: os espaços do unicode.IsSpace mais os
// separadores de arquivo, grupo, registro e unidade (U+001C a U+001F).
func IsSpace(r rune) bool { return unicode.IsSpace(r) || (r >= 0x1C && r <= 0x1F) }

// IsDigit segue o \d do re: dígito decimal Unicode (categoria Nd).
func IsDigit(r rune) bool { return unicode.Is(unicode.Nd, r) }

// IsWord segue o \w do re: letras (L), números (N) e o sublinhado.
func IsWord(r rune) bool { return unicode.IsLetter(r) || unicode.IsNumber(r) || r == '_' }

// IsLineBoundary segue as quebras de linha do str.splitlines(): \n, \r, \v, \f, os separadores
// de arquivo, grupo e registro (U+001C a U+001E), NEL (U+0085) e os separadores de linha e de
// parágrafo (U+2028, U+2029).
func IsLineBoundary(r rune) bool {
	switch r {
	case '\n', '\r', '\v', '\f', 0x1C, 0x1D, 0x1E, 0x85, 0x2028, 0x2029:
		return true
	}
	return false
}

// SplitLines é o str.splitlines() do Python: as linhas sem as quebras (\r\n conta como uma) e
// sem linha vazia depois da última quebra.
func SplitLines(s string) []string {
	out := []string{}
	start := 0
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		i += size
		if !IsLineBoundary(r) {
			continue
		}
		out = append(out, s[start:i-size])
		if r == '\r' && i < len(s) && s[i] == '\n' {
			i++
		}
		start = i
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}
