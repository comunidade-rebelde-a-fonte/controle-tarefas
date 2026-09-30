package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"controle/internal/platform/apperr"
)

func decodeError(t *testing.T, rec *httptest.ResponseRecorder) ErrorBody {
	t.Helper()
	var body ErrorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("resposta não é JSON: %v (%s)", err, rec.Body.String())
	}
	return body
}

func TestWriteError_DomainErrorsUseEnvelope(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"400", apperr.BadRequest("INVALID_PAYLOAD", "Dados inválidos."), 400, "INVALID_PAYLOAD"},
		{"404", apperr.NotFound("COMPANY_NOT_FOUND", "Empresa não encontrada."), 404, "COMPANY_NOT_FOUND"},
		{"409", apperr.VersionConflict(), 409, "VERSION_CONFLICT"},
		{"410", apperr.Gone("IMPORT_EXPIRED", "Prévia expirada."), 410, "IMPORT_EXPIRED"},
		{"422", apperr.Unprocessable("DEPENDENCY_BLOCKED", "Bloqueado."), 422, "DEPENDENCY_BLOCKED"},
		{"423", apperr.Locked("PERIOD_CLOSED", "Período fechado."), 423, "PERIOD_CLOSED"},
		{"wrapped", errors.Join(errors.New("ctx"), apperr.NotFound("X", "Não encontrado.")), 404, "X"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			WriteError(rec, httptest.NewRequest(http.MethodGet, "/", nil), tc.err)
			if rec.Code != tc.status {
				t.Fatalf("status = %d, want %d", rec.Code, tc.status)
			}
			if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
				t.Fatalf("content-type = %q", ct)
			}
			body := decodeError(t, rec)
			if body.Code != tc.code || body.Message == "" {
				t.Fatalf("envelope = %+v", body)
			}
		})
	}
}

func TestWriteError_DetailsAreSerialized(t *testing.T) {
	rec := httptest.NewRecorder()
	err := apperr.Conflict("CNPJ_CONFLICT", "CNPJ já cadastrado.").
		WithDetails(map[string]any{"conflicting_company": map[string]any{"internal_code": "101"}})
	WriteError(rec, httptest.NewRequest(http.MethodGet, "/", nil), err)
	body := decodeError(t, rec)
	cc, ok := body.Details["conflicting_company"].(map[string]any)
	if !ok || cc["internal_code"] != "101" {
		t.Fatalf("details = %#v", body.Details)
	}
}

func TestWriteError_UnknownErrorIsGeneric500(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteError(rec, httptest.NewRequest(http.MethodGet, "/", nil), errors.New("pq: senha=segredo"))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
	body := decodeError(t, rec)
	if body.Code != "INTERNAL_ERROR" || strings.Contains(body.Message, "segredo") {
		t.Fatalf("envelope vazou detalhes internos: %+v", body)
	}
}

func TestWriteError_ClientCanceledIsNot500(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	rec := httptest.NewRecorder()
	WriteError(rec, httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx), fmt.Errorf("consulta: %w", context.Canceled))
	if rec.Code != StatusClientClosedRequest || rec.Body.Len() != 0 {
		t.Fatalf("status = %d, corpo = %q", rec.Code, rec.Body.String())
	}
	// Cancelamento que não veio do cliente continua sendo erro interno.
	rec = httptest.NewRecorder()
	WriteError(rec, httptest.NewRequest(http.MethodGet, "/", nil), context.Canceled)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestDecodeJSON(t *testing.T) {
	type payload struct {
		Name string `json:"name"`
	}
	cases := []struct {
		name string
		body string
		code string
	}{
		{"ok", `{"name":"a"}`, ""},
		{"empty", ``, "EMPTY_BODY"},
		{"unknown field", `{"name":"a","x":1}`, "INVALID_JSON"},
		{"malformed", `{"name":`, "INVALID_JSON"},
		{"two objects", `{"name":"a"}{"name":"b"}`, "INVALID_JSON"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tc.body))
			r.Header.Set("Content-Type", "application/json")
			var p payload
			err := DecodeJSON(httptest.NewRecorder(), r, &p)
			if tc.code == "" {
				if err != nil {
					t.Fatalf("erro inesperado: %v", err)
				}
				return
			}
			ae, ok := apperr.As(err)
			if !ok || ae.Code != tc.code || ae.Status != http.StatusBadRequest {
				t.Fatalf("err = %v, want code %s", err, tc.code)
			}
		})
	}
}

func TestCORS(t *testing.T) {
	h := CORS([]string{"http://localhost:5173"})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	t.Run("origem listada recebe cabeçalho", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
		r.Header.Set("Origin", "http://localhost:5173")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
			t.Fatalf("ACAO = %q", got)
		}
	})

	t.Run("origem não listada não recebe cabeçalho", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
		r.Header.Set("Origin", "http://evil.example")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Fatalf("ACAO = %q, want vazio", got)
		}
	})

	t.Run("preflight de origem não listada é negado", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodOptions, "/api/v1/companies", nil)
		r.Header.Set("Origin", "http://evil.example")
		r.Header.Set("Access-Control-Request-Method", "POST")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		if rec.Code != http.StatusForbidden || rec.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Fatalf("status = %d, ACAO = %q", rec.Code, rec.Header().Get("Access-Control-Allow-Origin"))
		}
	})

	t.Run("preflight de origem listada é aceito", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodOptions, "/api/v1/companies", nil)
		r.Header.Set("Origin", "http://localhost:5173")
		r.Header.Set("Access-Control-Request-Method", "PATCH")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		if rec.Code != http.StatusNoContent || !strings.Contains(rec.Header().Get("Access-Control-Allow-Methods"), "PATCH") {
			t.Fatalf("status = %d, methods = %q", rec.Code, rec.Header().Get("Access-Control-Allow-Methods"))
		}
	})
}

func TestRequestIDAndRecoverer(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := RequestID(logger)(Recoverer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	})))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusInternalServerError || decodeError(t, rec).Code != "INTERNAL_ERROR" {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get(RequestIDHeader) == "" {
		t.Fatal("request id ausente")
	}

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set(RequestIDHeader, "abc-123")
	rec = httptest.NewRecorder()
	RequestID(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, RequestIDFromContext(r.Context()))
	})).ServeHTTP(rec, r)
	if rec.Body.String() != "abc-123" {
		t.Fatalf("request id = %q", rec.Body.String())
	}
}

type fakePinger struct{ err error }

func (f fakePinger) Ping(context.Context) error { return f.err }

func TestHealth(t *testing.T) {
	rec := httptest.NewRecorder()
	Health(fakePinger{})(rec, httptest.NewRequest(http.MethodGet, "/api/v1/health", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	Health(fakePinger{err: errors.New("connection refused")})(rec, httptest.NewRequest(http.MethodGet, "/api/v1/health", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}
