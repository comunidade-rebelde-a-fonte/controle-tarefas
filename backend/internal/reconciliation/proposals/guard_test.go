package proposals_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// TEST-019 [AC-019]: prova, no código, de que nenhum caminho cria conciliação sem ação humana.
// Só duas consultas gravam em reconciliations: a da rodada determinística (passes 1 a 4,
// AC-018) e a do aceite humano. A do aceite só é chamada pelo aceite; o agente não chama
// nenhuma das duas.

const backend = "../../.."

var insertReconciliation = regexp.MustCompile(`(?is)-- name: (\w+) :\w+[^;]*?INSERT INTO reconciliations\b`)

func TestTEST019_OnlyTwoQueriesCreateReconciliations(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(backend, "queries", "*.sql"))
	if err != nil || len(files) == 0 {
		t.Fatalf("consultas: %v", err)
	}
	var names []string
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		// Cada consulta separada: o nome mais próximo antes do INSERT.
		for _, q := range strings.Split(string(b), "-- name: ")[1:] {
			if m := insertReconciliation.FindStringSubmatch("-- name: " + q); m != nil {
				names = append(names, m[1])
			}
		}
	}
	slices.Sort(names)
	if want := []string{"InsertAutoReconciliation", "InsertManualReconciliation"}; !slices.Equal(names, want) {
		t.Fatalf("consultas que criam conciliação = %v, esperado %v", names, want)
	}
}

// callers devolve "arquivo:função" de cada chamada aos métodos dados, no código de produção.
func callers(t *testing.T, methods ...string) map[string][]string {
	t.Helper()
	out := map[string][]string{}
	fset := token.NewFileSet()
	err := filepath.WalkDir(filepath.Join(backend, "internal"), func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") ||
			strings.Contains(filepath.ToSlash(path), "/internal/store/") {
			return err
		}
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(backend, path)
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			name := fn.Name.Name
			if fn.Recv != nil && len(fn.Recv.List) > 0 {
				name = recvName(fn.Recv.List[0].Type) + "." + name
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				if sel, ok := n.(*ast.SelectorExpr); ok && slices.Contains(methods, sel.Sel.Name) {
					out[sel.Sel.Name] = append(out[sel.Sel.Name], filepath.ToSlash(rel)+":"+name)
				}
				return true
			})
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func recvName(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.StarExpr:
		return recvName(v.X)
	case *ast.Ident:
		return v.Name
	}
	return "?"
}

func TestTEST019_OnlyHumanAcceptCallsTheManualInsert(t *testing.T) {
	got := callers(t, "InsertManualReconciliation", "InsertAutoReconciliation", "InsertReconciliationItem", "DecideResolverProposal")
	want := map[string][]string{
		"InsertManualReconciliation": {"internal/reconciliation/proposals/service.go:Service.Accept"},
		"InsertAutoReconciliation":   {"internal/reconciliation/reconcile/service.go:Service.RunTx"},
		"DecideResolverProposal": {"internal/reconciliation/proposals/service.go:Service.Accept",
			"internal/reconciliation/proposals/service.go:Service.Reject"},
	}
	for method, places := range want {
		if !slices.Equal(got[method], places) {
			t.Errorf("%s chamado em %v, esperado só em %v", method, got[method], places)
		}
	}
	for _, place := range got["InsertReconciliationItem"] {
		if place != "internal/reconciliation/proposals/service.go:Service.Accept" && !strings.HasPrefix(place, "internal/reconciliation/reconcile/") {
			t.Errorf("InsertReconciliationItem chamado em %s", place)
		}
	}
	// O agente (agent.go) não chama nenhuma delas.
	for method, places := range got {
		for _, p := range places {
			if strings.HasPrefix(p, "internal/reconciliation/proposals/agent.go") {
				t.Errorf("o agente chama %s (%s)", method, p)
			}
		}
	}
}
