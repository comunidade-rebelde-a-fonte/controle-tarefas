// Package redact limpa mensagens de erro antes de gravá-las ou devolvê-las: sem chaves de API,
// sem tokens Bearer, numa linha só e com tamanho limitado. Usado pelo cliente do gateway de IA
// (TASK-004) e pela fila de jobs (TASK-005).
package redact

import (
	"regexp"
	"strings"
)

// MaxRunes é o tamanho máximo da mensagem, em caracteres.
const MaxRunes = 300

var (
	secretPattern = regexp.MustCompile(`(?i)\b(sk|pk|rk)-[A-Za-z0-9_*\-]{3,}`)
	bearerPattern = regexp.MustCompile(`(?i)bearer\s+\S+`)
)

// Message mascara chaves e tokens, junta as linhas e corta em MaxRunes caracteres.
func Message(msg string) string {
	msg = bearerPattern.ReplaceAllString(msg, "Bearer [oculto]")
	msg = secretPattern.ReplaceAllString(msg, "[chave oculta]")
	msg = strings.Join(strings.Fields(msg), " ")
	if r := []rune(msg); len(r) > MaxRunes {
		msg = string(r[:MaxRunes]) + "…"
	}
	return msg
}
