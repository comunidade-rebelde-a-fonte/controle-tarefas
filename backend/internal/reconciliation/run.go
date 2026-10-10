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

// IsPeriodClosed informa se o erro é a trava do fechamento Contábil (ErrPeriodClosed).
func IsPeriodClosed(err error) bool {
	var e *apperr.Error
	return errors.As(err, &e) && e.Code == "PERIOD_CLOSED"
}
