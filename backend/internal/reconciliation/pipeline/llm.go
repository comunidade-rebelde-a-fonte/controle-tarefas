package pipeline

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"controle/internal/ai"
	"controle/internal/ai/litellm"
	"controle/internal/ai/prompts"
	"controle/internal/ai/tracer"
	"controle/internal/jobs"
	"controle/internal/platform/decimal"
)

// Chamada ao LLM pelo gateway (ClienteLLM.complete da origem): o modelo é o snapshot do alias
// resolvido no início da corrida, a saída é estruturada (JSON Schema estrito) e validada pelo
// código. Resposta sem conteúdo útil (recusa, sem escolhas) conta como saída inválida, como o
// TentativaParseError da origem; falha do gateway é falha transitória do job.

// promptRef é o prompt de sistema enviado: o hash do texto (hash_texto da origem, sha256) e a
// referência nome@versão#hash de cada parte.
type promptRef struct {
	hash    string
	version string
}

func systemPrompt(parts ...prompts.Prompt) (string, promptRef) {
	texts := make([]string, 0, len(parts))
	refs := make([]string, 0, len(parts))
	for _, p := range parts {
		texts = append(texts, p.Text)
		refs = append(refs, p.Ref())
	}
	text := strings.Join(texts, "\n\n")
	sum := sha256.Sum256([]byte(text))
	return text, promptRef{hash: hex.EncodeToString(sum[:]), version: strings.Join(refs, ";")}
}

// llmCall é uma chamada estruturada.
type llmCall struct {
	name    string // nome da geração no trace
	model   ai.Resolved
	system  string
	user    string
	images  []litellm.Image
	schema  *litellm.JSONSchema
	attempt int
	// input vai para o trace no lugar do conteúdo (nunca o documento).
	input map[string]any
}

// llmReply é a resposta da chamada; unusable indica resposta HTTP 200 sem conteúdo útil.
type llmReply struct {
	resp     litellm.Response
	unusable bool
}

// call chama o gateway e registra a geração na corrida. Falha do gateway volta como erro.
func (p *Processor) call(ctx context.Context, run *tracer.Run, c llmCall) (llmReply, error) {
	req := litellm.Request{
		Model:       c.model.Model,
		Messages:    []litellm.Message{{Role: "system", Content: c.system}, {Role: "user", Content: c.user, Images: c.images}},
		Temperature: c.model.Parameters.Temperature,
		JSONSchema:  c.schema,
		Metadata:    map[string]string{"alias": string(c.model.Alias), "trace_id": run.TraceID()},
	}
	if c.model.Parameters.MaxTokens != nil {
		req.MaxTokens = *c.model.Parameters.MaxTokens
	}
	if p.gateway == nil {
		return llmReply{}, jobs.Coded(tracer.CodeLLM, litellm.ErrNotConfigured)
	}
	resp, err := p.gateway.Chat(ctx, req)
	var gw *litellm.Error
	unusable := errors.As(err, &gw) && gw.Status == http.StatusOK
	gen := tracer.Generation{Name: c.name, Model: c.model.Model, Err: err, Input: c.input}
	if err == nil {
		gen.Response = &resp
		gen.Output = map[string]any{"caracteres": len(resp.Content), "tentativa": c.attempt}
	}
	if gerr := run.Generation(gen); gerr != nil {
		return llmReply{}, gerr
	}
	switch {
	case unusable:
		return llmReply{unusable: true}, nil
	case err != nil:
		// Gateway fora do ar: falha transitória do job, com o código no job e na corrida.
		return llmReply{}, jobs.Coded(tracer.CodeLLM, err)
	}
	return llmReply{resp: resp}, nil
}

// usage devolve tokens e custo da resposta para o log de extração.
func usage(r litellm.Response) (in, out *int32, cost *decimal.Decimal) {
	i, o := clamp32(int64(r.Usage.PromptTokens)), clamp32(int64(r.Usage.CompletionTokens))
	in, out = &i, &o
	if c, err := decimal.ParseNumber(r.CostUSD); err == nil && c.Sign() >= 0 {
		q := c.Quantize(-8)
		cost = &q
	}
	return in, out, cost
}

func compact(s string) json.RawMessage {
	var b bytes.Buffer
	if err := json.Compact(&b, []byte(s)); err != nil {
		panic(err)
	}
	return b.Bytes()
}

// triageSchema é o schema estrito da saída do triador (ResultadoTriagem).
var triageSchema = compact(`{
	"type": "object",
	"properties": {
		"tipo": {"type": "string", "enum": ["extrato", "comprovante", "desconhecido"]},
		"banco": {"type": ["string", "null"]},
		"agencia": {"type": ["string", "null"]},
		"conta": {"type": ["string", "null"]},
		"periodo_inicio": {"type": ["string", "null"]},
		"periodo_fim": {"type": ["string", "null"]},
		"confianca": {"type": "number"},
		"justificativa": {"type": "string"}
	},
	"required": ["tipo", "banco", "agencia", "conta", "periodo_inicio", "periodo_fim", "confianca", "justificativa"],
	"additionalProperties": false
}`)

// statementSchema é o schema estrito da saída do extrator (ExtratoExtraido, PRD §7 da origem):
// dinheiro como texto decimal, nunca número.
var statementSchema = compact(`{
	"type": "object",
	"properties": {
		"banco": {"type": "string"},
		"agencia": {"type": "string"},
		"conta": {"type": "string"},
		"periodo_inicio": {"type": "string"},
		"periodo_fim": {"type": "string"},
		"saldo_inicial": {"type": "string"},
		"saldo_final": {"type": "string"},
		"leiaute": {"anyOf": [{"type": "null"}, {
			"type": "object",
			"properties": {
				"pagina_inicio_tabela": {"type": "integer"},
				"paginas_ignorar": {"type": "array", "items": {"type": "integer"}},
				"formato_data": {"type": "string"},
				"separador_decimal": {"type": "string"},
				"representacao_sinal": {"type": "string"},
				"localizacao_saldos": {"type": "string"},
				"estrategia": {"type": "string"}
			},
			"required": ["pagina_inicio_tabela", "paginas_ignorar", "formato_data", "separador_decimal",
				"representacao_sinal", "localizacao_saldos", "estrategia"],
			"additionalProperties": false
		}]},
		"transacoes": {"type": "array", "items": {
			"type": "object",
			"properties": {
				"ordem": {"type": "integer"},
				"data": {"type": "string"},
				"descricao": {"type": "string"},
				"documento": {"type": ["string", "null"]},
				"valor": {"type": "string"},
				"sentido": {"type": "string", "enum": ["C", "D"]},
				"saldo_apos": {"type": ["string", "null"]},
				"confianca": {"type": "number"}
			},
			"required": ["ordem", "data", "descricao", "documento", "valor", "sentido", "saldo_apos", "confianca"],
			"additionalProperties": false
		}}
	},
	"required": ["banco", "agencia", "conta", "periodo_inicio", "periodo_fim", "saldo_inicial", "saldo_final",
		"leiaute", "transacoes"],
	"additionalProperties": false
}`)
