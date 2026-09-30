package seed

import (
	"context"
	"fmt"
	"hash/fnv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"controle/internal/competence"
	"controle/internal/platform/clock"
	comp "controle/internal/platform/competence"
	"controle/internal/platform/db"
)

// HistoryMonths é quantas competências anteriores à corrente o seed de demonstração gera.
const HistoryMonths = 3

// ReopenReason é o motivo da reabertura de demonstração.
const ReopenReason = "Correção de obrigação transmitida com informação incorreta (demonstração)"

// demoHistory gera as 3 competências anteriores e a corrente e aplica status variados de
// forma determinística: a maioria concluída, algumas pendentes ou atrasadas, módulos
// fechados, um fechamento reaberto e um vencimento alterado, com histórico SYSTEM
// [T-M2-06]. Competências que já têm histórico não são alteradas (idempotente).
func demoHistory(ctx context.Context, pool *pgxpool.Pool, clk clock.Clock) error {
	engine := competence.NewEngine(pool, clk)
	cur := comp.Current(clk)
	for age := HistoryMonths; age >= 0; age-- {
		if _, err := engine.Ensure(ctx, cur.AddMonths(-age)); err != nil {
			return err
		}
	}
	return db.WithTx(ctx, pool, func(tx pgx.Tx) error {
		for age := HistoryMonths; age >= 0; age-- {
			if err := applyDemoCompetence(ctx, tx, cur.AddMonths(-age), age, clk); err != nil {
				return err
			}
		}
		return nil
	})
}

type demoInstance struct {
	id, company, typeCode, module, statusSet, payrollType string
	applicable                                            bool
	sortOrder                                             int32
	due                                                   *time.Time
}

// pick devolve um número estável em 0..99 para a combinação de chaves.
func pick(parts ...string) int {
	h := fnv.New32a()
	for _, p := range parts {
		_, _ = h.Write([]byte(p))
		_, _ = h.Write([]byte{0})
	}
	return int(h.Sum32() % 100)
}

// Distribuição por idade da competência (0 = corrente): % final, iniciado, pendência.
var demoMix = map[int][3]int{
	3: {92, 0, 4},
	2: {88, 0, 6},
	1: {60, 20, 8},
	0: {15, 25, 5},
}

var demoStatuses = map[string]struct{ final, started, pending []string }{
	"FISCAL_CLOSING": {[]string{"CLOSED"}, []string{"DOWNLOADED", "POSTED", "CHECKED"}, []string{"MISSING_DOCUMENTS", "ERROR"}},
	"FISCAL_ITEM":    {[]string{"CLOSED", "CLOSED", "CLOSED", "NO_MOVEMENT"}, []string{"DOWNLOADED", "POSTED"}, []string{"MISSING_DOCUMENTS"}},
	"DECLARATION":    {[]string{"TRANSMITTED", "TRANSMITTED", "TRANSMITTED", "NO_MOVEMENT", "EXEMPT"}, []string{"PENDING"}, []string{"PENDING"}},
	"PAYROLL":        {[]string{"CLOSED", "ARCHIVED"}, []string{"IN_PROGRESS", "SENT_TO_CLIENT"}, []string{"PENDING"}},
}

func choose(options []string, r int) string { return options[r%len(options)] }

