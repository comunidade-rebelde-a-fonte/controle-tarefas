// Package proposals é o resolvedor de exceções da Conciliação [REQ-025; TASK-019, D-030]: o
// pedir_hipotese da origem, agora no worker (job agent_resolver), e o aceite e a rejeição humanos
// (aceitar_hip e rejeitar_hip).
//
// O agente só grava a proposta: a hipótese, a justificativa e os pares, com a corrida no tracer.
// Nenhum caminho do agente cria conciliação; ela nasce do aceite humano, com origem manual, o
// método resolvedor (ou sem_comprovante), o autor e a proposta [AC-019].
package proposals

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"controle/internal/ai"
	"controle/internal/ai/litellm"
	"controle/internal/ai/prompts"
	"controle/internal/ai/tracer"
	"controle/internal/jobs"
	"controle/internal/platform/clock"
	"controle/internal/platform/competence"
	"controle/internal/platform/httpx"
	"controle/internal/reconciliation"
	"controle/internal/reconciliation/resolver"
	"controle/internal/store"
)

const (
	agentName      = "resolvedor"
	promptName     = "conciliacao/resolvedor"
	triggerRequest = "pedido"
	triggerRound   = "rodada"
	nearbyDays     = 10
)

// Códigos de falha do job.
const (
	CodeInvalidPayload = "INVALID_PAYLOAD"
	CodeLineNotFound   = "RECONCILIATION_LINE_NOT_FOUND"
	CodeAgentDisabled  = "AGENT_DISABLED"
	CodeInvalidOutput  = "RESOLVER_INVALID_OUTPUT"
)

// Gateway é o cliente do LiteLLM (litellm.Client).
type Gateway interface {
	Chat(ctx context.Context, req litellm.Request) (litellm.Response, error)
}

// Resolver resolve o alias no início da corrida (aliases.Resolver).
type Resolver interface {
	Resolve(ctx context.Context, db store.DBTX, a ai.Alias) (ai.Resolved, error)
}

// Agent executa o resolvedor no worker.
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

// HandleResolve é o handler do job agent_resolver: monta o contexto da linha, pede a hipótese ao
// resolvedor e grava a proposta. A linha conciliada enquanto o pedido esperava, ou com o Contábil
// fechado, não chega ao modelo.
func (a *Agent) HandleResolve(ctx context.Context, tx pgx.Tx, job jobs.Claimed) error {
	var p reconciliation.ResolvePayload
	if err := json.Unmarshal(job.Payload, &p); err != nil || !httpx.IsUUID(p.StatementTransactionID) {
		return jobs.Permanent(CodeInvalidPayload, errors.New("agent_resolver: payload sem a linha do extrato"))
	}
	q := store.New(tx)
	line, err := q.GetStatementLine(ctx, p.StatementTransactionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return jobs.Permanent(CodeLineNotFound, errors.New("linha do extrato inexistente"))
	}
	if err != nil {
		return err
	}
	if line.Reconciled {
		a.logger.Info("resolvedor: a linha já foi conciliada", "linha", line.ID)
		return nil
	}
	comp := competence.FromDate(line.TxnDate)
	if err := reconciliation.RequireOpen(ctx, tx, line.CompanyID, comp); err != nil {
		if reconciliation.IsLocked(err) {
			a.logger.Info("resolvedor: competência travada (Contábil fechado ou conciliação concluída)", "linha", line.ID)
			return nil
		}
		return err
	}
	model, err := a.resolver.Resolve(ctx, tx, ai.Resolvedor)
	if errors.Is(err, ai.ErrAliasDisabled) || errors.Is(err, ai.ErrNotConfigured) {
		return jobs.Permanent(CodeAgentDisabled, fmt.Errorf("alias %s: %w", ai.Resolvedor, err))
	}
	if err != nil {
		return err
	}
	rctx, err := buildContext(ctx, q, line)
	if err != nil {
		return err
	}
	prompt := prompts.Get(promptName)
	trigger := triggerRound
	if p.RequestedBy != nil {
		trigger = triggerRequest
	}
	reason := firstDivergence(line.Flags)
	run, err := a.tracer.Begin(ctx, tx, tracer.Spec{
		Agent: agentName, Trigger: trigger, UserID: p.RequestedBy, CompanyID: &line.CompanyID, Competence: &comp, Model: &model,
		PromptVersion: prompt.Ref(),
		Input: map[string]any{"statement_transaction_id": line.ID, "motivo": reason, "candidatos": rctx.Candidates,
			"proximos": rctx.Nearby, "irmas": rctx.Siblings},
	})
	if err != nil {
		return err
	}
	if err := a.propose(ctx, tx, run, line, rctx, model, prompt, p.RequestedBy); err != nil {
		return run.Fail(err)
	}
	return nil
}

