// Package redact limpa mensagens de erro antes de gravá-las ou devolvê-las: sem chaves de API,
// sem tokens Bearer, numa linha só e com tamanho limitado. Usado pelo cliente do gateway de IA
// (TASK-004), pela fila de jobs (TASK-005) e pela trilha local de IA (TASK-009).
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

// Secrets mascara chaves e tokens Bearer e preserva o resto do texto (conteúdo guardado na
// trilha local de IA, TASK-009).
func Secrets(s string) string {
	s = bearerPattern.ReplaceAllString(s, "Bearer [oculto]")
	return secretPattern.ReplaceAllString(s, "[chave oculta]")
}

// Message mascara chaves e tokens, junta as linhas e corta em MaxRunes caracteres.
func Message(msg string) string {
	msg = Secrets(msg)
	msg = strings.Join(strings.Fields(msg), " ")
	if r := []rune(msg); len(r) > MaxRunes {
		msg = string(r[:MaxRunes]) + "…"
	}
	return msg
}
