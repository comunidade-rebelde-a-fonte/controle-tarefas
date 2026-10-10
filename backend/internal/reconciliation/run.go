package reconciliation

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"controle/internal/jobs"
	"controle/internal/platform/apperr"
	"controle/internal/platform/competence"
)

// RunPayload é o payload do job reconciliation_run: só os ids.
type RunPayload struct {
	CompanyID  string `json:"company_id"`
	Competence string `json:"competence"`
}

// EnqueueRun enfileira, na transação de quem mudou os dados, a rodada do matching da empresa na
// competência. Sem chave de dedupe: a rodada é idempotente e serializada por empresa, e um gatilho
// que chega com outra rodada já em andamento não pode se perder (como na origem, que rodava o
// matching depois de cada envio).
func EnqueueRun(ctx context.Context, q *jobs.Queue, tx pgx.Tx, companyID string, c competence.Competence) error {
	_, _, err := q.Enqueue(ctx, tx, jobs.Spec{Kind: JobRun, Payload: RunPayload{CompanyID: companyID, Competence: c.String()}})
	return err
}

// ResolvePayload é o payload do job agent_resolver: a linha e quem pediu (nulo no pedido da
// rodada automática).
type ResolvePayload struct {
	StatementTransactionID string  `json:"statement_transaction_id"`
	RequestedBy            *string `json:"requested_by,omitempty"`
}

// EnqueueResolve enfileira o pedido de hipótese para a linha. A chave é a linha: com um pedido já
// na fila, devolve o mesmo job (o modelo não é chamado duas vezes pelo mesmo pedido).
func EnqueueResolve(ctx context.Context, q *jobs.Queue, tx pgx.Tx, lineID string, requestedBy *string) (jobs.Job, bool, error) {
	return q.Enqueue(ctx, tx, jobs.Spec{Kind: JobResolve, Payload: ResolvePayload{StatementTransactionID: lineID, RequestedBy: requestedBy},
		DedupeKey: lineID})
}

// ClassifyPayload é o payload do job agent_classifier: a linha e quem pediu (nulo no pedido da
// rodada automática).
type ClassifyPayload struct {
	StatementTransactionID string  `json:"statement_transaction_id"`
	RequestedBy            *string `json:"requested_by,omitempty"`
}

// EnqueueClassify enfileira o pedido de sugestão de conta para a linha. A chave é a linha: com um
// pedido já na fila, devolve o mesmo job.
func EnqueueClassify(ctx context.Context, q *jobs.Queue, tx pgx.Tx, lineID string, requestedBy *string) (jobs.Job, bool, error) {
	return q.Enqueue(ctx, tx, jobs.Spec{Kind: JobClassify, Payload: ClassifyPayload{StatementTransactionID: lineID, RequestedBy: requestedBy},
		DedupeKey: lineID})
}

// IsPeriodClosed informa se o erro é a trava do fechamento Contábil (ErrPeriodClosed).
func IsPeriodClosed(err error) bool {
	var e *apperr.Error
	return errors.As(err, &e) && e.Code == "PERIOD_CLOSED"
}
