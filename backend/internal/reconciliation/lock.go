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

// RequireOpen devolve ErrPeriodClosed se o Contábil estiver fechado na competência.
func RequireOpen(ctx context.Context, db store.DBTX, companyID string, c competence.Competence) error {
	closed, err := AccountingClosed(ctx, db, companyID, c)
	if err != nil {
		return err
	}
	if closed {
		return ErrPeriodClosed(c)
	}
	return nil
}
