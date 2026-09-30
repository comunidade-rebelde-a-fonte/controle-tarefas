package httpx

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func spaDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0o750); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"index.html": "<html>spa</html>", "assets/app-1a2b.js": "console.log(1)", "favicon.svg": "<svg/>"} {
		if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(name)), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestSPA_ServesFilesAndFallsBackToIndex(t *testing.T) {
	h := SecurityHeaders(SPA(spaDir(t)))
	cases := []struct {
		method, path string
		status       int
		body, cache  string
	}{
		{http.MethodGet, "/", 200, "<html>spa</html>", "no-cache"},
		{http.MethodGet, "/fiscal", 200, "<html>spa</html>", "no-cache"},
		{http.MethodGet, "/companies/123?tab=history", 200, "<html>spa</html>", "no-cache"},
		{http.MethodGet, "/assets/app-1a2b.js", 200, "console.log(1)", "public, max-age=31536000, immutable"},
		{http.MethodGet, "/favicon.svg", 200, "<svg/>", ""},
		{http.MethodGet, "/../../etc/passwd", 400, "invalid URL path", ""}, // recusado pela biblioteca padrão
		{http.MethodGet, "/api/v1/nao-existe", 404, `"code":"NOT_FOUND"`, ""},
		{http.MethodPost, "/fiscal", 405, "", ""},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(c.method, c.path, nil))
		if rec.Code != c.status || !strings.Contains(rec.Body.String(), c.body) || rec.Header().Get("Cache-Control") != c.cache {
			t.Errorf("%s %s = %d %q cache %q", c.method, c.path, rec.Code, rec.Body.String(), rec.Header().Get("Cache-Control"))
		}
		if rec.Header().Get("X-Content-Type-Options") != "nosniff" || rec.Header().Get("X-Frame-Options") != "DENY" ||
			rec.Header().Get("Referrer-Policy") != "same-origin" {
			t.Errorf("%s %s sem cabeçalhos de segurança: %v", c.method, c.path, rec.Header())
		}
	}
}
