package dashboard

import (
	"context"

	"controle/internal/obligation"
	"controle/internal/platform/clock"
	comp "controle/internal/platform/competence"
	"controle/internal/store"
)

// historyMonths é o tamanho da série do gráfico (a selecionada e as 5 anteriores).
const historyMonths = 6

// HistoryPoint são os números de uma competência na comparação [PRD §24.8].
type HistoryPoint struct {
	Competence          comp.Competence `json:"competence"`
	Applicable          int             `json:"applicable"`
	Done                int             `json:"done"`
	OperationalProgress float64         `json:"operational_progress"`
	FinalCompletion     float64         `json:"final_completion"`
	Overdue             int             `json:"overdue"`
	Pending             int             `json:"pending"`
}

// HistoryDelta é a variação da selecionada em relação à anterior.
type HistoryDelta struct {
	FinalCompletion     float64 `json:"final_completion"`
	OperationalProgress float64 `json:"operational_progress"`
	Overdue             int     `json:"overdue"`
	Pending             int     `json:"pending"`
}

// History é a resposta de GET /dashboard/history.
type History struct {
	Current  HistoryPoint   `json:"current"`
	Previous *HistoryPoint  `json:"previous"`
	Delta    *HistoryDelta  `json:"delta"`
	Series   []HistoryPoint `json:"series"`
}

// History compara a competência selecionada com a imediatamente anterior e devolve a série das
// últimas seis competências geradas; atrasos são contados em relação a hoje [PRD §24.8].
func (s *Service) History(ctx context.Context, c comp.Competence, f Filters) (History, error) {
	q := store.New(s.pool)
	if _, err := competenceID(ctx, q, c); err != nil {
		return History{}, err
	}
	rows, err := q.DashboardHistory(ctx, store.DashboardHistoryParams{
		Today: clock.Today(s.clock), FromKey: c.AddMonths(-(historyMonths - 1)).String(), ToKey: c.String(),
		RegimeGroup: f.RegimeGroup, TaxRegime: f.TaxRegime, ResponsibleID: f.ResponsibleID,
	})
	if err != nil {
		return History{}, err
	}
	out := History{Current: HistoryPoint{Competence: c, OperationalProgress: 1, FinalCompletion: 1}, Series: []HistoryPoint{}}
	prevKey := c.AddMonths(-1).String()
	for _, r := range rows {
		k, err := comp.Parse(r.CompetenceKey)
		if err != nil {
			return History{}, err
		}
		p := obligation.ProgressFromCounts(int(r.Applicable), int(r.OperationalDone), int(r.Done), int(r.Pending))
		pt := HistoryPoint{
			Competence: k, Applicable: int(r.Applicable), Done: int(r.Done), OperationalProgress: p.OperationalPct,
			FinalCompletion: p.FinalPct, Overdue: int(r.Overdue), Pending: int(r.Pending),
		}
		out.Series = append(out.Series, pt)
		switch r.CompetenceKey {
		case c.String():
			out.Current = pt
		case prevKey:
			prev := pt
			out.Previous = &prev
		}
	}
	if out.Previous != nil {
		out.Delta = &HistoryDelta{
			FinalCompletion:     out.Current.FinalCompletion - out.Previous.FinalCompletion,
			OperationalProgress: out.Current.OperationalProgress - out.Previous.OperationalProgress,
			Overdue:             out.Current.Overdue - out.Previous.Overdue,
			Pending:             out.Current.Pending - out.Previous.Pending,
		}
	}
	return out, nil
}
