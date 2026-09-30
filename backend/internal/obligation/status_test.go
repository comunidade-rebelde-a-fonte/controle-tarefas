package obligation_test

import (
	"context"
	"net/http"
	"testing"

	"controle/internal/testutil"
	"controle/internal/testutil/seeded"
	"controle/internal/user"
)

// server sobe a API sobre as 20 empresas com a competência 10/2026 gerada (hoje = 01/10/2026).
func server(t *testing.T) *testutil.Server {
	t.Helper()
	pool := seeded.NewCompaniesDB(t)
	srv := testutil.NewServer(t, pool, testutil.FixedClock(t, "2026-10-01T12:00:00Z"))
	srv.Post("/api/v1/competencies/ensure-current", nil).MustStatus(t, http.StatusOK)
	return srv
}

func instanceID(t *testing.T, srv *testutil.Server, company, typeCode string) (id string, version int) {
	t.Helper()
	if err := srv.Pool.QueryRow(context.Background(), `
		SELECT i.id, i.version FROM obligation_instances i JOIN companies c ON c.id = i.company_id
		JOIN obligation_types t ON t.id = i.obligation_type_id WHERE c.internal_code = $1 AND t.code = $2`, company, typeCode).Scan(&id, &version); err != nil {
		t.Fatalf("instância %s/%s: %v", company, typeCode, err)
	}
	return id, version
}

type changeResult struct {
	Instance struct {
		ID          string `json:"id"`
		StatusCode  string `json:"status_code"`
		StatusLabel string `json:"status_label"`
		IsFinal     bool   `json:"is_final"`
		IsOverdue   bool   `json:"is_overdue"`
		HasHistory  bool   `json:"has_history"`
		Version     int    `json:"version"`
	} `json:"instance"`
	Row struct {
		Module   string `json:"module"`
		Progress struct {
			Denominator int     `json:"denominator"`
			FinalDone   int     `json:"final_done"`
			FinalPct    float64 `json:"final_completion"`
		} `json:"progress"`
	} `json:"row"`
}

func TestChangeStatus_UpdatesVersionHistoryAndRow(t *testing.T) {
	srv := server(t)
	id, v := instanceID(t, srv, "101", "FISCAL_ENTRADAS")

	var res changeResult
	srv.Patch("/api/v1/obligation-instances/"+id+"/status", map[string]any{"status_code": "closed", "version": v}).
		MustStatus(t, http.StatusOK).JSON(t, &res)
	if res.Instance.StatusCode != "CLOSED" || res.Instance.StatusLabel != "Fechadas" || !res.Instance.IsFinal || res.Instance.Version != v+1 || !res.Instance.HasHistory {
		t.Fatalf("instância = %+v", res.Instance)
	}
	if res.Row.Module != "FISCAL" || res.Row.Progress.Denominator != 12 || res.Row.Progress.FinalDone != 1 {
		t.Fatalf("agregado da linha = %+v", res.Row)
	}

	var field, oldV, newV, source, uid, at string
	if err := srv.Pool.QueryRow(context.Background(), `SELECT field, old_value, new_value, change_source, user_id, created_at::text
		FROM obligation_status_history WHERE obligation_instance_id = $1`, id).Scan(&field, &oldV, &newV, &source, &uid, &at); err != nil {
		t.Fatal(err)
	}
	if field != "STATUS" || oldV != "NOT_STARTED" || newV != "CLOSED" || source != "INLINE" || uid != user.AdminID || at != "2026-10-01 12:00:00+00" {
		t.Fatalf("histórico = %s %s→%s %s %s %s", field, oldV, newV, source, uid, at)
	}
	var lastBy string
	if err := srv.Pool.QueryRow(context.Background(), `SELECT last_changed_by FROM obligation_instances WHERE id = $1`, id).Scan(&lastBy); err != nil || lastBy != user.AdminID {
		t.Fatalf("last_changed_by = %s (%v)", lastBy, err)
	}
}

func TestChangeStatus_InvalidStatus(t *testing.T) {
	srv := server(t)
	id, v := instanceID(t, srv, "101", "FISCAL_ENTRADAS")
	// "TRANSMITTED" existe em Declarações, não nos controles fiscais.
	if code := srv.Patch("/api/v1/obligation-instances/"+id+"/status", map[string]any{"status_code": "TRANSMITTED", "version": v}).
		MustStatus(t, http.StatusUnprocessableEntity).ErrorCode(t); code != "INVALID_STATUS" {
		t.Fatalf("code = %s", code)
	}
	srv.Patch("/api/v1/obligation-instances/"+id+"/status", map[string]any{"status_code": "CLOSED"}).MustStatus(t, http.StatusBadRequest)
	srv.Patch("/api/v1/obligation-instances/7b0f8d7e-0000-4000-8000-000000000000/status", map[string]any{"status_code": "CLOSED", "version": 1}).MustStatus(t, http.StatusNotFound)

	// Instância não aplicável (IPI da 102) não aceita status.
	na, nv := instanceID(t, srv, "102", "FISCAL_IPI")
	if code := srv.Patch("/api/v1/obligation-instances/"+na+"/status", map[string]any{"status_code": "CLOSED", "version": nv}).
		MustStatus(t, http.StatusUnprocessableEntity).ErrorCode(t); code != "INSTANCE_NOT_APPLICABLE" {
		t.Fatalf("code = %s", code)
	}
}

func TestChangeStatus_VersionConflict(t *testing.T) {
	srv := server(t)
	id, v := instanceID(t, srv, "101", "FISCAL_SAIDAS")
	srv.Patch("/api/v1/obligation-instances/"+id+"/status", map[string]any{"status_code": "DOWNLOADED", "version": v}).MustStatus(t, http.StatusOK)
	if code := srv.Patch("/api/v1/obligation-instances/"+id+"/status", map[string]any{"status_code": "POSTED", "version": v}).
		MustStatus(t, http.StatusConflict).ErrorCode(t); code != "VERSION_CONFLICT" {
		t.Fatalf("code = %s", code)
	}
	var n int
	if err := srv.Pool.QueryRow(context.Background(), `SELECT count(*) FROM obligation_status_history WHERE obligation_instance_id = $1`, id).Scan(&n); err != nil || n != 1 {
		t.Fatalf("históricos = %d (%v), want 1", n, err)
	}
}

func TestChangeStatus_ClosedPeriodIsLocked(t *testing.T) {
	srv := server(t)
	id, v := instanceID(t, srv, "101", "FISCAL_PIS")
	if _, err := srv.Pool.Exec(context.Background(), `
		UPDATE company_module_periods p SET state = 'CLOSED', closed_at = now(), closed_by = $1
		FROM companies c WHERE c.id = p.company_id AND c.internal_code = '101' AND p.module_code = 'FISCAL'`, user.AdminID); err != nil {
		t.Fatal(err)
	}
	if code := srv.Patch("/api/v1/obligation-instances/"+id+"/status", map[string]any{"status_code": "CLOSED", "version": v}).
		MustStatus(t, http.StatusLocked).ErrorCode(t); code != "PERIOD_CLOSED" {
		t.Fatalf("code = %s", code)
	}
	// Outros módulos da mesma empresa continuam editáveis (RN-006).
	other, ov := instanceID(t, srv, "101", "MIT")
	srv.Patch("/api/v1/obligation-instances/"+other+"/status", map[string]any{"status_code": "TRANSMITTED", "version": ov}).MustStatus(t, http.StatusOK)
}
