package period_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"controle/internal/platform/clock"
	"controle/internal/testutil"
	"controle/internal/testutil/seeded"
	"controle/internal/user"
)

// server sobe a API sobre as 20 empresas com a competência 10/2026 gerada (hoje = 01/10/2026).
func server(t *testing.T) (*testutil.Server, *clock.Fixed) {
	t.Helper()
	pool := seeded.NewCompaniesDB(t)
	clk := testutil.FixedClock(t, "2026-10-01T12:00:00Z")
	srv := testutil.NewServer(t, pool, clk)
	srv.Post("/api/v1/competencies/ensure-current", nil).MustStatus(t, http.StatusOK)
	return srv, clk
}

func at(t *testing.T, s string) time.Time {
	t.Helper()
	ts, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatal(err)
	}
	return ts
}

// periods devolve os períodos da empresa na competência gerada, por módulo.
func periods(t *testing.T, srv *testutil.Server, company string) map[string]string {
	t.Helper()
	rows, err := srv.Pool.Query(context.Background(), `SELECT p.module_code, p.id FROM company_module_periods p
		JOIN companies c ON c.id = p.company_id WHERE c.internal_code = $1`, company)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var m, id string
		if err := rows.Scan(&m, &id); err != nil {
			t.Fatal(err)
		}
		out[m] = id
	}
	return out
}

type periodBody struct {
	ID         string  `json:"id"`
	Competence string  `json:"competence"`
	Module     string  `json:"module"`
	State      string  `json:"state"`
	ClosedAt   *string `json:"closed_at"`
	ClosedBy   *struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"closed_by"`
	GeneralNote *string `json:"general_note"`
	Version     int     `json:"version"`
}

type eventsBody struct {
	Items []struct {
		EventType string  `json:"event_type"`
		Reason    *string `json:"reason"`
		User      struct {
			ID string `json:"id"`
		} `json:"user"`
		CreatedAt string `json:"created_at"`
	} `json:"items"`
}

type errorBody struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details"`
}

func state(t *testing.T, srv *testutil.Server, id string) string {
	t.Helper()
	var s string
	if err := srv.Pool.QueryRow(context.Background(), `SELECT state FROM company_module_periods WHERE id = $1`, id).Scan(&s); err != nil {
		t.Fatal(err)
	}
	return s
}

func instance(t *testing.T, srv *testutil.Server, company, typeCode string) (id, status string, version int) {
	t.Helper()
	if err := srv.Pool.QueryRow(context.Background(), `SELECT i.id, i.status_code, i.version FROM obligation_instances i
		JOIN companies c ON c.id = i.company_id JOIN obligation_types t ON t.id = i.obligation_type_id
		WHERE c.internal_code = $1 AND t.code = $2`, company, typeCode).Scan(&id, &status, &version); err != nil {
		t.Fatal(err)
	}
	return id, status, version
}

