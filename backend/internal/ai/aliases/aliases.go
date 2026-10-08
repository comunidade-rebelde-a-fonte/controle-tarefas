// Package aliases é a configuração dos modelos de IA por alias: a tela Configurações → IA
// [REQ-040, 06 §7, 08 §9, TASK-008] e a resolução feita no início de cada corrida [AC-027].
//
// Só configuração não secreta passa por aqui. As chaves dos provedores ficam no ambiente do
// LiteLLM; nenhum campo de credencial é aceito, gravado ou devolvido [REQ-047, 10 §9].
package aliases

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"controle/internal/ai"
	"controle/internal/ai/litellm"
	"controle/internal/platform/apperr"
	"controle/internal/platform/clock"
	"controle/internal/platform/opt"
	"controle/internal/platform/redact"
	"controle/internal/store"
	"controle/internal/user"
)

// ValidationTimeout limita a consulta ao gateway feita pela tela.
const ValidationTimeout = 15 * time.Second

// Origem do modelo efetivo de um alias.
const (
	SourceSettings = string(ai.SourceSettings)
	SourceEnv      = string(ai.SourceEnv)
	// SourceNone: sem escolha na tela e sem padrão no .env; as corridas falham até configurar.
	SourceNone = "none"
)

// Gateway é a parte do cliente do LiteLLM usada aqui.
type Gateway interface {
	Configured() bool
	ModelInfo(ctx context.Context, m ai.Model) (litellm.ModelInfo, error)
}

// Requirements são os requisitos do agente conferidos na validação.
type Requirements struct {
	StructuredOutput bool `json:"structured_output"`
	Vision           bool `json:"vision"`
}

// Alias é a configuração de um alias como a API devolve. Nunca contém credencial.
type Alias struct {
	Alias        string       `json:"alias"`
	Module       string       `json:"module"`
	Purpose      string       `json:"purpose"`
	Requirements Requirements `json:"requirements"`
	// Provider e Model são a escolha gravada na tela; null = padrão do ambiente.
	Provider *string `json:"provider"`
	Model    *string `json:"model"`
	// DefaultModel é o padrão do ambiente (<ALIAS>_MODEL), em provedor/modelo.
	DefaultModel *string `json:"default_model"`
	// EffectiveModel é o que as novas corridas usam; null = sem modelo.
	EffectiveModel *string       `json:"effective_model"`
	Source         string        `json:"source"`
	Parameters     ai.Parameters `json:"parameters"`
	Enabled        bool          `json:"enabled"`
	Version        int32         `json:"version"`
	UpdatedAt      time.Time     `json:"updated_at"`
}

// List é a tabela da tela, na ordem do ai.Catalog.
type List struct {
	GatewayConfigured bool    `json:"gateway_configured"`
	Items             []Alias `json:"items"`
}

// UpdateInput substitui a configuração do alias (PUT). Todos os campos são obrigatórios;
// provider e model nulos voltam ao padrão do ambiente. Campo desconhecido (ex.: api_key) é
// recusado na leitura do JSON.
type UpdateInput struct {
	Provider   opt.Optional[string] `json:"provider"`
	Model      opt.Optional[string] `json:"model"`
	Parameters *ai.Parameters       `json:"parameters"`
	Enabled    *bool                `json:"enabled"`
	Version    *int32               `json:"version"`
}

// ValidateInput é a configuração a validar, sem gravar.
type ValidateInput struct {
	Provider   opt.Optional[string] `json:"provider"`
	Model      opt.Optional[string] `json:"model"`
	Parameters *ai.Parameters       `json:"parameters"`
}

// Resolver resolve o alias no início de cada corrida: a escolha gravada na tela vence o .env
// [AC-027]. O resultado é o snapshot usado em todas as chamadas da corrida [F-010].
type Resolver struct{ env *ai.Resolver }

// NewResolver cria o resolvedor com os padrões do .env.
func NewResolver(defaults ai.Defaults) *Resolver { return &Resolver{env: ai.NewResolver(defaults)} }

