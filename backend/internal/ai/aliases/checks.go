package aliases

import (
	"fmt"
	"strconv"

	"controle/internal/ai"
	"controle/internal/ai/litellm"
)

// Códigos das conferências da validação [06 §7, D-014].
const (
	CodeModelConfigured  = "MODEL_CONFIGURED"
	CodeModelKnown       = "MODEL_KNOWN"
	CodeChatMode         = "CHAT_MODE"
	CodeStructuredOutput = "STRUCTURED_OUTPUT"
	CodeVision           = "VISION"
	CodeTemperature      = "PARAM_TEMPERATURE"
	// CodeOutputLimit confere o max_tokens (o nome evita o falso positivo de credencial do gosec).
	CodeOutputLimit = "PARAM_MAX_TOKENS"
)

// Check é uma conferência da validação, com a mensagem que a tela mostra.
type Check struct {
	Code    string `json:"code"`
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

// Capabilities são as capacidades do modelo segundo o gateway. Parameters lista só os
// parâmetros permitidos (ai.Parameters) que o modelo aceita.
type Capabilities struct {
	Mode             string   `json:"mode"`
	StructuredOutput bool     `json:"structured_output"`
	Vision           bool     `json:"vision"`
	MaxOutputTokens  *int     `json:"max_output_tokens"`
	Parameters       []string `json:"parameters"`
}

// Validation é o resultado de validar uma configuração. Nada é gravado.
type Validation struct {
	OK           bool          `json:"ok"`
	Model        *string       `json:"model"`
	Source       string        `json:"source"`
	Checks       []Check       `json:"checks"`
	Capabilities *Capabilities `json:"capabilities"`
}

// unknownModel é a conferência de um modelo fora da tabela do gateway.
func unknownModel(m ai.Model) Check {
	return Check{CodeModelKnown, false, "O gateway não conhece o modelo " + m.String() +
		". A tabela de modelos é a da versão instalada do LiteLLM: escolha outro modelo ou atualize o LiteLLM."}
}

// Evaluate confere o modelo contra os requisitos do agente e os parâmetros escolhidos. Com
// drop_params no gateway, parâmetro não aceito seria descartado em silêncio: por isso reprova.
func Evaluate(info ai.AliasInfo, m ai.Model, p ai.Parameters, mi litellm.ModelInfo) []Check {
	agent := string(info.Alias)
	checks := []Check{{CodeModelKnown, true, "O gateway conhece o modelo " + m.String() + "."}}
	add := func(code string, ok bool, okMsg, failMsg string) {
		msg := okMsg
		if !ok {
			msg = failMsg
		}
		checks = append(checks, Check{code, ok, msg})
	}
	add(CodeChatMode, mi.Mode == "chat", "É um modelo de chat.",
		fmt.Sprintf("É um modelo do tipo %q; os agentes precisam de um modelo de chat.", mi.Mode))
	if info.NeedsStructuredOutput {
		add(CodeStructuredOutput, mi.StructuredOutput, "Aceita saída estruturada (JSON Schema).",
			"Não aceita saída estruturada (JSON Schema), que o agente "+agent+" exige.")
	}
	if info.NeedsVision {
		add(CodeVision, mi.Vision, "Aceita imagens (visão).",
			"Não aceita imagens, e o agente "+agent+" lê páginas digitalizadas.")
	}
	if p.Temperature != nil {
		add(CodeTemperature, mi.Supports("temperature"), "Aceita temperature.",
			"Não aceita temperature: o gateway descartaria o valor sem avisar.")
	}
	if p.MaxTokens != nil {
		switch {
		case !supportsMaxTokens(mi):
			add(CodeOutputLimit, false, "", "Não aceita max_tokens: o gateway descartaria o valor sem avisar.")
		case mi.MaxOutputTokens > 0 && *p.MaxTokens > mi.MaxOutputTokens:
			add(CodeOutputLimit, false, "", "max_tokens acima do limite do modelo ("+thousands(mi.MaxOutputTokens)+").")
		default:
			add(CodeOutputLimit, true, "max_tokens dentro do limite do modelo.", "")
		}
	}
	return checks
}

// supportsMaxTokens: o LiteLLM traduz max_tokens para max_completion_tokens quando o modelo
// só aceita este.
func supportsMaxTokens(mi litellm.ModelInfo) bool {
	return mi.Supports("max_tokens") || mi.Supports("max_completion_tokens")
}

func capabilitiesOf(mi litellm.ModelInfo) *Capabilities {
	c := &Capabilities{Mode: mi.Mode, StructuredOutput: mi.StructuredOutput, Vision: mi.Vision, Parameters: []string{}}
	if mi.MaxOutputTokens > 0 {
		n := mi.MaxOutputTokens
		c.MaxOutputTokens = &n
	}
	if mi.Supports("temperature") {
		c.Parameters = append(c.Parameters, "temperature")
	}
	if supportsMaxTokens(mi) {
		c.Parameters = append(c.Parameters, "max_tokens")
	}
	return c
}

func allOK(checks []Check) bool {
	for _, c := range checks {
		if !c.OK {
			return false
		}
	}
	return true
}

func firstFailure(checks []Check) string {
	for _, c := range checks {
		if !c.OK {
			return c.Message
		}
	}
	return ""
}

// thousands formata com ponto de milhar (128000 → 128.000).
func thousands(n int) string {
	s := strconv.Itoa(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "." + s[i:]
	}
	return s
}
