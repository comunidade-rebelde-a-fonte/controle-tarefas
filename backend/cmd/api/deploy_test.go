package main

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// TEST-031 (TASK-031, D-044), a parte estática: o compose de execução sobe todos os serviços do
// produto com as garantias do T-M7-06, os segredos não têm valor padrão, o .env.prod.example
// declara cada variável e o backup cobre cada volume. A parte dinâmica (subir, copiar, destruir,
// restaurar e conferir) é o scripts/testar-restore.sh (make test-restore).

func repoFile(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "..", name))
	if err != nil {
		t.Fatal(err)
	}
	return strings.ReplaceAll(string(b), "\r\n", "\n")
}

// topSection devolve o corpo de uma seção de topo do compose ("services", "volumes"...).
func topSection(t *testing.T, text, name string) string {
	t.Helper()
	start := strings.Index(text, "\n"+name+":\n")
	if start < 0 {
		t.Fatalf("o compose não tem a seção %s", name)
	}
	body := text[start+len(name)+2:]
	if next := regexp.MustCompile(`(?m)^[a-z]`).FindStringIndex(body); next != nil {
		body = body[:next[0]]
	}
	return body
}

// serviceBlocks separa os blocos dos serviços (as chaves com dois espaços dentro de services:).
func serviceBlocks(t *testing.T, compose string) map[string]string {
	t.Helper()
	section := topSection(t, compose, "services")
	locs := regexp.MustCompile(`(?m)^  ([a-z][a-z0-9-]*):\n`).FindAllStringSubmatchIndex(section, -1)
	out := map[string]string{}
	for i, loc := range locs {
		end := len(section)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		out[section[loc[2]:loc[3]]] = section[loc[0]:end]
	}
	return out
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// A instalação fora do modo dev sobe o produto inteiro (REQ-001, REQ-043, REQ-045): o app (API e
// SPA na imagem única) e o worker, o banco, o MinIO dos originais, o LiteLLM, o n8n e o Langfuse
// com os seus armazenamentos; o serviço "backup" é só a ferramenta dos scripts.
func TestProdCompose_AllServices(t *testing.T) {
	got := sortedKeys(serviceBlocks(t, repoFile(t, "docker-compose.prod.yml")))
	want := []string{
		"app", "backup", "db", "langfuse-clickhouse", "langfuse-minio", "langfuse-postgres", "langfuse-redis",
		"langfuse-web", "langfuse-worker", "litellm", "minio", "minio-init", "n8n", "worker",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("serviços do docker-compose.prod.yml = %v, want %v", got, want)
	}
	blocks := serviceBlocks(t, repoFile(t, "docker-compose.prod.yml"))
	if !strings.Contains(blocks["backup"], `profiles: ["backup"]`) || !strings.Contains(blocks["backup"], "network_mode: none") {
		t.Fatalf("a ferramenta de backup não pode subir com o up nem ter rede:\n%s", blocks["backup"])
	}
}

// Só três portas vão para o host, em 127.0.0.1 por padrão (spec 09 §8): a aplicação (APP_BIND), o
// n8n e o Langfuse. O banco, os MinIO, o LiteLLM e o resto do Langfuse só existem na rede interna.
func TestProdCompose_PublishedPorts(t *testing.T) {
	want := map[string]string{
		"app":          `"${APP_BIND:-127.0.0.1}:${APP_PORT:-8080}:8080"`,
		"n8n":          `"127.0.0.1:${N8N_PORT:-5678}:5678"`,
		"langfuse-web": `"127.0.0.1:${LANGFUSE_WEB_PORT:-3100}:3000"`,
	}
	portRe := regexp.MustCompile(`(?m)^    ports:\n((?:      - .*\n)+)`)
	for name, block := range serviceBlocks(t, repoFile(t, "docker-compose.prod.yml")) {
		m := portRe.FindStringSubmatch(block)
		if strings.Contains(block, "expose:") {
			t.Errorf("%s: expose não é usado no compose de execução", name)
		}
		if m == nil {
			if _, ok := want[name]; ok {
				t.Errorf("%s deveria publicar %s", name, want[name])
			}
			continue
		}
		if got := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(m[1]), "-")); got != want[name] {
			t.Errorf("%s publica %q; esperado %q (nenhum outro serviço tem porta no host)", name, got, want[name])
		}
	}
}

