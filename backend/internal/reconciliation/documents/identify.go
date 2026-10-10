package documents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"controle/internal/jobs"
	"controle/internal/platform/apperr"
	"controle/internal/platform/competence"
	"controle/internal/platform/httpx"
	"controle/internal/reconciliation"
	"controle/internal/reconciliation/entries"
	"controle/internal/store"
	"controle/internal/user"
)

// Tipos que o contador informa em "que documento é esse?".
const (
	KindStatement = "extrato"
	KindReceipt   = "comprovante"
)

const maxBankField = 64

// IdentifyInput é a resposta do contador a "que documento é esse?" (TASK-021a): o tipo, a empresa
// e, no extrato, o banco, a agência, a conta e o período, quando a triagem não os leu.
type IdentifyInput struct {
	Kind        string  `json:"kind"`
	CompanyID   string  `json:"company_id"`
	Bank        *string `json:"bank"`
	Branch      *string `json:"branch"`
	Account     *string `json:"account"`
	PeriodStart *string `json:"period_start"`
	PeriodEnd   *string `json:"period_end"`
}

func optText(s *string, field string, fields map[string]any) *string {
	if s == nil {
		return nil
	}
	t := strings.TrimSpace(*s)
	if t == "" {
		return nil
	}
	if utf8.RuneCountInString(t) > maxBankField {
		fields[field] = fmt.Sprintf("Máximo de %d caracteres.", maxBankField)
	}
	return &t
}

func optDate(s *string, field string, fields map[string]any) *time.Time {
	if s == nil || strings.TrimSpace(*s) == "" {
		return nil
	}
	d, err := time.Parse(time.DateOnly, strings.TrimSpace(*s))
	if err != nil {
		fields[field] = "Data inválida: use AAAA-MM-DD."
		return nil
	}
	return &d
}

// identifiable informa se o documento pode ser identificado à mão: em revisão da triagem ou da
// extração, menos o comprovante que já tem lançamento (a correção dele é campo a campo).
func identifiable(status string, hasEntry bool) bool {
	return status == reconciliation.DocTriageReview || (status == reconciliation.DocExtractReview && !hasEntry)
}

// Identify grava o que o contador disse do documento e o devolve à extração do extrato ou do
// comprovante, com os mesmos portões (o validador do saldo, as regras de campo). A origem só
// listava esses documentos na fila (M2.6 previa a pergunta). Fica no log de extração.
func (s *Service) Identify(ctx context.Context, id string, in IdentifyInput) (UploadResult, error) {
	if !httpx.IsUUID(id) {
		return UploadResult{}, notFound()
	}
	fields := map[string]any{}
	if in.Kind != KindStatement && in.Kind != KindReceipt {
		fields["kind"] = "Use extrato ou comprovante."
	}
	bank, branch, account := optText(in.Bank, "bank", fields), optText(in.Branch, "branch", fields), optText(in.Account, "account", fields)
	start, end := optDate(in.PeriodStart, "period_start", fields), optDate(in.PeriodEnd, "period_end", fields)
	if start != nil && end != nil && end.Before(*start) {
		fields["period_end"] = "O fim do período vem antes do início."
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
		if !httpx.IsUUID(in.CompanyID) {
			fields["company_id"] = "Escolha a empresa do documento."
		} else if _, err := q.GetCompanyIdentity(ctx, in.CompanyID); errors.Is(err, pgx.ErrNoRows) {
			fields["company_id"] = "Empresa não encontrada."
		} else if err != nil {
			return err
		}
		if len(fields) > 0 {
			return apperr.BadRequest("VALIDATION_ERROR", "Dados inválidos. Verifique os campos destacados.").
				WithDetails(map[string]any{"fields": fields})
		}
		entry, err := entries.ByDocument(ctx, tx, id)
		if err != nil {
			return err
		}
		if !identifiable(doc.Status, entry != nil) {
			return apperr.Conflict("DOCUMENT_NOT_IDENTIFIABLE",
				"Só se identifica documento em revisão da triagem ou da extração (comprovante sem lançamento).").
				WithDetails(map[string]any{"status": doc.Status})
		}
		locked := []*time.Time{doc.Competence, start, end}
		if start == nil {
			locked = append(locked, doc.PeriodStart)
		}
		if end == nil {
			locked = append(locked, doc.PeriodEnd)
		}
		seen := map[string]bool{}
		for _, t := range locked {
			if t == nil || seen[competence.FromDate(*t).String()] {
				continue
			}
			seen[competence.FromDate(*t).String()] = true
			if err := reconciliation.RequireOpen(ctx, tx, in.CompanyID, competence.FromDate(*t)); err != nil {
				return err
			}
		}
		kind := reconciliation.JobReceiptExtract
		if in.Kind == KindStatement {
			kind = reconciliation.JobStatementExtract
		}
		job, created, err := s.queue.Enqueue(ctx, tx, jobs.Spec{Kind: kind, Payload: triagePayload{DocumentID: id}, DedupeKey: id})
		if err != nil {
			return err
		}
		if !created {
			return apperr.Conflict(codeNotRetryable, "O documento já está na fila de processamento.")
		}
		now := s.clock.Now()
		if err := q.IdentifyReconciliationDocument(ctx, store.IdentifyReconciliationDocumentParams{
			Kind: &in.Kind, CompanyID: &in.CompanyID, Bank: bank, Branch: branch, Account: account, PeriodStart: start, PeriodEnd: end,
			Status: reconciliation.DocTriaged, JobID: &job.ID, Now: now, ID: id,
		}); err != nil {
			return err
		}
		change, _ := json.Marshal(map[string]any{"acao": "identificar", "tipo": in.Kind, "empresa": in.CompanyID, "de": doc.Status,
			"job": kind})
		text := string(change)
		if _, err := q.InsertExtractionLog(ctx, store.InsertExtractionLogParams{
			CompanyID: &in.CompanyID, DocumentID: &id, FileSha256: &doc.Sha256, Alias: "humano", Attempt: 0, JobID: &job.ID, ActorID: &uid,
			Change: &text, Now: now,
		}); err != nil {
			return fmt.Errorf("registrar a identificação: %w", err)
		}
		out = UploadResult{DocumentID: id, JobID: &job.ID, Status: reconciliation.DocTriaged}
		return nil
	})
	return out, err
}
