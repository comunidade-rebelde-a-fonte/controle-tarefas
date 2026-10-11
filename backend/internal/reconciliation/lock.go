package reconciliation

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"controle/internal/platform/apperr"
	"controle/internal/platform/competence"
	"controle/internal/store"
)

// ErrPeriodClosed é o erro de mutação em competência com o Contábil fechado [AC-025]: 423, o
// mesmo código das obrigações.
func ErrPeriodClosed(c competence.Competence) error {
	return apperr.Locked("PERIOD_CLOSED",
		"O Contábil está fechado em "+c.Display()+". Reabra o fechamento do módulo Contábil para alterar a conciliação.")
}

// AccountingClosed indica se o módulo Contábil da empresa está fechado na competência. O
// fechamento do Controle é a única autoridade: a Conciliação não tem fechamento próprio
// [REQ-033]. Sem período gerado para a empresa na competência, não há trava.
func AccountingClosed(ctx context.Context, db store.DBTX, companyID string, c competence.Competence) (bool, error) {
	state, err := store.New(db).GetAccountingPeriodState(ctx, store.GetAccountingPeriodStateParams{
		CompanyID: companyID, Competence: c.FirstDay(),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return state == "CLOSED", nil
}

// Motivos da trava da competência na Conciliação (o motivo do diagnóstico do documento tardio).
const (
	LockAccountingClosed = "periodo_fechado"       // o Contábil está fechado (AC-025)
	LockCompleted        = "conciliacao_concluida" // a conciliação foi concluída (TASK-022, D-035)
)

// ErrCompleted é o erro de mutação em competência com a conciliação concluída (TASK-022, D-035):
// 423, como o Contábil fechado, até o contador reabrir a conciliação com motivo.
func ErrCompleted(c competence.Competence) error {
	return apperr.Locked("RECONCILIATION_COMPLETED",
		"A conciliação de "+c.Display()+" está concluída. Reabra a conciliação (com motivo) para alterá-la.")
}

// Completed indica se a conciliação da empresa está concluída na competência. Na transação, pega
// antes a trava compartilhada da empresa: a gravação que passa por aqui espera a conclusão em
// andamento (trava exclusiva) e, depois dela, vê a conciliação concluída.
func Completed(ctx context.Context, db store.DBTX, companyID string, c competence.Competence) (bool, error) {
	q := store.New(db)
	if err := q.LockReconciliationCompanyShared(ctx, companyID); err != nil {
		return false, err
	}
	_, err := q.GetActiveCompletion(ctx, store.GetActiveCompletionParams{CompanyID: companyID, Competence: c.FirstDay()})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// LockReason diz por que a competência está travada para a Conciliação: o Contábil fechado vem
// antes da conciliação concluída; aberta, "".
func LockReason(ctx context.Context, db store.DBTX, companyID string, c competence.Competence) (string, error) {
	closed, err := AccountingClosed(ctx, db, companyID, c)
	if err != nil {
		return "", err
	}
	if closed {
		return LockAccountingClosed, nil
	}
	done, err := Completed(ctx, db, companyID, c)
	if err != nil {
		return "", err
	}
	if done {
		return LockCompleted, nil
	}
	return "", nil
}

// RequireOpen devolve ErrPeriodClosed se o Contábil estiver fechado na competência e
// ErrCompleted se a conciliação estiver concluída (TASK-022): toda mutação da Conciliação passa
// por aqui.
func RequireOpen(ctx context.Context, db store.DBTX, companyID string, c competence.Competence) error {
	reason, err := LockReason(ctx, db, companyID, c)
	if err != nil {
		return err
	}
	switch reason {
	case LockAccountingClosed:
		return ErrPeriodClosed(c)
	case LockCompleted:
		return ErrCompleted(c)
	}
	return nil
}
