package docindex_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"controle/internal/testutil"
	"controle/internal/testutil/seeded"
)

// TEST-026 [REQ-037, REQ-043, REQ-044]: a área Documentos junta os documentos do Fator R (local),
// da Conciliação (MinIO) e as procurações e os certificados do Controle (local) sem mover nada,
// com o status de cada módulo e a situação comum de leitura (D-040), e cada um abre no detalhe do
// seu módulo.

var ctx = context.Background()

type item struct {
	ID             string `json:"id"`
	Module         string `json:"module"`
	StorageBackend string `json:"storage_backend"`
	Kind           string `json:"kind"`
	Status         string `json:"status"`
	Situation      string `json:"situation"`
	Company        *struct {
		ID           string `json:"id"`
		InternalCode string `json:"internal_code"`
	} `json:"company"`
	Competence  *string `json:"competence"`
	PeriodStart *string `json:"period_start"`
	Bank        *string `json:"bank"`
	FileName    string  `json:"file_name"`
	ExpiresAt   *string `json:"expires_at"`
	Detail      struct {
		Path  string            `json:"path"`
		Query map[string]string `json:"query"`
	} `json:"detail"`
}

type list struct {
	Items  []item `json:"items"`
	Total  int    `json:"total"`
	Counts []struct {
		Module    string `json:"module"`
		Situation string `json:"situation"`
		Status    string `json:"status"`
		Total     int    `json:"total"`
	} `json:"counts"`
}

type env struct {
	t   *testing.T
	srv *testutil.Server
	seq int
}

func (e *env) scalar(sql string, args ...any) string {
	e.t.Helper()
	var v string
	if err := e.srv.Pool.QueryRow(ctx, sql, args...).Scan(&v); err != nil {
		e.t.Fatalf("%v\n%s", err, sql)
	}
	return v
}

func (e *env) list(query string) list {
	e.t.Helper()
	var l list
	e.srv.Get("/api/v1/documents?"+query).MustStatus(e.t, http.StatusOK).JSON(e.t, &l)
	return l
}

// pgdas grava um extrato do PGDAS-D recebido há `minutes` minutos, com as regras da tabela: a
// empresa só no vinculado, o motivo na revisão e na rejeição e a corrida do parser depois do
// recebimento.
func (e *env) pgdas(company any, status, pa string, minutes int) string {
	e.t.Helper()
	e.seq++
	var p, motivo, trace any
	if pa != "" {
		p = pa + "-01"
	}
	if status == "needs_review" || status == "rejected" {
		motivo = "cnpj_nao_encontrado"
	}
	if status != "received" {
		trace = e.scalar(`INSERT INTO ai_runs (trace_id, agent, trigger, status, created_at, updated_at)
			VALUES (lpad(to_hex($1::int), 32, 'a'), 'parser_pgdas', 'upload', 'ok', now(), now()) RETURNING trace_id`, e.seq)
	}
	return e.scalar(`INSERT INTO factor_r_pgdas_documents (company_id, sha256, file_ref, mime, size_bytes, original_name, status, motivo, pa,
		trace_id, created_at, updated_at)
		VALUES ($1, lpad(to_hex($2::int), 64, 'b'), 'pgdas/' || lpad(to_hex($2::int), 64, 'b'), 'text/plain', 10, 'pgdas-' || $2::text || '.txt',
		        $3, $4, $5::date, $6, now() - make_interval(mins => $7::int), now() - make_interval(mins => $7::int))
		RETURNING id`, company, e.seq, status, motivo, p, trace, minutes)
}

// recon grava um documento da Conciliação recebido há `minutes` minutos.
func (e *env) recon(company any, status, kind, comp, start, end, bank string, minutes int) string {
	e.t.Helper()
	e.seq++
	null := func(v string) any {
		if v == "" {
			return nil
		}
		return v
	}
	return e.scalar(`INSERT INTO reconciliation_documents (company_id, competence, period_start, period_end, bucket, object_key, sha256, file_name,
		content_type, kind, status, bank, created_at, updated_at)
		VALUES ($1, $2::date, $3::date, $4::date, 'originais', 'originais/' || $5::int::text, lpad(to_hex($5::int), 64, 'c'), 'doc-' || $5::int::text || '.pdf',
		        'application/pdf', $6, $7, $8, now() - make_interval(mins => $9::int), now() - make_interval(mins => $9::int))
		RETURNING id`, company, null(comp), null(start), null(end), e.seq, null(kind), status, null(bank), minutes)
}

