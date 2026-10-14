package documents_test

import (
	"bytes"
	"net/http"
	"os"
	"testing"

	"controle/internal/user"
)

// TEST-021a: o visualizador da fila (páginas do PDF como imagem, no DPI do extrator) e "que
// documento é esse?" (o contador diz o tipo e a empresa, e o documento volta para a extração).

func TestTEST021_ViewerRendersPDFPages(t *testing.T) {
	e := setup(t)
	pdf, err := os.ReadFile("../testdata/conciliacao/comprovantes/05_pix_recebido_ana_lucia_1de4.pdf")
	if err != nil {
		t.Fatal(err)
	}
	var up uploadOut
	e.upload(t, "comprovante.pdf", pdf, nil).MustStatus(t, http.StatusAccepted).JSON(t, &up)
	var info struct {
		ContentType string `json:"content_type"`
		Pages       int    `json:"pages"`
		DPI         int    `json:"dpi"`
	}
	e.srv.Get("/api/v1/reconciliation/documents/"+up.DocumentID+"/pages").MustStatus(t, http.StatusOK).JSON(t, &info)
	if info.ContentType != "application/pdf" || info.Pages != 1 || info.DPI != 150 {
		t.Fatalf("páginas = %+v", info)
	}
	page := e.srv.Get("/api/v1/reconciliation/documents/"+up.DocumentID+"/pages/1").MustStatus(t, http.StatusOK)
	if page.Header.Get("Content-Type") != "image/png" || !bytes.HasPrefix(page.Body, []byte("\x89PNG\r\n\x1a\n")) {
		t.Fatalf("página 1: %s, %d bytes", page.Header.Get("Content-Type"), len(page.Body))
	}
	for _, p := range []string{"2", "0", "x"} {
		e.srv.Get("/api/v1/reconciliation/documents/"+up.DocumentID+"/pages/"+p).MustStatus(t, http.StatusNotFound)
	}
	// Sem páginas (CSV, imagem): a tela abre o próprio arquivo.
	e.upload(t, "extrato.csv", unique(t, "data;descricao;valor\n2026-08-01;x;1,00"), nil).MustStatus(t, http.StatusAccepted).JSON(t, &up)
	e.srv.Get("/api/v1/reconciliation/documents/"+up.DocumentID+"/pages").MustStatus(t, http.StatusOK).JSON(t, &info)
	if info.Pages != 0 {
		t.Fatalf("CSV com páginas: %+v", info)
	}
	e.srv.Get("/api/v1/reconciliation/documents/"+up.DocumentID+"/pages/1").MustStatus(t, http.StatusNotFound)
}

// "Que documento é esse?": só no documento em revisão; o tipo, a empresa e os dados do extrato
// informados voltam o documento à extração, no log de extração como ação humana.
func TestTEST021_IdentifyDocument(t *testing.T) {
	e := setup(t)
	company := e.company(t, "900", "11222333000181")
	var up uploadOut
	e.upload(t, "extrato.csv", unique(t, "data;descricao;valor\n2026-08-01;x;1,00"), nil).MustStatus(t, http.StatusAccepted).JSON(t, &up)
	path := "/api/v1/reconciliation/documents/" + up.DocumentID + "/identify"

	// Ainda na triagem: não se identifica.
	if code := e.srv.Post(path, map[string]any{"kind": "extrato", "company_id": company}).MustStatus(t, http.StatusConflict).ErrorCode(t); code != "DOCUMENT_NOT_IDENTIFIABLE" {
		t.Fatalf("código = %s", code)
	}
	if _, err := e.pool.Exec(ctx, `UPDATE jobs SET status = 'done', finished_at = now() WHERE kind = 'reconciliation_triage'`); err != nil {
		t.Fatal(err)
	}
	if _, err := e.pool.Exec(ctx, `UPDATE reconciliation_documents SET status = 'revisao_triagem', kind = 'desconhecido', bank = 'a_fonte',
		diagnosis = '{"motivo": "tipo_desconhecido"}' WHERE id = $1`, up.DocumentID); err != nil {
		t.Fatal(err)
	}
	for _, body := range []map[string]any{
		{"kind": "planilha", "company_id": company},
		{"kind": "extrato"},
		{"kind": "extrato", "company_id": company, "period_start": "2026-08-31", "period_end": "2026-08-01"},
	} {
		e.srv.Post(path, body).MustStatus(t, http.StatusBadRequest)
	}
	var out uploadOut
	e.srv.Post(path, map[string]any{"kind": "extrato", "company_id": company, "branch": " 0001 ", "account": "12345-6",
		"period_start": "2026-08-01", "period_end": "2026-08-31"}).MustStatus(t, http.StatusAccepted).JSON(t, &out)
	if out.Status != "triado" || out.JobID == nil {
		t.Fatalf("identificação = %+v", out)
	}
	if n := e.count(t, `SELECT count(*) FROM reconciliation_documents WHERE id = $1 AND kind = 'extrato' AND company_id = $2 AND status = 'triado'
		AND bank = 'a_fonte' AND branch = '0001' AND account = '12345-6' AND period_start = '2026-08-01' AND diagnosis IS NULL AND job_id = $3`,
		up.DocumentID, company, *out.JobID); n != 1 {
		t.Fatal("documento não identificado (o banco que a triagem leu devia ficar)")
	}
	if n := e.count(t, `SELECT count(*) FROM jobs WHERE id = $1 AND kind = 'statement_extract' AND status = 'pending'`, *out.JobID); n != 1 {
		t.Fatal("extração não enfileirada")
	}
	if n := e.count(t, `SELECT count(*) FROM extraction_logs WHERE document_id = $1 AND alias = 'humano' AND actor_id = $2
		AND change::jsonb->>'acao' = 'identificar'`, up.DocumentID, user.AdminID); n != 1 {
		t.Fatal("identificação fora do log")
	}
	// Na fila de novo: não se identifica outra vez.
	e.srv.Post(path, map[string]any{"kind": "extrato", "company_id": company}).MustStatus(t, http.StatusConflict)
}
