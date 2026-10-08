package app_test

import (
	"bufio"
	"context"
	"net/http"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"controle/internal/app"
	"controle/internal/platform/db"
	"controle/internal/testutil"
)

var (
	openAPIPath   = regexp.MustCompile(`^  (/\S*):$`)
	openAPIMethod = regexp.MustCompile(`^    (get|post|put|patch|delete):$`)
	pathParam     = regexp.MustCompile(`\{[^}]+\}`)
)

// openAPIOperations lê "MÉTODO /caminho" de api/openapi.yaml (formato do próprio arquivo:
// caminho com 2 espaços, método com 4).
func openAPIOperations(t *testing.T) map[string]bool {
	t.Helper()
	f, err := os.Open("../../api/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	ops := map[string]bool{}
	inPaths, current := false, ""
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case line == "paths:":
			inPaths = true
		case inPaths && line != "" && !strings.HasPrefix(line, " "):
			inPaths = false
		case inPaths:
			if m := openAPIPath.FindStringSubmatch(line); m != nil {
				current = pathParam.ReplaceAllString(m[1], "{}")
			} else if m := openAPIMethod.FindStringSubmatch(line); m != nil && current != "" {
				ops[strings.ToUpper(m[1])+" "+current] = true
			}
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return ops
}

// OpenAPI é o contrato frontend/backend (CLAUDE.md): toda rota da API precisa estar nele.
func TestOpenAPI_DocumentsEveryRoute(t *testing.T) {
	pool, err := db.NewPool(context.Background(), "postgres://u:p@127.0.0.1:1/nenhum", 1) // não conecta
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	a := app.New(app.Deps{Config: testutil.TestConfig(t), Pool: pool, Clock: testutil.FixedClock(t, "2026-10-01T12:00:00Z")})
	ops := openAPIOperations(t)
	var missing []string
	walked := 0
	err = chi.Walk(a.Handler.(chi.Routes), func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		p, ok := strings.CutPrefix(route, "/api/v1")
		if !ok {
			return nil
		}
		walked++
		key := method + " " + pathParam.ReplaceAllString(p, "{}")
		if !ops[key] {
			missing = append(missing, key)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if walked < 50 || !ops["GET /factor-r/portfolio"] || !ops["PUT /companies/{}/factor-r/movements/{}"] {
		t.Fatalf("leitura incompleta: %d rotas, %d operações no OpenAPI", walked, len(ops))
	}
	slices.Sort(missing)
	if len(missing) > 0 {
		t.Fatalf("rotas sem documentação em api/openapi.yaml:\n%s", strings.Join(missing, "\n"))
	}
}
