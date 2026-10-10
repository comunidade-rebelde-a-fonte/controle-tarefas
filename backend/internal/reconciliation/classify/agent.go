package classify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"controle/internal/ai"
	"controle/internal/ai/litellm"
	"controle/internal/ai/prompts"
	"controle/internal/ai/tracer"
	"controle/internal/jobs"
	"controle/internal/platform/clock"
	"controle/internal/platform/competence"
	"controle/internal/platform/decimal"
	"controle/internal/platform/httpx"
	"controle/internal/reconciliation"
	"controle/internal/reconciliation/classification"
	"controle/internal/reconciliation/matching"
	"controle/internal/store"
)

const (
	agentName      = "classificador"
	promptName     = "conciliacao/classificador"
	triggerRequest = "pedido"
	triggerRound   = "rodada"
	// Histórico parecido que vai ao agente: as chaves com Jaro-Winkler a partir de 0,80, até 10.
	similarMin = 0.80
	similarMax = 10
)

// Códigos de falha do job.
const (
	CodeInvalidPayload = "INVALID_PAYLOAD"
	CodeLineNotFound   = "RECONCILIATION_LINE_NOT_FOUND"
	CodeAgentDisabled  = "AGENT_DISABLED"
	CodeChartEmpty     = "CHART_EMPTY"
	CodeInvalidOutput  = "CLASSIFIER_INVALID_OUTPUT"
)

// Gateway é o cliente do LiteLLM (litellm.Client).
type Gateway interface {
	Chat(ctx context.Context, req litellm.Request) (litellm.Response, error)
}

// Resolver resolve o alias no início da corrida (aliases.Resolver).
type Resolver interface {
	Resolve(ctx context.Context, db store.DBTX, a ai.Alias) (ai.Resolved, error)
}

// Agent executa o classificador no worker: só para a linha que a regra e o histórico não
// classificaram, e só grava a sugestão.
type Agent struct {
	clock    clock.Clock
	tracer   *tracer.Tracer
	gateway  Gateway
	resolver Resolver
	logger   *slog.Logger
}

// NewAgent cria o agente.
func NewAgent(clk clock.Clock, tr *tracer.Tracer, gateway Gateway, resolver Resolver, logger *slog.Logger) *Agent {
	return &Agent{clock: clk, tracer: tr, gateway: gateway, resolver: resolver, logger: logger}
}