// buildContext carrega os lançamentos e as linhas vizinhas da empresa e monta o contexto.
func buildContext(ctx context.Context, q *store.Queries, line store.GetStatementLineRow) (resolver.Context, error) {
	rows, err := q.ListClientEntriesForMatching(ctx, line.CompanyID)
	if err != nil {
		return resolver.Context{}, err
	}
	near, err := q.ListStatementLinesNear(ctx, store.ListStatementLinesNearParams{
		CompanyID: line.CompanyID, DateFrom: line.TxnDate.AddDate(0, 0, -nearbyDays), DateTo: line.TxnDate.AddDate(0, 0, nearbyDays),
	})
	if err != nil {
		return resolver.Context{}, err
	}
	in := resolver.ContextInput{Line: resolver.Line{ID: line.ID, Date: line.TxnDate, Amount: line.Amount, Direction: line.Direction,
		Description: line.Description, Flags: line.Flags}}
	for _, r := range rows {
		in.Entries = append(in.Entries, resolver.EntryInfo{ID: r.ID, Date: r.EntryDate, Amount: r.Amount, Direction: r.Direction,
			Counterparty: r.Counterparty, DocumentRef: r.DocumentRef, OperationType: r.OperationType, Flags: stringFlags(r.Flags),
			Status: r.Status, Free: r.Free})
	}
	for _, s := range near {
		in.Lines = append(in.Lines, resolver.Line{ID: s.ID, Date: s.TxnDate, Amount: s.Amount, Direction: s.Direction,
			Description: s.Description, Reconciled: s.Reconciled})
	}
	return resolver.BuildContext(in), nil
}

