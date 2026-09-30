package seed

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"controle/internal/calendar"
	"controle/internal/platform/clock"
)

// Anos cobertos pelo seed de feriados nacionais [plan M1].
const (
	FirstHolidayYear = 2025
	LastHolidayYear  = 2030
)

// NationalCalendarName é o nome do calendário nacional.
const NationalCalendarName = "Nacional"

// calendarStep cria o calendário nacional e seus feriados (fixos + móveis pela Páscoa).
// Calendários de UF e município são criados sob demanda pela tela de feriados (Q7).
func calendarStep() Step {
	return Step{Name: "calendar (feriados nacionais)", Run: func(ctx context.Context, tx pgx.Tx, _ clock.Clock) error {
		var calID string
		err := tx.QueryRow(ctx, `SELECT id FROM business_calendars WHERE scope = 'NATIONAL'`).Scan(&calID)
		if errors.Is(err, pgx.ErrNoRows) {
			err = tx.QueryRow(ctx, `INSERT INTO business_calendars (name, scope) VALUES ($1, 'NATIONAL') RETURNING id`,
				NationalCalendarName).Scan(&calID)
		}
		if err != nil {
			return err
		}
		batch := &pgx.Batch{}
		for year := FirstHolidayYear; year <= LastHolidayYear; year++ {
			for _, h := range calendar.NationalHolidays(year) {
				batch.Queue(`INSERT INTO holidays (calendar_id, date, name, scope) VALUES ($1, $2, $3, 'NATIONAL')
				             ON CONFLICT (calendar_id, date) DO UPDATE SET name = EXCLUDED.name`, calID, h.Date, h.Name)
			}
		}
		return tx.SendBatch(ctx, batch).Close()
	}}
}
