package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoad_Defaults(t *testing.T) {
	cfg, err := Load(env(map[string]string{"DATABASE_URL": "postgres://x"}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Env != EnvDevelopment || cfg.HTTPPort != 8080 || cfg.DBMaxConns != 10 {
		t.Fatalf("defaults = %+v", cfg)
	}
	if cfg.Location.String() != "America/Sao_Paulo" {
		t.Fatalf("timezone = %s", cfg.Location)
	}
	if cfg.MigrateOnBoot {
		t.Fatal("MIGRATE_ON_BOOT deve ser false em dev")
	}
	if cfg.WorkerID != "worker-local" || cfg.WorkerHealthPort != 8090 {
		t.Fatalf("worker = %q:%d", cfg.WorkerID, cfg.WorkerHealthPort)
	}
}

func TestLoad_LiteLLM(t *testing.T) {
	cfg, err := Load(env(map[string]string{"DATABASE_URL": "x"}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LiteLLMBaseURL != "" || cfg.LiteLLMAPIKey != "" || cfg.LiteLLMTimeout != 120*time.Second {
		t.Fatalf("padrões do LiteLLM = %q %q %s", cfg.LiteLLMBaseURL, cfg.LiteLLMAPIKey.Reveal(), cfg.LiteLLMTimeout)
	}

	cfg, err = Load(env(map[string]string{
		"DATABASE_URL": "x", "LITELLM_BASE_URL": " http://litellm:4000/ ", "LITELLM_API_KEY": "sk-gateway", "LITELLM_TIMEOUT_SECONDS": "30",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LiteLLMBaseURL != "http://litellm:4000" || cfg.LiteLLMAPIKey.Reveal() != "sk-gateway" || cfg.LiteLLMTimeout != 30*time.Second {
		t.Fatalf("LiteLLM = %q %s", cfg.LiteLLMBaseURL, cfg.LiteLLMTimeout)
	}
}

// Os modelos padrão dos aliases vêm do .env; sem variável, o alias fica sem padrão.
func TestLoad_AIDefaults(t *testing.T) {
	cfg, err := Load(env(map[string]string{"DATABASE_URL": "x", "TRIADOR_MODEL": "openai/gpt-6-luna"}))
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.AIDefaults["triador"].String(); got != "openai/gpt-6-luna" || len(cfg.AIDefaults) != 1 {
		t.Fatalf("AIDefaults = %v", cfg.AIDefaults)
	}
}

// Langfuse (TASK-009): padrões da origem, chave secreta oculta e link do trace.
func TestLoad_Langfuse(t *testing.T) {
	cfg, err := Load(env(map[string]string{"DATABASE_URL": "x"}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LangfuseHost != "" || cfg.LangfuseEnvironment != "development" || cfg.LangfuseTimeout != 5*time.Second ||
		cfg.LangfuseRetry != 5*time.Minute || cfg.TracePayloadMaxKB != 32 || cfg.LangfuseTraceURL("abc") != nil {
		t.Fatalf("padrões = %+v", cfg)
	}
	cfg, err = Load(env(map[string]string{
		"DATABASE_URL": "x", "LANGFUSE_HOST": " http://langfuse:3000/ ", "LANGFUSE_PUBLIC_KEY": "pk-lf-1", "LANGFUSE_SECRET_KEY": "sk-lf-segredo",
		"LANGFUSE_PUBLIC_URL": "http://localhost:3100/", "LANGFUSE_PROJECT_ID": "controle-integrado-dev", "LANGFUSE_ENVIRONMENT": "Dev-Local",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LangfuseHost != "http://langfuse:3000" || cfg.LangfuseEnvironment != "dev-local" || cfg.LangfuseSecretKey.Reveal() != "sk-lf-segredo" {
		t.Fatalf("Langfuse = %q %q", cfg.LangfuseHost, cfg.LangfuseEnvironment)
	}
	if u := cfg.LangfuseTraceURL("0123456789abcdef0123456789abcdef"); u == nil || *u != "http://localhost:3100/project/controle-integrado-dev/traces/0123456789abcdef0123456789abcdef" {
		t.Fatalf("link = %v", u)
	}
	if b, _ := json.Marshal(cfg); strings.Contains(string(b), "segredo") || strings.Contains(fmt.Sprintf("%+v", cfg), "segredo") {
		t.Fatal("a chave secreta do Langfuse aparece na configuração impressa")
	}
}

// TASK-015: originais da Conciliação no MinIO; sem endpoint, o armazenamento fica desligado.
func TestLoad_S3(t *testing.T) {
	cfg, err := Load(env(map[string]string{"DATABASE_URL": "x"}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.S3Endpoint != "" || cfg.S3Region != "us-east-1" {
		t.Fatalf("padrões = %q %q", cfg.S3Endpoint, cfg.S3Region)
	}
	cfg, err = Load(env(map[string]string{
		"DATABASE_URL": "x", "S3_ENDPOINT": " http://minio:9000/ ", "S3_ACCESS_KEY": "conciliacao-app",
		"S3_SECRET_KEY": "segredo-do-minio", "S3_BUCKET": "conciliacao-originais", "S3_REGION": "sa-east-1",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.S3Endpoint != "http://minio:9000" || cfg.S3Region != "sa-east-1" || cfg.S3Bucket != "conciliacao-originais" || cfg.S3SecretKey.Reveal() != "segredo-do-minio" {
		t.Fatalf("S3 = %q %q %q", cfg.S3Endpoint, cfg.S3Region, cfg.S3Bucket)
	}
	if b, _ := json.Marshal(cfg); strings.Contains(string(b), "segredo-do-minio") || strings.Contains(fmt.Sprintf("%+v", cfg), "segredo-do-minio") {
		t.Fatal("a chave do MinIO aparece na configuração impressa")
	}
}

// A chave do gateway nunca aparece em print, log ou JSON [spec 09 §4].
func TestSecret_NeverPrinted(t *testing.T) {
	cfg, err := Load(env(map[string]string{"DATABASE_URL": "x", "LITELLM_API_KEY": "sk-segredo-real-123"}))
	if err != nil {
		t.Fatal(err)
	}
	var logged bytes.Buffer
	slog.New(slog.NewJSONHandler(&logged, nil)).Info("config", "key", cfg.LiteLLMAPIKey, "cfg", cfg)
	asJSON, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	outputs := map[string]string{
		"%v":     fmt.Sprintf("%v", cfg),
		"%+v":    fmt.Sprintf("%+v", cfg),
		"%#v":    fmt.Sprintf("%#v", cfg),
		"Sprint": fmt.Sprint(cfg.LiteLLMAPIKey),
		"slog":   logged.String(),
		"json":   string(asJSON),
	}
	for name, out := range outputs {
		if strings.Contains(out, "segredo-real") {
			t.Errorf("%s expõe a chave: %s", name, out)
		}
	}
	if cfg.LiteLLMAPIKey.Reveal() != "sk-segredo-real-123" {
		t.Fatal("Reveal deve devolver o valor real")
	}
}

func TestLoad_Worker(t *testing.T) {
	cfg, err := Load(env(map[string]string{"DATABASE_URL": "x", "WORKER_ID": " worker-dev ", "WORKER_HEALTH_PORT": "9191"}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.WorkerID != "worker-dev" || cfg.WorkerHealthPort != 9191 {
		t.Fatalf("worker = %q:%d", cfg.WorkerID, cfg.WorkerHealthPort)
	}
	// Padrões da fila: os da origem (3 tentativas, 1 s de espera) e posse de 5 min.
	if cfg.WorkerConcurrency != 1 || cfg.JobMaxAttempts != 3 || cfg.JobPollInterval != time.Second || cfg.JobLease != 5*time.Minute {
		t.Fatalf("fila = %d, %d, %s, %s", cfg.WorkerConcurrency, cfg.JobMaxAttempts, cfg.JobPollInterval, cfg.JobLease)
	}
	// Fila semanal do priorizador: domingo a partir das 7h, como a origem.
	if cfg.RoutinePriorizadorHour != 7 {
		t.Fatalf("hora da rotina = %d", cfg.RoutinePriorizadorHour)
	}
	if cfg, err := Load(env(map[string]string{"DATABASE_URL": "x", "ROTINA_PRIORIZADOR_HORA": "0"})); err != nil || cfg.RoutinePriorizadorHour != 0 {
		t.Fatalf("hora da rotina 0 = %d, %v", cfg.RoutinePriorizadorHour, err)
	}
	cfg, err = Load(env(map[string]string{"DATABASE_URL": "x", "WORKER_CONCURRENCY": "2", "JOB_MAX_TENTATIVAS": "5",
		"JOB_POLL_SECONDS": "0.5", "JOB_LEASE_SECONDS": "60"}))
	if err != nil || cfg.WorkerConcurrency != 2 || cfg.JobMaxAttempts != 5 || cfg.JobPollInterval != 500*time.Millisecond || cfg.JobLease != time.Minute {
		t.Fatalf("fila = %+v, %v", cfg, err)
	}
}

func TestLoad_ProductionMigratesOnBoot(t *testing.T) {
	cfg, err := Load(env(map[string]string{"DATABASE_URL": "postgres://x", "APP_ENV": "production"}))
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.MigrateOnBoot {
		t.Fatal("MIGRATE_ON_BOOT deve ser true em produção")
	}
}

func TestLoad_Invalid(t *testing.T) {
	cases := map[string]map[string]string{
		"sem DATABASE_URL":     {},
		"CORS curinga":         {"DATABASE_URL": "x", "CORS_ORIGINS": "http://a, *"},
		"timezone inválido":    {"DATABASE_URL": "x", "APP_TIMEZONE": "Marte/Base"},
		"porta inválida":       {"DATABASE_URL": "x", "HTTP_PORT": "abc"},
		"fixed em produção":    {"DATABASE_URL": "x", "APP_ENV": "production", "APP_FIXED_TIME": "2026-10-01T10:00:00Z"},
		"fixed mal formatado":  {"DATABASE_URL": "x", "APP_FIXED_TIME": "01/10/2026"},
		"env desconhecido":     {"DATABASE_URL": "x", "APP_ENV": "staging"},
		"max conns zero":       {"DATABASE_URL": "x", "DB_MAX_CONNS": "0"},
		"porta do worker":      {"DATABASE_URL": "x", "WORKER_HEALTH_PORT": "70000"},
		"concorrência zero":    {"DATABASE_URL": "x", "WORKER_CONCURRENCY": "0"},
		"tentativas demais":    {"DATABASE_URL": "x", "JOB_MAX_TENTATIVAS": "21"},
		"espera zero":          {"DATABASE_URL": "x", "JOB_POLL_SECONDS": "0"},
		"posse curta":          {"DATABASE_URL": "x", "JOB_LEASE_SECONDS": "5"},
		"hora da rotina":       {"DATABASE_URL": "x", "ROTINA_PRIORIZADOR_HORA": "24"},
		"litellm sem esquema":  {"DATABASE_URL": "x", "LITELLM_BASE_URL": "litellm:4000"},
		"litellm timeout":      {"DATABASE_URL": "x", "LITELLM_TIMEOUT_SECONDS": "0"},
		"modelo sem provedor":  {"DATABASE_URL": "x", "TRIADOR_MODEL": "gpt-6-luna"},
		"modelo com chave":     {"DATABASE_URL": "x", "EXTRATOR_MODEL": "openai/sk-proj-abc"},
		"langfuse sem chaves":  {"DATABASE_URL": "x", "LANGFUSE_HOST": "http://langfuse:3000"},
		"langfuse sem esquema": {"DATABASE_URL": "x", "LANGFUSE_HOST": "langfuse:3000", "LANGFUSE_PUBLIC_KEY": "pk", "LANGFUSE_SECRET_KEY": "sk"},
		"ambiente reservado":   {"DATABASE_URL": "x", "LANGFUSE_ENVIRONMENT": "langfuse-dev"},
		"reenvio curto":        {"DATABASE_URL": "x", "LANGFUSE_RETRY_SECONDS": "5"},
		"payload enorme":       {"DATABASE_URL": "x", "TRACE_PAYLOAD_MAX_KB": "4096"},
		"projeto inválido":     {"DATABASE_URL": "x", "LANGFUSE_PROJECT_ID": "a/b"},
		"s3 sem chaves":        {"DATABASE_URL": "x", "S3_ENDPOINT": "http://minio:9000", "S3_BUCKET": "b-1"},
		"s3 sem esquema":       {"DATABASE_URL": "x", "S3_ENDPOINT": "minio:9000", "S3_ACCESS_KEY": "a", "S3_SECRET_KEY": "s", "S3_BUCKET": "b-1"},
		"s3 com caminho":       {"DATABASE_URL": "x", "S3_ENDPOINT": "http://minio:9000/bucket", "S3_ACCESS_KEY": "a", "S3_SECRET_KEY": "s", "S3_BUCKET": "b-1"},
		"bucket inválido":      {"DATABASE_URL": "x", "S3_BUCKET": "Bucket_Errado"},
	}
	for name, m := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Load(env(m)); err == nil {
				t.Fatal("esperava erro")
			}
		})
	}
}

func TestLoad_CORSOrigins(t *testing.T) {
	cfg, err := Load(env(map[string]string{"DATABASE_URL": "x", "CORS_ORIGINS": " http://a:5173 , ,http://b "}))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(cfg.CORSOrigins, "|") != "http://a:5173|http://b" {
		t.Fatalf("origens = %v", cfg.CORSOrigins)
	}
}

// Limiares da Conciliação, com os padrões da origem: o da triagem (TASK-016) e o por campo do
// comprovante (TASK-017), 0,70; o do passe 3 do matching (TASK-018), 0,86; decimal de 0 a 1.
func TestLoad_ReconciliationThresholds(t *testing.T) {
	cfg, err := Load(env(map[string]string{"DATABASE_URL": "x"}))
	if err != nil || cfg.TriageThreshold.String() != "0.70" || cfg.FieldThreshold.String() != "0.70" || cfg.FuzzyThreshold.String() != "0.86" {
		t.Fatalf("padrão = %s, %s e %s, %v", cfg.TriageThreshold, cfg.FieldThreshold, cfg.FuzzyThreshold, err)
	}
	cfg, err = Load(env(map[string]string{"DATABASE_URL": "x", "LIMIAR_TRIAGEM": " 0.85 ", "LIMIAR_CAMPO": "0.9", "LIMIAR_FUZZY": "0.8"}))
	if err != nil || cfg.TriageThreshold.String() != "0.85" || cfg.FieldThreshold.String() != "0.9" || cfg.FuzzyThreshold.String() != "0.8" {
		t.Fatalf("0.85, 0.9 e 0.8 = %s, %s e %s, %v", cfg.TriageThreshold, cfg.FieldThreshold, cfg.FuzzyThreshold, err)
	}
	if _, err := Load(env(map[string]string{"DATABASE_URL": "x", "LIMIAR_FUZZY": "0"})); err == nil || !strings.Contains(err.Error(), "LIMIAR_FUZZY") {
		t.Errorf("LIMIAR_FUZZY=0 aceito: %v", err)
	}
	// O resolvedor automático (TASK-019) é desligado por padrão: cada pedido é uma chamada de modelo.
	if cfg, err := Load(env(map[string]string{"DATABASE_URL": "x"})); err != nil || cfg.ResolverAuto {
		t.Errorf("RESOLVEDOR_AUTOMATICO padrão = %v, %v", cfg.ResolverAuto, err)
	}
	if cfg, err := Load(env(map[string]string{"DATABASE_URL": "x", "RESOLVEDOR_AUTOMATICO": "true"})); err != nil || !cfg.ResolverAuto {
		t.Errorf("RESOLVEDOR_AUTOMATICO=true = %v, %v", cfg.ResolverAuto, err)
	}
	if _, err := Load(env(map[string]string{"DATABASE_URL": "x", "RESOLVEDOR_AUTOMATICO": "talvez"})); err == nil {
		t.Error("RESOLVEDOR_AUTOMATICO inválido aceito")
	}
	// O classificador automático (TASK-020) também nasce desligado.
	if cfg, err := Load(env(map[string]string{"DATABASE_URL": "x"})); err != nil || cfg.ClassifierAuto {
		t.Errorf("CLASSIFICADOR_AUTOMATICO padrão = %v, %v", cfg.ClassifierAuto, err)
	}
	if cfg, err := Load(env(map[string]string{"DATABASE_URL": "x", "CLASSIFICADOR_AUTOMATICO": "1"})); err != nil || !cfg.ClassifierAuto {
		t.Errorf("CLASSIFICADOR_AUTOMATICO=1 = %v, %v", cfg.ClassifierAuto, err)
	}
	if _, err := Load(env(map[string]string{"DATABASE_URL": "x", "CLASSIFICADOR_AUTOMATICO": "sim"})); err == nil ||
		!strings.Contains(err.Error(), "CLASSIFICADOR_AUTOMATICO") {
		t.Errorf("CLASSIFICADOR_AUTOMATICO inválido aceito: %v", err)
	}
	// A meta da conciliação automática (TASK-021c): 80% da origem, percentual de 0 a 100.
	if cfg, err := Load(env(map[string]string{"DATABASE_URL": "x"})); err != nil || cfg.AutoTarget.String() != "80" {
		t.Errorf("META_CONCILIACAO_AUTO padrão = %s, %v", cfg.AutoTarget, err)
	}
	if cfg, err := Load(env(map[string]string{"DATABASE_URL": "x", "META_CONCILIACAO_AUTO": " 92.5 "})); err != nil || cfg.AutoTarget.String() != "92.5" {
		t.Errorf("META_CONCILIACAO_AUTO=92.5 = %s, %v", cfg.AutoTarget, err)
	}
	for _, bad := range []string{"101", "-1", "oitenta", "80%"} {
		if _, err := Load(env(map[string]string{"DATABASE_URL": "x", "META_CONCILIACAO_AUTO": bad})); err == nil ||
			!strings.Contains(err.Error(), "META_CONCILIACAO_AUTO") {
			t.Errorf("META_CONCILIACAO_AUTO=%q aceito: %v", bad, err)
		}
	}
	for _, name := range []string{"LIMIAR_TRIAGEM", "LIMIAR_CAMPO", "LIMIAR_FUZZY"} {
		for _, bad := range []string{"1.5", "-0.1", "abc", "0,7"} {
			if _, err := Load(env(map[string]string{"DATABASE_URL": "x", name: bad})); err == nil || !strings.Contains(err.Error(), name) {
				t.Errorf("%s=%q aceito: %v", name, bad, err)
			}
		}
	}
}
