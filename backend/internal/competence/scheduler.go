package competence

import (
	"context"
	"log/slog"
	"time"

	"controle/internal/platform/clock"
)

// DailyRunHour e DailyRunMinute definem o horário do agendador (00:05 no fuso de negócio).
const (
	DailyRunHour   = 0
	DailyRunMinute = 5
)

// NextRun devolve o próximo horário de execução (00:05 no fuso loc) estritamente após now.
func NextRun(now time.Time, loc *time.Location) time.Time {
	local := now.In(loc)
	next := time.Date(local.Year(), local.Month(), local.Day(), DailyRunHour, DailyRunMinute, 0, 0, loc)
	if !next.After(local) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}

// RunDaily executa EnsureCurrent todos os dias às 00:05 no fuso de negócio até o contexto
// ser cancelado (desligamento da API) [PRD §10.2].
func RunDaily(ctx context.Context, e *Engine, clk clock.Clock, logger *slog.Logger) {
	for {
		now := clk.Now()
		wait := NextRun(now, clk.Location()).Sub(now)
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		s, err := e.EnsureCurrent(ctx)
		if err != nil {
			logger.Error("agendador: falha ao garantir a competência corrente", "error", err)
			continue
		}
		logger.Info("agendador: competência garantida", "competence", s.Competence.String(),
			"companies", s.CompaniesProcessed, "instances", s.InstancesCreated)
	}
}
