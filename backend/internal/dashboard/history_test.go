package dashboard_test

import (
	"math"
	"net/http"
	"testing"
)

type historyPoint struct {
	Competence      string  `json:"competence"`
	Applicable      int     `json:"applicable"`
	Done            int     `json:"done"`
	FinalCompletion float64 `json:"final_completion"`
	Overdue         int     `json:"overdue"`
	Pending         int     `json:"pending"`
}

func TestHistory_ComparesWithPreviousCompetence(t *testing.T) {
	srv := server(t)
	var h struct {
		Current  historyPoint  `json:"current"`
		Previous *historyPoint `json:"previous"`
		Delta    *struct {
			FinalCompletion float64 `json:"final_completion"`
			Overdue         int     `json:"overdue"`
			Pending         int     `json:"pending"`
		} `json:"delta"`
		Series []historyPoint `json:"series"`
	}
	srv.Get("/api/v1/dashboard/history?competence=2026-10").MustStatus(t, http.StatusOK).JSON(t, &h)

	// Seed demo: 07, 08, 09 e 10/2026, em ordem.
	if len(h.Series) != 4 || h.Series[0].Competence != "2026-07" || h.Series[3].Competence != "2026-10" {
		t.Fatalf("série = %+v", h.Series)
	}
	if h.Previous == nil || h.Previous.Competence != "2026-09" || h.Current.Competence != "2026-10" {
		t.Fatalf("atual %+v anterior %+v", h.Current, h.Previous)
	}
	// Cada ponto bate com o dashboard da competência.
	for _, p := range h.Series {
		d := get(t, srv, "competence="+p.Competence)
		if p.Applicable != d.KPIs.Applicable.Value || p.Done != d.KPIs.Done.Value || p.Overdue != d.KPIs.Overdue.Value ||
			p.Pending != d.KPIs.Pending.Value || math.Abs(p.FinalCompletion-d.KPIs.FinalCompletion) > 1e-9 {
			t.Fatalf("%s: histórico %+v × dashboard %+v", p.Competence, p, d.KPIs.countsBody)
		}
	}
	// Competências anteriores do seed estão mais concluídas que a corrente.
	if h.Previous.FinalCompletion <= h.Current.FinalCompletion {
		t.Fatalf("conclusão anterior %.2f ≤ atual %.2f", h.Previous.FinalCompletion, h.Current.FinalCompletion)
	}
	if h.Delta == nil || math.Abs(h.Delta.FinalCompletion-(h.Current.FinalCompletion-h.Previous.FinalCompletion)) > 1e-9 ||
		h.Delta.Pending != h.Current.Pending-h.Previous.Pending || h.Delta.Overdue != h.Current.Overdue-h.Previous.Overdue {
		t.Fatalf("delta = %+v", h.Delta)
	}

	// A mais antiga não tem anterior para comparar.
	var first struct {
		Previous *historyPoint  `json:"previous"`
		Delta    any            `json:"delta"`
		Series   []historyPoint `json:"series"`
	}
	srv.Get("/api/v1/dashboard/history?competence=2026-07").MustStatus(t, http.StatusOK).JSON(t, &first)
	if first.Previous != nil || first.Delta != nil || len(first.Series) != 1 {
		t.Fatalf("07/2026 = %+v", first)
	}
	srv.Get("/api/v1/dashboard/history?competence=2027-01").MustStatus(t, http.StatusNotFound)
}
