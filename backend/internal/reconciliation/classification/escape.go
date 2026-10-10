package classification

import "strings"

// specialChars são os caracteres que o re.escape do Python 3.7+ escapa (_special_chars_map).
const specialChars = "()[]{}?*+-|^$\\.&~# \t\n\r\v\f"

// Escape é o re.escape do Python: barra antes de cada caractere especial; o resto, inclusive
// letras acentuadas, fica como está.
func Escape(s string) string {
	var b strings.Builder
	b.Grow(len(s) + len(s)/4)
	for _, r := range s {
		if strings.ContainsRune(specialChars, r) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}
