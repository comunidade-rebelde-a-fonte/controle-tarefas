package ai

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

func TestParseModel(t *testing.T) {
	valid := map[string]Model{
		"openai/gpt-6-luna":            {"openai", "gpt-6-luna"},
		" openai/gpt-5.4-mini ":        {"openai", "gpt-5.4-mini"},
		"anthropic/claude-sonnet-4-5":  {"anthropic", "claude-sonnet-4-5"},
		"openai/gpt-4o-2024-08-06":     {"openai", "gpt-4o-2024-08-06"},
		"openai/ft:gpt-4o:org:custom1": {"openai", "ft:gpt-4o:org:custom1"},
	}
	for in, want := range valid {
		got, err := ParseModel(in)
		if err != nil || got != want {
			t.Errorf("ParseModel(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	invalid := []string{"", "gpt-6-luna", "openai/", "/gpt-6", "azure/gpt-6", "openai/gpt 6", "openai/-x", "openai/a/b",
		// Chave colada no campo do modelo: recusada, nunca gravada [REQ-047].
		"openai/sk-proj-abc123", "openai/SK-abc", "anthropic/pk-live-1", "openai/rk-x"}
	for _, in := range invalid {
		if _, err := ParseModel(in); err == nil {
			t.Errorf("ParseModel(%q) deveria falhar", in)
		}
	}
	if _, err := NewModel("openai", "sk-proj-abc123"); err == nil || strings.Contains(err.Error(), "abc123") {
		t.Fatalf("NewModel com chave: err = %v (deve recusar sem ecoar o valor)", err)
	}
}

func TestDefaultsFromEnv(t *testing.T) {
	env := map[string]string{"TRIADOR_MODEL": "openai/gpt-6-luna", "EXTRATOR_MODEL": " openai/gpt-6.1-sol "}
	d, err := DefaultsFromEnv(func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	if d[Triador].String() != "openai/gpt-6-luna" || d[Extrator].String() != "openai/gpt-6.1-sol" {
		t.Fatalf("defaults = %v", d)
	}
	if _, ok := d[Resolvedor]; ok {
		t.Fatal("alias sem variável deve ficar sem modelo")
	}

	_, err = DefaultsFromEnv(func(k string) string {
		if k == "CONSULTOR_MODEL" {
			return "gpt-6-luna"
		}
		return ""
	})
	if err == nil || !strings.Contains(err.Error(), "CONSULTOR_MODEL") {
		t.Fatalf("erro deve citar a variável inválida, veio %v", err)
	}
}

func TestResolver(t *testing.T) {
	defaults := Defaults{Triador: {"openai", "gpt-6-luna"}}
	r := NewResolver(defaults)

	got, err := r.Resolve(Triador)
	if err != nil || got != (Resolved{Alias: Triador, Model: Model{"openai", "gpt-6-luna"}, Source: SourceEnv}) {
		t.Fatalf("Resolve(triador) = %+v, %v", got, err)
	}
	if _, err := r.Resolve(Extrator); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("alias sem modelo: err = %v, want ErrNotConfigured", err)
	}
	if _, err := r.Resolve("parser_pgdas"); !errors.Is(err, ErrUnknownAlias) {
		t.Fatalf("parser_pgdas: err = %v, want ErrUnknownAlias (ADR-006)", err)
	}

	// O resolver copia os padrões: mudar o mapa original não altera a resolução nem o snapshot.
	defaults[Triador] = Model{"openai", "outro"}
	again, _ := r.Resolve(Triador)
	if again.Model.Name != "gpt-6-luna" || got.Model.Name != "gpt-6-luna" {
		t.Fatalf("snapshot alterado: %+v / %+v", got, again)
	}
}

// A escolha da tela vence o .env; sem escolha vale o .env; alias desligado não resolve
// [AC-027, F-010].
func TestResolver_ResolveWith(t *testing.T) {
	r := NewResolver(Defaults{Triador: {"openai", "gpt-6-luna"}})
	temp, maxTokens := 0.2, 900
	chosen := Model{"anthropic", "claude-sonnet-5"}

	got, err := r.ResolveWith(Triador, Setting{Model: &chosen, Parameters: Parameters{Temperature: &temp}, Enabled: true, Version: 4})
	if err != nil || got.Model != chosen || got.Source != SourceSettings || got.Version != 4 || *got.Parameters.Temperature != 0.2 {
		t.Fatalf("escolha da tela: %+v, %v", got, err)
	}

	got, err = r.ResolveWith(Triador, Setting{Parameters: Parameters{MaxTokens: &maxTokens}, Enabled: true, Version: 2})
	if err != nil || got.Model != (Model{"openai", "gpt-6-luna"}) || got.Source != SourceEnv || *got.Parameters.MaxTokens != 900 {
		t.Fatalf("sem escolha, vale o .env com os parâmetros da tela: %+v, %v", got, err)
	}

	// Snapshot: mudar a configuração depois não altera o que a corrida resolveu.
	maxTokens = 5
	chosen.Name = "outro"
	if *got.Parameters.MaxTokens != 900 {
		t.Fatalf("snapshot compartilha parâmetros com a configuração: %+v", got.Parameters)
	}

	if _, err := r.ResolveWith(Triador, Setting{Model: &chosen, Enabled: false}); !errors.Is(err, ErrAliasDisabled) {
		t.Fatalf("alias desligado: err = %v", err)
	}
	if _, err := r.ResolveWith(Extrator, Setting{Enabled: true}); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("sem escolha e sem .env: err = %v", err)
	}
	if m, ok := r.Default(Triador); !ok || m.Name != "gpt-6-luna" {
		t.Fatalf("Default(triador) = %v, %v", m, ok)
	}
	if _, ok := r.Default(Extrator); ok {
		t.Fatal("Default(extrator) sem .env deveria faltar")
	}
}

func TestParameters_Equal(t *testing.T) {
	a, b, c := 0.5, 0.5, 0.7
	n := 10
	cases := []struct {
		x, y Parameters
		want bool
	}{
		{Parameters{}, Parameters{}, true},
		{Parameters{Temperature: &a}, Parameters{Temperature: &b}, true},
		{Parameters{Temperature: &a}, Parameters{Temperature: &c}, false},
		{Parameters{Temperature: &a}, Parameters{}, false},
		{Parameters{MaxTokens: &n}, Parameters{MaxTokens: &n, Temperature: &a}, false},
	}
	for i, tc := range cases {
		if got := tc.x.Equal(tc.y); got != tc.want {
			t.Errorf("caso %d: Equal = %v, want %v", i, got, tc.want)
		}
	}
}

// Todos os agentes devolvem JSON validado por schema; só triador e extratores leem imagem.
func TestCatalog_Requirements(t *testing.T) {
	vision := map[Alias]bool{Triador: true, Extrator: true, ExtratorForte: true}
	for _, info := range Catalog {
		if !info.NeedsStructuredOutput {
			t.Errorf("%s deveria exigir saída estruturada", info.Alias)
		}
		if info.NeedsVision != vision[info.Alias] {
			t.Errorf("%s: NeedsVision = %v, want %v", info.Alias, info.NeedsVision, vision[info.Alias])
		}
	}
}

// A lista de aliases também vive no gateway, no compose e no .env.example. Este teste falha se
// algum deles divergir do Catalog — e garante que o parser_pgdas não vire alias (ADR-006).
func TestCatalog_MatchesGatewayAndCompose(t *testing.T) {
	root := filepath.Join("..", "..", "..")
	read := func(name string) string {
		b, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}

	want := map[string]string{}
	for _, info := range Catalog {
		want[string(info.Alias)] = info.EnvVar
	}
	if _, ok := want["parser_pgdas"]; ok {
		t.Fatal("parser_pgdas é determinístico e não pode ser alias de modelo (ADR-006)")
	}

	gateway := map[string]string{}
	for _, m := range regexp.MustCompile(`(?m)^\s*- model_name: (\S+)\s*\n\s*litellm_params:\s*\n\s*model: (\S+)`).FindAllStringSubmatch(read("litellm/config.yaml"), -1) {
		if strings.HasSuffix(m[1], "/*") {
			continue // rotas por provedor
		}
		gateway[m[1]] = strings.TrimPrefix(m[2], "os.environ/")
	}
	if !equalMaps(gateway, want) {
		t.Fatalf("litellm/config.yaml = %v\nCatalog = %v", gateway, want)
	}

	compose := map[string]string{}
	for _, m := range regexp.MustCompile(`(?m)^\s*([A-Z_]+_MODEL): \$\{([A-Z_]+_MODEL):-([^}]+)\}`).FindAllStringSubmatch(read("docker-compose.yml"), -1) {
		if m[1] != m[2] {
			t.Fatalf("docker-compose.yml: %s usa a variável %s", m[1], m[2])
		}
		compose[m[1]] = m[3]
	}
	example := map[string]string{}
	for _, m := range regexp.MustCompile(`(?m)^([A-Z_]+_MODEL)=(.*)$`).FindAllStringSubmatch(read(".env.example"), -1) {
		example[m[1]] = strings.TrimSpace(m[2])
	}
	for _, info := range Catalog {
		for source, values := range map[string]map[string]string{"docker-compose.yml": compose, ".env.example": example} {
			v, ok := values[info.EnvVar]
			if !ok {
				t.Errorf("%s sem %s", source, info.EnvVar)
				continue
			}
			if _, err := ParseModel(v); err != nil {
				t.Errorf("%s: %s=%q: %v", source, info.EnvVar, v, err)
			}
		}
		if compose[info.EnvVar] != example[info.EnvVar] {
			t.Errorf("padrão de %s difere: compose %q, .env.example %q", info.EnvVar, compose[info.EnvVar], example[info.EnvVar])
		}
	}
	if len(compose) != len(Catalog) || len(example) != len(Catalog) {
		t.Errorf("variáveis de modelo: compose %v, .env.example %v; esperado só as do Catalog", keys(compose), keys(example))
	}
}

// As chaves dos provedores só entram no container do LiteLLM; a aplicação usa a chave do
// gateway [spec 09 §4, ADR-004].
func TestCompose_ProviderKeysOnlyInGateway(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "docker-compose.yml"))
	if err != nil {
		t.Fatal(err)
	}
	services := regexp.MustCompile(`(?m)^  ([a-z][a-z0-9-]*):\s*$`)
	text := string(b)
	locs := services.FindAllStringSubmatchIndex(text, -1)
	seen := 0
	for i, loc := range locs {
		name := text[loc[2]:loc[3]]
		end := len(text)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		block := text[loc[0]:end]
		for _, key := range []string{"OPENAI_API_KEY", "ANTHROPIC_API_KEY"} {
			has := strings.Contains(block, key)
			if name == "litellm" && !has {
				t.Errorf("o serviço litellm deveria receber %s", key)
			}
			if name != "litellm" && has {
				t.Errorf("o serviço %s não pode receber %s: só o gateway conhece as chaves dos provedores", name, key)
			}
		}
		seen++
	}
	if seen < 5 {
		t.Fatalf("só %d serviços encontrados no docker-compose.yml", seen)
	}
}

func equalMaps(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
