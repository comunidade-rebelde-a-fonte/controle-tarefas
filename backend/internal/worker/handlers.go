package worker

import (
	"context"

	"github.com/jackc/pgx/v5"

	"controle/internal/jobs"
)

// KindPing é o job de verificação da fila, sem efeito (origem: handle_ping do Conciliador).
const KindPing = "ping"

// KindLangfuseSync envia uma corrida de agente e as notas dela ao Langfuse (TASK-009).
const KindLangfuseSync = "langfuse_sync"

// KindPgdasParse processa um extrato PGDAS-D enviado ao inbox do Fator R (TASK-012).
const KindPgdasParse = "pgdas_parse"

// KindReconciliationTriage tria o documento enviado à Conciliação (TASK-015/016).
const KindReconciliationTriage = "reconciliation_triage"

// KindStatementExtract extrai o extrato triado (TASK-016).
const KindStatementExtract = "statement_extract"

// HandlerDeps são os handlers que dependem de configuração.
type HandlerDeps struct {
	// LangfuseSync é o tracer.Syncer.Handle.
	LangfuseSync jobs.Handler
	// PgdasParse é o inbox.Agent.HandleParse (o parser_pgdas no upload).
	PgdasParse jobs.Handler
	// ReconciliationTriage e StatementExtract são o pipeline.Processor da Conciliação.
	ReconciliationTriage jobs.Handler
	StatementExtract     jobs.Handler
}

// Handlers são os tipos de job que este worker executa. Cada TASK que cria um tipo de job
// (pgdas_parse na TASK-012, reconciliation_triage e statement_extract na TASK-016,
// receipt_extract na TASK-017…) registra o seu handler aqui [04 §7].
func Handlers(d HandlerDeps) map[string]jobs.Handler {
	h := map[string]jobs.Handler{
		KindPing: func(context.Context, pgx.Tx, jobs.Claimed) error { return nil },
	}
	for kind, handler := range map[string]jobs.Handler{
		KindLangfuseSync:         d.LangfuseSync,
		KindPgdasParse:           d.PgdasParse,
		KindReconciliationTriage: d.ReconciliationTriage,
		KindStatementExtract:     d.StatementExtract,
	} {
		if handler != nil {
			h[kind] = handler
		}
	}
	return h
}
