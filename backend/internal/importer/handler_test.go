package importer_test

import (
	"bytes"
	"net/http"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"

	"controle/internal/importer"
	"controle/internal/testutil"
)

func TestTemplateEndpoint_DownloadsXLSX(t *testing.T) {
	srv := testutil.NewServer(t, testutil.NewDB(t), testutil.FixedClock(t, "2026-10-01T12:00:00Z"))
	res := srv.Get("/api/v1/imports/companies/template").MustStatus(t, http.StatusOK)
	if ct := res.Header.Get("Content-Type"); ct != "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" {
		t.Fatalf("Content-Type = %q", ct)
	}
	if cd := res.Header.Get("Content-Disposition"); !strings.Contains(cd, importer.TemplateFileName) {
		t.Fatalf("Content-Disposition = %q", cd)
	}
	f, err := excelize.OpenReader(bytes.NewReader(res.Body))
	if err != nil {
		t.Fatalf("download não abre como XLSX: %v", err)
	}
	defer func() { _ = f.Close() }()
	if v, _ := f.GetCellValue(importer.SheetCompanies, "A1"); v != "Código interno" {
		t.Fatalf("A1 = %q", v)
	}
}
