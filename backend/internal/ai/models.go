// Package ai reúne o catálogo de aliases de modelo dos agentes e a resolução alias → modelo
// [ADR-004, spec 04 §8, ai-behavior-spec §3].
//
// O domínio nunca cita um modelo concreto: usa o alias. O modelo vem da configuração — padrão do
// .env (<ALIAS>_MODEL) e, a partir da TASK-008, a escolha da tela Configurações → IA. A resolução
// acontece no início de cada corrida e vira o snapshot gravado nela: trocar a configuração não
// muda uma corrida em andamento [spec 07 F-010].
package ai

import (
	"errors"
	"fmt"
	"regexp"
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
}

// Catalog é a lista de aliases, na ordem de exibição. A mesma lista existe no
// litellm/config.yaml e no docker-compose.yml; TestCatalog_MatchesGatewayAndCompose confere.
var Catalog = []AliasInfo{
	{Triador, "conciliacao", "Classifica o documento recebido (tipo, banco, conta, período)", "TRIADOR_MODEL"},
	{Extrator, "conciliacao", "Extrai extratos e comprovantes (tentativas 1 e 2)", "EXTRATOR_MODEL"},
	{ExtratorForte, "conciliacao", "Extrator na 3ª tentativa, com modelo mais forte", "EXTRATOR_FORTE_MODEL"},
	{Resolvedor, "conciliacao", "Propõe hipóteses para exceções da conciliação (nunca concilia)", "RESOLVEDOR_MODEL"},
	{Classificador, "conciliacao", "Sugere conta contábil sem regra nem histórico (sempre confirmada)", "CLASSIFICADOR_MODEL"},
	{Consultor, "fator_r", "Entende a pergunta e redige a resposta a partir do cálculo já feito", "CONSULTOR_MODEL"},
	{Priorizador, "fator_r", "Entende a pergunta e redige a fila de prioridades já calculada", "PRIORIZADOR_MODEL"},
}

// Providers são os provedores com rota no gateway (litellm/config.yaml: openai/*, anthropic/*).
var Providers = []string{"openai", "anthropic"}

var (
	ErrUnknownAlias  = errors.New("alias de modelo desconhecido")
	ErrNotConfigured = errors.New("alias sem modelo configurado")
)

// Model é um modelo concreto no formato do gateway: provedor/modelo.
type Model struct {
	Provider string
	Name     string
}

func (m Model) String() string { return m.Provider + "/" + m.Name }

var modelName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,199}$`)

// ParseModel valida "provedor/modelo": provedor com rota no gateway e nome sem espaços.
func ParseModel(s string) (Model, error) {
	provider, name, ok := strings.Cut(strings.TrimSpace(s), "/")
	if !ok || provider == "" || name == "" {
		return Model{}, fmt.Errorf("modelo %q fora do formato provedor/modelo (ex.: openai/gpt-6-luna)", s)
	}
	if !isProvider(provider) {
		return Model{}, fmt.Errorf("provedor %q sem rota no gateway (use %s)", provider, strings.Join(Providers, " ou "))
	}
	if !modelName.MatchString(name) {
		return Model{}, fmt.Errorf("nome de modelo inválido: %q", name)
	}
	return Model{Provider: provider, Name: name}, nil
}

func isProvider(p string) bool {
	for _, known := range Providers {
		if p == known {
			return true
		}
	}
	return false
}

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

// SourceEnv: padrão do .env. A TASK-008 acrescenta a escolha da tela de configuração.
const SourceEnv Source = "env"

// Resolved é o snapshot da resolução, gravado na corrida e usado em todas as chamadas dela.
type Resolved struct {
	Alias  Alias
	Model  Model
	Source Source
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

// Resolve devolve o modelo do alias. O resultado é um valor: mudanças posteriores na
// configuração não o alteram.
func (r *Resolver) Resolve(a Alias) (Resolved, error) {
	if _, ok := Lookup(a); !ok {
		return Resolved{}, fmt.Errorf("%w: %q", ErrUnknownAlias, a)
	}
	m, ok := r.defaults[a]
	if !ok {
		return Resolved{}, fmt.Errorf("%w: %s", ErrNotConfigured, a)
	}
	return Resolved{Alias: a, Model: m, Source: SourceEnv}, nil
}