func TestCA013_IndependentClose(t *testing.T) {
	srv, clk := server(t)
	ps := periods(t, srv, "101")
	if len(ps) != 4 {
		t.Fatalf("a 101 deveria ter os 4 módulos: %v", ps)
	}
	fiscal := ps["FISCAL"]

	// Fechamento (coluna) ainda "A iniciar": sem confirmação → 409 CLOSING_STATUS_REQUIRED [§13.6].
	var e errorBody
	srv.Post("/api/v1/company-module-periods/"+fiscal+"/close", map[string]any{"version": 1}).
		MustStatus(t, http.StatusConflict).JSON(t, &e)
	if e.Code != "CLOSING_STATUS_REQUIRED" || e.Message != "Fechamento está como A iniciar. Confirme para marcá-lo como Encerrado e fechar a competência." ||
		e.Details["target_status_code"] != "CLOSED" {
		t.Fatalf("erro = %+v", e)
	}
	if state(t, srv, fiscal) != "OPEN" {
		t.Fatal("o período não deveria fechar sem confirmação")
	}

	// Com a confirmação, o Fechamento vira Encerrado na mesma transação e o Fiscal fecha.
	clk.Advance(time.Hour)
	var p periodBody
	srv.Post("/api/v1/company-module-periods/"+fiscal+"/close", map[string]any{"version": 1, "confirm_set_closing": true}).
		MustStatus(t, http.StatusOK).JSON(t, &p)
	if p.State != "CLOSED" || p.Module != "FISCAL" || p.Competence != "2026-10" || p.Version != 2 ||
		p.ClosedAt == nil || p.ClosedBy == nil || p.ClosedBy.ID != user.AdminID {
		t.Fatalf("período = %+v", p)
	}
	closingID, st, _ := instance(t, srv, "101", "FISCAL_FECHAMENTO")
	if st != "CLOSED" {
		t.Fatalf("Fechamento = %s", st)
	}
	var hist int
	if err := srv.Pool.QueryRow(context.Background(), `SELECT count(*) FROM obligation_status_history
		WHERE obligation_instance_id = $1 AND old_value = 'NOT_STARTED' AND new_value = 'CLOSED'`, closingID).Scan(&hist); err != nil || hist != 1 {
		t.Fatalf("histórico do Fechamento = %d (%v)", hist, err)
	}

	// Os outros 3 módulos da empresa e o Fiscal das outras empresas continuam abertos [CA-013].
	for _, m := range []string{"DECLARATIONS", "PAYROLL", "ACCOUNTING"} {
		if s := state(t, srv, ps[m]); s != "OPEN" {
			t.Fatalf("%s = %s", m, s)
		}
	}
	if s := state(t, srv, periods(t, srv, "102")["FISCAL"]); s != "OPEN" {
		t.Fatalf("Fiscal da 102 = %s", s)
	}

	// Evento CLOSED com usuário e data/hora.
	var ev eventsBody
	srv.Get("/api/v1/company-module-periods/"+fiscal+"/events").MustStatus(t, http.StatusOK).JSON(t, &ev)
	if len(ev.Items) != 2 || ev.Items[0].EventType != "CLOSED" || ev.Items[0].User.ID != user.AdminID ||
		!at(t, ev.Items[0].CreatedAt).Equal(at(t, "2026-10-01T13:00:00Z")) || ev.Items[1].EventType != "OPENED" {
		t.Fatalf("eventos = %+v", ev.Items)
	}

	// Fiscal fechado trava as instâncias do Fiscal (423); o Contábil segue editável.
	id, _, v := instance(t, srv, "101", "FISCAL_ENTRADAS")
	srv.Patch("/api/v1/obligation-instances/"+id+"/status", map[string]any{"status_code": "CLOSED", "version": v}).MustStatus(t, http.StatusLocked)
	id, _, v = instance(t, srv, "101", "ACCOUNTING_MOVEMENT_RECEIVED")
	srv.Patch("/api/v1/obligation-instances/"+id+"/status", map[string]any{"status_code": "OK", "version": v}).MustStatus(t, http.StatusOK)

	// Fechar de novo → 422; versão desatualizada → 409.
	srv.Post("/api/v1/company-module-periods/"+fiscal+"/close", map[string]any{"version": 2}).MustStatus(t, http.StatusUnprocessableEntity)
	srv.Post("/api/v1/company-module-periods/"+ps["PAYROLL"]+"/close", map[string]any{"version": 7}).MustStatus(t, http.StatusConflict)
}

func TestClose_ModuleWithoutClosingStage(t *testing.T) {
	srv, _ := server(t)
	acc := periods(t, srv, "101")["ACCOUNTING"]
	var p periodBody
	srv.Post("/api/v1/company-module-periods/"+acc+"/close", map[string]any{"version": 1}).MustStatus(t, http.StatusOK).JSON(t, &p)
	if p.State != "CLOSED" {
		t.Fatalf("período = %+v", p)
	}
}

func TestClose_ClosingStageAlreadyFinal(t *testing.T) {
	srv, _ := server(t)
	id, _, v := instance(t, srv, "101", "FISCAL_FECHAMENTO")
	srv.Patch("/api/v1/obligation-instances/"+id+"/status", map[string]any{"status_code": "CLOSED", "version": v}).MustStatus(t, http.StatusOK)
	fiscal := periods(t, srv, "101")["FISCAL"]
	srv.Post("/api/v1/company-module-periods/"+fiscal+"/close", map[string]any{"version": 1}).MustStatus(t, http.StatusOK)
}

