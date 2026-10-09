package documents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"controle/internal/jobs"
	"controle/internal/platform/apperr"
	"controle/internal/platform/competence"
	"controle/internal/platform/httpx"
	"controle/internal/reconciliation"
	"controle/internal/store"
	"controle/internal/user"
)

const codeNotRetryable = "DOCUMENT_NOT_RETRYABLE"

// retryPlan é o que o reprocessamento faz: o job a enfileirar e o estado em que o documento volta.
type retryPlan struct {
	kind   string
	status string
}

// planRetry decide o reprocessamento pelo estado do documento [06 §4]: a revisão da triagem e a
// pendência de período fechado voltam à triagem; a revisão da extração do extrato, à extração;
// documento com o job encerrado sem sucesso (dead) repete o mesmo job. O resto (importado, em
// processamento) não é reprocessado.
func planRetry(doc store.GetReconciliationDocumentRow) (retryPlan, bool) {
	jobDead := doc.JobStatus != nil && *doc.JobStatus == jobs.StatusDead
	switch doc.Status {
	case reconciliation.DocTriageReview, reconciliation.DocClosedPeriod:
		return retryPlan{kind: reconciliation.JobTriage, status: reconciliation.DocPending}, true
	case reconciliation.DocExtractReview:
		if doc.Kind != nil && *doc.Kind == "extrato" {
			return retryPlan{kind: reconciliation.JobStatementExtract, status: reconciliation.DocTriaged}, true
		}
	case reconciliation.DocPending:
		if jobDead || doc.JobID == nil {
			return retryPlan{kind: reconciliation.JobTriage, status: reconciliation.DocPending}, true
		}
	case reconciliation.DocTriaged:
		if jobDead && doc.Kind != nil {
			kind := reconciliation.JobReceiptExtract
			if *doc.Kind == "extrato" {
				kind = reconciliation.JobStatementExtract
			}
			return retryPlan{kind: kind, status: reconciliation.DocTriaged}, true
		}
	}
	return retryPlan{}, false
}

// lockedCompetences são as competências que travam o reprocessamento: a do envio e as do
// período lido (AC-025). Sem empresa, não há trava.
func lockedCompetences(doc store.GetReconciliationDocumentRow) []competence.Competence {
	var out []competence.Competence
	for _, t := range []*time.Time{doc.Competence, doc.PeriodStart, doc.PeriodEnd} {
		if t != nil {
			out = append(out, competence.FromDate(*t))
		}
	}
	return out
}

// Retry reprocessa o documento em revisão, parado por período fechado ou com o job encerrado sem
// sucesso. Não ignora portão nenhum: o documento volta para a fila e passa de novo pela triagem
// ou pela extração, com o validador [06 §4]. Fica no log de extração como intervenção humana.
func (s *Service) Retry(ctx context.Context, id string) (UploadResult, error) {
	if !httpx.IsUUID(id) {
		return UploadResult{}, notFound()
	}
	uid := user.FromContext(ctx)
	var out UploadResult
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		if _, err := q.GetReconciliationDocumentForUpdate(ctx, id); errors.Is(err, pgx.ErrNoRows) {
			return notFound()
		} else if err != nil {
			return err
		}
		doc, err := q.GetReconciliationDocument(ctx, id)
		if err != nil {
			return err
		}
		plan, ok := planRetry(doc)
		if !ok {
			return apperr.Conflict(codeNotRetryable,
				"Só se reprocessa documento em revisão, parado por período fechado ou com o processamento encerrado sem sucesso.").
				WithDetails(map[string]any{"status": doc.Status})
		}
		if doc.CompanyID != nil {
			for _, c := range lockedCompetences(doc) {
				if err := reconciliation.RequireOpen(ctx, tx, *doc.CompanyID, c); err != nil {
					return err
				}
			}
		}
		job, created, err := s.queue.Enqueue(ctx, tx, jobs.Spec{Kind: plan.kind, Payload: triagePayload{DocumentID: id}, DedupeKey: id})
		if err != nil {
			return err
		}
		if !created {
			return apperr.Conflict(codeNotRetryable, "O documento já está na fila de processamento.")
		}
		now := s.clock.Now()
		if err := q.SetReconciliationDocumentState(ctx, store.SetReconciliationDocumentStateParams{
			Status: plan.status, Diagnosis: doc.Diagnosis, JobID: &job.ID, Now: now, ID: id,
		}); err != nil {
			return err
		}
		change, _ := json.Marshal(map[string]string{"acao": "reprocessar", "de": doc.Status, "para": plan.status, "job": plan.kind})
		text := string(change)
		if _, err := q.InsertExtractionLog(ctx, store.InsertExtractionLogParams{
			CompanyID: doc.CompanyID, DocumentID: &id, FileSha256: &doc.Sha256, Alias: "humano", Attempt: 0,
			JobID: &job.ID, ActorID: &uid, Change: &text, Now: now,
		}); err != nil {
			return fmt.Errorf("registrar o reprocessamento: %w", err)
		}
		out = UploadResult{DocumentID: id, JobID: &job.ID, Status: plan.status}
		return nil
	})
	return out, err
}