func applyDemoCompetence(ctx context.Context, tx pgx.Tx, c comp.Competence, age int, clk clock.Clock) error {
	var already bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (
		SELECT 1 FROM obligation_status_history h JOIN obligation_instances i ON i.id = h.obligation_instance_id
		JOIN competencies k ON k.id = i.competence_id WHERE k.competence_key = $1)`, c.String()).Scan(&already); err != nil {
		return err
	}
	if already {
		return nil
	}
	rows, err := tx.Query(ctx, `
		SELECT i.id, co.internal_code, t.code, t.primary_module, s.code, co.payroll_type, i.applicable, t.sort_order, i.due_date_effective
		FROM obligation_instances i
		JOIN competencies k ON k.id = i.competence_id
		JOIN companies co ON co.id = i.company_id
		JOIN obligation_types t ON t.id = i.obligation_type_id
		JOIN status_sets s ON s.id = t.status_set_id
		WHERE k.competence_key = $1
		ORDER BY co.internal_code, t.sort_order`, c.String())
	if err != nil {
		return err
	}
	insts, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (demoInstance, error) {
		var d demoInstance
		err := r.Scan(&d.id, &d.company, &d.typeCode, &d.module, &d.statusSet, &d.payrollType, &d.applicable, &d.sortOrder, &d.due)
		return d, err
	})
	if err != nil {
		return err
	}

	// Momento das alterações: dia 5 do mês seguinte à competência, sem passar do "agora".
	at := c.Next().FirstDay().AddDate(0, 0, 4).Add(12 * time.Hour)
	if now := clk.Now(); at.After(now) {
		at = now.Add(-time.Hour)
	}

	mix := demoMix[age]
	fiscalAllFinal := map[string]bool{}
	for _, d := range insts {
		if d.module == "FISCAL" && d.statusSet == "FISCAL_ITEM" {
			if _, ok := fiscalAllFinal[d.company]; !ok {
				fiscalAllFinal[d.company] = true
			}
		}
	}
	decided := map[string]string{} // instance id → status
	for _, d := range insts {
		if !d.applicable || d.statusSet == "FISCAL_CLOSING" || d.statusSet == "ACCOUNTING" {
			continue
		}
		set, ok := demoStatuses[d.statusSet]
		if !ok {
			continue
		}
		r := pick(d.company, d.typeCode, c.String())
		var status string
		switch {
		case r < mix[0]:
			status = choose(set.final, r)
		case r < mix[0]+mix[1]:
			status = choose(set.started, r)
		case r < mix[0]+mix[1]+mix[2]:
			status = choose(set.pending, r)
		default:
			continue // permanece "A iniciar"
		}
		decided[d.id] = status
		if d.statusSet == "FISCAL_ITEM" && status != "CLOSED" && status != "NO_MOVEMENT" {
			fiscalAllFinal[d.company] = false
		}
	}
	for _, d := range insts {
		if d.statusSet == "FISCAL_ITEM" && d.applicable && decided[d.id] == "" {
			fiscalAllFinal[d.company] = false
		}
	}

	// Contábil: respeita a sequência (etapas anteriores concluídas) [RN-011].
	levels := map[string]int{}
	for _, d := range insts {
		if d.statusSet != "ACCOUNTING" || !d.applicable {
			continue
		}
		lvl, ok := levels[d.company]
		if !ok {
			r := pick(d.company, "ACCOUNTING", c.String())
			switch {
			case age >= 2 && r < 90, age == 1 && r < 55:
				lvl = 6
			case age >= 1:
				lvl = 2 + r%4
			default:
				lvl = r % 3
			}
			levels[d.company] = lvl
		}
		step := int(d.sortOrder-410)/10 + 1 // 410..460 → etapas 1..6
		switch {
		case step <= lvl:
			if d.typeCode == "ACCOUNTING_PAYROLL_POSTED" && d.payrollType == "NO_PAYROLL" {
				decided[d.id] = "NO_MOVEMENT"
			} else {
				decided[d.id] = "OK"
			}
		case step == lvl+1 && pick(d.company, d.typeCode, "pending", c.String()) < 50:
			decided[d.id] = "PENDING"
		}
	}

	// Fechamento fiscal acompanha os controles.
	for _, d := range insts {
		if d.statusSet != "FISCAL_CLOSING" || !d.applicable {
			continue
		}
		switch {
		case fiscalAllFinal[d.company] && (age >= 1 || pick(d.company, "FECH", c.String()) < 40):
			decided[d.id] = "CLOSED"
		case age <= 1:
			decided[d.id] = choose(demoStatuses["FISCAL_CLOSING"].started, pick(d.company, "FECH", c.String()))
		}
	}

	batch := &pgx.Batch{}
	for _, d := range insts {
		status, ok := decided[d.id]
		if !ok {
			continue
		}
		batch.Queue(`UPDATE obligation_instances SET status_code = $2, version = version + 1, last_changed_by = $3,
			last_changed_at = $4, updated_at = $4 WHERE id = $1`, d.id, status, adminID, at)
		batch.Queue(`INSERT INTO obligation_status_history (obligation_instance_id, field, old_value, new_value, user_id, change_source, created_at)
			SELECT $1, 'STATUS', 'NOT_STARTED', $2, $3, 'SYSTEM', $4`, d.id, status, adminID, at)
		if status == "TRANSMITTED" && d.due != nil {
			batch.Queue(`UPDATE obligation_instances SET transmission_date = $2, protocol = $3 WHERE id = $1`,
				d.id, d.due.AddDate(0, 0, -1), fmt.Sprintf("DEMO-%s-%s-%s", c.String(), d.company, d.typeCode))
		}
	}
	if err := tx.SendBatch(ctx, batch).Close(); err != nil {
		return fmt.Errorf("status de demonstração em %s: %w", c, err)
	}

	if age >= 1 {
		// Competências passadas geradas agora pelo seed: a abertura fica datada no início do mês
		// seguinte, antes dos fechamentos e reaberturas de demonstração.
		if _, err := tx.Exec(ctx, `UPDATE company_module_period_events e SET created_at = $2
			FROM company_module_periods p JOIN competencies k ON k.id = p.competence_id
			WHERE e.company_module_period_id = p.id AND k.competence_key = $1 AND e.event_type = 'OPENED'`,
			c.String(), c.Next().FirstDay()); err != nil {
			return err
		}
	}
	if age >= 2 {
		if err := closeReadyPeriods(ctx, tx, c, at); err != nil {
			return err
		}
	}
	if age == 2 {
		if err := reopenDemo(ctx, tx, c, at); err != nil {
			return err
		}
	}
	if age == 1 {
		return overrideDueDateDemo(ctx, tx, c, at)
	}
	if age == 0 {
		return attentionDemo(ctx, tx, c, at, clock.Today(clk))
	}
	return nil
}

// AttentionDueReason é o motivo dos vencimentos antecipados da demonstração.
const AttentionDueReason = "Prazo antecipado a pedido do cliente (demonstração)"

// attentionDemo garante casos do "Exige atenção" na competência corrente [T-M5-02]: duas
// obrigações com vencimento antecipado para antes de hoje (atrasadas) e um Fechamento Fiscal
// com Erro, com histórico.
func attentionDemo(ctx context.Context, tx pgx.Tx, c comp.Competence, at, today time.Time) error {
	if _, err := tx.Exec(ctx, `
		WITH target AS (
		    SELECT i.id, i.due_date_effective FROM obligation_instances i
		    JOIN competencies k ON k.id = i.competence_id AND k.competence_key = $1
		    JOIN companies co ON co.id = i.company_id
		    JOIN obligation_types t ON t.id = i.obligation_type_id
		    JOIN status_definitions d ON d.status_set_id = t.status_set_id AND d.code = i.status_code
		    WHERE t.code = 'FISCAL_ENTRADAS' AND i.applicable AND NOT d.is_final AND i.due_date_effective IS NOT NULL
		    ORDER BY co.internal_code LIMIT 2),
		upd AS (
		    UPDATE obligation_instances i SET due_date_effective = $4::date - 3, due_date_override_reason = $5,
		           due_date_overridden_by = $3, due_date_overridden_at = $2, version = i.version + 1,
		           last_changed_by = $3, last_changed_at = $2, updated_at = $2
		    FROM target WHERE i.id = target.id
		    RETURNING i.id, target.due_date_effective AS old_due, i.due_date_effective AS new_due)
		INSERT INTO obligation_status_history (obligation_instance_id, field, old_value, new_value, user_id, change_source, override_reason, created_at)
		SELECT id, 'DUE_DATE', old_due::text, new_due::text, $3, 'SYSTEM', $5, $2 FROM upd`,
		c.String(), at, adminID, today, AttentionDueReason); err != nil {
		return fmt.Errorf("atrasos de demonstração: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		WITH target AS (
		    SELECT i.id, i.status_code FROM obligation_instances i
		    JOIN competencies k ON k.id = i.competence_id AND k.competence_key = $1
		    JOIN companies co ON co.id = i.company_id
		    JOIN obligation_types t ON t.id = i.obligation_type_id
		    WHERE t.code = 'FISCAL_FECHAMENTO' AND i.applicable AND i.status_code <> 'CLOSED'
		    ORDER BY co.internal_code DESC LIMIT 1),
		upd AS (
		    UPDATE obligation_instances i SET status_code = 'ERROR', version = i.version + 1,
		           last_changed_by = $3, last_changed_at = $2, updated_at = $2
		    FROM target WHERE i.id = target.id
		    RETURNING i.id, target.status_code AS old_status)
		INSERT INTO obligation_status_history (obligation_instance_id, field, old_value, new_value, user_id, change_source, created_at)
		SELECT id, 'STATUS', old_status, 'ERROR', $3, 'SYSTEM', $2 FROM upd`, c.String(), at, adminID); err != nil {
		return fmt.Errorf("erro de demonstração: %w", err)
	}
	return nil
}

