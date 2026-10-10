package classify

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"controle/internal/jobs"
	"controle/internal/platform/apperr"
	"controle/internal/platform/clock"
	"controle/internal/platform/competence"
	"controle/internal/platform/httpx"
	"controle/internal/reconciliation"
	"controle/internal/reconciliation/classification"
	"controle/internal/store"
	"controle/internal/user"
)

// Estados da sugestão do agente.
const (
	SuggestionPending  = "pendente"
	SuggestionAccepted = "aceita"
	SuggestionRejected = "rejeitada"
)

// Service atende a classificação pela API: rodar a competência, corrigir a linha e pedir, aceitar
// ou rejeitar a sugestão do agente. Toda ação humana fica no log de extração e passa pela trava do
// Contábil (AC-025).
type Service struct {
	pool  *pgxpool.Pool
	clock clock.Clock
	queue *jobs.Queue
}

// NewService cria o serviço.
func NewService(pool *pgxpool.Pool, clk clock.Clock, queue *jobs.Queue) *Service {
	return &Service{pool: pool, clock: clk, queue: queue}
}

// Classification é a classificação da linha e o que a produziu.
type Classification struct {
	StatementTransactionID string     `json:"statement_transaction_id"`
	AccountCode            *string    `json:"account_code"`
	Origin                 *string    `json:"origin"`
	Pending                bool       `json:"pending"`
	RuleID                 *string    `json:"rule_id"`
	HistoryLineID          *string    `json:"history_line_id"`
	SuggestionID           *string    `json:"suggestion_id"`
	ClassifiedBy           *string    `json:"classified_by"`
	ClassifiedAt           *time.Time `json:"classified_at"`
}

func toClassification(l store.GetLineClassificationRow) Classification {
	return Classification{StatementTransactionID: l.ID, AccountCode: l.AccountCode, Origin: l.ClassificationOrigin,
		Pending: l.ClassificationPending, RuleID: l.ClassificationRuleID, HistoryLineID: l.ClassificationHistoryID,
		SuggestionID: l.ClassificationSuggestionID, ClassifiedBy: l.ClassifiedBy, ClassifiedAt: l.ClassifiedAt}
}

// RuleProposal é a regra que o sistema propõe ao contador depois da correção ou do aceite (M5.5):
// só vale quando ele a confirma em POST /companies/{id}/classification-rules.
type RuleProposal struct {
	RuleType     string `json:"rule_type"`
	Pattern      string `json:"pattern"`
	AccountCode  string `json:"account_code"`
	SourceLineID string `json:"source_statement_transaction_id"`
}

func companyNotFound() error { return apperr.NotFound("COMPANY_NOT_FOUND", "Empresa não encontrada.") }

func lineNotFound() error {
	return apperr.NotFound("RECONCILIATION_LINE_NOT_FOUND", "Linha do extrato não encontrada.")
}

func validation(fields map[string]any) error {
	return apperr.BadRequest("VALIDATION_ERROR", "Dados inválidos. Verifique os campos destacados.").WithDetails(map[string]any{"fields": fields})
}

// reconciledLine lê a linha (404) e exige a conciliação ativa (409): só o conciliado é classificado.
func reconciledLine(ctx context.Context, q *store.Queries, id string) (store.GetLineClassificationRow, error) {
	if !httpx.IsUUID(id) {
		return store.GetLineClassificationRow{}, lineNotFound()
	}
	l, err := q.GetLineClassification(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return l, lineNotFound()
	}
	if err != nil {
		return l, err
	}
	if !l.Reconciled {
		return l, apperr.Conflict("LINE_NOT_RECONCILED", "A linha não está conciliada: só o conciliado é classificado.")
	}
	return l, nil
}

func confirmed(l store.GetLineClassificationRow) bool {
	return l.ClassificationOrigin != nil && !l.ClassificationPending
}

// audit registra a ação humana no log de extração (o _auditar da origem).
func (s *Service) audit(ctx context.Context, q *store.Queries, companyID string, change map[string]any) error {
	uid := user.FromContext(ctx)
	b, err := json.Marshal(change)
	if err != nil {
		return err
	}
	text := string(b)
	_, err = q.InsertExtractionLog(ctx, store.InsertExtractionLogParams{
		CompanyID: &companyID, Alias: "humano", Attempt: 0, ActorID: &uid, Change: &text, Now: s.clock.Now(),
	})
	return err
}

