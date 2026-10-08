package tracer

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"controle/internal/platform/apperr"
	"controle/internal/platform/decimal"
	"controle/internal/store"
)

// Status da nota ouro por campo, como na origem (evals_gold).
const (
	GoldOK          = "ok"
	GoldError       = "erro"
	GoldUnavailable = "ouro_indisponivel"
)

// Notas humanas, como na origem (evals_human): o mesmo vocabulário vai ao Langfuse no score
// categórico human_eval.
const (
	RatingCorrect = "acerto"
	RatingPartial = "parcial"
	RatingWrong   = "erro"
)

// MaxCommentChars limita o comentário da nota humana (como a origem).
const MaxCommentChars = 2000

// ErrRunNotFound: trace inexistente.
var ErrRunNotFound = apperr.NotFound("AI_RUN_NOT_FOUND", "Corrida de IA não encontrada.")

// GoldResult é a nota ouro de um campo (ResultadoCampoOuro da origem).
type GoldResult struct {
	Field           string
	Expected        *string
	Obtained        *string
	WithinTolerance *bool
	Status          string
}

// RecordGold grava a nota ouro por campo na transação de quem chama e enfileira o envio ao
// Langfuse (scores gold_<campo> e gold_acerto). Devolve o acerto entre os campos comparáveis
// (os que não são ouro_indisponivel), ou nil sem nenhum, como gold() da origem.
func (t *Tracer) RecordGold(ctx context.Context, tx pgx.Tx, traceID string, results []GoldResult) (*decimal.Decimal, error) {
	q := store.New(tx)
	if ok, err := q.AIRunExists(ctx, traceID); err != nil {
		return nil, err
	} else if !ok {
		return nil, ErrRunNotFound
	}
	now := t.clock.Now()
	comparable, hits := 0, 0
	for _, r := range results {
		switch r.Status {
		case GoldOK:
			hits++
			comparable++
		case GoldError:
			comparable++
		case GoldUnavailable:
		default:
			return nil, fmt.Errorf("tracer: status de nota ouro inválido %q", r.Status)
		}
		if _, err := q.InsertAIGoldEval(ctx, store.InsertAIGoldEvalParams{
			TraceID: traceID, Field: r.Field, Expected: r.Expected, Obtained: r.Obtained,
			WithinTolerance: r.WithinTolerance, Status: r.Status, CreatedAt: now,
		}); err != nil {
			return nil, fmt.Errorf("tracer: gravar a nota ouro de %s: %w", r.Field, err)
		}
	}
	if err := t.enqueueSync(ctx, tx, traceID); err != nil {
		return nil, err
	}
	if comparable == 0 {
		return nil, nil
	}
	acerto := decimal.FromInt(int64(hits)).Quo(decimal.FromInt(int64(comparable)))
	return &acerto, nil
}

// HumanEval é uma nota humana gravada.
type HumanEval = store.AiHumanEval

// RecordHuman grava a nota humana (acerto, parcial ou erro; erro exige comentário) e enfileira o
// envio ao Langfuse como score human_eval [AC-031].
func (t *Tracer) RecordHuman(ctx context.Context, tx pgx.Tx, traceID, userID, rating string, comment *string) (HumanEval, error) {
	fields := map[string]any{}
	switch rating {
	case RatingCorrect, RatingPartial, RatingWrong:
	default:
		fields["rating"] = "Use acerto, parcial ou erro."
	}
	var text *string
	if comment != nil {
		if c := strings.TrimSpace(*comment); c != "" {
			text = &c
			if utf8.RuneCountInString(c) > MaxCommentChars {
				fields["comment"] = fmt.Sprintf("Máximo de %d caracteres.", MaxCommentChars)
			}
		}
	}
	if len(fields) > 0 {
		return HumanEval{}, apperr.BadRequest("VALIDATION_ERROR", "Dados inválidos. Verifique os campos destacados.").
			WithDetails(map[string]any{"fields": fields})
	}
	if rating == RatingWrong && text == nil {
		return HumanEval{}, apperr.Unprocessable("EVAL_COMMENT_REQUIRED", "O comentário é obrigatório quando a nota é \"erro\".")
	}
	q := store.New(tx)
	if ok, err := q.AIRunExists(ctx, traceID); err != nil {
		return HumanEval{}, err
	} else if !ok {
		return HumanEval{}, ErrRunNotFound
	}
	ev, err := q.InsertAIHumanEval(ctx, store.InsertAIHumanEvalParams{
		TraceID: traceID, UserID: userID, Rating: rating, Comment: text, CreatedAt: t.clock.Now(),
	})
	if err != nil {
		return HumanEval{}, fmt.Errorf("tracer: gravar a nota humana: %w", err)
	}
	if err := t.enqueueSync(ctx, tx, traceID); err != nil {
		return HumanEval{}, err
	}
	return ev, nil
}