// Resolve lê a configuração do alias em db (pool ou transação da corrida) e a aplica.
func (r *Resolver) Resolve(ctx context.Context, db store.DBTX, a ai.Alias) (ai.Resolved, error) {
	if _, ok := ai.Lookup(a); !ok {
		return ai.Resolved{}, fmt.Errorf("%w: %q", ai.ErrUnknownAlias, a)
	}
	row, err := store.New(db).GetAIModelAlias(ctx, string(a))
	if err != nil {
		return ai.Resolved{}, fmt.Errorf("ler a configuração do alias %s: %w", a, err)
	}
	setting, err := settingOf(row)
	if err != nil {
		return ai.Resolved{}, err
	}
	return r.env.ResolveWith(a, setting)
}

// effective devolve o modelo que as novas corridas usariam com a escolha dada.
func (r *Resolver) effective(a ai.Alias, chosen *ai.Model) (*ai.Model, string) {
	if chosen != nil {
		return chosen, SourceSettings
	}
	if m, ok := r.env.Default(a); ok {
		return &m, SourceEnv
	}
	return nil, SourceNone
}

// settingOf lê a linha gravada. A linha passou pelos CHECKs da migração; erro aqui é dado
// corrompido, e a corrida não deve seguir com ele.
func settingOf(row store.AiModelAlias) (ai.Setting, error) {
	s := ai.Setting{Enabled: row.Enabled, Version: row.Version}
	if row.Provider != nil && row.Model != nil {
		m, err := ai.NewModel(*row.Provider, *row.Model)
		if err != nil {
			return ai.Setting{}, fmt.Errorf("configuração gravada do alias %s: %w", row.Alias, err)
		}
		s.Model = &m
	}
	if err := json.Unmarshal(row.Parameters, &s.Parameters); err != nil {
		return ai.Setting{}, fmt.Errorf("parâmetros gravados do alias %s: %w", row.Alias, err)
	}
	return s, nil
}

// Service atende a tela Configurações → IA.
type Service struct {
	pool     *pgxpool.Pool
	clock    clock.Clock
	resolver *Resolver
	gateway  Gateway
}

// NewService cria o serviço. defaults são os padrões do .env; gateway é o cliente do LiteLLM.
func NewService(pool *pgxpool.Pool, clk clock.Clock, defaults ai.Defaults, gateway Gateway) *Service {
	return &Service{pool: pool, clock: clk, resolver: NewResolver(defaults), gateway: gateway}
}

// Resolver devolve o resolvedor usado pelas corridas.
func (s *Service) Resolver() *Resolver { return s.resolver }

// List devolve a configuração de todos os aliases, na ordem do catálogo.
func (s *Service) List(ctx context.Context) (List, error) {
	rows, err := store.New(s.pool).ListAIModelAliases(ctx)
	if err != nil {
		return List{}, err
	}
	byAlias := make(map[string]store.AiModelAlias, len(rows))
	for _, r := range rows {
		byAlias[r.Alias] = r
	}
	out := List{GatewayConfigured: s.gateway.Configured(), Items: make([]Alias, 0, len(ai.Catalog))}
	for _, info := range ai.Catalog {
		row, ok := byAlias[string(info.Alias)]
		if !ok {
			return List{}, fmt.Errorf("alias %s sem linha em ai_model_aliases", info.Alias)
		}
		a, err := s.toAlias(info, row)
		if err != nil {
			return List{}, err
		}
		out.Items = append(out.Items, a)
	}
	return out, nil
}

