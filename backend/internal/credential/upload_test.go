package credential_test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"io"
	"math/big"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"controle/internal/platform/config"
	"controle/internal/testutil"
	"controle/internal/testutil/seeded"
)

type fileBody struct {
	Name      string  `json:"name"`
	MimeType  *string `json:"mime_type"`
	SizeBytes *int64  `json:"size_bytes"`
}

type controlWithFile struct {
	Type    string    `json:"type"`
	File    *fileBody `json:"file"`
	Version int       `json:"version"`
}

type errBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

const pdfDoc = "%PDF-1.4\n1 0 obj << /Type /Catalog >> endobj\n%%EOF\n"

func uploadServer(t *testing.T) (*testutil.Server, string) {
	t.Helper()
	dir := t.TempDir()
	srv := testutil.NewServer(t, seeded.NewCompaniesDB(t), testutil.FixedClock(t, "2026-10-01T12:00:00Z"),
		func(c *config.Config) { c.FileStorageDir = dir })
	return srv, dir
}

// upload envia o documento como multipart/form-data (campo "file" e "version" opcional).
func upload(t *testing.T, srv *testutil.Server, company, typ, name string, data []byte, version string) testutil.Response {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	if version != "" {
		if err := mw.WriteField("version", version); err != nil {
			t.Fatal(err)
		}
	}
	if name != "" {
		fw, err := mw.CreateFormFile("file", name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fw.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	return send(t, srv, http.MethodPut, "/api/v1/companies/"+company+"/credential-controls/"+typ+"/file", mw.FormDataContentType(), &body)
}

func send(t *testing.T, srv *testutil.Server, method, path, contentType string, body io.Reader) testutil.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), method, srv.URL+path, body)
	if err != nil {
		t.Fatal(err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return testutil.Response{Status: resp.StatusCode, Header: resp.Header, Body: b}
}

func storedFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

func storageKey(t *testing.T, srv *testutil.Server, company, typ string) *string {
	t.Helper()
	var key *string
	if err := srv.Pool.QueryRow(context.Background(),
		`SELECT file_storage_key FROM credential_controls WHERE company_id = $1 AND type = $2`, company, typ).Scan(&key); err != nil {
		t.Fatal(err)
	}
	return key
}

// syntheticCert gera um certificado autoassinado e a chave, apenas em memória.
func syntheticCert(t *testing.T) (der, certPEM, keyPEM []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(7), Subject: pkix.Name{CommonName: "Certificado sintético de teste"},
		NotBefore: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), NotAfter: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	der, err = x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return der, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
}

func TestCA020_UploadEndpointRejectsPrivateKeys(t *testing.T) {
	srv, dir := uploadServer(t)
	alfa := companyID(t, srv, "101")
	_, certPEM, keyPEM := syntheticCert(t)

	for _, c := range []struct {
		name string
		data []byte
	}{
		{"certificado.pfx", []byte{0x30, 0x80, 0x02, 0x01, 0x03}},
		{"chave.pem", keyPEM},
		{"completo.pem", append(append([]byte{}, certPEM...), keyPEM...)},
	} {
		res := upload(t, srv, alfa, "DIGITAL_CERTIFICATE", c.name, c.data, "").MustStatus(t, http.StatusUnprocessableEntity)
		var e errBody
		res.JSON(t, &e)
		if e.Code != "PRIVATE_KEY_REJECTED" || e.Message != "Chaves privadas não são armazenadas pelo sistema." {
			t.Fatalf("%s: erro = %+v", c.name, e)
		}
	}
	if files := storedFiles(t, dir); len(files) != 0 {
		t.Fatalf("nada deveria ir para o disco: %v", files)
	}
	var n int
	if err := srv.Pool.QueryRow(context.Background(), `SELECT count(*) FROM credential_controls`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("nenhum registro deveria ser criado: n=%d err=%v", n, err)
	}
}