func stringFlags(raw []byte) []string {
	var list []any
	if json.Unmarshal(raw, &list) != nil {
		return nil
	}
	var out []string
	for _, x := range list {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// firstDivergence é o motivo da primeira divergência marcada na linha (o _divergencia da origem).
func firstDivergence(flags []byte) any {
	var list []map[string]any
	if json.Unmarshal(flags, &list) != nil {
		return nil
	}
	for _, m := range list {
		if m["tipo"] == "divergencia" {
			return m["motivo"]
		}
	}
	return nil
}

// propose chama o resolvedor, valida a hipótese e grava a proposta pendente com a decisão.
func (a *Agent) propose(ctx context.Context, tx pgx.Tx, run *tracer.Run, line store.GetStatementLineRow, rctx resolver.Context,
	model ai.Resolved, prompt prompts.Prompt, requestedBy *string) error {
	var h resolver.Hypothesis
	err := run.Span(tracer.SpanPlan, map[string]any{"alias": string(model.Alias)}, func(s tracer.SpanHandle) error {
		s.Meta("prompt_version", prompt.Ref())
		resp, unusable, err := a.call(ctx, run, model, prompt.Text, rctx.Payload)
		if err != nil {
			return err
		}
		perr := resolver.ErrInvalidOutput
		if !unusable {
			h, perr = resolver.Parse(resp.Content)
		}
		if perr != nil {
			// Como na origem, a saída inválida não vira proposta. Sem nova tentativa automática: o
			// contador pede de novo se quiser (cada chamada tem custo).
			return jobs.Permanent(CodeInvalidOutput, fmt.Errorf("saída do resolvedor fora do schema: %w", perr))
		}
		s.Output(map[string]any{"pares": len(h.Pairs), "confianca": h.Confidence.String(), "pede_humano": h.AsksHuman})
		return nil
	})
	if err != nil {
		return err
	}
	// Ids que o modelo cita fora do contexto: o aceite usa só linhas e lançamentos livres da
	// empresa; aqui eles ficam registrados na decisão (os que nem são UUID, só contados).
	unknown, malformed := resolver.OutsideContext(h, rctx.Offered)
	q := store.New(tx)
	var decision map[string]any
	err = run.Span(tracer.SpanDecide, nil, func(s tracer.SpanHandle) error {
		trace := run.TraceID()
		id, err := q.InsertResolverProposal(ctx, store.InsertResolverProposalParams{
			CompanyID: line.CompanyID, StatementTransactionID: line.ID, Payload: h.Payload(), TraceID: &trace, CreatedBy: requestedBy,
			Now: a.clock.Now(),
		})
		if err != nil {
			return fmt.Errorf("gravar a proposta do resolvedor: %w", err)
		}
		decision = map[string]any{"proposta_id": id, "statement_transaction_id": line.ID, "status": "pendente", "pares": len(h.Pairs),
			"confianca": h.Confidence.String(), "pede_humano": h.AsksHuman, "ids_fora_do_contexto": unknown, "ids_invalidos": malformed}
		if len(unknown) > 0 || malformed > 0 {
			s.Warn("ids_fora_do_contexto")
		}
		s.Output(decision)
		return run.RecordDecision(ctx, "proposta_resolvedor", decision)
	})
	if err != nil {
		return err
	}
	text := "Hipótese registrada como proposta: o contador aceita ou rejeita na fila; nada foi conciliado."
	if len(unknown) > 0 || malformed > 0 {
		text += " A hipótese cita ids que não estavam no contexto; o aceite os ignora se não forem linhas ou lançamentos livres da empresa."
	}
	if err := run.Span(tracer.SpanRender, nil, func(s tracer.SpanHandle) error {
		s.Output(map[string]any{"texto": text})
		return nil
	}); err != nil {
		return err
	}
	conf := h.Confidence
	return run.Finish(ctx, tracer.Result{Status: tracer.StatusNeedsReview, Decision: decision, Text: text, Confidence: &conf})
}

// call chama o gateway com a saída estruturada e registra a geração na corrida. Resposta sem
// conteúdo útil conta como saída inválida; falha do gateway é falha transitória do job.
func (a *Agent) call(ctx context.Context, run *tracer.Run, model ai.Resolved, system, user string) (litellm.Response, bool, error) {
	req := litellm.Request{
		Model:       model.Model,
		Messages:    []litellm.Message{{Role: "system", Content: system}, {Role: "user", Content: user}},
		Temperature: model.Parameters.Temperature,
		JSONSchema:  &litellm.JSONSchema{Name: "hipotese_resolvedor", Schema: hypothesisSchema},
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
	gen := tracer.Generation{Name: "propor_hipotese", Model: model.Model, Err: err,
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

// hypothesisSchema é o schema estrito da saída do resolvedor (HipoteseResolvedor).
var hypothesisSchema = func() json.RawMessage {
	pair := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"extrato_transacao_id":   map[string]any{"type": []string{"string", "null"}},
			"lancamento_cliente_ids": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"tipo":                   map[string]any{"type": "string", "enum": resolver.Kinds},
			"confianca":              map[string]any{"type": "number"},
		},
		"required":             []string{"extrato_transacao_id", "lancamento_cliente_ids", "tipo", "confianca"},
		"additionalProperties": false,
	}
	b, err := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"hipotese":        map[string]any{"type": "string"},
			"justificativa":   map[string]any{"type": "string"},
			"pares_propostos": map[string]any{"type": "array", "items": pair},
			"confianca":       map[string]any{"type": "number"},
			"pede_humano":     map[string]any{"type": "boolean"},
		},
		"required":             []string{"hipotese", "justificativa", "pares_propostos", "confianca", "pede_humano"},
		"additionalProperties": false,
	})
	if err != nil {
		panic(err)
	}
	return b
}()
