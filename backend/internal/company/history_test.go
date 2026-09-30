package company_test

import (
	"context"
	"net/http"
	"testing"

	"controle/internal/testutil"
	"controle/internal/testutil/seeded"
	"controle/seed"
)

func TestCompanyHistory_ShowsReopenEvents(t *testing.T) {
	srv := testutil.NewServer(t, seeded.NewDemoDB(t), seeded.DemoClock())
	var companyID, key, module string
	if err := srv.Pool.QueryRow(context.Background(), `SELECT p.company_id, k.competence_key, p.module_code
		FROM company_module_period_events e JOIN company_module_periods p ON p.id = e.company_module_period_id
		JOIN competencies k ON k.id = p.competence_id WHERE e.event_type = 'REOPENED' LIMIT 1`).Scan(&companyID, &key, &module); err != nil {
		t.Fatal(err)
	}
	var h struct {
		CompanyID   string `json:"company_id"`
		Competences []struct {
			Competence string `json:"competence"`
			TaxRegime  string `json:"tax_regime"`
			Modules    []struct {
				Module   string `json:"module"`
				PeriodID string `json:"period_id"`
				State    string `json:"state"`
				Progress struct {
					FinalCompletion float64 `json:"final_completion"`
				} `json:"progress"`
				Events []struct {
					EventType string  `json:"event_type"`
					Reason    *string `json:"reason"`
					UserName  string  `json:"user_name"`
				} `json:"events"`
			} `json:"modules"`
		} `json:"competences"`
	}
	srv.Get("/api/v1/companies/"+companyID+"/history").MustStatus(t, http.StatusOK).JSON(t, &h)
	if len(h.Competences) != 4 || h.Competences[0].Competence != "2026-10" || h.Competences[3].Competence != "2026-07" {
		t.Fatalf("competências = %+v", h.Competences)
	}
	found := false
	for _, c := range h.Competences {
		if len(c.Modules) == 0 || c.TaxRegime == "" {
			t.Fatalf("competência %s sem módulos ou regime", c.Competence)
		}
		for _, m := range c.Modules {
			if m.PeriodID == "" || (m.State != "OPEN" && m.State != "CLOSED") || len(m.Events) == 0 || m.Events[0].EventType != "OPENED" {
				t.Fatalf("%s %s = %+v", c.Competence, m.Module, m)
			}
			if c.Competence != key || m.Module != module {
				continue
			}
			var seq []string
			for _, e := range m.Events {
				seq = append(seq, e.EventType)
			}
			// CLOSED → REOPENED com o motivo, na ordem [T-M5-08].
			for i := 1; i < len(m.Events); i++ {
				if m.Events[i].EventType == "REOPENED" && m.Events[i-1].EventType == "CLOSED" &&
					m.Events[i].Reason != nil && *m.Events[i].Reason == seed.ReopenReason && m.Events[i].UserName != "" {
					found = true
				}
			}
			if !found {
				t.Fatalf("eventos de %s %s = %v", key, module, seq)
			}
		}
	}
	if !found {
		t.Fatalf("período reaberto %s/%s não encontrado no histórico", key, module)
	}
	srv.Get("/api/v1/companies/00000000-0000-0000-0000-00000000abcd/history").MustStatus(t, http.StatusNotFound)
}
