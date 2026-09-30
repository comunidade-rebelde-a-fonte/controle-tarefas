package company

import (
	"context"
	"net/http"
	"time"

	"controle/internal/obligation"
	comp "controle/internal/platform/competence"
	"controle/internal/platform/httpx"
)

// HistoryEvent é uma abertura, fechamento ou reabertura do período.
type HistoryEvent struct {
	EventType string    `json:"event_type"`
	Reason    *string   `json:"reason"`
	UserName  string    `json:"user_name"`
	CreatedAt time.Time `json:"created_at"`
}

// HistoryModule é o estado de um módulo da empresa numa competência.
type HistoryModule struct {
	Module   string              `json:"module"`
	PeriodID string              `json:"period_id"`
	State    string              `json:"state"`
	ClosedAt *time.Time          `json:"closed_at"`
	Progress obligation.Progress `json:"progress"`
	Events   []HistoryEvent      `json:"events"`
}

// HistoryCompetence é uma linha da matriz competência × módulo.
type HistoryCompetence struct {
	Competence comp.Competence `json:"competence"`
	TaxRegime  string          `json:"tax_regime"`
	Modules    []HistoryModule `json:"modules"`
}

// CompanyHistory é a resposta de GET /companies/{id}/history.
type CompanyHistory struct {
	CompanyID   string              `json:"company_id"`
	Competences []HistoryCompetence `json:"competences"`
}

// History devolve as competências da empresa (mais recente primeiro) com o estado, a conclusão
// e os eventos de cada módulo [PRD §27.3, §41.1].
func (s *Service) History(ctx context.Context, companyID string) (CompanyHistory, error) {
	if _, err := s.Get(ctx, companyID); err != nil {
		return CompanyHistory{}, err
	}
	periods, err := s.q.CompanyHistoryPeriods(ctx, companyID)
	if err != nil {
		return CompanyHistory{}, err
	}
	events, err := s.q.CompanyHistoryEvents(ctx, companyID)
	if err != nil {
		return CompanyHistory{}, err
	}
	byPeriod := map[string][]HistoryEvent{}
	for _, e := range events {
		byPeriod[e.CompanyModulePeriodID] = append(byPeriod[e.CompanyModulePeriodID],
			HistoryEvent{EventType: e.EventType, Reason: e.Reason, UserName: e.UserName, CreatedAt: e.CreatedAt})
	}
	out := CompanyHistory{CompanyID: companyID, Competences: []HistoryCompetence{}}
	for _, p := range periods {
		c, err := comp.Parse(p.CompetenceKey)
		if err != nil {
			return CompanyHistory{}, err
		}
		if n := len(out.Competences); n == 0 || out.Competences[n-1].Competence != c {
			out.Competences = append(out.Competences, HistoryCompetence{Competence: c, TaxRegime: p.TaxRegime, Modules: []HistoryModule{}})
		}
		evs := byPeriod[p.PeriodID]
		if evs == nil {
			evs = []HistoryEvent{}
		}
		last := &out.Competences[len(out.Competences)-1]
		last.Modules = append(last.Modules, HistoryModule{
			Module: p.ModuleCode, PeriodID: p.PeriodID, State: p.State, ClosedAt: p.ClosedAt,
			Progress: obligation.ProgressFromCounts(int(p.Denominator), int(p.OperationalDone), int(p.FinalDone), int(p.Blocking)),
			Events:   evs,
		})
	}
	return out, nil
}

func (h *Handler) history(w http.ResponseWriter, r *http.Request) {
	id, err := companyID(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	out, err := h.svc.History(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}
