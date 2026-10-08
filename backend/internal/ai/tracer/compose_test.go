package tracer_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// serviceBlocks separa o compose por serviço (nível de indentação de dois espaços).
func serviceBlocks(t *testing.T, file string) map[string]string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "..", file))
	if err != nil {
		t.Fatal(err)
	}
	text := strings.ReplaceAll(string(b), "\r\n", "\n")
	re := regexp.MustCompile(`(?m)^  ([a-z][a-z0-9-]*):\s*$`)
	locs := re.FindAllStringSubmatchIndex(text, -1)
	out := map[string]string{}
	for i, loc := range locs {
		end := len(text)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		out[text[loc[2]:loc[3]]] = text[loc[0]:end]
	}
	return out
}

// Só o worker envia ao Langfuse: a chave secreta não chega à API nem aos demais serviços
// [09 §4]; worker e langfuse-web dividem a rede controle-obs (D-022).
func TestCompose_LangfuseSecretOnlyInWorker(t *testing.T) {
	main := serviceBlocks(t, "docker-compose.yml")
	if len(main) < 5 {
		t.Fatalf("só %d serviços no docker-compose.yml", len(main))
	}
	for name, block := range main {
		has := strings.Contains(block, "LANGFUSE_SECRET_KEY")
		if name == "worker" && !has {
			t.Error("o worker deveria receber LANGFUSE_SECRET_KEY")
		}
		if name != "worker" && has {
			t.Errorf("o serviço %s não pode receber LANGFUSE_SECRET_KEY: só o worker envia ao Langfuse", name)
		}
	}
	if !strings.Contains(main["worker"], "networks: [default, observabilidade]") || !strings.Contains(main["worker"], "LANGFUSE_HOST: http://langfuse:3000") {
		t.Error("o worker deveria estar na rede observabilidade e falar com http://langfuse:3000")
	}
	if !strings.Contains(main["api"], "LANGFUSE_PUBLIC_URL") || strings.Contains(main["api"], "LANGFUSE_HOST") {
		t.Error("a API só monta o link do trace (LANGFUSE_PUBLIC_URL), sem acesso ao Langfuse")
	}
	lf := serviceBlocks(t, "docker-compose.langfuse.yml")
	if !strings.Contains(lf["langfuse-web"], "aliases: [langfuse]") {
		t.Error("o langfuse-web deveria responder como \"langfuse\" na rede observabilidade")
	}
	for _, f := range []string{"docker-compose.yml", "docker-compose.langfuse.yml"} {
		b, _ := os.ReadFile(filepath.Join("..", "..", "..", "..", f))
		text := strings.ReplaceAll(string(b), "\r\n", "\n")
		if !regexp.MustCompile(`(?m)^  observabilidade:\n    name: controle-obs\n    external: true$`).MatchString(text) {
			t.Errorf("%s: a rede observabilidade deveria ser a externa controle-obs", f)
		}
	}
}
