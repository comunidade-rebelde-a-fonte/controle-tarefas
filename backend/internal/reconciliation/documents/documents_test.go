package documents_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"controle/internal/platform/config"
	"controle/internal/platform/objectstore"
	"controle/internal/reconciliation/documents"
	"controle/internal/testutil"
	"controle/internal/user"
)

var ctx = context.Background()

// env é a API completa sobre Postgres e MinIO reais (o mesmo init.sh do ambiente: bucket
// versionado e usuário da aplicação sem permissão de apagar).
type env struct {
	srv   *testutil.Server
	pool  *pgxpool.Pool
	minio testutil.MinIO
}

func withS3(m testutil.MinIO) func(*config.Config) {
	return func(c *config.Config) {
		c.S3Endpoint, c.S3Region, c.S3AccessKey, c.S3SecretKey, c.S3Bucket = m.Endpoint, "us-east-1", m.AccessKey, config.Secret(m.SecretKey), m.Bucket
	}
}

func setup(t *testing.T, opts ...func(*config.Config)) env {
	t.Helper()
	m := testutil.NewMinIO(t)
	pool := testutil.NewDB(t)
	srv := testutil.NewServer(t, pool, testutil.FixedClock(t, "2026-09-15T12:00:00-03:00"), append([]func(*config.Config){withS3(m)}, opts...)...)
	return env{srv: srv, pool: pool, minio: m}
}

// unique deixa cada arquivo de teste diferente: o MinIO é compartilhado no pacote e a chave é
// o hash do conteúdo.
func unique(t *testing.T, content string) []byte {
	return []byte(content + "\n% " + t.Name() + "\n")
}

func sha(data []byte) string {
	s := sha256.Sum256(data)
	return hex.EncodeToString(s[:])
}

