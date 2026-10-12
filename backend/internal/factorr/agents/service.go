// Package agents tem o simulador persistido e os agentes consultor e priorizador do Fator R
// [REQ-013, REQ-014, REQ-015; 06 §3; TASK-013]: porte de services/simulacao.py,
// agents/{consultor,priorizador,etapas,intencao,guardas,render,llm}.py e api/{simulations,
// agents}.py do Fator R (commit 9dd7a74).
//
// Todo número vem do motor e do simulador (factorr/engine). O LLM, pelo LiteLLM com os aliases
// consultor e priorizador, só classifica a intenção e redige o texto; o texto é validado contra
// a decisão e, se trouxer número ou data de fora, cai no template [AC-010].
package agents

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"controle/internal/ai"
	"controle/internal/ai/aliases"
	"controle/internal/ai/tracer"
	"controle/internal/factorr"
	"controle/internal/factorr/engine"
	"controle/internal/platform/apperr"
	"controle/internal/platform/clock"
	"controle/internal/platform/competence"
	"controle/internal/platform/decimal"
	"controle/internal/platform/httpx"
	"controle/internal/store"
	"controle/internal/user"
)

// Agentes e gatilhos (como na origem).
const (
	AgentConsultor   = "consultor"
	AgentPriorizador = "priorizador"
	TriggerFicha     = "ficha"
	TriggerChat      = "chat"
	TriggerRoutine   = "rotina"
)

// queueTextLimit é quantas empresas o template do priorizador cita (LIMITE_TEXTO).
const queueTextLimit = 5

// ConfiguredGateway é o gateway com a checagem de configuração (litellm.Client).
type ConfiguredGateway interface {
	Gateway
	Configured() bool
}

// Service é o simulador e os agentes.
type Service struct {
	pool     *pgxpool.Pool
	clock    clock.Clock // datas de negócio (PA corrente)
	ops      clock.Clock // tempo operacional (registros)
	tracer   *tracer.Tracer
	resolver *aliases.Resolver
	gateway  ConfiguredGateway
	logger   *slog.Logger
}

// NewService cria o serviço. resolver e gateway podem ser nulos (o worker não usa LLM).
func NewService(pool *pgxpool.Pool, businessClock, opsClock clock.Clock, tr *tracer.Tracer, resolver *aliases.Resolver, gateway ConfiguredGateway, logger *slog.Logger) *Service {
	return &Service{pool: pool, clock: businessClock, ops: opsClock, tracer: tr, resolver: resolver, gateway: gateway, logger: logger}
}

// llmFor resolve o alias no início da corrida (snapshot gravado nela). Sem gateway, alias
// desligado ou sem modelo: sem LLM, e a corrida usa palavras-chave e template.
func (s *Service) llmFor(ctx context.Context, alias ai.Alias) (*LLM, *ai.Resolved, string, error) {
	if s.gateway == nil || !s.gateway.Configured() || s.resolver == nil {
		return nil, nil, "gateway_nao_configurado", nil
	}
	r, err := s.resolver.Resolve(ctx, s.pool, alias)
	switch {
	case errors.Is(err, ai.ErrAliasDisabled):
		return nil, nil, "alias_desligado", nil
	case errors.Is(err, ai.ErrNotConfigured):
		return nil, nil, "alias_sem_modelo", nil
	case err != nil:
		return nil, nil, "", err
	}
	return &LLM{gateway: s.gateway, model: r}, &r, "", nil
}

func metadata(llmReason string) map[string]string {
	m := map[string]string{"prompts": PromptRef()}
	if llmReason != "" {
		m["llm_indisponivel"] = llmReason
	}
	return m
}

// --- Simulador [AC-010] ---

// SimulationInput é o corpo de POST /companies/{id}/factor-r/simulations (SimulacaoIn).
type SimulationInput struct {
	PA              string  `json:"pa"`
	Meta            *string `json:"meta"`
	FracaoProLabore *string `json:"fracao_pro_labore"`
	INSS            *string `json:"inss"`
	IRRF            *string `json:"irrf"`
	HorizonteMeses  *int    `json:"horizonte_meses"`
}

