package factorr_test

import (
	"net/http"
	"slices"
	"testing"

	"controle/internal/testutil"
	"controle/internal/testutil/seeded"
)

// Os dados de demonstração (make seed-demo) cobrem os quatro casos da carteira no PA corrente
// e meses declarados no PGDAS-D (FR-03), para a fatia poder ser vista na interface.
func TestDemo_FactorRPortfolioCoversEveryCase(t *testing.T) {
	srv := testutil.NewServer(t, seeded.NewDemoDB(t), seeded.DemoClock())
	p := portfolio(t, srv, "2026-10")
	if want := []string{"208", "205", "206", "202", "210"}; !slices.Equal(codesOf(p), want) {
		t.Fatalf("ordem = %v; esperado %v", codesOf(p), want)
	}
	lights := make([]string, len(p.Linhas))
	for i, l := range p.Linhas {
		lights[i] = str(l.Semaforo)
	}
	if want := []string{"vermelho", "vermelho", "amarelo", "verde", "<null>"}; !slices.Equal(lights, want) {
		t.Fatalf("semáforos = %v", lights)
	}
	if !slices.Equal(p.Linhas[1].MesesFaltantes, []string{"2026-05", "2026-08"}) {
		t.Fatalf("faltantes da 205 = %v", p.Linhas[1].MesesFaltantes)
	}
	var list struct {
		Items []movementOut `json:"items"`
	}
	id := companyID(t, srv, "202")
	srv.Get("/api/v1/companies/"+id+"/factor-r/movements?from=2025-10&to=2026-01").MustStatus(t, http.StatusOK).JSON(t, &list)
	for _, m := range list.Items {
		if m.Origem != "pgdas" || str(m.FolhaTotalDeclaradaPgdas) != "30180.00" {
			t.Fatalf("mês do PGDAS-D = %+v", m)
		}
	}
	if len(list.Items) != 4 {
		t.Fatalf("meses do PGDAS-D = %d", len(list.Items))
	}
}