func (e env) upload(t *testing.T, name string, data []byte, fields map[string]string) testutil.Response {
	t.Helper()
	r, err := e.post(name, data, fields)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// post envia o multipart sem depender de t (pode rodar em outra goroutine).
func (e env) post(name string, data []byte, fields map[string]string) (testutil.Response, error) {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	for k, v := range fields {
		if err := mw.WriteField(k, v); err != nil {
			return testutil.Response{}, err
		}
	}
	if data != nil {
		fw, err := mw.CreateFormFile("file", name)
		if err != nil {
			return testutil.Response{}, err
		}
		if _, err := fw.Write(data); err != nil {
			return testutil.Response{}, err
		}
	}
	if err := mw.Close(); err != nil {
		return testutil.Response{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.srv.URL+"/api/v1/reconciliation/documents", &body)
	if err != nil {
		return testutil.Response{}, err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := e.srv.Client().Do(req)
	if err != nil {
		return testutil.Response{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	return testutil.Response{Status: resp.StatusCode, Header: resp.Header, Body: b}, err
}

func (e env) count(t *testing.T, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := e.pool.QueryRow(ctx, sql, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (e env) company(t *testing.T, code, cnpj string) string {
	t.Helper()
	var id string
	if err := e.pool.QueryRow(ctx, `INSERT INTO companies (internal_code, legal_name, cnpj, service_start_date, payroll_type, created_by, updated_by)
		VALUES ($1, 'Empresa ' || $1, $2, '2025-01-01', 'PAYROLL', $3, $3) RETURNING id`, code, cnpj, user.AdminID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

type uploadOut struct {
	DocumentID string  `json:"document_id"`
	JobID      *string `json:"job_id"`
	Status     string  `json:"status"`
}

type docOut struct {
	ID              string              `json:"id"`
	Status          string              `json:"status"`
	CompanyID       *string             `json:"company_id"`
	Competence      *string             `json:"competence"`
	FileName        string              `json:"file_name"`
	ContentType     string              `json:"content_type"`
	SizeBytes       *int64              `json:"size_bytes"`
	SHA256          string              `json:"sha256"`
	ObjectVersionID *string             `json:"object_version_id"`
	Job             *documents.JobState `json:"job"`
	FileURL         string              `json:"file_url"`
	UploadedBy      *string             `json:"uploaded_by"`
}

// AC-013: o original vai para o MinIO pelo hash, o documento nasce pendente e o job de triagem
// entra na fila na mesma transação; a resposta volta sem esperar o processamento [AC-014].
func TestUpload_StoresOriginalByHashAndEnqueuesTriage(t *testing.T) {
	e := setup(t)
	data := unique(t, "%PDF-1.4 extrato do Banco A Fonte")
	var u uploadOut
	e.upload(t, "../Extrato agosto.pdf", data, nil).MustStatus(t, http.StatusAccepted).JSON(t, &u)
	if u.Status != documents.StatusPending || u.JobID == nil || u.DocumentID == "" {
		t.Fatalf("resposta = %+v", u)
	}
	var d docOut
	e.srv.Get("/api/v1/reconciliation/documents/"+u.DocumentID).MustStatus(t, http.StatusOK).JSON(t, &d)
	if d.SHA256 != sha(data) || d.ContentType != "application/pdf" || d.SizeBytes == nil || *d.SizeBytes != int64(len(data)) ||
		d.FileName != "Extrato agosto.pdf" || d.ObjectVersionID == nil || d.UploadedBy == nil ||
		d.Job == nil || d.Job.ID != *u.JobID || d.Job.Status != "pending" || d.FileURL != "/api/v1/reconciliation/documents/"+u.DocumentID+"/file" {
		t.Fatalf("documento = %+v", d)
	}
	// Chave só com o hash: o nome enviado nunca entra no armazenamento [09 §5].
	if n := e.count(t, `SELECT count(*) FROM reconciliation_documents WHERE id = $1 AND object_key = $2 AND bucket = $3 AND job_id = $4`,
		u.DocumentID, "originais/"+sha(data), e.minio.Bucket, *u.JobID); n != 1 {
		t.Fatal("documento sem a chave do hash, o bucket ou o job")
	}
	if n := e.count(t, `SELECT count(*) FROM jobs WHERE id = $1 AND kind = $2 AND status = 'pending' AND dedupe_key = $3 AND payload = jsonb_build_object('document_id', $3::text)`,
		*u.JobID, documents.KindTriage, u.DocumentID); n != 1 {
		t.Fatal("job de triagem não enfileirado com o documento")
	}
	// O original volta byte a byte, pela versão gravada.
	store := objectstore.New(objectstore.Config{Endpoint: e.minio.Endpoint, AccessKey: e.minio.AccessKey, SecretKey: e.minio.SecretKey, Bucket: e.minio.Bucket})
	obj, err := store.Get(ctx, "originais/"+sha(data), *d.ObjectVersionID)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(obj.Body)
	_ = obj.Body.Close()
	if !bytes.Equal(got, data) {
		t.Fatal("original diferente do enviado")
	}
	// Leitura pela API: inline para ver, attachment para baixar, sem o navegador adivinhar o tipo.
	r := e.srv.Get("/api/v1/reconciliation/documents/"+u.DocumentID+"/file").MustStatus(t, http.StatusOK)
	if !bytes.Equal(r.Body, data) || r.Header.Get("Content-Type") != "application/pdf" || r.Header.Get("X-Content-Type-Options") != "nosniff" ||
		!strings.HasPrefix(r.Header.Get("Content-Disposition"), "inline") || !strings.Contains(r.Header.Get("Content-Disposition"), "Extrato agosto.pdf") ||
		!strings.Contains(r.Header.Get("Cache-Control"), "no-store") {
		t.Fatalf("leitura do original: %v", r.Header)
	}
	r = e.srv.Get("/api/v1/reconciliation/documents/"+u.DocumentID+"/file?download=1").MustStatus(t, http.StatusOK)
	if !strings.HasPrefix(r.Header.Get("Content-Disposition"), "attachment") {
		t.Fatalf("download: %v", r.Header)
	}
}

// Dedupe pelo SHA-256, como na origem: o mesmo arquivo é recusado e aponta o documento que
// já existe; nada é gravado de novo (nem documento, nem job, nem versão no bucket).
func TestUpload_DuplicateIsRefused(t *testing.T) {
	e := setup(t)
	data := unique(t, "%PDF-1.4 comprovante PIX")
	var u uploadOut
	e.upload(t, "pix.pdf", data, nil).MustStatus(t, http.StatusAccepted).JSON(t, &u)
	var d docOut
	e.srv.Get("/api/v1/reconciliation/documents/"+u.DocumentID).MustStatus(t, http.StatusOK).JSON(t, &d)
	r := e.upload(t, "outro-nome.pdf", data, nil).MustStatus(t, http.StatusConflict)
	var errBody struct {
		Code    string         `json:"code"`
		Details map[string]any `json:"details"`
	}
	r.JSON(t, &errBody)
	if errBody.Code != "DUPLICATE_DOCUMENT" || errBody.Details["document_id"] != u.DocumentID {
		t.Fatalf("duplicata = %s", r.Body)
	}
	if n := e.count(t, `SELECT count(*) FROM reconciliation_documents`); n != 1 {
		t.Fatalf("documentos = %d", n)
	}
	if n := e.count(t, `SELECT count(*) FROM jobs WHERE kind = $1`, documents.KindTriage); n != 1 {
		t.Fatalf("jobs = %d", n)
	}
	store := objectstore.New(objectstore.Config{Endpoint: e.minio.Endpoint, AccessKey: e.minio.AccessKey, SecretKey: e.minio.SecretKey, Bucket: e.minio.Bucket})
	obj, err := store.Get(ctx, "originais/"+sha(data), "")
	if err != nil {
		t.Fatal(err)
	}
	_ = obj.Body.Close()
	if obj.VersionID != *d.ObjectVersionID {
		t.Fatalf("a duplicata gravou outra versão: %s ≠ %s", obj.VersionID, *d.ObjectVersionID)
	}
}

// Envios simultâneos do mesmo arquivo: um documento e um job; os demais recebem 409 (o índice
// único do SHA-256 decide a corrida, 06 §11).
func TestUpload_ConcurrentDuplicatesCreateOneDocument(t *testing.T) {
	e := setup(t)
	data := unique(t, "%PDF-1.4 extrato enviado duas vezes ao mesmo tempo")
	const n = 6
	statuses := make(chan int, n)
	var wg sync.WaitGroup
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := e.post("extrato.pdf", data, nil)
			if err != nil {
				t.Error(err)
			}
			statuses <- r.Status
		}()
	}
	wg.Wait()
	close(statuses)
	got := map[int]int{}
	for s := range statuses {
		got[s]++
	}
	if got[http.StatusAccepted] != 1 || got[http.StatusConflict] != n-1 {
		t.Fatalf("status = %v", got)
	}
	if docs, jobs := e.count(t, `SELECT count(*) FROM reconciliation_documents`), e.count(t, `SELECT count(*) FROM jobs`); docs != 1 || jobs != 1 {
		t.Fatalf("documentos = %d, jobs = %d", docs, jobs)
	}
}

// F-009: documento tardio, com o Contábil fechado na competência informada, guarda o original
// e fica como pendência de período fechado, sem processamento (ingerir_arquivo da origem).
func TestUpload_ClosedAccountingPeriodBecomesPendingWithoutJob(t *testing.T) {
	e := setup(t)
	id := e.company(t, "301", "11222333000181")
	var comp string
	if err := e.pool.QueryRow(ctx, `INSERT INTO competencies (competence_key, year, month, first_day) VALUES ('2026-08', 2026, 8, '2026-08-01') RETURNING id`).Scan(&comp); err != nil {
		t.Fatal(err)
	}
	if _, err := e.pool.Exec(ctx, `INSERT INTO company_module_periods (company_id, competence_id, module_code, state, closed_at, closed_by)
		VALUES ($1, $2, 'ACCOUNTING', 'CLOSED', now(), $3)`, id, comp, user.AdminID); err != nil {
		t.Fatal(err)
	}
	var u uploadOut
	e.upload(t, "extrato-agosto.pdf", unique(t, "%PDF-1.4 extrato de agosto"), map[string]string{"company_id": id, "competence": "2026-08"}).
		MustStatus(t, http.StatusAccepted).JSON(t, &u)
	if u.Status != documents.StatusClosedPeriod || u.JobID != nil {
		t.Fatalf("resposta = %+v", u)
	}
	if n := e.count(t, `SELECT count(*) FROM jobs`); n != 0 {
		t.Fatalf("jobs = %d", n)
	}
	var d docOut
	e.srv.Get("/api/v1/reconciliation/documents/"+u.DocumentID).MustStatus(t, http.StatusOK).JSON(t, &d)
	if d.CompanyID == nil || *d.CompanyID != id || d.Competence == nil || *d.Competence != "2026-08" || d.Job != nil || d.ObjectVersionID == nil {
		t.Fatalf("documento = %+v", d)
	}
	// Outra competência, aberta: processamento normal.
	e.upload(t, "extrato-setembro.pdf", unique(t, "%PDF-1.4 extrato de setembro"), map[string]string{"company_id": id, "competence": "2026-09"}).
		MustStatus(t, http.StatusAccepted).JSON(t, &u)
	if u.Status != documents.StatusPending || u.JobID == nil {
		t.Fatalf("competência aberta = %+v", u)
	}
}

// TEST-015 / 11 §9: com o MinIO fora (ou sem configuração), o envio falha antes de criar estado
// e não guarda o original em outro lugar.
func TestUpload_StorageUnavailableWritesNothing(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	down := "http://" + ln.Addr().String()
	_ = ln.Close()
	for name, opt := range map[string]func(*config.Config){
		"fora":            func(c *config.Config) { c.S3Endpoint = down },
		"não configurado": func(c *config.Config) { c.S3Endpoint = "" },
	} {
		t.Run(name, func(t *testing.T) {
			e := setup(t, opt)
			code := e.upload(t, "extrato.pdf", unique(t, "%PDF-1.4 extrato"), nil).MustStatus(t, http.StatusServiceUnavailable).ErrorCode(t)
			if code != "STORAGE_UNAVAILABLE" {
				t.Fatalf("código = %s", code)
			}
			if n := e.count(t, `SELECT (SELECT count(*) FROM reconciliation_documents) + (SELECT count(*) FROM jobs)`); n != 0 {
				t.Fatalf("estado criado sem o original: %d", n)
			}
		})
	}
}

func TestUpload_TypeSizeAndFieldsAreValidated(t *testing.T) {
	e := setup(t)
	cases := []struct {
		name   string
		data   []byte
		fields map[string]string
		status int
		code   string
	}{
		{"executável", []byte("MZ\x90\x00\x03\x00binário"), nil, http.StatusUnsupportedMediaType, "UNSUPPORTED_FILE"},
		{"vazio", []byte{}, nil, http.StatusUnsupportedMediaType, "UNSUPPORTED_FILE"},
		{"sem arquivo", nil, map[string]string{"competence": "2026-08"}, http.StatusBadRequest, "FILE_REQUIRED"},
		{"empresa inválida", unique(t, "%PDF-1.4 a"), map[string]string{"company_id": "x"}, http.StatusBadRequest, "VALIDATION_ERROR"},
		{"empresa inexistente", unique(t, "%PDF-1.4 b"), map[string]string{"company_id": "00000000-0000-4000-8000-000000000000"}, http.StatusBadRequest, "VALIDATION_ERROR"},
		{"competência inválida", unique(t, "%PDF-1.4 c"), map[string]string{"competence": "08/2026"}, http.StatusBadRequest, "VALIDATION_ERROR"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if code := e.upload(t, "x.pdf", c.data, c.fields).MustStatus(t, c.status).ErrorCode(t); code != c.code {
				t.Fatalf("código = %s", code)
			}
		})
	}
	big := append([]byte("%PDF-1.4 "), bytes.Repeat([]byte("a"), documents.MaxFileSize)...)
	if code := e.upload(t, "grande.pdf", big, nil).MustStatus(t, http.StatusRequestEntityTooLarge).ErrorCode(t); code != "FILE_TOO_LARGE" {
		t.Fatalf("grande = %s", code)
	}
	if n := e.count(t, `SELECT count(*) FROM reconciliation_documents`); n != 0 {
		t.Fatalf("recusa gravou documento: %d", n)
	}
	// Imagem e texto (latin-1) são aceitos pelo conteúdo, com o charset do texto.
	png := append([]byte("\x89PNG\r\n\x1a\n"), unique(t, "imagem")...)
	jpeg := append([]byte{0xFF, 0xD8, 0xFF, 0xE0}, unique(t, "foto")...)
	csv := append([]byte("data;descri\xe7\xe3o;valor\n01/08/2026;Tarifa;-10,00\n"), unique(t, "csv")...)
	for name, want := range map[string]struct {
		data []byte
		mime string
	}{"comprovante.png": {png, "image/png"}, "foto.jpg": {jpeg, "image/jpeg"}, "extrato.csv": {csv, "text/plain; charset=windows-1252"}} {
		var u uploadOut
		e.upload(t, name, want.data, nil).MustStatus(t, http.StatusAccepted).JSON(t, &u)
		var d docOut
		e.srv.Get("/api/v1/reconciliation/documents/"+u.DocumentID).MustStatus(t, http.StatusOK).JSON(t, &d)
		if d.ContentType != want.mime {
			t.Fatalf("%s: tipo = %s", name, d.ContentType)
		}
	}
}

func TestList_FiltersAndCountByStatus(t *testing.T) {
	e := setup(t)
	alfa := e.company(t, "301", "11222333000181")
	var a, b uploadOut
	e.upload(t, "a.pdf", unique(t, "%PDF-1.4 a"), map[string]string{"company_id": alfa, "competence": "2026-08"}).MustStatus(t, http.StatusAccepted).JSON(t, &a)
	e.upload(t, "b.pdf", unique(t, "%PDF-1.4 b"), nil).MustStatus(t, http.StatusAccepted).JSON(t, &b)
	// O período lido na triagem também casa com a competência (agosto, mês do extrato).
	if _, err := e.pool.Exec(ctx, `UPDATE reconciliation_documents SET status = 'triado', kind = 'extrato', bank = 'Banco A Fonte',
		period_start = '2026-07-25', period_end = '2026-08-31' WHERE id = $1`, b.DocumentID); err != nil {
		t.Fatal(err)
	}
	type list struct {
		Items []docOut         `json:"items"`
		Total int64            `json:"total"`
		Count map[string]int64 `json:"count_by_status"`
	}
	get := func(q string) list {
		t.Helper()
		var l list
		e.srv.Get("/api/v1/reconciliation/documents"+q).MustStatus(t, http.StatusOK).JSON(t, &l)
		return l
	}
	if l := get(""); len(l.Items) != 2 || l.Items[0].ID != b.DocumentID || l.Count["pendente"] != 1 || l.Count["triado"] != 1 {
		t.Fatalf("todos = %+v", l)
	}
	if l := get("?company_id=" + alfa); len(l.Items) != 1 || l.Items[0].ID != a.DocumentID || l.Total != 1 {
		t.Fatalf("empresa = %+v", l)
	}
	if l := get("?competence=2026-08"); len(l.Items) != 2 {
		t.Fatalf("competência (envio e período) = %+v", l)
	}
	if l := get("?competence=2026-06"); len(l.Items) != 0 {
		t.Fatalf("competência sem documentos = %+v", l)
	}
	if l := get("?status=triado&kind=extrato&bank=banco%20a%20fonte"); len(l.Items) != 1 || l.Items[0].ID != b.DocumentID {
		t.Fatalf("status, tipo e banco = %+v", l)
	}
	for _, q := range []string{"?status=x", "?kind=nota", "?company_id=x", "?competence=2026", "?limit=201", "?offset=-1"} {
		e.srv.Get("/api/v1/reconciliation/documents"+q).MustStatus(t, http.StatusBadRequest)
	}
	e.srv.Get("/api/v1/reconciliation/documents/00000000-0000-4000-8000-000000000000").MustStatus(t, http.StatusNotFound)
	e.srv.Get("/api/v1/reconciliation/documents/00000000-0000-4000-8000-000000000000/file").MustStatus(t, http.StatusNotFound)
}