// Simulation é a simulação gravada (SimulacaoOut).
type Simulation struct {
	SimulationID string                `json:"simulation_id"`
	TraceID      string                `json:"trace_id"`
	PA           competence.Competence `json:"pa"`
	Veredito     *string               `json:"veredito"`
	Texto        string                `json:"texto"`
	Parametros   map[string]any        `json:"parametros"`
	Resultado    map[string]any        `json:"resultado"`
	CreatedAt    time.Time             `json:"created_at"`
}

// SimulationList é o histórico de simulações da empresa.
type SimulationList struct {
	Items []Simulation `json:"items"`
}

// fieldErrors acumula erros de campo (400 VALIDATION_ERROR, como a base).
type fieldErrors map[string]any

// rate lê uma taxa de 0 a 1 com até 6 casas (Taxa da origem: max_digits 7, decimal_places 6).
func (f fieldErrors) rate(field string, v *string, def decimal.Decimal) decimal.Decimal {
	if v == nil {
		return def
	}
	d, err := decimal.Parse(strings.TrimSpace(*v))
	if err != nil || !d.Fits(1, 6) || d.Sign() < 0 || d.Cmp(one) > 0 {
		f[field] = "Informe um valor de 0 a 1 com até 6 casas decimais."
		return def
	}
	return d
}

func (s *Service) parseSimulation(in SimulationInput) (SimulationParams, error) {
	errs := fieldErrors{}
	pa, err := competence.Parse(strings.TrimSpace(in.PA))
	if err != nil {
		errs["pa"] = "Informe o PA no formato AAAA-MM."
	}
	p := defaultParams(pa, nil)
	if in.Meta != nil {
		d, err := decimal.Parse(strings.TrimSpace(*in.Meta))
		if err != nil || !d.Fits(1, 6) || !metaInRange(d) {
			errs["meta"] = "A meta vai de 0.28 a menos de 1, com até 6 casas decimais."
		} else {
			p.Meta = &d
		}
	}
	p.Fraction = errs.rate("fracao_pro_labore", in.FracaoProLabore, defaultFraction)
	p.INSS = errs.rate("inss", in.INSS, defaultINSS)
	p.IRRF = errs.rate("irrf", in.IRRF, defaultIRRF)
	if in.HorizonteMeses != nil {
		if *in.HorizonteMeses < 1 || *in.HorizonteMeses > 60 {
			errs["horizonte_meses"] = "O horizonte vai de 1 a 60 meses."
		} else {
			p.Horizon = *in.HorizonteMeses
		}
	}
	if len(errs) > 0 {
		return p, apperr.New(400, "VALIDATION_ERROR", "Dados inválidos. Verifique os campos destacados.").
			WithDetails(map[string]any{"fields": map[string]any(errs)})
	}
	return p, nil
}

func companyName(ctx context.Context, q *store.Queries, id string) (string, error) {
	co, err := q.GetCompany(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", apperr.NotFound("COMPANY_NOT_FOUND", "Empresa não encontrada.")
	}
	return co.LegalName, err
}

// Simulate simula a correção da folha no PA numa corrida do consultor (gatilho ficha, sem
// LLM) e grava a simulação no trace (simular_empresa da origem).
func (s *Service) Simulate(ctx context.Context, companyID string, in SimulationInput) (Simulation, error) {
	p, err := s.parseSimulation(in)
	if err != nil {
		return Simulation{}, err
	}
	name, err := companyName(ctx, store.New(s.pool), companyID)
	if err != nil {
		return Simulation{}, err
	}
	uid := user.FromContext(ctx)
	input := map[string]any{
		"intencao": IntentSimulate, "pa": p.PA.String(), "meta": p.Meta, "fracao_pro_labore": p.Fraction,
		"inss": p.INSS, "irrf": p.IRRF, "horizonte_meses": p.Horizon,
	}
	var sim simulated
	var text string
	traceID, err := s.tracer.Do(ctx, s.pool, tracer.Spec{
		Agent: AgentConsultor, Trigger: TriggerFicha, UserID: &uid, CompanyID: &companyID, Competence: &p.PA,
		Input: input, Metadata: map[string]string{"pa": p.PA.String()},
	}, func(ctx context.Context, tx pgx.Tx, run *tracer.Run) (tracer.Result, error) {
		var err error
		if sim, err = simulateAndRecord(ctx, run, store.New(tx), companyID, p, &uid, s.ops.Now()); err != nil {
			return tracer.Result{}, err
		}
		text = simulationText(name, p.PA, sim.Result)
		if err := run.Span(tracer.SpanRender, nil, func(sp tracer.SpanHandle) error {
			sp.Output(map[string]any{"texto": text})
			return nil
		}); err != nil {
			return tracer.Result{}, err
		}
		decision := map[string]any{"veredito": verdictText(sim.Result.Verdict), "simulation_id": sim.ID}
		return tracer.Result{Status: tracer.StatusOK, Decision: decision, Text: text}, nil
	})
	if err != nil {
		return Simulation{}, err
	}
	paramsJSON, err := jsonObject(sim.Params)
	if err != nil {
		return Simulation{}, err
	}
	resultJSON, err := jsonObject(sim.Result)
	if err != nil {
		return Simulation{}, err
	}
	return toSimulation(sim.ID, traceID, p.PA, verdictText(sim.Result.Verdict), text, paramsJSON, resultJSON, s.ops.Now())
}

