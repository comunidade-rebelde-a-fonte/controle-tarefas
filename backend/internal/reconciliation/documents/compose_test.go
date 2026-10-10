package documents_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func repoFile(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "..", rel))
	if err != nil {
		t.Fatal(err)
	}
	return strings.ReplaceAll(string(b), "\r\n", "\n")
}

// services separa o compose por serviço (nível de indentação de dois espaços).
func services(t *testing.T) map[string]string {
	t.Helper()
	text := repoFile(t, "docker-compose.yml")
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

// 09 §4/§5: a API grava e o worker lê com o usuário da aplicação (sem permissão de apagar); a
// credencial de administrador do MinIO fica só no MinIO e no minio-init.
func TestCompose_MinIOCredentials(t *testing.T) {
	svc := services(t)
	for _, name := range []string{"api", "worker"} {
		if !strings.Contains(svc[name], "S3_ENDPOINT: http://minio:9000") || !strings.Contains(svc[name], "S3_SECRET_KEY") {
			t.Errorf("o serviço %s deveria usar o MinIO com o usuário da aplicação", name)
		}
	}
	for name, block := range svc {
		if strings.Contains(block, "S3_ROOT_PASSWORD") && name != "minio" && name != "minio-init" {
			t.Errorf("o serviço %s não pode receber a credencial de administrador do MinIO", name)
		}
		if strings.Contains(block, "S3_SECRET_KEY") && name != "api" && name != "worker" && name != "minio-init" {
			t.Errorf("o serviço %s não precisa da credencial do MinIO", name)
		}
	}
	if !strings.Contains(svc["worker"], "LIMIAR_TRIAGEM: ${LIMIAR_TRIAGEM:-0.70}") {
		t.Error("o worker deveria receber o limiar da triagem (0,70 da origem)")
	}
	if !strings.Contains(svc["worker"], "LIMIAR_CAMPO: ${LIMIAR_CAMPO:-0.70}") {
		t.Error("o worker deveria receber o limiar por campo do comprovante (0,70 da origem)")
	}
	for _, name := range []string{"api", "worker"} {
		if !strings.Contains(svc[name], "LIMIAR_FUZZY: ${LIMIAR_FUZZY:-0.86}") {
			t.Errorf("o serviço %s deveria receber o limiar do passe 3 do matching (0,86 da origem)", name)
		}
		// TASK-019: o resolvedor automático nasce desligado (custo de provedor por linha).
		if !strings.Contains(svc[name], "RESOLVEDOR_AUTOMATICO: ${RESOLVEDOR_AUTOMATICO:-false}") {
			t.Errorf("o serviço %s deveria receber RESOLVEDOR_AUTOMATICO desligado por padrão", name)
		}
		// TASK-020: o classificador automático também nasce desligado.
		if !strings.Contains(svc[name], "CLASSIFICADOR_AUTOMATICO: ${CLASSIFICADOR_AUTOMATICO:-false}") {
			t.Errorf("o serviço %s deveria receber CLASSIFICADOR_AUTOMATICO desligado por padrão", name)
		}
	}
	if strings.Contains(svc["api"], "depends_on:\n      minio") {
		t.Error("a API não depende do MinIO para subir [D-013]")
	}
}

// REQ-045: o workflow do n8n observa n8n/inbox e envia cada arquivo, no campo "file", ao
// endpoint de ingestão da Conciliação; no n8n 2.x o gatilho de pasta precisa ser religado.
func TestN8N_IngestionWorkflowPostsToReconciliation(t *testing.T) {
	n8n := services(t)["n8n"]
	for _, want := range []string{
		`NODES_EXCLUDE: '["n8n-nodes-base.executeCommand"]'`,
		`N8N_BLOCK_ENV_ACCESS_IN_NODE: "false"`,
		"CONTROLE_API_URL: http://api:${HTTP_PORT:-8080}",
		"./n8n/inbox:/data/inbox",
	} {
		if !strings.Contains(n8n, want) {
			t.Errorf("serviço n8n sem %q", want)
		}
	}
	var wf struct {
		ID    string `json:"id"`
		Nodes []struct {
			Type       string         `json:"type"`
			Parameters map[string]any `json:"parameters"`
		} `json:"nodes"`
	}
	if err := json.Unmarshal([]byte(repoFile(t, "n8n/workflows/ingestao.json")), &wf); err != nil {
		t.Fatal(err)
	}
	byType := map[string]map[string]any{}
	for _, n := range wf.Nodes {
		byType[n.Type] = n.Parameters
	}
	trigger, http := byType["n8n-nodes-base.localFileTrigger"], byType["n8n-nodes-base.httpRequest"]
	if trigger == nil || trigger["path"] != "/data/inbox" || http == nil || byType["n8n-nodes-base.readWriteFile"] == nil {
		t.Fatalf("workflow incompleto: %+v", byType)
	}
	if http["url"] != "={{ $env.CONTROLE_API_URL }}/api/v1/reconciliation/documents" || http["contentType"] != "multipart-form-data" {
		t.Fatalf("envio = %v", http)
	}
	b, _ := json.Marshal(http["bodyParameters"])
	if !strings.Contains(string(b), `"name":"file"`) || !strings.Contains(string(b), `"parameterType":"formBinaryData"`) {
		t.Fatalf("o arquivo deveria ir no campo file: %s", b)
	}
	if !strings.Contains(repoFile(t, "Makefile"), "--id="+wf.ID+" --active=true") {
		t.Errorf("make n8n-import deveria ativar o workflow %s", wf.ID)
	}
}