// Run classifica a competência (regra e histórico) e devolve as contagens. Serve para aplicar uma
// regra recém-confirmada às linhas que ainda esperam classificação.
func (s *Service) Run(ctx context.Context, companyID string, comp competence.Competence) (Summary, error) {
	if !httpx.IsUUID(companyID) {
		return Summary{}, companyNotFound()
	}
	var out Summary
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		if _, err := q.GetCompanyIdentity(ctx, companyID); errors.Is(err, pgx.ErrNoRows) {
			return companyNotFound()
		} else if err != nil {
			return err
		}
		if err := q.LockReconciliationCompany(ctx, companyID); err != nil {
			return err
		}
		if err := reconciliation.RequireOpen(ctx, tx, companyID, comp); err != nil {
			return err
		}
		var err error
		out, err = Competence(ctx, tx, companyID, comp, s.clock.Now())
		return err
	})
	return out, err
}

// lineInput é a entrada da classificação da linha (os comprovantes da conciliação ativa).
func lineInput(ctx context.Context, q *store.Queries, l store.GetLineClassificationRow) (classification.Input, error) {
	rows, err := q.ListClassificationLines(ctx, store.ListClassificationLinesParams{
		CompanyID: l.CompanyID, DateFrom: sinceForever, DateTo: farFuture, LineID: &l.ID,
	})
	if err != nil {
		return classification.Input{}, err
	}
	if len(rows) == 0 {
		return inputOf(l.Description, nil, nil), nil
	}
	return inputOf(rows[0].Description, rows[0].Counterparties, rows[0].Docs), nil
}

func proposal(r classification.Rule, ok bool, lineID string) *RuleProposal {
	if !ok {
		return nil
	}
	return &RuleProposal{RuleType: r.Type, Pattern: r.Pattern, AccountCode: r.AccountCode, SourceLineID: lineID}
}

// derived é a regra derivada da classificação da linha (regra_derivada_de_correcao).
func derived(in classification.Input, accountCode, lineID string) *RuleProposal {
	r, ok := classification.DerivedRule(in, accountCode)
	return proposal(r, ok, lineID)
}

// CorrectResult é a correção: a classificação gravada e a regra derivada proposta (nula quando não
// há padrão).
type CorrectResult struct {
	Classification Classification `json:"classification"`
	ProposedRule   *RuleProposal  `json:"proposed_rule"`
}

// Correct grava a classificação do contador (origem manual, com o autor) e propõe a regra derivada
// da correção (M5.5), que só vale depois de confirmada.
func (s *Service) Correct(ctx context.Context, lineID, accountCode string) (CorrectResult, error) {
	var out CorrectResult
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		l, err := reconciledLine(ctx, q, lineID)
		if err != nil {
			return err
		}
		if err := q.LockReconciliationCompany(ctx, l.CompanyID); err != nil {
			return err
		}
		if err := reconciliation.RequireOpen(ctx, tx, l.CompanyID, competence.FromDate(l.TxnDate)); err != nil {
			return err
		}
		active, err := q.AccountIsActive(ctx, store.AccountIsActiveParams{CompanyID: l.CompanyID, Code: accountCode})
		if err != nil {
			return err
		}
		if !active {
			return validation(map[string]any{"account_code": "Conta fora do plano ativo da empresa."})
		}
		uid := user.FromContext(ctx)
		now := s.clock.Now()
		origin := classification.OriginManual
		if err := q.SetLineClassification(ctx, store.SetLineClassificationParams{
			AccountCode: &accountCode, Origin: &origin, ClassifiedBy: &uid, ClassifiedAt: &now, ID: l.ID, CompanyID: l.CompanyID,
		}); err != nil {
			return err
		}
		in, err := lineInput(ctx, q, l)
		if err != nil {
			return err
		}
		out.ProposedRule = derived(in, accountCode, l.ID)
		if l, err = q.GetLineClassification(ctx, l.ID); err != nil {
			return err
		}
		out.Classification = toClassification(l)
		return s.audit(ctx, q, l.CompanyID, map[string]any{"acao": "classificar", "linha": l.ID, "conta": accountCode})
	})
	return out, err
}

// RequestResult é o pedido de sugestão: o job e se ele já estava na fila.
type RequestResult struct {
	JobID         string `json:"job_id"`
	Status        string `json:"status"`
	AlreadyQueued bool   `json:"already_queued"`
}