// Update grava a configuração do alias, com concorrência otimista. Toda mudança no modelo
// efetivo ou nos parâmetros é conferida no gateway antes de gravar; ligar ou desligar o alias
// não depende dele. Vale só para as novas corridas [F-010].
func (s *Service) Update(ctx context.Context, alias string, in UpdateInput) (Alias, error) {
	info, ok := ai.Lookup(ai.Alias(alias))
	if !ok {
		return Alias{}, errAliasNotFound()
	}
	fields := fieldErrors{}
	cand := parseCandidate(fields, in.Provider, in.Model, in.Parameters)
	if in.Enabled == nil {
		fields["enabled"] = "Campo obrigatório."
	}
	if in.Version == nil {
		fields["version"] = "Campo obrigatório."
	}
	if err := fields.err(); err != nil {
		return Alias{}, err
	}

	cur, err := store.New(s.pool).GetAIModelAlias(ctx, alias)
	if err != nil {
		return Alias{}, fmt.Errorf("ler a configuração do alias %s: %w", alias, err)
	}
	if cur.Version != *in.Version {
		return Alias{}, apperr.VersionConflict().WithDetails(map[string]any{"current_version": cur.Version})
	}
	curSetting, err := settingOf(cur)
	if err != nil {
		return Alias{}, err
	}
	before, _ := s.resolver.effective(info.Alias, curSetting.Model)
	after, _ := s.resolver.effective(info.Alias, cand.model)
	if after != nil && (!sameModel(before, after) || !curSetting.Parameters.Equal(cand.params)) {
		v, err := s.check(ctx, info, *after, cand.params)
		if err != nil {
			return Alias{}, err
		}
		if !v.OK {
			return Alias{}, apperr.Unprocessable("AI_MODEL_INVALID",
				"A configuração não foi gravada. "+firstFailure(v.Checks)).
				WithDetails(map[string]any{"model": after.String(), "checks": v.Checks})
		}
	}

	params, err := json.Marshal(cand.params)
	if err != nil {
		return Alias{}, err
	}
	var provider, model *string
	if cand.model != nil {
		provider, model = &cand.model.Provider, &cand.model.Name
	}
	uid := user.FromContext(ctx)
	var row store.AiModelAlias
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		var err error
		row, err = q.UpdateAIModelAlias(ctx, store.UpdateAIModelAliasParams{
			Provider: provider, Model: model, Parameters: params, Enabled: *in.Enabled,
			UpdatedBy: &uid, UpdatedAt: s.clock.Now(), Alias: alias, Version: *in.Version,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.VersionConflict()
		}
		if err != nil {
			return err
		}
		return q.InsertAIModelAliasHistory(ctx, store.InsertAIModelAliasHistoryParams{
			Alias: row.Alias, Version: row.Version, Provider: row.Provider, Model: row.Model,
			Parameters: row.Parameters, Enabled: row.Enabled, ChangedBy: row.UpdatedBy, ChangedAt: row.UpdatedAt,
		})
	})
	if err != nil {
		return Alias{}, err
	}
	return s.toAlias(info, row)
}

// Validate confere a configuração proposta no gateway, sem gravar nada e sem chamar o
// provedor: consulta só a tabela de modelos do LiteLLM [06 §7].
func (s *Service) Validate(ctx context.Context, alias string, in ValidateInput) (Validation, error) {
	info, ok := ai.Lookup(ai.Alias(alias))
	if !ok {
		return Validation{}, errAliasNotFound()
	}
	fields := fieldErrors{}
	cand := parseCandidate(fields, in.Provider, in.Model, in.Parameters)
	if err := fields.err(); err != nil {
		return Validation{}, err
	}
	model, source := s.resolver.effective(info.Alias, cand.model)
	if model == nil {
		return Validation{Source: SourceNone, Checks: []Check{{CodeModelConfigured, false,
			"Sem modelo: escolha um modelo ou defina " + info.EnvVar + " no ambiente."}}}, nil
	}
	v, err := s.check(ctx, info, *model, cand.params)
	if err != nil {
		return Validation{}, err
	}
	v.Source = source
	return v, nil
}

// check consulta o modelo no gateway e confere requisitos e parâmetros.
func (s *Service) check(ctx context.Context, info ai.AliasInfo, m ai.Model, p ai.Parameters) (Validation, error) {
	ctx, cancel := context.WithTimeout(ctx, ValidationTimeout)
	defer cancel()
	name := m.String()
	mi, err := s.gateway.ModelInfo(ctx, m)
	switch {
	case errors.Is(err, litellm.ErrUnknownModel):
		return Validation{Model: &name, Checks: []Check{unknownModel(m)}}, nil
	case errors.Is(err, litellm.ErrNotConfigured):
		return Validation{}, apperr.New(http.StatusServiceUnavailable, "AI_GATEWAY_NOT_CONFIGURED",
			"O gateway de IA não está configurado (LITELLM_BASE_URL): sem ele não é possível validar nem trocar o modelo.")
	case err != nil:
		return Validation{}, errGatewayUnavailable(err)
	}
	checks := Evaluate(info, m, p, mi)
	return Validation{OK: allOK(checks), Model: &name, Checks: checks, Capabilities: capabilitiesOf(mi)}, nil
}