func verdictText(v *engine.Verdict) *string {
	if v == nil {
		return nil
	}
	s := string(*v)
	return &s
}

func toSimulation(id, traceID string, pa competence.Competence, verdict *string, text string, params, result []byte, created time.Time) (Simulation, error) {
	p, err := forDisplay(params)
	if err != nil {
		return Simulation{}, err
	}
	r, err := forDisplay(result)
	if err != nil {
		return Simulation{}, err
	}
	return Simulation{SimulationID: id, TraceID: traceID, PA: pa, Veredito: verdict, Texto: text, Parametros: p, Resultado: r, CreatedAt: created}, nil
}

// ListSimulations devolve o histórico da empresa, mais recentes primeiro (limite de 1 a 50).
func (s *Service) ListSimulations(ctx context.Context, companyID string, limit int32) (SimulationList, error) {
	if limit == 0 {
		limit = 10
	}
	if limit < 1 || limit > 50 {
		return SimulationList{}, apperr.New(400, "VALIDATION_ERROR", "Dados inválidos. Verifique os campos destacados.").
			WithDetails(map[string]any{"fields": map[string]any{"limit": "Use um limite entre 1 e 50."}})
	}
	q := store.New(s.pool)
	if _, err := companyName(ctx, q, companyID); err != nil {
		return SimulationList{}, err
	}
	rows, err := q.ListFactorRSimulations(ctx, store.ListFactorRSimulationsParams{CompanyID: companyID, RowLimit: limit})
	if err != nil {
		return SimulationList{}, err
	}
	out := SimulationList{Items: make([]Simulation, 0, len(rows))}
	for _, r := range rows {
		sim, err := toSimulation(r.ID, r.TraceID, competence.FromDate(r.Pa), r.Veredito, r.Texto, r.ParametrosJson, r.ResultadoJson, r.CreatedAt)
		if err != nil {
			return SimulationList{}, err
		}
		out.Items = append(out.Items, sim)
	}
	return out, nil
}

// --- Agentes [REQ-014] ---

// AgentResponse é a resposta dos agentes: texto, decisão estruturada e trace (RespostaAgente).
type AgentResponse struct {
	Texto   string         `json:"texto"`
	Decisao map[string]any `json:"decisao"`
	TraceID string         `json:"trace_id"`
}

// ConsultInput é o corpo de POST /factor-r/agents/consult (ConsultorIn).
type ConsultInput struct {
	Mensagem  string  `json:"mensagem"`
	CompanyID *string `json:"company_id"`
}

// PrioritizeInput é o corpo de POST /factor-r/agents/prioritize (PriorizadorIn).
type PrioritizeInput struct {
	Mensagem string  `json:"mensagem"`
	PA       *string `json:"pa"`
}

func validMessage(m string) error {
	if n := len([]rune(m)); n < 1 || n > 1000 {
		return apperr.New(400, "VALIDATION_ERROR", "Dados inválidos. Verifique os campos destacados.").
			WithDetails(map[string]any{"fields": map[string]any{"mensagem": "Escreva o pedido (até 1000 caracteres)."}})
	}
	return nil
}

func din(v *decimal.Decimal) any {
	if v == nil {
		return nil
	}
	return v.Quantize(-2).String()
}

func pct(v *decimal.Decimal) any {
	if v == nil {
		return nil
	}
	return v.Quantize(-6).String()
}