// RequestSuggestion pede ao agente uma conta para a linha (job agent_classifier). Só para a linha
// sem regra nem histórico, ainda não confirmada: o agente nunca passa na frente deles.
func (s *Service) RequestSuggestion(ctx context.Context, lineID string) (RequestResult, error) {
	var out RequestResult
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		l, err := reconciledLine(ctx, q, lineID)
		if err != nil {
			return err
		}
		if confirmed(l) {
			return apperr.Conflict("LINE_ALREADY_CLASSIFIED",
				"A linha já está classificada (regra, histórico ou contador): corrija a conta em vez de pedir sugestão.")
		}
		if err := reconciliation.RequireOpen(ctx, tx, l.CompanyID, competence.FromDate(l.TxnDate)); err != nil {
			return err
		}
		if n, err := q.CountActiveChartAccounts(ctx, l.CompanyID); err != nil {
			return err
		} else if n == 0 {
			return apperr.Conflict("CHART_EMPTY", "A empresa não tem plano de contas: cadastre, importe ou copie o plano antes.")
		}
		uid := user.FromContext(ctx)
		job, created, err := reconciliation.EnqueueClassify(ctx, s.queue, tx, l.ID, &uid)
		if err != nil {
			return err
		}
		out = RequestResult{JobID: job.ID, Status: job.Status, AlreadyQueued: !created}
		if !created {
			return nil
		}
		return s.audit(ctx, q, l.CompanyID, map[string]any{"acao": "pedir_sugestao_conta", "linha": l.ID, "job": job.ID})
	})
	return out, err
}

// Suggestion é a sugestão do agente.
type Suggestion struct {
	ID                     string          `json:"id"`
	CompanyID              string          `json:"company_id"`
	StatementTransactionID string          `json:"statement_transaction_id"`
	Status                 string          `json:"status"`
	AccountCode            *string         `json:"account_code"`
	Suggestion             json.RawMessage `json:"suggestion"`
	TraceID                *string         `json:"trace_id"`
	CreatedBy              *string         `json:"created_by"`
	CreatedAt              time.Time       `json:"created_at"`
	DecidedBy              *string         `json:"decided_by"`
	DecidedAt              *time.Time      `json:"decided_at"`
}

func toSuggestion(r store.ClassificationSuggestion) Suggestion {
	return Suggestion{ID: r.ID, CompanyID: r.CompanyID, StatementTransactionID: r.StatementTransactionID, Status: r.Status,
		AccountCode: r.AccountCode, Suggestion: json.RawMessage(r.Payload), TraceID: r.TraceID, CreatedBy: r.CreatedBy,
		CreatedAt: r.CreatedAt, DecidedBy: r.DecidedBy, DecidedAt: r.DecidedAt}
}

// Suggestions lista as sugestões da linha, da mais recente para a mais antiga.
func (s *Service) Suggestions(ctx context.Context, lineID string) ([]Suggestion, error) {
	q := store.New(s.pool)
	if !httpx.IsUUID(lineID) {
		return nil, lineNotFound()
	}
	if _, err := q.GetLineClassification(ctx, lineID); errors.Is(err, pgx.ErrNoRows) {
		return nil, lineNotFound()
	} else if err != nil {
		return nil, err
	}
	rows, err := q.ListClassificationSuggestionsByLine(ctx, lineID)
	if err != nil {
		return nil, err
	}
	out := make([]Suggestion, 0, len(rows))
	for _, r := range rows {
		out = append(out, toSuggestion(r))
	}
	return out, nil
}

func suggestionNotFound() error {
	return apperr.NotFound("SUGGESTION_NOT_FOUND", "Sugestão de conta não encontrada para esta linha.")
}

// pendingSuggestion trava a sugestão pendente da linha.
func pendingSuggestion(ctx context.Context, q *store.Queries, lineID, id string) (store.ClassificationSuggestion, error) {
	if !httpx.IsUUID(id) {
		return store.ClassificationSuggestion{}, suggestionNotFound()
	}
	sg, err := q.GetClassificationSuggestionForUpdate(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && sg.StatementTransactionID != lineID) {
		return sg, suggestionNotFound()
	}
	if err != nil {
		return sg, err
	}
	if sg.Status != SuggestionPending {
		return sg, apperr.Conflict("SUGGESTION_DECIDED", "A sugestão já foi decidida.").WithDetails(map[string]any{"status": sg.Status})
	}
	return sg, nil
}

// AcceptResult é o aceite: a classificação gravada, a sugestão e a regra proposta.
type AcceptResult struct {
	Classification Classification `json:"classification"`
	Suggestion     Suggestion     `json:"suggestion"`
	ProposedRule   *RuleProposal  `json:"proposed_rule"`
}

