package tracer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"unicode/utf8"

	"controle/internal/ai/langfuse"
	"controle/internal/platform/redact"
)

// MaxTextChars limita cada texto guardado na trilha local: o banco guarda a auditoria da
// decisão, não o documento (o original fica no MinIO). O mesmo teto da origem para o Langfuse.
const MaxTextChars = 32 * 1024

// sanitize converte v para JSON genérico e o limpa para a trilha local [AC-029, "entrada
// sanitizada"]: campos sensíveis (texto bruto do PDF, bytes do arquivo) viram "[removido]",
// chaves de API em textos são mascaradas e textos longos são cortados. O CNPJ continua: a
// trilha local é a auditoria; o mascaramento de CNPJ e CPF vale no envio ao Langfuse.
func sanitize(v any) (any, error) {
	if v == nil {
		return nil, nil
	}
	b, err := json.Marshal(v) // Decimal, competência e datas viram texto pelos próprios MarshalJSON
	if err != nil {
		return nil, fmt.Errorf("tracer: valor não serializável em JSON: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var g any
	if err := dec.Decode(&g); err != nil {
		return nil, fmt.Errorf("tracer: %w", err)
	}
	return clean(g), nil
}

func clean(v any) any {
	switch x := v.(type) {
	case map[string]any:
		for k, val := range x {
			if langfuse.SensitiveFields[k] {
				x[k] = langfuse.Removed
				continue
			}
			x[k] = clean(val)
		}
		return x
	case []any:
		for i, val := range x {
			x[i] = clean(val)
		}
		return x
	case string:
		s := redact.Secrets(x)
		if utf8.RuneCountInString(s) > MaxTextChars {
			s = string([]rune(s)[:MaxTextChars]) + langfuse.Truncated
		}
		return s
	}
	return v
}

// objectJSON sanitiza um objeto e o serializa ("{}" quando vazio).
func objectJSON(v map[string]any) ([]byte, error) {
	if len(v) == 0 {
		return []byte("{}"), nil
	}
	s, err := sanitize(v)
	if err != nil {
		return nil, err
	}
	return json.Marshal(s)
}