// Imagem única e usuário não-root (T-M7-06): o worker usa a imagem do app, construída por ele, e o
// Dockerfile roda como o usuário app (uid 10001).
func TestProdCompose_SingleImageNonRoot(t *testing.T) {
	blocks := serviceBlocks(t, repoFile(t, "docker-compose.prod.yml"))
	const image = "image: controle-app:${APP_VERSION:-local}"
	for _, name := range []string{"app", "worker"} {
		if !strings.Contains(blocks[name], image) {
			t.Errorf("%s deveria usar %q", name, image)
		}
	}
	if !strings.Contains(blocks["app"], "build:") || strings.Contains(blocks["worker"], "build:") || !strings.Contains(blocks["worker"], "pull_policy: never") {
		t.Error("só o app constrói a imagem; o worker a usa sem baixar de registro (pull_policy: never)")
	}
	if !strings.Contains(blocks["worker"], `command: ["worker"]`) {
		t.Error("o worker roda o subcomando worker do mesmo binário")
	}
	dockerfile := repoFile(t, "Dockerfile")
	if !strings.Contains(dockerfile, "adduser -S -D -H -u 10001") || !strings.Contains(dockerfile, "\nUSER app\n") {
		t.Error("a imagem final deve rodar como o usuário não-root app (uid 10001)")
	}
}

var composeVarRe = regexp.MustCompile(`\$\{([A-Z][A-Z0-9_]*)(:[-?][^}]*)?\}`)

// isSecret diz se a variável guarda segredo (senha, chave ou sal).
func isSecret(name string) bool {
	return regexp.MustCompile(`PASSWORD|SECRET|SALT|_AUTH$|ENCRYPTION_KEY|MASTER_KEY|_API_KEY$`).MatchString(name)
}

// Nenhum segredo tem valor padrão no compose de execução: sem ele, o compose recusa subir (${X:?}).
// As chaves dos provedores são a exceção: vazias, os agentes que usam modelo ficam indisponíveis.
func TestProdCompose_SecretsWithoutDefaults(t *testing.T) {
	providerKeys := []string{"OPENAI_API_KEY", "ANTHROPIC_API_KEY"}
	seen := map[string]bool{}
	for _, m := range composeVarRe.FindAllStringSubmatch(repoFile(t, "docker-compose.prod.yml"), -1) {
		name, mod := m[1], m[2]
		if !isSecret(name) {
			continue
		}
		seen[name] = true
		switch {
		case slices.Contains(providerKeys, name):
			if mod != ":-" {
				t.Errorf("%s: a chave do provedor fica vazia por padrão (${%s:-}), veio %q", name, name, m[0])
			}
		case !strings.HasPrefix(mod, ":?"):
			t.Errorf("%s: segredo sem valor padrão, sempre ${%s:?...}; veio %q", name, name, m[0])
		}
	}
	for _, name := range []string{"POSTGRES_PASSWORD", "S3_ROOT_PASSWORD", "S3_SECRET_KEY", "LITELLM_MASTER_KEY", "NEXTAUTH_SECRET",
		"SALT", "ENCRYPTION_KEY", "LF_POSTGRES_PASSWORD", "CLICKHOUSE_PASSWORD", "MINIO_ROOT_PASSWORD", "REDIS_AUTH",
		"LANGFUSE_SECRET_KEY", "LANGFUSE_ADMIN_PASSWORD"} {
		if !seen[name] {
			t.Errorf("o compose de execução deveria exigir %s", name)
		}
	}
}

// O .env.prod.example declara cada variável do compose de execução e dos scripts de backup, com
// os segredos vazios (o operador gera os seus; nada de valor de exemplo em produção).
func TestProdEnvExample_DeclaresEveryVariable(t *testing.T) {
	example := map[string]string{}
	for _, m := range regexp.MustCompile(`(?m)^([A-Z][A-Z0-9_]*)=(.*)$`).FindAllStringSubmatch(repoFile(t, ".env.prod.example"), -1) {
		example[m[1]] = m[2]
	}
	used := map[string]bool{"BACKUP_KEEP_DAILY": true, "BACKUP_KEEP_WEEKLY": true, "BACKUP_KEEP_MONTHLY": true}
	for _, m := range composeVarRe.FindAllStringSubmatch(repoFile(t, "docker-compose.prod.yml"), -1) {
		used[m[1]] = true
	}
	for _, name := range sortedKeys(used) {
		v, ok := example[name]
		if !ok {
			t.Errorf(".env.prod.example não declara %s", name)
			continue
		}
		if isSecret(name) && v != "" {
			t.Errorf(".env.prod.example: o segredo %s vem vazio no modelo", name)
		}
	}
	for _, name := range sortedKeys(example) {
		if !used[name] {
			t.Errorf(".env.prod.example declara %s, que o compose de execução e os scripts não usam", name)
		}
	}
}