func optText[T ~string](v *T) any {
	if v == nil {
		return nil
	}
	return string(*v)
}

// engineSummary é o resumo do motor que o consultor explica (resumo_motor).
func engineSummary(name string, r engine.Result) map[string]any {
	missing := make([]string, len(r.MissingMonths))
	for i, m := range r.MissingMonths {
		missing[i] = m.String()
	}
	rbt12, fs12, target := r.RBT12, r.FS12, r.Target
	return map[string]any{
		"empresa": name, "pa": r.PA.String(), "status": string(r.Status), "motivo": optText(r.Reason),
		"fator_r": pct(r.FactorR), "anexo": optText(r.Annex), "semaforo": optText(r.Light),
		"rbt12": din(&rbt12), "fs12": din(&fs12), "gap_12m_28": din(r.Gap28), "reforco_mensal_28": din(r.MonthlyTopUp28),
		"gap_12m_meta": din(r.GapTarget), "reforco_mensal_meta": din(r.MonthlyTopUpTarget),
		"meta_operacional": pct(&target), "economia_12m": din(r.Savings12m), "meses_faltantes": missing,
	}
}

// pyStr formata como a f-string do Python: nulo vira "None".
func pyStr(v any) string {
	if v == nil {
		return "None"
	}
	return fmt.Sprint(v)
}

func textPtr(v any) *string {
	s, ok := v.(string)
	if !ok {
		return nil
	}
	return &s
}

// statusTemplate é o texto padrão da situação (template_status).
func statusTemplate(d map[string]any) string {
	if d["status"] != string(engine.StatusOK) {
		return fmt.Sprintf("%s, PA %s: dados insuficientes para calcular (%s).", pyStr(d["empresa"]), pyStr(d["pa"]), pyStr(d["motivo"]))
	}
	return fmt.Sprintf("%s, PA %s: Fator R %s (Anexo %s, semáforo %s). Reforço mensal até a meta: R$ %s. Economia estimada de DAS em 12 meses: R$ %s.",
		pyStr(d["empresa"]), pyStr(d["pa"]), FormatPercentBR(textPtr(d["fator_r"])), pyStr(d["anexo"]), pyStr(d["semaforo"]),
		FormatBRL(textPtr(d["reforco_mensal_meta"])), FormatBRL(textPtr(d["economia_12m"])))
}

type companyRef struct{ id, name string }

