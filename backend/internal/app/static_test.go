package app_test

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"controle/internal/platform/config"
	"controle/internal/testutil"
)

// A imagem de produção serve a SPA e a API pelo mesmo binário [T-M7-06].
func TestApp_ServesSPAWhenStaticDirIsSet(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html>controle</html>"), 0o600); err != nil {
		t.Fatal(err)
	}
	srv := testutil.NewServer(t, testutil.NewDB(t), testutil.FixedClock(t, "2026-10-01T12:00:00Z"), func(c *config.Config) { c.StaticDir = dir })

	get := func(path string) (int, string, http.Header) {
		res, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = res.Body.Close() }()
		b, _ := io.ReadAll(res.Body)
		return res.StatusCode, string(b), res.Header
	}
	if code, body, h := get("/dashboard"); code != 200 || body != "<html>controle</html>" || h.Get("X-Frame-Options") != "DENY" {
		t.Fatalf("/dashboard = %d %q", code, body)
	}
	if code, body, _ := get("/api/v1/health"); code != 200 || !strings.Contains(body, `"status":"ok"`) {
		t.Fatalf("/api/v1/health = %d %q", code, body)
	}
	if code, body, _ := get("/api/v1/nao-existe"); code != 404 || !strings.Contains(body, "NOT_FOUND") {
		t.Fatalf("/api/v1/nao-existe = %d %q", code, body)
	}
}