func TestTEST026_DocumentsFromEveryModuleOpenInTheirModule(t *testing.T) {
	pool := seeded.NewCompaniesDB(t)
	e := &env{t: t, srv: testutil.NewServer(t, pool, testutil.FixedClock(t, "2026-10-01T12:00:00Z"))}
	a := e.scalar(`SELECT id FROM companies WHERE internal_code = '101'`)
	b := e.scalar(`SELECT id FROM companies WHERE internal_code = '102'`)

	linked := e.pgdas(a, "linked", "2026-09", 10)
	review := e.pgdas(nil, "needs_review", "2026-08", 20)
	e.pgdas(nil, "received", "", 30)
	e.pgdas(nil, "rejected", "2026-07", 40)
	statement := e.recon(a, "importado", "extrato", "", "2026-09-01", "2026-09-30", "beta", 50)
	e.recon(nil, "pendente", "", "", "", "", "", 60)
	receipt := e.recon(b, "revisao_triagem", "comprovante", "2026-09-01", "", "", "", 70)
	e.recon(nil, "duplicado", "extrato", "", "", "", "", 80)
	poa := e.scalar(`INSERT INTO credential_controls (company_id, type, expires_at, file_storage_key, file_name, file_mime_type, file_size_bytes,
		file_uploaded_at) VALUES ($1, 'RFB_POWER_OF_ATTORNEY', '2027-01-31', 'credentials/x', 'procuracao.pdf', 'application/pdf', 100,
		now() - interval '90 minutes') RETURNING id`, a)
	e.scalar(`INSERT INTO credential_controls (company_id, type, expires_at) VALUES ($1, 'DIGITAL_CERTIFICATE', '2027-03-31') RETURNING id`, b)

	all := e.list("")
	if all.Total != 9 || len(all.Items) != 9 {
		t.Fatalf("todos = %d (%d itens)", all.Total, len(all.Items))
	}
	if all.Items[0].ID != linked || all.Items[8].ID != poa {
		t.Errorf("ordem (o mais recente primeiro) = %s … %s", all.Items[0].ID, all.Items[8].ID)
	}
	want := map[string][4]string{ // módulo, armazenamento, situação, caminho do detalhe
		linked:    {"factor_r", "local", "concluido", "/factor-r/inbox/" + linked},
		review:    {"factor_r", "local", "em_revisao", "/factor-r/inbox/" + review},
		statement: {"reconciliation", "minio", "concluido", "/reconciliation/documents/" + statement},
		receipt:   {"reconciliation", "minio", "em_revisao", "/reconciliation/documents/" + receipt},
		poa:       {"credentials", "local", "concluido", "/companies/" + a},
	}
	situations := map[string]int{}
	for _, it := range all.Items {
		situations[it.Situation]++
		if w, ok := want[it.ID]; ok && (it.Module != w[0] || it.StorageBackend != w[1] || it.Situation != w[2] || it.Detail.Path != w[3]) {
			t.Errorf("%s = %+v", it.ID, it)
		}
		switch it.ID {
		case statement:
			if it.Company == nil || it.Company.ID != a || it.Bank == nil || *it.Bank != "beta" || it.PeriodStart == nil || *it.PeriodStart != "2026-09-01" {
				t.Errorf("extrato = %+v", it)
			}
		case poa:
			if it.Kind != "procuracao" || it.Status != "arquivado" || it.Detail.Query["tab"] != "credentials" || it.ExpiresAt == nil || *it.ExpiresAt != "2027-01-31" {
				t.Errorf("procuração = %+v", it)
			}
		case linked:
			if it.Competence == nil || *it.Competence != "2026-09" || it.Kind != "pgdas" {
				t.Errorf("PGDAS-D vinculado = %+v", it)
			}
		}
	}
	if fmt.Sprint(situations) != "map[concluido:3 descartado:2 em_processamento:2 em_revisao:2]" {
		t.Errorf("situações = %v", situations)
	}
	sum := 0
	for _, c := range all.Counts {
		sum += c.Total
	}
	if sum != 9 {
		t.Errorf("contagens = %+v", all.Counts)
	}

	// Os filtros.
	for query, n := range map[string]int{
		"module=factor_r":                        4,
		"module=credentials":                     1,
		"situation=em_revisao":                   2,
		"module=reconciliation&status=duplicado": 1,
		"company_id=" + a:                        3,
		"competence=2026-09":                     3,
		"kind=procuracao":                        1,
		"bank=BETA":                              1,
		"module=reconciliation&situation=em_processamento": 1,
	} {
		if l := e.list(query); l.Total != n || len(l.Items) != n {
			t.Errorf("%s = %d (%d itens), esperado %d", query, l.Total, len(l.Items), n)
		}
	}
	if l := e.list("limit=2&offset=2"); l.Total != 9 || len(l.Items) != 2 || l.Items[0].ID != all.Items[2].ID {
		t.Errorf("página = %+v", l)
	}
	for _, q := range []string{"status=linked", "module=x", "situation=x", "kind=x", "company_id=x", "competence=2026", "limit=500", "offset=-1", "module=factor_r&status=importado"} {
		e.srv.Get("/api/v1/documents?"+q).MustStatus(t, http.StatusBadRequest)
	}

	// Cada documento abre no detalhe do seu módulo (o índice não guarda nem copia nada).
	for _, it := range all.Items {
		switch it.Module {
		case "factor_r":
			var d struct {
				ID string `json:"id"`
			}
			e.srv.Get("/api/v1/factor-r/pgdas/"+it.ID).MustStatus(t, http.StatusOK).JSON(t, &d)
			if d.ID != it.ID {
				t.Errorf("detalhe do PGDAS-D %s = %s", it.ID, d.ID)
			}
		case "reconciliation":
			var d struct {
				ID string `json:"id"`
			}
			e.srv.Get("/api/v1/reconciliation/documents/"+it.ID).MustStatus(t, http.StatusOK).JSON(t, &d)
			if d.ID != it.ID {
				t.Errorf("detalhe da Conciliação %s = %s", it.ID, d.ID)
			}
		default:
			type control struct {
				File *struct {
					Name string `json:"name"`
				} `json:"file"`
			}
			var row struct {
				Company struct {
					ID string `json:"id"`
				} `json:"company"`
				RFB         control `json:"rfb_power_of_attorney"`
				Certificate control `json:"digital_certificate"`
			}
			e.srv.Get("/api/v1/companies/"+it.Company.ID+"/credential-controls").MustStatus(t, http.StatusOK).JSON(t, &row)
			found := row.Company.ID == it.Company.ID && row.RFB.File != nil && row.RFB.File.Name == it.FileName
			if !found {
				t.Errorf("a ficha não mostra o arquivo %s: %+v", it.FileName, row)
			}
		}
	}
}