// HandleSuggest é o handler do job agent_classifier. Antes do modelo, roda de novo a regra e o
// histórico (uma regra pode ter sido confirmada enquanto o pedido esperava): classificando, o
// agente não é chamado. Linha desconciliada, já confirmada ou com o Contábil fechado também não
// chega ao modelo.
func (a *Agent) HandleSuggest(ctx context.Context, tx pgx.Tx, job jobs.Claimed) error {
	var p reconciliation.ClassifyPayload
	if err := json.Unmarshal(job.Payload, &p); err != nil || !httpx.IsUUID(p.StatementTransactionID) {
		return jobs.Permanent(CodeInvalidPayload, errors.New("agent_classifier: payload sem a linha do extrato"))
	}
	q := store.New(tx)
	line, err := q.GetLineClassification(ctx, p.StatementTransactionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return jobs.Permanent(CodeLineNotFound, errors.New("linha do extrato inexistente"))
	}
	if err != nil {
		return err
	}
	if !line.Reconciled || confirmed(line) {
		a.logger.Info("classificador: a linha não espera classificação", "linha", line.ID)
		return nil
	}
	comp := competence.FromDate(line.TxnDate)
	if err := reconciliation.RequireOpen(ctx, tx, line.CompanyID, comp); err != nil {
		if reconciliation.IsPeriodClosed(err) {
			a.logger.Info("classificador: Contábil fechado na competência da linha", "linha", line.ID)
			return nil
		}
		return err
	}
	applied, err := Lines(ctx, tx, line.CompanyID, []string{line.ID}, a.clock.Now())
	if err != nil {
		return err
	}
	if o := applied[line.ID]; o == classification.OriginRule || o == classification.OriginHistory {
		a.logger.Info("classificador: a regra ou o histórico classificaram a linha", "linha", line.ID, "origem", o)
		return nil
	}
	chart, err := q.ListActiveChartForAgent(ctx, line.CompanyID)
	if err != nil {
		return err
	}
	if len(chart) == 0 {
		return jobs.Permanent(CodeChartEmpty, errors.New("a empresa não tem plano de contas"))
	}
	model, err := a.resolver.Resolve(ctx, tx, ai.Classificador)
	if errors.Is(err, ai.ErrAliasDisabled) || errors.Is(err, ai.ErrNotConfigured) {
		return jobs.Permanent(CodeAgentDisabled, fmt.Errorf("alias %s: %w", ai.Classificador, err))
	}
	if err != nil {
		return err
	}
	in, err := lineInput(ctx, q, line)
	if err != nil {
		return err
	}
	m, err := load(ctx, q, line.CompanyID, comp)
	if err != nil {
		return err
	}
	rctx := buildContext(line, in, chart, m.history)
	prompt := prompts.Get(promptName)
	trigger := triggerRound
	if p.RequestedBy != nil {
		trigger = triggerRequest
	}
	run, err := a.tracer.Begin(ctx, tx, tracer.Spec{
		Agent: agentName, Trigger: trigger, UserID: p.RequestedBy, CompanyID: &line.CompanyID, Competence: &comp, Model: &model,
		PromptVersion: prompt.Ref(),
		Input:         map[string]any{"statement_transaction_id": line.ID, "contas_no_plano": len(chart), "historico_parecido": rctx.similar},
	})
	if err != nil {
		return err
	}
	if err := a.suggest(ctx, tx, run, line, rctx, model, prompt, p.RequestedBy); err != nil {
		return run.Fail(err)
	}
	return nil
}

// agentContext é o que o modelo recebe e o que a corrida registra.
type agentContext struct {
	payload string
	chart   map[string]bool
	similar int
}

type entryOut struct {
	ID           string          `json:"id"`
	Date         string          `json:"data"`
	Amount       decimal.Decimal `json:"valor"`
	Direction    string          `json:"sentido"`
	Description  string          `json:"descritivo"`
	Counterparty *string         `json:"contraparte"`
	CNPJ         *string         `json:"cnpj_contraparte"`
}

type accountOut struct {
	Code string `json:"codigo"`
	Name string `json:"nome"`
}

type historyOut struct {
	Key         string `json:"contraparte_ou_descritivo"`
	AccountCode string `json:"conta_codigo"`
	AccountName string `json:"conta_nome"`
}

type payloadOut struct {
	Entry   entryOut     `json:"lancamento"`
	Chart   []accountOut `json:"plano_contas"`
	Missed  string       `json:"regra_e_historico"`
	Similar []historyOut `json:"historico_classificacao"`
	Rules   string       `json:"regras"`
}

const (
	missedText = "Nenhuma regra ativa casou e não há histórico da mesma contraparte ou descritivo nos meses anteriores."
	rulesText  = "Use somente códigos do plano_contas. Sem conta que caiba, conta_codigo nulo e confiança baixa. " +
		"Você não classifica: o contador confirma. regra_sugerida é só proposta."
)