func TestCA015_ReopenRequiresReason(t *testing.T) {
	srv, clk := server(t)
	fiscal := periods(t, srv, "101")["FISCAL"]
	url := "/api/v1/company-module-periods/" + fiscal

	// Reabrir período aberto → 422.
	srv.Post(url+"/reopen", map[string]any{"reason": "Correção"}).MustStatus(t, http.StatusUnprocessableEntity)
	clk.Advance(time.Hour)
	srv.Post(url+"/close", map[string]any{"version": 1, "confirm_set_closing": true}).MustStatus(t, http.StatusOK)

	// Sem motivo (ausente ou em branco) → 422 REASON_REQUIRED [CA-015].
	for _, body := range []map[string]any{{}, {"reason": "   "}} {
		var e errorBody
		srv.Post(url+"/reopen", body).MustStatus(t, http.StatusUnprocessableEntity).JSON(t, &e)
		if e.Code != "REASON_REQUIRED" || !strings.Contains(e.Message, "motivo") {
			t.Fatalf("erro = %+v", e)
		}
	}
	if state(t, srv, fiscal) != "CLOSED" {
		t.Fatal("reabertura sem motivo não pode alterar o estado")
	}

	reason := "Correção de obrigação transmitida com informação incorreta"
	clk.Advance(time.Hour)
	var p periodBody
	srv.Post(url+"/reopen", map[string]any{"reason": reason}).MustStatus(t, http.StatusOK).JSON(t, &p)
	if p.State != "OPEN" || p.ClosedAt != nil || p.ClosedBy != nil || p.Version != 3 {
		t.Fatalf("período = %+v", p)
	}

	var ev eventsBody
	srv.Get(url+"/events").MustStatus(t, http.StatusOK).JSON(t, &ev)
	if len(ev.Items) != 3 || ev.Items[0].EventType != "REOPENED" || ev.Items[0].Reason == nil || *ev.Items[0].Reason != reason ||
		ev.Items[0].User.ID != user.AdminID || !at(t, ev.Items[0].CreatedAt).Equal(at(t, "2026-10-01T14:00:00Z")) || ev.Items[1].EventType != "CLOSED" {
		t.Fatalf("eventos = %+v", ev.Items)
	}

	// Reaberto, as instâncias voltam a ser editáveis.
	id, _, v := instance(t, srv, "101", "FISCAL_ENTRADAS")
	srv.Patch("/api/v1/obligation-instances/"+id+"/status", map[string]any{"status_code": "CLOSED", "version": v}).MustStatus(t, http.StatusOK)
}

func TestNote(t *testing.T) {
	srv, _ := server(t)
	url := "/api/v1/company-module-periods/" + periods(t, srv, "101")["FISCAL"]

	var p periodBody
	srv.Patch(url+"/note", map[string]any{"version": 1, "general_note": "  Aguardando documentação de retenções.  "}).
		MustStatus(t, http.StatusOK).JSON(t, &p)
	if p.GeneralNote == nil || *p.GeneralNote != "Aguardando documentação de retenções." || p.Version != 2 {
		t.Fatalf("período = %+v", p)
	}
	srv.Patch(url+"/note", map[string]any{"version": 1, "general_note": "x"}).MustStatus(t, http.StatusConflict)
	srv.Patch(url+"/note", map[string]any{"version": 2, "general_note": strings.Repeat("a", 2001)}).MustStatus(t, http.StatusBadRequest)

	p = periodBody{}
	srv.Patch(url+"/note", map[string]any{"version": 2, "general_note": " "}).MustStatus(t, http.StatusOK).JSON(t, &p)
	if p.GeneralNote != nil {
		t.Fatalf("observação deveria ser limpa: %+v", p)
	}
	srv.Get(url).MustStatus(t, http.StatusOK)
	srv.Get("/api/v1/company-module-periods/00000000-0000-0000-0000-00000000abcd").MustStatus(t, http.StatusNotFound)
	srv.Get("/api/v1/company-module-periods/xyz/events").MustStatus(t, http.StatusNotFound)
}