func escapeLike(s string) string { return strings.ReplaceAll(s, `\`, `\\`) }

// resolveCompany acha a empresa do pedido (_empresa): a da ficha; senão a citada, buscada pelo
// nome como a origem (nome exato primeiro).
func resolveCompany(ctx context.Context, q *store.Queries, companyID *string, in Intent) (*companyRef, error) {
	if companyID != nil {
		name, err := companyName(ctx, q, *companyID)
		if err != nil {
			return nil, err
		}
		return &companyRef{id: *companyID, name: name}, nil
	}
	if in.CompanyRef == nil || *in.CompanyRef == "" {
		return nil, nil
	}
	ref := *in.CompanyRef
	digits := strings.Map(func(r rune) rune {
		if unicode.IsDigit(r) {
			return r
		}
		return -1
	}, ref)
	rows, err := q.SearchActiveCompanies(ctx, store.SearchActiveCompaniesParams{Q: escapeLike(strings.TrimSpace(ref)), Digits: digits})
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	pick := rows[0]
	for _, r := range rows {
		if strings.EqualFold(r.LegalName, ref) || (r.TradeName != nil && strings.EqualFold(*r.TradeName, ref)) {
			pick = r
			break
		}
	}
	return &companyRef{id: pick.ID, name: pick.LegalName}, nil
}

func (s *Service) activeNames(ctx context.Context, q *store.Queries) ([]string, error) {
	rows, err := q.ListActiveCompanyNames(ctx)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, 2*len(rows))
	for _, r := range rows {
		names = append(names, r.LegalName)
		if r.TradeName != nil && *r.TradeName != "" {
			names = append(names, *r.TradeName)
		}
	}
	return names, nil
}

func (s *Service) paOrCurrent(pa *string) competence.Competence {
	if pa != nil {
		if c, err := competence.Parse(*pa); err == nil {
			return c
		}
	}
	return competence.Current(s.clock)
}

// Consult responde ao pedido do analista sobre uma empresa ou a carteira (consultor.responder).
func (s *Service) Consult(ctx context.Context, in ConsultInput) (AgentResponse, error) {
	if err := validMessage(in.Mensagem); err != nil {
		return AgentResponse{}, err
	}
	if in.CompanyID != nil {
		if !httpx.IsUUID(*in.CompanyID) {
			return AgentResponse{}, apperr.New(400, "VALIDATION_ERROR", "Dados inválidos. Verifique os campos destacados.").
				WithDetails(map[string]any{"fields": map[string]any{"company_id": "Informe a empresa (id)."}})
		}
		if _, err := companyName(ctx, store.New(s.pool), *in.CompanyID); err != nil {
			return AgentResponse{}, err
		}
	}
	llm, resolved, reason, err := s.llmFor(ctx, ai.Consultor)
	if err != nil {
		return AgentResponse{}, err
	}
	uid := user.FromContext(ctx)
	var resp AgentResponse
	traceID, err := s.tracer.Do(ctx, s.pool, tracer.Spec{
		Agent: AgentConsultor, Trigger: TriggerChat, UserID: &uid, CompanyID: in.CompanyID, Model: resolved,
		PromptVersion: PromptRef(), Metadata: metadata(reason),
		Input: map[string]any{"mensagem": in.Mensagem, "company_id": in.CompanyID},
	}, func(ctx context.Context, tx pgx.Tx, run *tracer.Run) (tracer.Result, error) {
		q := store.New(tx)
		names, err := s.activeNames(ctx, q)
		if err != nil {
			return tracer.Result{}, err
		}
		intent, err := plan(ctx, run, llm, in.Mensagem, names)
		if err != nil {
			return tracer.Result{}, err
		}
		decision, template, err := s.consultDecision(ctx, run, q, in.CompanyID, intent)
		if err != nil {
			return tracer.Result{}, err
		}
		if err := run.RecordDecision(ctx, "consultor_resposta", decision); err != nil {
			return tracer.Result{}, err
		}
		text, err := render(ctx, run, llm, decision, template, true)
		if err != nil {
			return tracer.Result{}, err
		}
		resp = AgentResponse{Texto: text, Decisao: plainMap(decision)}
		status := tracer.StatusOK
		if _, failed := decision["erro"]; failed {
			status = tracer.StatusNeedsReview
		}
		return tracer.Result{Status: status, Decision: decision, Text: text}, nil
	})
	if err != nil {
		return AgentResponse{}, err
	}
	resp.TraceID = traceID
	return resp, nil
}

func (s *Service) consultDecision(ctx context.Context, run *tracer.Run, q *store.Queries, companyID *string, intent Intent) (map[string]any, string, error) {
	if intent.Kind == IntentPrioritize {
		decision, template, err := s.queue(ctx, run, q, intent.PA)
		if err != nil {
			return nil, "", err
		}
		return decision, template, decide(ctx, run, q, decision)
	}
	co, err := resolveCompany(ctx, q, companyID, intent)
	if err != nil {
		return nil, "", err
	}
	pa := s.paOrCurrent(intent.PA)
	switch {
	case co == nil:
		decision := map[string]any{"intencao": intent.Kind, "erro": "empresa_nao_identificada"}
		return decision, "Não identifiquei a empresa. Abra a ficha da empresa ou cite o nome.", decide(ctx, run, q, decision)
	case intent.Kind == IntentSimulate:
		uid := user.FromContext(ctx)
		sim, err := simulateAndRecord(ctx, run, q, co.id, defaultParams(pa, intent.Meta), &uid, s.ops.Now())
		if err != nil {
			return nil, "", err
		}
		r := sim.Result
		decision := map[string]any{
			"intencao": IntentSimulate, "empresa": co.name, "pa": pa.String(), "veredito": verdictText(r.Verdict),
			"simulation_id": sim.ID, "reforco_mensal": r.MonthlyTopUp.Quantize(-2).String(),
			"custo_total": r.TotalCost.Quantize(-2).String(), "liquido": din(r.Net), "horizonte_meses": r.Params.HorizonMonths,
		}
		if err := ensureSimulationForCorrect(ctx, q, decision); err != nil {
			return nil, "", err
		}
		verdict := "dados insuficientes para simular."
		if r.Verdict != nil {
			verdict = verdictTexts[*r.Verdict]
		}
		template := fmt.Sprintf("%s, PA %s: %s Líquido estimado: R$ %s.", co.name, pa, verdict, FormatBRL(textPtr(decision["liquido"])))
		return decision, template, nil
	default:
		var decision map[string]any
		err := run.Span(tracer.SpanTool, map[string]any{"company_id": co.id, "pa": pa.String()}, func(sp tracer.SpanHandle) error {
			calc, err := factorr.CalcWith(ctx, q, co.id, pa)
			if err != nil {
				return err
			}
			decision = engineSummary(co.name, calc.Result)
			decision["intencao"] = intent.Kind
			sp.Output(plain(decision))
			return nil
		})
		if err != nil {
			return nil, "", err
		}
		return decision, statusTemplate(decision), decide(ctx, run, q, decision)
	}
}

// queue monta a fila da semana: empresas no Anexo V ou entre 28% e a meta, por economia de DAS
// (fila_no_run). Nada é alterado na carteira.
func (s *Service) queue(ctx context.Context, run *tracer.Run, q *store.Queries, paText *string) (map[string]any, string, error) {
	pa := s.paOrCurrent(paText)
	var items []map[string]any
	err := run.Span(tracer.SpanTool, map[string]any{"pa": pa.String()}, func(sp tracer.SpanHandle) error {
		port, err := factorr.PortfolioWith(ctx, q, pa, nil)
		if err != nil {
			return err
		}
		var lines []factorr.PortfolioLine
		for _, l := range port.Linhas {
			if l.Semaforo != nil && (*l.Semaforo == engine.LightRed || *l.Semaforo == engine.LightYellow) {
				lines = append(lines, l)
			}
		}
		savings := func(l factorr.PortfolioLine) decimal.Decimal {
			if l.Economia12m == nil {
				return decimal.Zero
			}
			return *l.Economia12m
		}
		sort.SliceStable(lines, func(a, b int) bool { return savings(lines[a]).Cmp(savings(lines[b])) > 0 })
		for i, l := range lines {
			items = append(items, map[string]any{
				"posicao": i + 1, "company_id": l.Company.ID, "empresa": l.Company.LegalName,
				"semaforo": string(*l.Semaforo), "fator_r": pct(l.FatorR), "economia_12m": din(l.Economia12m), "acao": l.AcaoTexto,
			})
		}
		sp.Output(map[string]any{"total_fila": len(items)})
		return nil
	})
	if err != nil {
		return nil, "", err
	}
	fila := make([]any, len(items))
	for i, it := range items {
		fila[i] = it
	}
	decision := map[string]any{"intencao": IntentPrioritize, "pa": pa.String(), "fila": fila}
	if len(items) == 0 {
		return decision, fmt.Sprintf("PA %s: nenhuma empresa no Anexo V ou no limite. Carteira segura.", pa), nil
	}
	var parts []string
	for _, it := range items[:min(queueTextLimit, len(items))] {
		parts = append(parts, fmt.Sprintf("%d. %s (%s, economia R$ %s)", it["posicao"], it["empresa"], it["semaforo"], FormatBRL(textPtr(it["economia_12m"]))))
	}
	return decision, fmt.Sprintf("PA %s: %d empresa(s) na fila. Priorize: %s.", pa, len(items), strings.Join(parts, "; ")), nil
}

// Prioritize responde ao pedido de priorização da carteira (priorizador.responder).
func (s *Service) Prioritize(ctx context.Context, in PrioritizeInput) (AgentResponse, error) {
	if err := validMessage(in.Mensagem); err != nil {
		return AgentResponse{}, err
	}
	if in.PA != nil {
		if _, err := competence.Parse(*in.PA); err != nil {
			return AgentResponse{}, apperr.New(400, "VALIDATION_ERROR", "Dados inválidos. Verifique os campos destacados.").
				WithDetails(map[string]any{"fields": map[string]any{"pa": "Informe o PA no formato AAAA-MM."}})
		}
	}
	llm, resolved, reason, err := s.llmFor(ctx, ai.Priorizador)
	if err != nil {
		return AgentResponse{}, err
	}
	uid := user.FromContext(ctx)
	var resp AgentResponse
	traceID, err := s.tracer.Do(ctx, s.pool, tracer.Spec{
		Agent: AgentPriorizador, Trigger: TriggerChat, UserID: &uid, Model: resolved,
		PromptVersion: PromptRef(), Metadata: metadata(reason),
		Input: map[string]any{"mensagem": in.Mensagem, "pa": in.PA},
	}, func(ctx context.Context, tx pgx.Tx, run *tracer.Run) (tracer.Result, error) {
		q := store.New(tx)
		intent, err := plan(ctx, run, llm, in.Mensagem, nil)
		if err != nil {
			return tracer.Result{}, err
		}
		pa := in.PA
		if pa == nil {
			pa = intent.PA
		}
		decision, template, err := s.queue(ctx, run, q, pa)
		if err != nil {
			return tracer.Result{}, err
		}
		if err := decide(ctx, run, q, decision); err != nil {
			return tracer.Result{}, err
		}
		if err := run.RecordDecision(ctx, "fila_priorizacao", decision); err != nil {
			return tracer.Result{}, err
		}
		text, err := render(ctx, run, llm, decision, template, true)
		if err != nil {
			return tracer.Result{}, err
		}
		resp = AgentResponse{Texto: text, Decisao: plainMap(decision)}
		return tracer.Result{Status: tracer.StatusOK, Decision: decision, Text: text}, nil
	})
	if err != nil {
		return AgentResponse{}, err
	}
	resp.TraceID = traceID
	return resp, nil
}

// RoutineDue diz se a fila semanal do priorizador deve rodar agora: aos domingos, a partir da
// hora configurada (no fuso de now), como o cron da origem (domingo, ROTINA_PRIORIZADOR_HORA).
// A cada semana roda uma vez só (WeeklyRoutine confere).
func RoutineDue(now time.Time, hour int) bool {
	return now.Weekday() == time.Sunday && now.Hour() >= hour
}

// weekStart é a segunda-feira 00:00 da semana de now, no fuso das datas de negócio.
func weekStart(now time.Time) time.Time {
	d := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	offset := (int(d.Weekday()) + 6) % 7 // segunda = 0
	return d.AddDate(0, 0, -offset)
}

// WeeklyRoutine roda o priorizador da semana sem LLM (gatilho rotina), uma vez por semana
// (rotina_semanal). Devolve quantas corridas fez (0 se a semana já tem a dela).
func (s *Service) WeeklyRoutine(ctx context.Context, now time.Time) (int, error) {
	n, err := store.New(s.pool).CountAIRunsSince(ctx, store.CountAIRunsSinceParams{
		Agent: AgentPriorizador, Trigger: TriggerRoutine, Since: weekStart(now),
	})
	if err != nil || n > 0 {
		return 0, err
	}
	_, err = s.tracer.Do(ctx, s.pool, tracer.Spec{
		Agent: AgentPriorizador, Trigger: TriggerRoutine, Input: map[string]any{"gatilho": TriggerRoutine},
		Metadata: map[string]string{"prompts": PromptRef()},
	}, func(ctx context.Context, tx pgx.Tx, run *tracer.Run) (tracer.Result, error) {
		q := store.New(tx)
		if err := run.Span(tracer.SpanPlan, map[string]any{"gatilho": TriggerRoutine}, func(sp tracer.SpanHandle) error {
			sp.Output(map[string]any{"intencao": IntentPrioritize, "llm": false})
			return nil
		}); err != nil {
			return tracer.Result{}, err
		}
		decision, template, err := s.queue(ctx, run, q, nil)
		if err != nil {
			return tracer.Result{}, err
		}
		if err := decide(ctx, run, q, decision); err != nil {
			return tracer.Result{}, err
		}
		if err := run.RecordDecision(ctx, "fila_priorizacao", decision); err != nil {
			return tracer.Result{}, err
		}
		text, err := render(ctx, run, nil, decision, template, false)
		if err != nil {
			return tracer.Result{}, err
		}
		return tracer.Result{Status: tracer.StatusOK, Decision: decision, Text: text}, nil
	})
	if err != nil {
		return 0, err
	}
	return 1, nil
}

func plainMap(decision map[string]any) map[string]any {
	m, _ := plain(decision).(map[string]any)
	return m
}
