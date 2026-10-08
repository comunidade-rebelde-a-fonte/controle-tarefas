package redact

import (
	"strings"
	"testing"
)

func TestMessage(t *testing.T) {
	cases := map[string]string{
		"Authorization: Bearer sk-live-abc123 inválido": "Authorization: Bearer [oculto] inválido",
		"chave sk-proj-AbC_123-xyz recusada":            "chave [chave oculta] recusada",
		"falha\n\tna   linha\r\n2":                      "falha na linha 2",
		"pk-test-1234 e RK-abcd":                        "[chave oculta] e [chave oculta]",
	}
	for in, want := range cases {
		if got := Message(in); got != want {
			t.Errorf("Message(%q) = %q; esperado %q", in, got, want)
		}
	}
	if got := Message(strings.Repeat("á", 500)); len([]rune(got)) != MaxRunes+1 || !strings.HasSuffix(got, "…") {
		t.Errorf("mensagem longa não foi cortada: %d caracteres", len([]rune(got)))
	}
}
