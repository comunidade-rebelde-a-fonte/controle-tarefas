package obligation_test

import (
	"context"
	"net/http"
	"testing"

	"controle/internal/testutil"
)

type bulkResult struct {
	DryRun   bool `json:"dry_run"`
	Selected int  `json:"selected"`
	Updated  int  `json:"updated"`
	Skipped  []struct {
		ID          string `json:"id"`
		CompanyCode string `json:"company_code"`
		Reason      string `json:"reason"`
		Message     string `json:"message"`
	} `json:"skipped"`
	SoftWarnings int `json:"soft_warnings"`
}

// typeInstances devolve as instâncias aplicáveis de um tipo na competência gerada, por código da empresa.
func typeInstances(t *testing.T, srv *testutil.Server, typeCode string) map[string]string {
	t.Helper()
	rows, err := srv.Pool.Query(context.Background(), `SELECT c.internal_code, i.id FROM obligation_instances i
		JOIN companies c ON c.id = i.company_id JOIN obligation_types t ON t.id = i.obligation_type_id
		WHERE t.code = $1 AND i.applicable ORDER BY c.internal_code`, typeCode)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var code, id string
		if err := rows.Scan(&code, &id); err != nil {
			t.Fatal(err)
		}
		out[code] = id
	}
	return out
}

func values(m map[string]string, except ...string) []string {
	var out []string
	for k, v := range m {
		skip := false
		for _, e := range except {
			skip = skip || k == e
		}
		if !skip {
			out = append(out, v)
		}
	}
	return out
}

