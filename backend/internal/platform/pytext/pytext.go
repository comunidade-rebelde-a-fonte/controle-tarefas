// Package pytext reproduz classes de caractere do Python 3 usadas pelo código portado das
// origens: str.isspace() e as classes \s, \d e \w das expressões regulares em texto (Unicode).
// O regexp do Go trata \s, \d, \w e \b só em ASCII; quem porta uma regex da origem usa estas
// funções para dar o mesmo resultado (provado por goldens gerados pelo próprio Python).
package pytext

import "unicode"

// IsSpace segue str.isspace() e o \s do re: os espaços do unicode.IsSpace mais os
// separadores de arquivo, grupo, registro e unidade (U+001C a U+001F).
func IsSpace(r rune) bool { return unicode.IsSpace(r) || (r >= 0x1C && r <= 0x1F) }

// IsDigit segue o \d do re: dígito decimal Unicode (categoria Nd).
func IsDigit(r rune) bool { return unicode.Is(unicode.Nd, r) }

// IsWord segue o \w do re: letras (L), números (N) e o sublinhado.
func IsWord(r rune) bool { return unicode.IsLetter(r) || unicode.IsNumber(r) || r == '_' }
