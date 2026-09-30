package user

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMiddleware_InjectsImplicitAdmin(t *testing.T) {
	var got string
	h := Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = FromContext(r.Context())
		_, _ = io.WriteString(w, got)
	}))
	for _, path := range []string{"/api/v1/health", "/api/v1/companies", "/qualquer"} {
		got = ""
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil))
		if got != AdminID {
			t.Fatalf("%s: usuário no contexto = %q, want %q", path, got, AdminID)
		}
	}
}