func (s *Service) toAlias(info ai.AliasInfo, row store.AiModelAlias) (Alias, error) {
	setting, err := settingOf(row)
	if err != nil {
		return Alias{}, err
	}
	a := Alias{
		Alias: string(info.Alias), Module: info.Module, Purpose: info.Purpose,
		Requirements: Requirements{StructuredOutput: info.NeedsStructuredOutput, Vision: info.NeedsVision},
		Provider:     row.Provider, Model: row.Model, Parameters: setting.Parameters,
		Enabled: row.Enabled, Version: row.Version, UpdatedAt: row.UpdatedAt,
	}
	if m, ok := s.resolver.env.Default(info.Alias); ok {
		d := m.String()
		a.DefaultModel = &d
	}
	eff, source := s.resolver.effective(info.Alias, setting.Model)
	a.Source = source
	if eff != nil {
		e := eff.String()
		a.EffectiveModel = &e
	}
	return a, nil
}

// candidate é a configuração proposta, já validada localmente.
type candidate struct {
	model  *ai.Model // nil = padrão do ambiente
	params ai.Parameters
}

func parseCandidate(f fieldErrors, provider, model opt.Optional[string], params *ai.Parameters) candidate {
	var c candidate
	if !provider.Set {
		f["provider"] = "Campo obrigatório (null = padrão do ambiente)."
	}
	if !model.Set {
		f["model"] = "Campo obrigatório (null = padrão do ambiente)."
	}
	if params == nil {
		f["parameters"] = "Campo obrigatório ({} = padrão do provedor)."
	} else {
		c.params = *params
		if t := params.Temperature; t != nil && (*t < 0 || *t > ai.MaxTemperature) {
			f["parameters.temperature"] = "Informe um valor de 0 a 2."
		}
		if n := params.MaxTokens; n != nil && (*n < 1 || *n > ai.MaxTokensLimit) {
			f["parameters.max_tokens"] = "Informe um número inteiro de 1 a " + thousands(ai.MaxTokensLimit) + "."
		}
	}
	p, m := trimmed(provider.Value), trimmed(model.Value)
	switch {
	case p == "" && m == "":
		// Padrão do ambiente.
	case p == "":
		f["provider"] = "Informe o provedor junto com o modelo, ou nenhum dos dois para usar o padrão do ambiente."
	case m == "":
		f["model"] = "Informe o modelo junto com o provedor, ou nenhum dos dois para usar o padrão do ambiente."
	default:
		parsed, err := ai.NewModel(p, m)
		switch {
		case errors.Is(err, ai.ErrUnknownProvider):
			f["provider"] = "Provedor sem rota no gateway: use " + strings.Join(ai.Providers, " ou ") + "."
		case errors.Is(err, ai.ErrCredentialLike):
			f["model"] = "Isto parece uma chave de API. Credenciais não são configuradas nesta tela: ficam só no ambiente do LiteLLM."
		case err != nil:
			f["model"] = "Nome de modelo inválido: use letras, números, ponto, hífen, dois-pontos ou sublinhado, sem espaços e sem o provedor (ex.: gpt-6-luna)."
		default:
			c.model = &parsed
		}
	}
	return c
}

func trimmed(v *string) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(*v)
}

func sameModel(a, b *ai.Model) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

type fieldErrors map[string]any

func (f fieldErrors) err() error {
	if len(f) == 0 {
		return nil
	}
	return apperr.BadRequest("VALIDATION_ERROR", "Dados inválidos. Verifique os campos destacados.").
		WithDetails(map[string]any{"fields": map[string]any(f)})
}

func errAliasNotFound() error {
	return apperr.NotFound("AI_ALIAS_NOT_FOUND", "Alias de IA não encontrado.")
}

// errGatewayUnavailable traduz a falha do gateway. O motivo já vem sanitizado do cliente e é
// sanitizado de novo aqui: nada de chave ou Bearer na resposta [10 §9].
func errGatewayUnavailable(err error) error {
	msg := "O gateway de IA não respondeu. Tente de novo em instantes; nada foi alterado."
	var gw *litellm.Error
	if errors.As(err, &gw) && gw.Status >= 400 && gw.Status < 500 {
		msg = "O gateway de IA recusou a consulta (HTTP " + strconv.Itoa(gw.Status) +
			"). Confira a LITELLM_API_KEY no ambiente da API; nada foi alterado."
	}
	return apperr.New(http.StatusServiceUnavailable, "AI_GATEWAY_UNAVAILABLE", msg).
		WithDetails(map[string]any{"reason": redact.Message(err.Error())})
}