// closeReadyPeriods fecha os módulos cujas instâncias aplicáveis que contam estão todas finais.
func closeReadyPeriods(ctx context.Context, tx pgx.Tx, c comp.Competence, at time.Time) error {
	_, err := tx.Exec(ctx, `
		WITH ready AS (
		    SELECT p.id FROM company_module_periods p
		    JOIN competencies k ON k.id = p.competence_id AND k.competence_key = $1
		    WHERE p.state = 'OPEN' AND NOT EXISTS (
		        SELECT 1 FROM obligation_instances i
		        JOIN obligation_types t ON t.id = i.obligation_type_id
		        JOIN status_definitions d ON d.status_set_id = t.status_set_id AND d.code = i.status_code
		        WHERE i.company_id = p.company_id AND i.competence_id = p.competence_id
		          AND t.primary_module = p.module_code AND i.applicable AND NOT d.is_final
		          AND (i.counts_in_progress OR t.is_closing_stage))),
		closed AS (
		    UPDATE company_module_periods p SET state = 'CLOSED', closed_at = $2, closed_by = $3, updated_at = $2,
		           version = version + 1
		    FROM ready WHERE p.id = ready.id RETURNING p.id)
		INSERT INTO company_module_period_events (company_module_period_id, event_type, user_id, created_at)
		SELECT id, 'CLOSED', $3, $2 FROM closed`, c.String(), at, adminID)
	return err
}

