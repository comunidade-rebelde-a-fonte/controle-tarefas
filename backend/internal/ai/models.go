// Package ai reúne o catálogo de aliases de modelo dos agentes e a resolução alias → modelo
// [ADR-004, spec 04 §8, ai-behavior-spec §3].
//
// O domínio nunca cita um modelo concreto: usa o alias. O modelo vem da configuração — padrão do
// .env (<ALIAS>_MODEL) ou a escolha da tela Configurações → IA (TASK-008, pacote ai/aliases). A
// resolução acontece no início de cada corrida e vira o snapshot gravado nela: trocar a
// configuração não muda uma corrida em andamento [spec 07 F-010].
package ai

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// Alias identifica o modelo de um agente sem fixar o modelo concreto.
type Alias string

const (
	Triador       Alias = "triador"
	Extrator      Alias = "extrator"
	ExtratorForte Alias = "extrator_forte"
	Resolvedor    Alias = "resolvedor"
	Classificador Alias = "classificador"
	Consultor     Alias = "consultor"
	Priorizador   Alias = "priorizador"
)

// AliasInfo descreve um alias. O parser_pgdas não aparece aqui: é determinístico (ADR-006).
type AliasInfo struct {
	Alias   Alias
	Module  string // conciliacao | fator_r
	Purpose string
	EnvVar  string // variável do .env com o modelo padrão
	// Requisitos do agente, conferidos no gateway quando o modelo muda (TASK-008, D-014).
	NeedsStructuredOutput bool // a saída é JSON validado por schema
	NeedsVision           bool // lê a página renderizada quando o PDF não tem camada de texto
}

// Catalog é a lista de aliases, na ordem de exibição. A mesma lista existe no
// litellm/config.yaml, no docker-compose.yml e na tabela ai_model_aliases; testes conferem.
//
// Visão: triador e extratores leem a página renderizada quando o documento não tem camada de
// texto (plano do Conciliador, M1.4 e M3.3). Os demais recebem só dados já extraídos.
var Catalog = []AliasInfo{
	{Triador, "conciliacao", "Classifica o documento recebido (tipo, banco, conta, período)", "TRIADOR_MODEL", true, true},
	{Extrator, "conciliacao", "Extrai extratos e comprovantes (tentativas 1 e 2)", "EXTRATOR_MODEL", true, true},
	{ExtratorForte, "conciliacao", "Extrator na 3ª tentativa, com modelo mais forte", "EXTRATOR_FORTE_MODEL", true, true},
	{Resolvedor, "conciliacao", "Propõe hipóteses para exceções da conciliação (nunca concilia)", "RESOLVEDOR_MODEL", true, false},
	{Classificador, "conciliacao", "Sugere conta contábil sem regra nem histórico (sempre confirmada)", "CLASSIFICADOR_MODEL", true, false},
	{Consultor, "fator_r", "Entende a pergunta e redige a resposta a partir do cálculo já feito", "CONSULTOR_MODEL", true, false},
	{Priorizador, "fator_r", "Entende a pergunta e redige a fila de prioridades já calculada", "PRIORIZADOR_MODEL", true, false},
}

// Providers são os provedores com rota no gateway (litellm/config.yaml: openai/*, anthropic/*).
var Providers = []string{"openai", "anthropic"}

var (
	ErrUnknownAlias  = errors.New("alias de modelo desconhecido")
	ErrNotConfigured = errors.New("alias sem modelo configurado")
	ErrAliasDisabled = errors.New("alias desligado em Configurações → IA")

	ErrUnknownProvider  = errors.New("provedor sem rota no gateway")
	ErrInvalidModelName = errors.New("nome de modelo inválido")
	// ErrCredentialLike nunca cita o valor recebido: pode ser uma chave de verdade.
	ErrCredentialLike = errors.New("o nome do modelo parece uma chave de API; credenciais ficam só no ambiente do LiteLLM")
)

// Model é um modelo concreto no formato do gateway: provedor/modelo.
type Model struct {
	Provider string
	Name     string
}

func (m Model) String() string { return m.Provider + "/" + m.Name }

