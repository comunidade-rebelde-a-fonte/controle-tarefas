package obligation_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestCA010_ContabilDependency(t *testing.T) {
	srv := server(t)
	change := func(typeCode, status string) (int, string) {
		id, v := instanceID(t, srv, "101", typeCode)
		res := srv.Patch("/api/v1/obligation-instances/"+id+"/status", map[string]any{"status_code": status, "version": v})
		return res.Status, string(res.Body)
	}
	for _, step := range []string{"ACCOUNTING_MOVEMENT_RECEIVED", "ACCOUNTING_PAYROLL_POSTED", "ACCOUNTING_TAXES_POSTED"} {
		if st, body := change(step, "OK"); st != http.StatusOK {
			t.Fatalf("%s OK: %d %s", step, st, body)
		}
	}
	// Contábil digitado/processado = A iniciar → Conciliação = OK é bloqueada [CA-010].
	st, body := change("ACCOUNTING_RECONCILIATION", "OK")
	if st != http.StatusUnprocessableEntity || !strings.Contains(body, "DEPENDENCY_BLOCKED") ||
		!strings.Contains(body, "Não é possível concluir Conciliação. A etapa Contábil digitado/processado ainda não foi concluída.") {
		t.Fatalf("Conciliação OK: %d %s", st, body)
	}
	// Pendente também bloqueia; Sem movimento na Conciliação não exige a etapa anterior.
	if st, _ := change("ACCOUNTING_PROCESSED", "PENDING"); st != http.StatusOK {
		t.Fatal("marcar Pendente deveria ser permitido")
	}
	if st, _ := change("ACCOUNTING_RECONCILIATION", "OK"); st != http.StatusUnprocessableEntity {
		t.Fatal("etapa anterior Pendente deveria bloquear")
	}
	if st, body := change("ACCOUNTING_PROCESSED", "NO_MOVEMENT"); st != http.StatusOK {
		t.Fatalf("Sem movimento: %d %s", st, body)
	}
	// Sem movimento satisfaz a dependência.
	if st, body := change("ACCOUNTING_RECONCILIATION", "OK"); st != http.StatusOK {
		t.Fatalf("Conciliação OK após Sem movimento: %d %s", st, body)
	}
}

func TestCA010_NotApplicablePrerequisiteIgnored(t *testing.T) {
	srv := server(t)
	// Contabilização da folha não aplicável para a 101.
	if _, err := srv.Pool.Exec(context.Background(), `UPDATE obligation_instances i SET applicable = false
		FROM companies c, obligation_types t WHERE c.id = i.company_id AND t.id = i.obligation_type_id
		AND c.internal_code = '101' AND t.code = 'ACCOUNTING_PAYROLL_POSTED'`); err != nil {
		t.Fatal(err)
	}
	id, v := instanceID(t, srv, "101", "ACCOUNTING_MOVEMENT_RECEIVED")
	srv.Patch("/api/v1/obligation-instances/"+id+"/status", map[string]any{"status_code": "OK", "version": v}).MustStatus(t, http.StatusOK)
	id, v = instanceID(t, srv, "101", "ACCOUNTING_TAXES_POSTED")
	srv.Patch("/api/v1/obligation-instances/"+id+"/status", map[string]any{"status_code": "OK", "version": v}).MustStatus(t, http.StatusOK)
}
