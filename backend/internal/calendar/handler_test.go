package calendar_test

import (
	"net/http"
	"testing"

	"controle/internal/testutil"
	"controle/internal/testutil/seeded"
)

func newServer(t *testing.T) *testutil.Server {
	t.Helper()
	return testutil.NewServer(t, seeded.NewDB(t), testutil.FixedClock(t, "2026-10-01T12:00:00Z"))
}

type ruleDTO struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	BaseDay     int    `json:"base_day"`
	DayType     string `json:"day_type"`
	MonthOffset int    `json:"month_offset"`
	Version     int    `json:"version"`
}

func TestDueDateRules_CRUDAndValidation(t *testing.T) {
	srv := newServer(t)
	var list struct{ Items []ruleDTO }
	srv.Get("/api/v1/due-date-rules").MustStatus(t, http.StatusOK).JSON(t, &list)
	if len(list.Items) != 6 {
		t.Fatalf("regras = %d, want 6 (seed provisório)", len(list.Items))
	}

	payload := map[string]any{"name": "Dia 7 do mês seguinte", "month_offset": 1, "base_day": 7, "day_type": "CALENDAR", "non_business_day_adjustment": "NEXT"}
	var r ruleDTO
	srv.Post("/api/v1/due-date-rules", payload).MustStatus(t, http.StatusCreated).JSON(t, &r)
	srv.Post("/api/v1/due-date-rules", payload).MustStatus(t, http.StatusConflict)

	payload["version"] = r.Version
	payload["base_day"] = 12
	payload["day_type"] = "BUSINESS"
	var upd ruleDTO
	srv.Patch("/api/v1/due-date-rules/"+r.ID, payload).MustStatus(t, http.StatusOK).JSON(t, &upd)
	if upd.BaseDay != 12 || upd.DayType != "BUSINESS" || upd.Version != 2 {
		t.Fatalf("regra atualizada = %+v", upd)
	}
	// Reler devolve o valor atualizado.
	srv.Get("/api/v1/due-date-rules").MustStatus(t, http.StatusOK).JSON(t, &list)
	for _, x := range list.Items {
		if x.ID == r.ID && x.BaseDay != 12 {
			t.Fatalf("releitura = %+v", x)
		}
	}
	srv.Patch("/api/v1/due-date-rules/"+r.ID, payload).MustStatus(t, http.StatusConflict) // versão antiga

	for name, mutate := range map[string]func(map[string]any){
		"dia 32":         func(p map[string]any) { p["base_day"] = 32 },
		"offset 13":      func(p map[string]any) { p["month_offset"] = 13 },
		"tipo de dia":    func(p map[string]any) { p["day_type"] = "UTIL" },
		"ajuste":         func(p map[string]any) { p["non_business_day_adjustment"] = "NEAREST" },
		"nome em branco": func(p map[string]any) { p["name"] = " " },
	} {
		p := map[string]any{"name": "Outra " + name, "month_offset": 1, "base_day": 5, "day_type": "CALENDAR", "non_business_day_adjustment": "NONE"}
		mutate(p)
		if st := srv.Post("/api/v1/due-date-rules", p).Status; st != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", name, st)
		}
	}
}

func TestHolidays_StateAndMunicipalCalendars(t *testing.T) {
	srv := newServer(t)
	var h struct {
		ID           string `json:"id"`
		CalendarName string `json:"calendar_name"`
		Scope        string `json:"scope"`
	}
	srv.Post("/api/v1/holidays", map[string]any{"scope": "STATE", "state": "pr", "date": "2026-12-19", "name": "Emancipação do Paraná"}).
		MustStatus(t, http.StatusCreated).JSON(t, &h)
	if h.CalendarName != "UF PR" || h.Scope != "STATE" {
		t.Fatalf("feriado = %+v", h)
	}
	srv.Post("/api/v1/holidays", map[string]any{"scope": "STATE", "state": "PR", "date": "2026-12-19", "name": "Dup"}).MustStatus(t, http.StatusConflict)
	srv.Post("/api/v1/holidays", map[string]any{"scope": "MUNICIPAL", "state": "PR", "city": "Curitiba", "date": "2026-09-08", "name": "Nossa Senhora da Luz"}).
		MustStatus(t, http.StatusCreated)
	srv.Post("/api/v1/holidays", map[string]any{"scope": "MUNICIPAL", "state": "PR", "date": "2026-09-08", "name": "Sem cidade"}).MustStatus(t, http.StatusBadRequest)

	var cals struct {
		Items []struct {
			Scope        string `json:"scope"`
			HolidayCount int    `json:"holiday_count"`
		}
	}
	srv.Get("/api/v1/calendars").MustStatus(t, http.StatusOK).JSON(t, &cals)
	if len(cals.Items) != 3 || cals.Items[0].Scope != "NATIONAL" || cals.Items[0].HolidayCount != 78 {
		t.Fatalf("calendários = %+v", cals.Items)
	}

	var hols struct {
		Items []struct {
			ID    string `json:"id"`
			Scope string `json:"scope"`
			Date  string `json:"date"`
		}
	}
	srv.Get("/api/v1/holidays?year=2026").MustStatus(t, http.StatusOK).JSON(t, &hols)
	if len(hols.Items) != 15 {
		t.Fatalf("feriados de 2026 = %d, want 13 nacionais + 2", len(hols.Items))
	}
	srv.Do(http.MethodDelete, "/api/v1/holidays/"+h.ID, nil).MustStatus(t, http.StatusNoContent)
	for _, x := range hols.Items {
		if x.Scope == "NATIONAL" {
			srv.Do(http.MethodDelete, "/api/v1/holidays/"+x.ID, nil).MustStatus(t, http.StatusUnprocessableEntity)
			break
		}
	}
}

func TestCredentialThresholds(t *testing.T) {
	srv := newServer(t)
	var th struct{ Days []int }
	srv.Get("/api/v1/settings/credential-thresholds").MustStatus(t, http.StatusOK).JSON(t, &th)
	if len(th.Days) != 4 || th.Days[0] != 7 || th.Days[3] != 60 {
		t.Fatalf("faixas = %v", th.Days)
	}
	srv.Put("/api/v1/settings/credential-thresholds", map[string]any{"days": []int{5, 10, 20, 45, 90}}).MustStatus(t, http.StatusOK)
	srv.Get("/api/v1/settings/credential-thresholds").MustStatus(t, http.StatusOK).JSON(t, &th)
	if len(th.Days) != 5 || th.Days[4] != 90 {
		t.Fatalf("faixas = %v", th.Days)
	}
	for _, bad := range [][]int{{10, 5}, {}, {0, 7}, {7, 7}} {
		srv.Put("/api/v1/settings/credential-thresholds", map[string]any{"days": bad}).MustStatus(t, http.StatusBadRequest)
	}
}