var modelName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,199}$`)

// credentialLike reconhece o formato das chaves de API (sk-…, pk-…, rk-…). Um nome de modelo
// assim é quase certamente uma chave colada no campo errado e nunca é aceito [REQ-047].
var credentialLike = regexp.MustCompile(`(?i)^(sk|pk|rk)-`)

// ParseModel valida "provedor/modelo": provedor com rota no gateway e nome sem espaços.
func ParseModel(s string) (Model, error) {
	provider, name, ok := strings.Cut(strings.TrimSpace(s), "/")
	if !ok || provider == "" || name == "" {
		return Model{}, fmt.Errorf("modelo %q fora do formato provedor/modelo (ex.: openai/gpt-6-luna)", s)
	}
	return NewModel(provider, name)
}

// NewModel valida provedor e nome separados, como chegam da tela Configurações → IA.
func NewModel(provider, name string) (Model, error) {
	if !IsProvider(provider) {
		return Model{}, fmt.Errorf("%w: %q (use %s)", ErrUnknownProvider, provider, strings.Join(Providers, " ou "))
	}
	if credentialLike.MatchString(name) {
		return Model{}, ErrCredentialLike
	}
	if !modelName.MatchString(name) {
		return Model{}, fmt.Errorf("%w: %q", ErrInvalidModelName, name)
	}
	return Model{Provider: provider, Name: name}, nil
}

// IsProvider indica se o provedor tem rota no gateway.
func IsProvider(p string) bool { return slices.Contains(Providers, p) }

// Lookup devolve a descrição de um alias.
func Lookup(a Alias) (AliasInfo, bool) {
	for _, info := range Catalog {
		if info.Alias == a {
			return info, true
		}
	}
	return AliasInfo{}, false
}

// Defaults é o modelo padrão de cada alias, lido do .env.
type Defaults map[Alias]Model

// DefaultsFromEnv lê <ALIAS>_MODEL de cada alias. Variável vazia deixa o alias sem modelo (a
// corrida falha com ErrNotConfigured); valor fora do formato é erro de configuração.
func DefaultsFromEnv(getenv func(string) string) (Defaults, error) {
	defaults := Defaults{}
	var errs []error
	for _, info := range Catalog {
		raw := strings.TrimSpace(getenv(info.EnvVar))
		if raw == "" {
			continue
		}
		m, err := ParseModel(raw)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", info.EnvVar, err))
			continue
		}
		defaults[info.Alias] = m
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return defaults, nil
}

// Source diz de onde veio o modelo resolvido.
type Source string

const (
	// SourceEnv: padrão do .env (<ALIAS>_MODEL).
	SourceEnv Source = "env"
	// SourceSettings: escolha gravada na tela Configurações → IA (TASK-008).
	SourceSettings Source = "settings"
)

// Parameters são os parâmetros de chamada permitidos por alias. Nenhum é secreto; ausente vale
// o padrão do provedor. A lista é fechada (também no banco): parâmetro novo exige código.
type Parameters struct {
	Temperature *float64 `json:"temperature,omitempty"`
	MaxTokens   *int     `json:"max_tokens,omitempty"`
}

// Limites locais dos parâmetros. A faixa de temperature é a da OpenAI; o teto de max_tokens de
// cada modelo vem do gateway na validação.
const (
	MaxTemperature = 2.0
	MaxTokensLimit = 1_000_000
)

// Equal compara os valores, não os ponteiros.
func (p Parameters) Equal(o Parameters) bool {
	return eqPtr(p.Temperature, o.Temperature) && eqPtr(p.MaxTokens, o.MaxTokens)
}

func eqPtr[T comparable](a, b *T) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// clone copia os valores apontados: o snapshot não compartilha memória com a origem.
func (p Parameters) clone() Parameters {
	var out Parameters
	if p.Temperature != nil {
		v := *p.Temperature
		out.Temperature = &v
	}
	if p.MaxTokens != nil {
		v := *p.MaxTokens
		out.MaxTokens = &v
	}
	return out
}

// Setting é a configuração de um alias gravada pela tela Configurações → IA.
type Setting struct {
	Model      *Model // nil = padrão do .env
	Parameters Parameters
	Enabled    bool
	Version    int32
}

// Resolved é o snapshot da resolução, gravado na corrida e usado em todas as chamadas dela.
type Resolved struct {
	Alias      Alias
	Model      Model
	Source     Source
	Parameters Parameters
	// Version é a versão da configuração do alias usada (0 = só o .env, sem a tela).
	Version int32
}

// Resolver resolve o alias no início da corrida.
type Resolver struct {
	defaults Defaults
}

func NewResolver(defaults Defaults) *Resolver {
	copied := make(Defaults, len(defaults))
	for a, m := range defaults {
		copied[a] = m
	}
	return &Resolver{defaults: copied}
}

// Resolve devolve o modelo do alias só pelo .env. O resultado é um valor: mudanças
// posteriores na configuração não o alteram.
func (r *Resolver) Resolve(a Alias) (Resolved, error) {
	return r.ResolveWith(a, Setting{Enabled: true})
}

// ResolveWith aplica a configuração da tela sobre o padrão do .env: o modelo escolhido vence e,
// sem escolha, vale o .env. Alias desligado não resolve. Como Resolve, devolve um snapshot.
func (r *Resolver) ResolveWith(a Alias, s Setting) (Resolved, error) {
	if _, ok := Lookup(a); !ok {
		return Resolved{}, fmt.Errorf("%w: %q", ErrUnknownAlias, a)
	}
	if !s.Enabled {
		return Resolved{}, fmt.Errorf("%w: %s", ErrAliasDisabled, a)
	}
	out := Resolved{Alias: a, Parameters: s.Parameters.clone(), Version: s.Version}
	if s.Model != nil {
		out.Model, out.Source = *s.Model, SourceSettings
		return out, nil
	}
	m, ok := r.defaults[a]
	if !ok {
		return Resolved{}, fmt.Errorf("%w: %s", ErrNotConfigured, a)
	}
	out.Model, out.Source = m, SourceEnv
	return out, nil
}

// Default devolve o padrão do .env do alias, se houver.
func (r *Resolver) Default(a Alias) (Model, bool) {
	m, ok := r.defaults[a]
	return m, ok
}
