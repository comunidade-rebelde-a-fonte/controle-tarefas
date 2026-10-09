package objectstore_test

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"controle/internal/platform/objectstore"
	"controle/internal/testutil"
)

func newStore(m testutil.MinIO) *objectstore.Store {
	return objectstore.New(objectstore.Config{Endpoint: m.Endpoint, AccessKey: m.AccessKey, SecretKey: m.SecretKey, Bucket: m.Bucket})
}

func read(t *testing.T, s *objectstore.Store, key, version string) (string, objectstore.Object) {
	t.Helper()
	obj, err := s.Get(context.Background(), key, version)
	if err != nil {
		t.Fatalf("Get %s@%s: %v", key, version, err)
	}
	defer func() { _ = obj.Body.Close() }()
	b, err := io.ReadAll(obj.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b), obj
}

// TEST-015: o original volta byte a byte, pela versão gravada; regravar a chave cria outra
// versão e a primeira continua legível (bucket versionado, REQ-043).
func TestPutGet_VersionedOriginal(t *testing.T) {
	m := testutil.NewMinIO(t)
	s := newStore(m)
	ctx := context.Background()
	if err := s.Ping(ctx); err != nil {
		t.Fatalf("Ping: %v", err)
	}
	key := "originais/" + strings.Repeat("ab", 32)
	first, err := s.Put(ctx, key, []byte("%PDF-1.4 original\n"), "application/pdf")
	if err != nil {
		t.Fatal(err)
	}
	if first.VersionID == "" || first.ETag == "" {
		t.Fatalf("sem versão ou ETag: %+v", first)
	}
	got, obj := read(t, s, key, first.VersionID)
	if got != "%PDF-1.4 original\n" || obj.ContentType != "application/pdf" || obj.Size != int64(len(got)) || obj.VersionID != first.VersionID {
		t.Fatalf("lido = %q %+v", got, obj)
	}
	second, err := s.Put(ctx, key, []byte("outra versão"), "text/plain")
	if err != nil {
		t.Fatal(err)
	}
	if second.VersionID == "" || second.VersionID == first.VersionID {
		t.Fatalf("versões = %s, %s", first.VersionID, second.VersionID)
	}
	if got, _ := read(t, s, key, first.VersionID); got != "%PDF-1.4 original\n" {
		t.Fatalf("a primeira versão mudou: %q", got)
	}
	if got, _ := read(t, s, key, ""); got != "outra versão" {
		t.Fatalf("versão mais recente = %q", got)
	}
}

// 09 §5: a credencial da aplicação não apaga o original nem uma versão dele (política do
// docker/minio/init.sh), e o cliente de produção nem tem operação de apagar.
func TestOriginalIsImmutableForTheApplication(t *testing.T) {
	m := testutil.NewMinIO(t)
	s := newStore(m)
	ctx := context.Background()
	key := "originais/" + strings.Repeat("cd", 32)
	put, err := s.Put(ctx, key, []byte("comprovante"), "application/pdf")
	if err != nil {
		t.Fatal(err)
	}
	for _, version := range []string{"", put.VersionID} {
		if err := objectstore.RawDelete(ctx, s, key, version); !errors.Is(err, objectstore.ErrDenied) {
			t.Fatalf("apagar (versão %q): esperado ErrDenied, veio %v", version, err)
		}
	}
	if got, _ := read(t, s, key, put.VersionID); got != "comprovante" {
		t.Fatalf("original alterado: %q", got)
	}
	for _, name := range []string{"Delete", "Remove", "DeleteObject", "RemoveObject"} {
		if _, ok := reflect.TypeFor[*objectstore.Store]().MethodByName(name); ok {
			t.Fatalf("o cliente não pode ter %s", name)
		}
	}
}

func TestErrors_NotFoundDeniedUnavailableNotConfigured(t *testing.T) {
	m := testutil.NewMinIO(t)
	ctx := context.Background()
	if _, err := newStore(m).Get(ctx, "originais/inexistente", ""); !errors.Is(err, objectstore.ErrNotFound) {
		t.Fatalf("objeto inexistente: %v", err)
	}
	wrong := m
	wrong.SecretKey = "segredo-errado"
	if _, err := newStore(wrong).Put(ctx, "originais/x", []byte("x"), "text/plain"); !errors.Is(err, objectstore.ErrDenied) {
		t.Fatalf("credencial errada: %v", err)
	}
	// MinIO fora: porta fechada.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	down := m
	down.Endpoint = "http://" + ln.Addr().String()
	_ = ln.Close()
	if _, err := newStore(down).Put(ctx, "originais/x", []byte("x"), "text/plain"); !errors.Is(err, objectstore.ErrUnavailable) {
		t.Fatalf("MinIO fora: %v", err)
	}
	if err := objectstore.New(objectstore.Config{}).Ping(ctx); !errors.Is(err, objectstore.ErrNotConfigured) {
		t.Fatalf("sem configuração: %v", err)
	}
}

// Erro 5xx do serviço é indisponibilidade; a requisição sai assinada e com o hash do corpo.
func TestServerErrorIsUnavailableAndRequestIsSigned(t *testing.T) {
	var auth, sha string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth, sha = r.Header.Get("Authorization"), r.Header.Get("x-amz-content-sha256")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, "<Error><Code>SlowDown</Code></Error>")
	}))
	defer srv.Close()
	s := objectstore.New(objectstore.Config{Endpoint: srv.URL, AccessKey: "app", SecretKey: "s", Bucket: "b", Timeout: 5 * time.Second})
	_, err := s.Put(context.Background(), "originais/k", []byte("abc"), "text/plain")
	if !errors.Is(err, objectstore.ErrUnavailable) || !strings.Contains(err.Error(), "SlowDown") {
		t.Fatalf("erro = %v", err)
	}
	if !strings.HasPrefix(auth, "AWS4-HMAC-SHA256 Credential=app/") || !strings.Contains(auth, "SignedHeaders=content-type;host;x-amz-content-sha256;x-amz-date") ||
		sha != "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad" {
		t.Fatalf("Authorization = %q, sha = %q", auth, sha)
	}
}
