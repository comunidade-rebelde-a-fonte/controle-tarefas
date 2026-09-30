package grid_test

import (
	"context"
	"net/http"
	"testing"

	"controle/internal/testutil"
)

type sharedCell struct {
	ID         string `json:"id"`
	StatusCode string `json:"status_code"`
	Version    int    `json:"version"`
}

func sharedCellOf(t *testing.T, srv *testutil.Server, module, company, typeCode string) sharedCell {
	t.Helper()
	var g struct {
		Rows []struct {
			Company struct {
				InternalCode string `json:"internal_code"`
			} `json:"company"`
			Cells map[string]sharedCell `json:"cells"`
		} `json:"rows"`
	}
	srv.Get("/api/v1/modules/"+module+"/grid?competence=2026-10").MustStatus(t, http.StatusOK).JSON(t, &g)
	for _, r := range g.Rows {
		if r.Company.InternalCode == company {
			c, ok := r.Cells[typeCode]
			if !ok {
				t.Fatalf("%s sem %s na grade %s", company, typeCode, module)
			}
			return c
		}
	}
	t.Fatalf("empresa %s fora da grade %s", company, module)
	return sharedCell{}
}

// assertNoDuplicates é a auditoria do RN-009: nenhuma combinação empresa + competência + tipo
// aparece mais de uma vez.
func assertNoDuplicates(t *testing.T, srv *testutil.Server) {
	t.Helper()
	var dups int
	if err := srv.Pool.QueryRow(context.Background(), `SELECT count(*) FROM (
		SELECT company_id, competence_id, obligation_type_id FROM obligation_instances
		GROUP BY 1, 2, 3 HAVING count(*) > 1) d`).Scan(&dups); err != nil {
		t.Fatal(err)
	}
	if dups != 0 {
		t.Fatalf("%d combinações empresa + competência + tipo duplicadas", dups)
	}
}

func TestCA008_MitShared(t *testing.T) {
	srv := server(t)
	fromDeclarations := sharedCellOf(t, srv, "declarations", "101", "MIT")
	srv.Patch("/api/v1/obligation-instances/"+fromDeclarations.ID+"/status", map[string]any{"status_code": "TRANSMITTED", "version": fromDeclarations.Version}).
		MustStatus(t, http.StatusOK)

	// A DCTFWeb lê a mesma instância, já com o status novo [CA-008, RN-009].
	fromDctfweb := sharedCellOf(t, srv, "dctfweb", "101", "MIT")
	if fromDctfweb.ID != fromDeclarations.ID || fromDctfweb.StatusCode != "TRANSMITTED" || fromDctfweb.Version != fromDeclarations.Version+1 {
		t.Fatalf("Declarações %+v × DCTFWeb %+v", fromDeclarations, fromDctfweb)
	}
	assertNoDuplicates(t, srv)
}

func TestCA009_EsocialShared(t *testing.T) {
	srv := server(t)
	fromPayroll := sharedCellOf(t, srv, "payroll", "101", "ESOCIAL")
	srv.Patch("/api/v1/obligation-instances/"+fromPayroll.ID+"/status", map[string]any{"status_code": "CLOSED", "version": fromPayroll.Version}).
		MustStatus(t, http.StatusOK)

	fromDctfweb := sharedCellOf(t, srv, "dctfweb", "101", "ESOCIAL")
	if fromDctfweb.ID != fromPayroll.ID || fromDctfweb.StatusCode != "CLOSED" {
		t.Fatalf("Folha %+v × DCTFWeb %+v", fromPayroll, fromDctfweb)
	}

	// E no sentido inverso: alterar pela DCTFWeb reflete na Folha.
	srv.Patch("/api/v1/obligation-instances/"+fromDctfweb.ID+"/status", map[string]any{"status_code": "PENDING", "version": fromDctfweb.Version}).
		MustStatus(t, http.StatusOK)
	if back := sharedCellOf(t, srv, "payroll", "101", "ESOCIAL"); back.ID != fromPayroll.ID || back.StatusCode != "PENDING" {
		t.Fatalf("Folha após alterar pela DCTFWeb = %+v", back)
	}
	assertNoDuplicates(t, srv)
}
