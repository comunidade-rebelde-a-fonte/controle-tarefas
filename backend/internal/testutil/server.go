package testutil

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"controle/internal/app"
	"controle/internal/platform/clock"
	"controle/internal/platform/config"
)

// SaoPaulo é o fuso de negócio padrão.
var SaoPaulo = mustLoad("America/Sao_Paulo")

func mustLoad(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return loc
}

// FixedClock cria um relógio parado no instante RFC 3339 informado, no fuso de São Paulo.
func FixedClock(t testing.TB, rfc3339 string) *clock.Fixed {
	t.Helper()
	ts, err := time.Parse(time.RFC3339, rfc3339)
	if err != nil {
		t.Fatalf("testutil: instante inválido %q: %v", rfc3339, err)
	}
	return clock.NewFixed(ts, SaoPaulo)
}

// TestConfig devolve uma configuração válida para testes.
func TestConfig(t testing.TB) config.Config {
	t.Helper()
	return config.Config{
		Env:            config.EnvTest,
		HTTPPort:       0,
		DatabaseURL:    "postgres://unused",
		DBMaxConns:     10,
		TimezoneName:   "America/Sao_Paulo",
		Location:       SaoPaulo,
		CORSOrigins:    []string{"http://localhost:5173"},
		FileStorageDir: t.TempDir(),
		LogLevel:       slog.LevelError,
	}
}

// Server é um servidor HTTP de teste com a aplicação completa.
type Server struct {
	*httptest.Server
	Pool  *pgxpool.Pool
	Clock *clock.Fixed
	t     testing.TB
}

// NewServer sobe a API completa sobre o pool informado, com relógio fixo.
func NewServer(t testing.TB, pool *pgxpool.Pool, clk *clock.Fixed, opts ...func(*config.Config)) *Server {
	t.Helper()
	cfg := TestConfig(t)
	for _, o := range opts {
		o(&cfg)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := app.NewRouter(app.Deps{Config: cfg, Pool: pool, Clock: clk, Logger: logger})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &Server{Server: srv, Pool: pool, Clock: clk, t: t}
}

// Response é uma resposta HTTP já lida.
type Response struct {
	Status int
	Header http.Header
	Body   []byte
}

// JSON decodifica o corpo em v (falha o teste se não for possível).
func (r Response) JSON(t testing.TB, v any) {
	t.Helper()
	if err := json.Unmarshal(r.Body, v); err != nil {
		t.Fatalf("resposta não é JSON válido (%d): %v\n%s", r.Status, err, r.Body)
	}
}

// ErrorCode devolve o campo code do envelope de erro.
func (r Response) ErrorCode(t testing.TB) string {
	t.Helper()
	var e struct {
		Code string `json:"code"`
	}
	r.JSON(t, &e)
	return e.Code
}

// Do executa uma requisição; body (se não nil) é serializado como JSON.
func (s *Server) Do(method, path string, body any) Response {
	s.t.Helper()
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			s.t.Fatalf("serializar corpo: %v", err)
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(context.Background(), method, s.URL+path, rdr)
	if err != nil {
		s.t.Fatalf("montar requisição: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := s.Client().Do(req)
	if err != nil {
		s.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		s.t.Fatalf("ler resposta: %v", err)
	}
	return Response{Status: resp.StatusCode, Header: resp.Header, Body: b}
}

// Get, Post, Put e Patch são atalhos de Do.
func (s *Server) Get(path string) Response             { return s.Do(http.MethodGet, path, nil) }
func (s *Server) Post(path string, body any) Response  { return s.Do(http.MethodPost, path, body) }
func (s *Server) Put(path string, body any) Response   { return s.Do(http.MethodPut, path, body) }
func (s *Server) Patch(path string, body any) Response { return s.Do(http.MethodPatch, path, body) }

// MustStatus falha o teste se o status for diferente do esperado.
func (r Response) MustStatus(t testing.TB, want int) Response {
	t.Helper()
	if r.Status != want {
		t.Fatalf("status = %d, want %d\n%s", r.Status, want, r.Body)
	}
	return r
}
