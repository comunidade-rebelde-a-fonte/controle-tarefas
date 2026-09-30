package exporter_test

import (
	"bytes"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"

	"controle/internal/testutil"
	"controle/internal/testutil/seeded"
)

type gridBody struct {
	Columns []struct {
		ObligationType struct {
			Name string `json:"name"`
		} `json:"obligation_type"`
	} `json:"columns"`
	Rows []struct {
		Company struct {
			LegalName string `json:"legal_name"`
		} `json:"company"`
	} `json:"rows"`
}

func server(t *testing.T) *testutil.Server {
	t.Helper()
	return testutil.NewServer(t, seeded.NewDemoDB(t), seeded.DemoClock())
}

func open(t *testing.T, body []byte) (*excelize.File, string) {
	t.Helper()
	f, err := excelize.OpenReader(bytes.NewReader(body))
	if err != nil {
		t.Fatalf("XLSX não abre: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f, f.GetSheetName(0)
}

// CA-017: o XLSX reflete competência, módulo e filtros atuais — mesmas linhas da grade.
func TestCA017_ExportRespectsFilters(t *testing.T) {
	srv := server(t)
	query := "competence=2026-10&group=normal&tax_regime=LUCRO_REAL"

	var g gridBody
	srv.Get("/api/v1/modules/fiscal/grid?" + query).MustStatus(t, http.StatusOK).JSON(t, &g)
	if len(g.Rows) == 0 || len(g.Rows) >= 10 {
		t.Fatalf("filtro por regime deveria reduzir a grade Normal: %d linhas", len(g.Rows))
	}

	res := srv.Get("/api/v1/exports/fiscal.xlsx?" + query).MustStatus(t, http.StatusOK)
	if ct := res.Header.Get("Content-Type"); ct != "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" {
		t.Fatalf("Content-Type = %q", ct)
	}
	if cd := res.Header.Get("Content-Disposition"); !strings.Contains(cd, `filename="fiscal-normal-2026-10.xlsx"`) {
		t.Fatalf("Content-Disposition = %q", cd)
	}
	f, sheet := open(t, res.Body)
	rows, err := f.GetRows(sheet)
	if err != nil {
		t.Fatal(err)
	}
	if rows[0][0] != "Controle de Obrigações — Fiscal — Normais" || rows[1][0] != "Competência: 10/2026" ||
		rows[2][0] != "Filtros: Regime: Lucro Real" || !strings.HasPrefix(rows[3][0], "Gerado em 01/10/2026") {
		t.Fatalf("cabeçalho = %v", rows[:4])
	}

	header := rows[5]
	if len(header) != 12+3*len(g.Columns) {
		t.Fatalf("colunas = %d, want %d (12 fixas + status/vencimento/atraso por obrigação)", len(header), 12+3*len(g.Columns))
	}
	first := g.Columns[0].ObligationType.Name
	if header[12] != first || header[13] != first+" — vencimento" || header[14] != first+" — atrasada" {
		t.Fatalf("colunas da obrigação = %v", header[12:15])
	}

	data := rows[6:]
	if len(data) != len(g.Rows) {
		t.Fatalf("linhas = %d, want %d (as da grade filtrada)", len(data), len(g.Rows))
	}
	for i, r := range g.Rows {
		if data[i][1] != r.Company.LegalName {
			t.Fatalf("linha %d = %q, want %q", i, data[i][1], r.Company.LegalName)
		}
		if data[i][3] != "Lucro Real" {
			t.Fatalf("linha %d fora do filtro de regime: %q", i, data[i][3])
		}
	}
	// Status em texto (nunca códigos) nas colunas de obrigação.
	statuses := map[string]bool{}
	for _, d := range data {
		for c := 12; c < len(d); c += 3 {
			statuses[d[c]] = true
		}
	}
	for s := range statuses {
		if s != "" && strings.ToUpper(s) == s && strings.Contains(s, "_") {
			t.Fatalf("status exportado como código: %q", s)
		}
	}

	// Autofiltro e cabeçalho congelado.
	panes, err := f.GetPanes(sheet)
	if err != nil || !panes.Freeze || panes.YSplit != 6 {
		t.Fatalf("painéis = %+v (%v)", panes, err)
	}
}

func TestExport_FreeSearchAndOtherModules(t *testing.T) {
	srv := server(t)
	res := srv.Get("/api/v1/exports/fiscal.xlsx?competence=2026-10&group=simples&q=00100003").MustStatus(t, http.StatusOK)
	f, sheet := open(t, res.Body)
	rows, _ := f.GetRows(sheet)
	if len(rows) != 7 || rows[6][1] != "Canto Sul Restaurante Ltda" || rows[2][0] != "Filtros: Busca: 00100003" {
		t.Fatalf("busca por CNPJ parcial: %d linhas, %v", len(rows), rows[len(rows)-1])
	}

	for _, m := range []string{"declarations", "payroll", "accounting", "dctfweb"} {
		res := srv.Get("/api/v1/exports/" + m + ".xlsx?competence=2026-10&overdue=true").MustStatus(t, http.StatusOK)
		f, sheet := open(t, res.Body)
		rows, _ := f.GetRows(sheet)
		if !slices.Contains(rows[2], "Filtros: Somente atrasadas") {
			t.Fatalf("%s: filtros = %v", m, rows[2])
		}
	}

	srv.Get("/api/v1/exports/fiscal.xlsx?competence=2026-10").MustStatus(t, http.StatusBadRequest)
	srv.Get("/api/v1/exports/credentials.xlsx?competence=2026-10").MustStatus(t, http.StatusNotFound)
	srv.Get("/api/v1/exports/fiscal.csv?competence=2026-10&group=normal").MustStatus(t, http.StatusNotFound)
	srv.Get("/api/v1/exports/payroll.xlsx?competence=10-2026").MustStatus(t, http.StatusBadRequest)
}