func TestUpload_StoresReplacesAndDeletes(t *testing.T) {
	srv, dir := uploadServer(t)
	alfa := companyID(t, srv, "101")

	var c controlWithFile
	upload(t, srv, alfa, "RFB_POWER_OF_ATTORNEY", "../../procuração 2026.pdf", []byte(pdfDoc), "").MustStatus(t, http.StatusOK).JSON(t, &c)
	if c.File == nil || c.File.Name != "procuração 2026.pdf" || *c.File.MimeType != "application/pdf" || *c.File.SizeBytes != int64(len(pdfDoc)) {
		t.Fatalf("arquivo = %+v", c.File)
	}
	first := storageKey(t, srv, alfa, "RFB_POWER_OF_ATTORNEY")
	if files := storedFiles(t, dir); len(files) != 1 || files[0] != *first {
		t.Fatalf("disco = %v, want [%s]", files, *first)
	}
	stored, err := os.ReadFile(filepath.Join(dir, *first))
	if err != nil || string(stored) != pdfDoc {
		t.Fatalf("conteúdo gravado difere: %v", err)
	}

	// Substituição remove o arquivo anterior [PRD §19.1].
	second := pdfDoc + "% nova versão\n"
	upload(t, srv, alfa, "RFB_POWER_OF_ATTORNEY", "procuracao-nova.pdf", []byte(second), "").MustStatus(t, http.StatusOK).JSON(t, &c)
	key := storageKey(t, srv, alfa, "RFB_POWER_OF_ATTORNEY")
	if *key == *first {
		t.Fatal("a substituição deveria gerar outro arquivo")
	}
	if files := storedFiles(t, dir); len(files) != 1 || files[0] != *key {
		t.Fatalf("disco após substituir = %v", files)
	}

	// Versão divergente não troca o arquivo.
	upload(t, srv, alfa, "RFB_POWER_OF_ATTORNEY", "outra.pdf", []byte(pdfDoc), "1").MustStatus(t, http.StatusConflict)
	if files := storedFiles(t, dir); len(files) != 1 || files[0] != *key {
		t.Fatalf("disco após conflito = %v", files)
	}

	// A grade mostra o documento vigente.
	var l struct {
		Items []struct {
			Company struct {
				ID string `json:"id"`
			} `json:"company"`
			PowerOfAttorney controlWithFile `json:"rfb_power_of_attorney"`
		} `json:"items"`
	}
	srv.Get("/api/v1/credential-controls").MustStatus(t, http.StatusOK).JSON(t, &l)
	found := false
	for _, it := range l.Items {
		if it.Company.ID == alfa {
			found = it.PowerOfAttorney.File != nil && it.PowerOfAttorney.File.Name == "procuracao-nova.pdf"
		}
	}
	if !found {
		t.Fatal("grade deveria trazer o documento vigente")
	}

	// Remoção apaga o arquivo e mantém os demais dados.
	res := send(t, srv, http.MethodDelete, "/api/v1/companies/"+alfa+"/credential-controls/RFB_POWER_OF_ATTORNEY/file", "", nil).MustStatus(t, http.StatusOK)
	var after controlWithFile
	if err := json.Unmarshal(res.Body, &after); err != nil || after.File != nil {
		t.Fatalf("após remover = %+v (%v)", after, err)
	}
	if files := storedFiles(t, dir); len(files) != 0 {
		t.Fatalf("disco após remover = %v", files)
	}
	if code := send(t, srv, http.MethodDelete, "/api/v1/companies/"+alfa+"/credential-controls/RFB_POWER_OF_ATTORNEY/file", "", nil).
		MustStatus(t, http.StatusNotFound).ErrorCode(t); code != "FILE_NOT_FOUND" {
		t.Fatalf("code = %s", code)
	}
}

func TestUpload_CertificateAndLimits(t *testing.T) {
	srv, dir := uploadServer(t)
	beta := companyID(t, srv, "102")
	der, certPEM, _ := syntheticCert(t)

	var c controlWithFile
	upload(t, srv, beta, "DIGITAL_CERTIFICATE", "certificado.cer", der, "").MustStatus(t, http.StatusOK).JSON(t, &c)
	if c.File == nil || *c.File.MimeType != "application/pkix-cert" {
		t.Fatalf("certificado DER = %+v", c.File)
	}
	upload(t, srv, beta, "DIGITAL_CERTIFICATE", "certificado.pem", certPEM, "").MustStatus(t, http.StatusOK).JSON(t, &c)
	if *c.File.MimeType != "application/x-pem-file" {
		t.Fatalf("certificado PEM = %+v", c.File)
	}
	if code := upload(t, srv, beta, "DIGITAL_CERTIFICATE", "procuracao.pdf", []byte(pdfDoc), "").
		MustStatus(t, http.StatusUnprocessableEntity).ErrorCode(t); code != "UNSUPPORTED_FILE" {
		t.Fatalf("PDF como certificado: code = %s", code)
	}

	big := append([]byte("%PDF-1.4\n"), bytes.Repeat([]byte("x"), 10<<20)...)
	if code := upload(t, srv, beta, "RFB_POWER_OF_ATTORNEY", "grande.pdf", big, "").
		MustStatus(t, http.StatusBadRequest).ErrorCode(t); code != "FILE_TOO_LARGE" {
		t.Fatalf("arquivo grande: code = %s", code)
	}
	if code := upload(t, srv, beta, "RFB_POWER_OF_ATTORNEY", "", nil, "1").
		MustStatus(t, http.StatusBadRequest).ErrorCode(t); code != "FILE_REQUIRED" {
		t.Fatalf("sem arquivo: code = %s", code)
	}
	if code := send(t, srv, http.MethodPut, "/api/v1/companies/"+beta+"/credential-controls/RFB_POWER_OF_ATTORNEY/file", "application/json", bytes.NewReader([]byte("{}"))).
		MustStatus(t, http.StatusBadRequest).ErrorCode(t); code != "INVALID_MULTIPART" {
		t.Fatalf("sem multipart: code = %s", code)
	}
	upload(t, srv, beta, "PASSWORD", "x.pdf", []byte(pdfDoc), "").MustStatus(t, http.StatusNotFound)
	upload(t, srv, "00000000-0000-4000-8000-000000000000", "RFB_POWER_OF_ATTORNEY", "x.pdf", []byte(pdfDoc), "").MustStatus(t, http.StatusNotFound)

	// Só o certificado PEM vigente ficou no disco.
	if files := storedFiles(t, dir); len(files) != 1 {
		t.Fatalf("disco = %v", files)
	}
}