// buildContext monta a mensagem: o lançamento (CNPJ só de empresa, nunca CPF), o plano ativo e o
// histórico de chaves parecidas (historico_classificacao), sem a mesma chave.
func buildContext(line store.GetLineClassificationRow, in classification.Input, chart []store.ListActiveChartForAgentRow,
	history []classification.HistoryItem) agentContext {
	e := entryOut{ID: line.ID, Date: line.TxnDate.Format(time.DateOnly), Amount: line.Amount, Direction: line.Direction,
		Description: line.Description, Counterparty: in.Counterparty}
	if in.CNPJ != nil && len(*in.CNPJ) == 14 {
		e.CNPJ = in.CNPJ
	}
	out := payloadOut{Entry: e, Chart: []accountOut{}, Missed: missedText, Similar: []historyOut{}, Rules: rulesText}
	names := map[string]string{}
	offered := map[string]bool{}
	for _, c := range chart {
		out.Chart = append(out.Chart, accountOut{Code: c.Code, Name: c.Name})
		names[c.Code], offered[c.Code] = c.Name, true
	}
	key := classification.HistoryKey(in)
	type scored struct {
		item  classification.HistoryItem
		score float64
	}
	var list []scored
	seen := map[string]bool{}
	for _, h := range history { // da mais recente para a mais antiga: vale a primeira de cada chave
		if seen[h.Key] || h.Key == "" || h.Key == key {
			continue
		}
		seen[h.Key] = true
		if s := matching.JaroWinkler(key, h.Key); s >= similarMin {
			list = append(list, scored{h, s})
		}
	}
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].score != list[j].score {
			return list[i].score > list[j].score
		}
		return list[i].item.Key < list[j].item.Key
	})
	for i, s := range list {
		if i == similarMax {
			break
		}
		out.Similar = append(out.Similar, historyOut{Key: s.item.Key, AccountCode: s.item.AccountCode, AccountName: names[s.item.AccountCode]})
	}
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		panic(err) // só tipos do pacote
	}
	return agentContext{payload: string(bytes.TrimRight(b.Bytes(), "\n")), chart: offered, similar: len(out.Similar)}
}

// suggest chama o classificador, valida a sugestão e a grava pendente, com a decisão.
func (a *Agent) suggest(ctx context.Context, tx pgx.Tx, run *tracer.Run, line store.GetLineClassificationRow, rctx agentContext,
	model ai.Resolved, prompt prompts.Prompt, requestedBy *string) error {
	var sg classification.Suggestion
	err := run.Span(tracer.SpanPlan, map[string]any{"alias": string(model.Alias)}, func(s tracer.SpanHandle) error {
		s.Meta("prompt_version", prompt.Ref())
		resp, unusable, err := a.call(ctx, run, model, prompt.Text, rctx.payload)
		if err != nil {
			return err
		}
		perr := classification.ErrInvalidOutput
		if !unusable {
			sg, perr = classification.ParseSuggestion(resp.Content)
		}
		if perr != nil {
			// Saída inválida não vira sugestão, sem nova tentativa automática (cada chamada tem
			// custo): o contador pede de novo ou classifica à mão.
			return jobs.Permanent(CodeInvalidOutput, fmt.Errorf("saída do classificador fora do schema: %w", perr))
		}
		s.Output(map[string]any{"com_conta": sg.AccountCode != nil, "confianca": sg.Confidence.String()})
		return nil
	})
	if err != nil {
		return err
	}
	// A conta precisa estar no plano enviado: fora dele, a sugestão fica sem conta (nunca inventada).
	var account *string
	inChart := sg.AccountCode != nil && rctx.chart[*sg.AccountCode]
	if inChart {
		account = sg.AccountCode
	}
	var ruleType any
	if t, ok := sg.SuggestedRule["tipo"].(string); ok {
		ruleType = t
	}
	q := store.New(tx)
	var decision map[string]any
	err = run.Span(tracer.SpanDecide, nil, func(s tracer.SpanHandle) error {
		trace := run.TraceID()
		id, err := q.InsertClassificationSuggestion(ctx, store.InsertClassificationSuggestionParams{
			CompanyID: line.CompanyID, StatementTransactionID: line.ID, Payload: sg.Payload(), AccountCode: account, TraceID: &trace,
			CreatedBy: requestedBy, Now: a.clock.Now(),
		})
		if err != nil {
			return fmt.Errorf("gravar a sugestão do classificador: %w", err)
		}
		decision = map[string]any{"sugestao_id": id, "statement_transaction_id": line.ID, "status": SuggestionPending, "conta": account,
			"conta_no_plano": inChart, "conta_proposta": sg.AccountCode != nil, "confianca": sg.Confidence.String(), "regra_sugerida_tipo": ruleType}
		if sg.AccountCode != nil && !inChart {
			s.Warn("conta_fora_do_plano")
		}
		s.Output(decision)
		return run.RecordDecision(ctx, "sugestao_classificacao", decision)
	})
	if err != nil {
		return err
	}
	text := "Sugestão de conta registrada: o contador confirma ou rejeita na fila; a classificação só muda com a confirmação."
	if sg.AccountCode != nil && !inChart {
		text += " A conta proposta não está no plano ativo da empresa e foi descartada."
	}
	if err := run.Span(tracer.SpanRender, nil, func(s tracer.SpanHandle) error {
		s.Output(map[string]any{"texto": text})
		return nil
	}); err != nil {
		return err
	}
	conf := sg.Confidence
	return run.Finish(ctx, tracer.Result{Status: tracer.StatusNeedsReview, Decision: decision, Text: text, Confidence: &conf})
}

