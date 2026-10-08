package langfuse

import (
	"bytes"
	"encoding/json"
	"strings"
	"unicode/utf8"

	"controle/internal/platform/pytext"
)

// Marcas do mascaramento, as mesmas da origem (tracing/client.py do Fator R).
const (
	Removed   = "[removido]"
	Truncated = "…[truncado]"
)

// SensitiveFields nunca saem para o Langfuse: texto bruto do PDF e bytes do arquivo [09 §PII].
var SensitiveFields = map[string]bool{"texto_extraido": true, "arquivo_bytes": true, "conteudo_arquivo": true}

// part é um trecho das regex de CNPJ e CPF da origem: n dígitos (\d{n}) ou um separador
// opcional (sep ou espaço).
type part struct {
	digits int
	sep    rune
}

// Regex da origem: \b\d{2}[.\s]?\d{3}[.\s]?\d{3}[/\s]?\d{4}[-\s]?\d{2}\b e
// \b\d{3}[.\s]?\d{3}[.\s]?\d{3}[-\s]?\d{2}\b, com \d, \s e \b Unicode como no Python.
var (
	cnpjShape = []part{{digits: 2}, {sep: '.'}, {digits: 3}, {sep: '.'}, {digits: 3}, {sep: '/'}, {digits: 4}, {sep: '-'}, {digits: 2}}
	cpfShape  = []part{{digits: 3}, {sep: '.'}, {digits: 3}, {sep: '.'}, {digits: 3}, {sep: '-'}, {digits: 2}}
)

// MaskDocuments é mascarar_cnpj da origem: CNPJ e CPF, com ou sem pontuação, viram
// ***<4 últimos dígitos>. Primeiro o CNPJ, depois o CPF, como lá.
func MaskDocuments(s string) string {
	return maskShape(maskShape(s, cnpjShape), cpfShape)
}

// maskShape substitui, da esquerda para a direita, cada trecho que casa com shape entre
// fronteiras de palavra (\b), como re.sub. Os separadores nunca são dígitos, então cada
// posição tem no máximo um casamento possível: não há retrocesso a imitar.
func maskShape(s string, shape []part) string {
	rs := []rune(s)
	var b strings.Builder
	copied := 0
	for i := 0; i < len(rs); {
		if i == 0 || !pytext.IsWord(rs[i-1]) {
			if end, ok := matchShape(rs, i, shape); ok && (end == len(rs) || !pytext.IsWord(rs[end])) {
				b.WriteString(string(rs[copied:i]))
				b.WriteString("***")
				b.WriteString(lastDigits(rs[i:end], 4))
				i, copied = end, end
				continue
			}
		}
		i++
	}
	if copied == 0 {
		return s
	}
	b.WriteString(string(rs[copied:]))
	return b.String()
}

func matchShape(rs []rune, i int, shape []part) (int, bool) {
	for _, p := range shape {
		if p.digits > 0 {
			for range p.digits {
				if i >= len(rs) || !pytext.IsDigit(rs[i]) {
					return 0, false
				}
				i++
			}
			continue
		}
		if i < len(rs) && (rs[i] == p.sep || pytext.IsSpace(rs[i])) {
			i++
		}
	}
	return i, true
}

// lastDigits é re.sub(r"\D", "", trecho)[-n:].
func lastDigits(rs []rune, n int) string {
	var d []rune
	for _, r := range rs {
		if pytext.IsDigit(r) {
			d = append(d, r)
		}
	}
	if len(d) > n {
		d = d[len(d)-n:]
	}
	return string(d)
}

// cleanPayload é _limpar_payload da origem: remove os campos sensíveis e mascara e corta
// (em maxChars caracteres) cada texto de uma estrutura JSON.
func cleanPayload(v any, maxChars int) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			if SensitiveFields[k] {
				out[k] = Removed
				continue
			}
			out[k] = cleanPayload(val, maxChars)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, val := range x {
			out[i] = cleanPayload(val, maxChars)
		}
		return out
	case string:
		s := MaskDocuments(x)
		if utf8.RuneCountInString(s) > maxChars {
			s = string([]rune(s)[:maxChars]) + Truncated
		}
		return s
	}
	return v
}

// MaskAttribute é mascarar_atributo da origem, aplicado a todo atributo de texto que sai para o
// Langfuse: JSON (objeto ou lista) passa por cleanPayload; outro texto tem CNPJ e CPF
// mascarados. O resultado é limitado a maxBytes, sem cortar caractere ao meio.
func MaskAttribute(s string, maxBytes int) string {
	text := s
	if parsed, ok := decodeStructure(s); ok {
		if b, err := marshal(cleanPayload(parsed, maxBytes)); err == nil {
			text = string(b)
		}
	} else {
		text = MaskDocuments(s)
	}
	if len(text) > maxBytes {
		text = cutBytes(text, maxBytes) + Truncated
	}
	return text
}

// decodeStructure lê s como JSON e só aceita objeto ou lista (json.loads + isinstance).
func decodeStructure(s string) (any, bool) {
	dec := json.NewDecoder(strings.NewReader(s))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil || dec.More() {
		return nil, false
	}
	switch v.(type) {
	case map[string]any, []any:
		return v, true
	}
	return nil, false
}

// cutBytes é texto.encode()[:n].decode(errors="ignore"): corta em n bytes e descarta o
// caractere que ficou incompleto no fim.
func cutBytes(s string, n int) string {
	t := s[:n]
	for len(t) > 0 {
		r, size := utf8.DecodeLastRuneInString(t)
		if r != utf8.RuneError || size > 1 {
			break
		}
		t = t[:len(t)-1]
	}
	return t
}

// marshal serializa sem escapar <, > e & (como o json.dumps da origem).
func marshal(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}