// reopenDemo reabre o Fiscal de uma empresa com motivo, deixando o histórico CLOSED → REOPENED.
func reopenDemo(ctx context.Context, tx pgx.Tx, c comp.Competence, at time.Time) error {
	_, err := tx.Exec(ctx, `
		WITH target AS (
		    SELECT p.id FROM company_module_periods p
		    JOIN competencies k ON k.id = p.competence_id AND k.competence_key = $1
		    JOIN companies co ON co.id = p.company_id
		    WHERE p.state = 'CLOSED' AND p.module_code = 'FISCAL'
		    ORDER BY co.internal_code LIMIT 1),
		reopened AS (
		    UPDATE company_module_periods p SET state = 'OPEN', closed_at = NULL, closed_by = NULL,
		           updated_at = $2, version = version + 1
		    FROM target WHERE p.id = target.id RETURNING p.id)
		INSERT INTO company_module_period_events (company_module_period_id, event_type, reason, user_id, created_at)
		SELECT id, 'REOPENED', $4, $3, $2 + interval '2 days' FROM reopened`, c.String(), at, adminID, ReopenReason)
	return err
}

// overrideDueDateDemo altera excepcionalmente um vencimento, com motivo e histórico [§21.3].
func overrideDueDateDemo(ctx context.Context, tx pgx.Tx, c comp.Competence, at time.Time) error {
	_, err := tx.Exec(ctx, `
		WITH target AS (
		    SELECT i.id, i.due_date_effective FROM obligation_instances i
		    JOIN competencies k ON k.id = i.competence_id AND k.competence_key = $1
		    JOIN companies co ON co.id = i.company_id
		    JOIN obligation_types t ON t.id = i.obligation_type_id
		    WHERE t.code = 'SPED_FISCAL' AND i.applicable AND i.due_date_effective IS NOT NULL
		    ORDER BY co.internal_code LIMIT 1),
		upd AS (
		    UPDATE obligation_instances i SET due_date_effective = target.due_date_effective + 5,
		           due_date_override_reason = 'Prorrogação combinada com o cliente (demonstração)',
		           due_date_overridden_by = $3, due_date_overridden_at = $2, version = i.version + 1,
		           last_changed_by = $3, last_changed_at = $2, updated_at = $2
		    FROM target WHERE i.id = target.id
		    RETURNING i.id, target.due_date_effective AS old_due, i.due_date_effective AS new_due)
		INSERT INTO obligation_status_history (obligation_instance_id, field, old_value, new_value, user_id, change_source, override_reason, created_at)
		SELECT id, 'DUE_DATE', old_due::text, new_due::text, $3, 'SYSTEM', 'Prorrogação combinada com o cliente (demonstração)', $2 FROM upd`,
		c.String(), at, adminID)
	return err
}
