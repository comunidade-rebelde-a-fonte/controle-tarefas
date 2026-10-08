package worker

import (
	"context"

	"github.com/jackc/pgx/v5"

	"controle/internal/jobs"
)

// KindPing é o job de verificação da fila, sem efeito (origem: handle_ping do Conciliador).
const KindPing = "ping"

// Handlers são os tipos de job que este worker executa. Cada TASK que cria um tipo de job
// (pgdas_parse na TASK-012, statement_triage e statement_extract na TASK-016, receipt_extract na
// TASK-017, langfuse_sync_retry na TASK-009…) registra o seu handler aqui [04 §7].
func Handlers() map[string]jobs.Handler {
	return map[string]jobs.Handler{
		KindPing: func(context.Context, pgx.Tx, jobs.Claimed) error { return nil },
	}
}