// AcceptSuggestion é a confirmação humana da sugestão (AC-020): a linha ganha a conta, com a origem
// agente, o autor e a sugestão. A regra sugerida pelo agente (ou, sem ela, a derivada) é proposta e
// só vale depois de confirmada.
func (s *Service) AcceptSuggestion(ctx context.Context, lineID, suggestionID string) (AcceptResult, error) {
	var out AcceptResult
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		sg, err := pendingSuggestion(ctx, q, lineID, suggestionID)
		if err != nil {
			return err
		}
		l, err := reconciledLine(ctx, q, lineID)
		if err != nil {
			return err
		}
		if sg.AccountCode == nil {
			return apperr.Conflict("SUGGESTION_WITHOUT_ACCOUNT",
				"A sugestão não tem conta do plano da empresa: rejeite e classifique à mão.")
		}
		if err := q.LockReconciliationCompany(ctx, l.CompanyID); err != nil {
			return err
		}
		if err := reconciliation.RequireOpen(ctx, tx, l.CompanyID, competence.FromDate(l.TxnDate)); err != nil {
			return err
		}
		if active, err := q.AccountIsActive(ctx, store.AccountIsActiveParams{CompanyID: l.CompanyID, Code: *sg.AccountCode}); err != nil {
			return err
		} else if !active {
			return apperr.Conflict("ACCOUNT_INACTIVE", "A conta sugerida foi desativada no plano: rejeite e classifique à mão.")
		}
		uid := user.FromContext(ctx)
		now := s.clock.Now()
		origin := classification.OriginAgent
		if err := q.SetLineClassification(ctx, store.SetLineClassificationParams{
			AccountCode: sg.AccountCode, Origin: &origin, SuggestionID: &sg.ID, ClassifiedBy: &uid, ClassifiedAt: &now,
			ID: l.ID, CompanyID: l.CompanyID,
		}); err != nil {
			return err
		}
		if err := q.DecideClassificationSuggestion(ctx, store.DecideClassificationSuggestionParams{
			Status: SuggestionAccepted, DecidedBy: &uid, Now: &now, ID: sg.ID,
		}); err != nil {
			return err
		}
		parsed, err := classification.ParseSuggestion(string(sg.Payload))
		if err != nil {
			return err
		}
		if r, ok := parsed.ProposedRule(*sg.AccountCode); ok {
			out.ProposedRule = proposal(r, true, l.ID)
		} else {
			in, err := lineInput(ctx, q, l)
			if err != nil {
				return err
			}
			out.ProposedRule = derived(in, *sg.AccountCode, l.ID)
		}
		sg.Status, sg.DecidedBy, sg.DecidedAt = SuggestionAccepted, &uid, &now
		out.Suggestion = toSuggestion(sg)
		if l, err = q.GetLineClassification(ctx, l.ID); err != nil {
			return err
		}
		out.Classification = toClassification(l)
		return s.audit(ctx, q, l.CompanyID, map[string]any{"acao": "aceitar_sugestao_conta", "sugestao": sg.ID, "conta": *sg.AccountCode})
	})
	return out, err
}

// RejectSuggestion rejeita a sugestão: a linha continua esperando a classificação do contador.
func (s *Service) RejectSuggestion(ctx context.Context, lineID, suggestionID string) (Suggestion, error) {
	var out Suggestion
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		sg, err := pendingSuggestion(ctx, q, lineID, suggestionID)
		if err != nil {
			return err
		}
		l, err := q.GetLineClassification(ctx, lineID)
		if err != nil {
			return err
		}
		if err := reconciliation.RequireOpen(ctx, tx, l.CompanyID, competence.FromDate(l.TxnDate)); err != nil {
			return err
		}
		uid := user.FromContext(ctx)
		now := s.clock.Now()
		if err := q.DecideClassificationSuggestion(ctx, store.DecideClassificationSuggestionParams{
			Status: SuggestionRejected, DecidedBy: &uid, Now: &now, ID: sg.ID,
		}); err != nil {
			return err
		}
		sg.Status, sg.DecidedBy, sg.DecidedAt = SuggestionRejected, &uid, &now
		out = toSuggestion(sg)
		return s.audit(ctx, q, l.CompanyID, map[string]any{"acao": "rejeitar_sugestao_conta", "sugestao": sg.ID})
	})
	return out, err
}