// call chama o gateway com a saída estruturada e registra a geração na corrida. Resposta sem
// conteúdo útil conta como saída inválida; falha do gateway é falha transitória do job.
func (a *Agent) call(ctx context.Context, run *tracer.Run, model ai.Resolved, system, user string) (litellm.Response, bool, error) {
	req := litellm.Request{
		Model:       model.Model,
		Messages:    []litellm.Message{{Role: "system", Content: system}, {Role: "user", Content: user}},
		Temperature: model.Parameters.Temperature,
		JSONSchema:  &litellm.JSONSchema{Name: "sugestao_classificacao", Schema: suggestionSchema},
		Metadata:    map[string]string{"alias": string(model.Alias), "trace_id": run.TraceID()},
	}
	if model.Parameters.MaxTokens != nil {
		req.MaxTokens = *model.Parameters.MaxTokens
	}
	if a.gateway == nil {
		return litellm.Response{}, false, jobs.Coded(tracer.CodeLLM, litellm.ErrNotConfigured)
	}
	resp, err := a.gateway.Chat(ctx, req)
	var gw *litellm.Error
	unusable := errors.As(err, &gw) && gw.Status == http.StatusOK
	gen := tracer.Generation{Name: "sugerir_conta", Model: model.Model, Err: err,
		Input: map[string]any{"caracteres": utf8.RuneCountInString(user)}}
	if err == nil {
		gen.Response = &resp
		gen.Output = map[string]any{"caracteres": len(resp.Content)}
	}
	if gerr := run.Generation(gen); gerr != nil {
		return litellm.Response{}, false, gerr
	}
	switch {
	case unusable:
		return litellm.Response{}, true, nil
	case err != nil:
		return litellm.Response{}, false, jobs.Coded(tracer.CodeLLM, err)
	}
	return resp, false, nil
}

// suggestionSchema é o schema estrito da saída do classificador (SugestaoClassificacao).
var suggestionSchema = func() json.RawMessage {
	rule := map[string]any{
		"anyOf": []any{
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"tipo":   map[string]any{"type": []string{"string", "null"}, "enum": []any{classification.RuleCNPJ, classification.RuleCounterparty, classification.RuleRegex, nil}},
					"padrao": map[string]any{"type": []string{"string", "null"}},
				},
				"required":             []string{"tipo", "padrao"},
				"additionalProperties": false,
			},
			map[string]any{"type": "null"},
		},
	}
	b, err := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"conta_codigo":   map[string]any{"type": []string{"string", "null"}},
			"conta_nome":     map[string]any{"type": []string{"string", "null"}},
			"justificativa":  map[string]any{"type": "string"},
			"regra_sugerida": rule,
			"confianca":      map[string]any{"type": "number"},
		},
		"required":             []string{"conta_codigo", "conta_nome", "justificativa", "regra_sugerida", "confianca"},
		"additionalProperties": false,
	})
	if err != nil {
		panic(err)
	}
	return b
}()