func bulkHistory(t *testing.T, srv *testutil.Server, typeCode string) int {
	t.Helper()
	var n int
	if err := srv.Pool.QueryRow(context.Background(), `SELECT count(*) FROM obligation_status_history h
		JOIN obligation_instances i ON i.id = h.obligation_instance_id JOIN obligation_types t ON t.id = i.obligation_type_id
		WHERE t.code = $1 AND h.change_source = 'BULK' AND h.field = 'STATUS'`, typeCode).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestCA016_BulkUpdate(t *testing.T) {
	srv := server(t)

	// 20 selecionadas geram 20 históricos individuais com origem BULK [CA-016].
	entradas := typeInstances(t, srv, "FISCAL_ENTRADAS")
	if len(entradas) != 20 {
		t.Fatalf("esperava 20 instâncias, veio %d", len(entradas))
	}
	var res bulkResult
	srv.Post("/api/v1/obligation-instances/bulk-status", map[string]any{"instance_ids": values(entradas), "status_code": "CLOSED"}).
		MustStatus(t, http.StatusOK).JSON(t, &res)
	if res.Selected != 20 || res.Updated != 20 || len(res.Skipped) != 0 || res.DryRun {
		t.Fatalf("lote = %+v", res)
	}
	if n := bulkHistory(t, srv, "FISCAL_ENTRADAS"); n != 20 {
		t.Fatalf("históricos BULK = %d", n)
	}
	var closed, version2 int
	if err := srv.Pool.QueryRow(context.Background(), `SELECT count(*) FILTER (WHERE i.status_code = 'CLOSED'), count(*) FILTER (WHERE i.version = 2)
		FROM obligation_instances i JOIN obligation_types t ON t.id = i.obligation_type_id WHERE t.code = 'FISCAL_ENTRADAS'`).Scan(&closed, &version2); err != nil {
		t.Fatal(err)
	}
	if closed != 20 || version2 != 20 {
		t.Fatalf("instâncias CLOSED = %d, versão 2 = %d", closed, version2)
	}

	// Um item bloqueado por dependência HARD é pulado sem abortar o lote.
	movement := typeInstances(t, srv, "ACCOUNTING_MOVEMENT_RECEIVED")
	srv.Post("/api/v1/obligation-instances/bulk-status", map[string]any{"instance_ids": values(movement, "101"), "status_code": "OK"}).
		MustStatus(t, http.StatusOK)
	payroll := typeInstances(t, srv, "ACCOUNTING_PAYROLL_POSTED")
	res = bulkResult{}
	srv.Post("/api/v1/obligation-instances/bulk-status", map[string]any{"instance_ids": values(payroll), "status_code": "OK"}).
		MustStatus(t, http.StatusOK).JSON(t, &res)
	if res.Selected != len(payroll) || res.Updated != len(payroll)-1 || len(res.Skipped) != 1 ||
		res.Skipped[0].CompanyCode != "101" || res.Skipped[0].Reason != "DEPENDENCY_BLOCKED" ||
		res.Skipped[0].Message != "Não é possível concluir Contabilização da folha. A etapa Movimento recebido ainda não foi concluída." {
		t.Fatalf("lote com bloqueio = %+v", res)
	}
	if n := bulkHistory(t, srv, "ACCOUNTING_PAYROLL_POSTED"); n != len(payroll)-1 {
		t.Fatalf("históricos BULK da folha = %d", n)
	}
	var st string
	if err := srv.Pool.QueryRow(context.Background(), `SELECT status_code FROM obligation_instances WHERE id = $1`, payroll["101"]).Scan(&st); err != nil || st != "NOT_STARTED" {
		t.Fatalf("instância bloqueada = %s (%v)", st, err)
	}
}

func TestBulkStatus_TargetDryRunAndSkips(t *testing.T) {
	srv := server(t)
	var typeID string
	if err := srv.Pool.QueryRow(context.Background(), `SELECT id FROM obligation_types WHERE code = 'FISCAL_SAIDAS'`).Scan(&typeID); err != nil {
		t.Fatal(err)
	}
	var companies []string
	rows, err := srv.Pool.Query(context.Background(), `SELECT id FROM companies WHERE internal_code IN ('101', '102', '103', '104') ORDER BY internal_code`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		companies = append(companies, id)
	}
	rows.Close()

	// 102: período do Fiscal fechado; 103: não aplicável; 104: já no status.
	ctx := context.Background()
	if _, err := srv.Pool.Exec(ctx, `UPDATE company_module_periods p SET state = 'CLOSED', closed_at = now(), closed_by = '00000000-0000-0000-0000-000000000001'
		FROM companies c WHERE c.id = p.company_id AND c.internal_code = '102' AND p.module_code = 'FISCAL'`); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.Pool.Exec(ctx, `UPDATE obligation_instances i SET applicable = false FROM companies c, obligation_types t
		WHERE c.id = i.company_id AND t.id = i.obligation_type_id AND c.internal_code = '103' AND t.code = 'FISCAL_SAIDAS'`); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.Pool.Exec(ctx, `UPDATE obligation_instances i SET status_code = 'CLOSED' FROM companies c, obligation_types t
		WHERE c.id = i.company_id AND t.id = i.obligation_type_id AND c.internal_code = '104' AND t.code = 'FISCAL_SAIDAS'`); err != nil {
		t.Fatal(err)
	}

	body := map[string]any{"target": map[string]any{"competence": "2026-10", "obligation_type_id": typeID, "company_ids": companies},
		"status_code": "closed", "dry_run": true}
	var res bulkResult
	srv.Post("/api/v1/obligation-instances/bulk-status", body).MustStatus(t, http.StatusOK).JSON(t, &res)
	reasons := map[string]string{}
	for _, s := range res.Skipped {
		reasons[s.CompanyCode] = s.Reason
	}
	if !res.DryRun || res.Selected != 4 || res.Updated != 1 || reasons["102"] != "PERIOD_CLOSED" || reasons["103"] != "NOT_APPLICABLE" || reasons["104"] != "UNCHANGED" {
		t.Fatalf("dry_run = %+v", res)
	}
	if n := bulkHistory(t, srv, "FISCAL_SAIDAS"); n != 0 {
		t.Fatalf("dry_run não deveria gravar: %d históricos", n)
	}

	body["dry_run"] = false
	res = bulkResult{}
	srv.Post("/api/v1/obligation-instances/bulk-status", body).MustStatus(t, http.StatusOK).JSON(t, &res)
	if res.DryRun || res.Updated != 1 || len(res.Skipped) != 3 {
		t.Fatalf("lote = %+v", res)
	}
	if n := bulkHistory(t, srv, "FISCAL_SAIDAS"); n != 1 {
		t.Fatalf("históricos = %d", n)
	}

	// Status que não pertence ao conjunto da obrigação é pulado.
	res = bulkResult{}
	body["status_code"] = "TRANSMITTED"
	srv.Post("/api/v1/obligation-instances/bulk-status", body).MustStatus(t, http.StatusOK).JSON(t, &res)
	if res.Updated != 0 || len(res.Skipped) != 4 || res.Skipped[0].Reason != "INVALID_STATUS" {
		t.Fatalf("status inválido = %+v", res)
	}
}

func TestBulkStatus_SoftRequiresException(t *testing.T) {
	srv := server(t)
	dctf := typeInstances(t, srv, "DCTFWEB")
	body := map[string]any{"instance_ids": values(dctf), "status_code": "TRANSMITTED"}

	var e errorBody
	srv.Post("/api/v1/obligation-instances/bulk-status", body).MustStatus(t, http.StatusConflict).JSON(t, &e)
	if e.Code != "DEPENDENCY_WARNING" {
		t.Fatalf("alerta = %+v", e)
	}
	if n := bulkHistory(t, srv, "DCTFWEB"); n != 0 {
		t.Fatalf("alerta não deveria gravar: %d", n)
	}

	body["confirm_exception"] = true
	srv.Post("/api/v1/obligation-instances/bulk-status", body).MustStatus(t, http.StatusBadRequest)

	reason := "Componentes transmitidos pelo cliente"
	body["exception_reason"] = reason
	var res bulkResult
	srv.Post("/api/v1/obligation-instances/bulk-status", body).MustStatus(t, http.StatusOK).JSON(t, &res)
	if res.Updated != len(dctf) || res.SoftWarnings != len(dctf) {
		t.Fatalf("lote = %+v", res)
	}
	var withReason int
	if err := srv.Pool.QueryRow(context.Background(), `SELECT count(*) FROM obligation_status_history WHERE change_source = 'BULK' AND override_reason = $1`, reason).Scan(&withReason); err != nil || withReason != len(dctf) {
		t.Fatalf("históricos com motivo = %d (%v)", withReason, err)
	}
}

func TestBulkStatus_Validation(t *testing.T) {
	srv := server(t)
	srv.Post("/api/v1/obligation-instances/bulk-status", map[string]any{"status_code": "OK"}).MustStatus(t, http.StatusBadRequest)
	srv.Post("/api/v1/obligation-instances/bulk-status", map[string]any{"instance_ids": []string{"x"}, "status_code": "OK"}).MustStatus(t, http.StatusBadRequest)
	var res bulkResult
	srv.Post("/api/v1/obligation-instances/bulk-status", map[string]any{"instance_ids": []string{"00000000-0000-0000-0000-00000000abcd"}, "status_code": "OK"}).
		MustStatus(t, http.StatusOK).JSON(t, &res)
	if res.Selected != 0 || res.Updated != 0 {
		t.Fatalf("ids inexistentes = %+v", res)
	}
}
