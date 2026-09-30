package obligation

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"controle/internal/store"
)

// ModuleProgressRow é o agregado de uma empresa × módulo na competência.
type ModuleProgressRow struct {
	CompanyID       string
	Module          string
	Denominator     int
	OperationalDone int
	FinalDone       int
	Blocking        int
	Overdue         int
	NearestDeadline *time.Time
}

// Progress converte o agregado nos percentuais (mesma regra de Aggregate).
func (r ModuleProgressRow) Progress() Progress {
	return ProgressFromCounts(r.Denominator, r.OperationalDone, r.FinalDone, r.Blocking)
}

const moduleProgressSQL = `
SELECT
    v.company_id,
    v.primary_module,
    count(*) FILTER (WHERE v.in_denominator)::int                                      AS denominator,
    count(*) FILTER (WHERE v.in_denominator AND v.counts_as_operational_progress)::int AS operational_done,
    count(*) FILTER (WHERE v.in_denominator AND v.is_final)::int                       AS final_done,
    count(*) FILTER (WHERE v.in_denominator AND v.is_blocking)::int                    AS blocking,
    count(*) FILTER (WHERE v.applicable AND NOT v.is_final AND v.due_date_effective < $2)::int AS overdue,
    min(v.due_date_effective) FILTER (WHERE v.applicable AND NOT v.is_final)          AS nearest_deadline
FROM v_obligation_instances v
WHERE v.competence_id = $1
  AND ($3::uuid IS NULL OR v.company_id = $3::uuid)
GROUP BY v.company_id, v.primary_module
ORDER BY v.company_id, v.primary_module`

// ModuleProgress agrega por empresa × módulo na competência [PRD §12]. companyID opcional.
// "today" é a data de negócio (Clock). É a versão SQL de Aggregate + IsOverdue.
func ModuleProgress(ctx context.Context, db store.DBTX, competenceID string, today time.Time, companyID *string) ([]ModuleProgressRow, error) {
	rows, err := db.Query(ctx, moduleProgressSQL, competenceID, today, companyID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (ModuleProgressRow, error) {
		var m ModuleProgressRow
		err := r.Scan(&m.CompanyID, &m.Module, &m.Denominator, &m.OperationalDone, &m.FinalDone, &m.Blocking, &m.Overdue, &m.NearestDeadline)
		return m, err
	})
}