// Todo volume do compose de execução entra no backup (REQ-043, REQ-044; FR-21), ou está na lista
// do que é descartável. Volume novo quebra este teste até alguém decidir como copiá-lo.
func TestProdBackup_CoversEveryVolume(t *testing.T) {
	compose := repoFile(t, "docker-compose.prod.yml")
	backup := repoFile(t, "scripts/backup.sh")
	common := repoFile(t, "scripts/backup-comum.sh")
	tool := serviceBlocks(t, compose)["backup"]
	// Volume → evidência no script: os bancos por dump, o ClickHouse pelo BACKUP nativo e os
	// volumes de arquivos pelo tar da ferramenta (montados em /volumes/<pasta>).
	byDump := map[string]string{
		"pgdata":                   `compose exec -T db sh -c 'exec pg_dump -Fc`,
		"langfuse_postgres_data":   `compose exec -T langfuse-postgres pg_dump -Fc`,
		"langfuse_clickhouse_data": `BACKUP DATABASE default TO File(`,
	}
	byTar := map[string]string{"storage": "storage", "minio_data": "minio", "n8n_data": "n8n", "langfuse_minio_data": "langfuse-minio"}
	discarded := map[string]string{
		"langfuse_redis_data":         "fila e cache do Langfuse",
		"langfuse_clickhouse_logs":    "logs do ClickHouse",
		"langfuse_clickhouse_backups": "área temporária do BACKUP nativo",
	}
	var volumes []string
	for _, m := range regexp.MustCompile(`(?m)^  ([a-z][a-z0-9_]*):`).FindAllStringSubmatch(topSection(t, compose, "volumes"), -1) {
		volumes = append(volumes, m[1])
	}
	for _, v := range volumes {
		if evidence, ok := byDump[v]; ok {
			if !strings.Contains(backup, evidence) {
				t.Errorf("volume %s: o backup.sh deveria ter %q", v, evidence)
			}
			continue
		}
		if dir, ok := byTar[v]; ok {
			if !strings.Contains(tool, "- "+v+":/volumes/"+dir+"\n") {
				t.Errorf("volume %s: a ferramenta de backup deveria montá-lo em /volumes/%s", v, dir)
			}
			if !strings.Contains(common, "]="+dir) {
				t.Errorf("volume %s: o backup-comum.sh deveria copiar a pasta %s", v, dir)
			}
			continue
		}
		if _, ok := discarded[v]; !ok {
			t.Errorf("volume %s sem backup: inclua-o no scripts/backup.sh (e neste teste) ou justifique-o como descartável", v)
		}
	}
	if len(volumes) != len(byDump)+len(byTar)+len(discarded) {
		t.Errorf("volumes do compose = %v; o teste conhece %d", volumes, len(byDump)+len(byTar)+len(discarded))
	}

	// Cada armazenamento tem a cópia no backup.sh e a volta no restore.sh.
	restore := repoFile(t, "scripts/restore.sh")
	m := regexp.MustCompile(`COMPONENTES=\(([^)]*)\)`).FindStringSubmatch(common)
	if m == nil {
		t.Fatal("backup-comum.sh sem a lista COMPONENTES")
	}
	for _, comp := range strings.Fields(m[1]) {
		if !strings.Contains(backup, "copia_"+comp+"()") {
			t.Errorf("componente %s sem a função copia_%s no backup.sh", comp, comp)
		}
		if !regexp.MustCompile(`(?m)^    (\S+ \| )?` + comp + `( \| \S+)?\)$`).MatchString(restore) {
			t.Errorf("componente %s sem o caso de restauração no restore.sh", comp)
		}
	}
}
