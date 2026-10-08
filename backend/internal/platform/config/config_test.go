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
		"sem DATABASE_URL":    {},
		"CORS curinga":        {"DATABASE_URL": "x", "CORS_ORIGINS": "http://a, *"},
		"timezone inválido":   {"DATABASE_URL": "x", "APP_TIMEZONE": "Marte/Base"},
		"porta inválida":      {"DATABASE_URL": "x", "HTTP_PORT": "abc"},
		"fixed em produção":   {"DATABASE_URL": "x", "APP_ENV": "production", "APP_FIXED_TIME": "2026-10-01T10:00:00Z"},
		"fixed mal formatado": {"DATABASE_URL": "x", "APP_FIXED_TIME": "01/10/2026"},
		"env desconhecido":    {"DATABASE_URL": "x", "APP_ENV": "staging"},
		"max conns zero":      {"DATABASE_URL": "x", "DB_MAX_CONNS": "0"},
		"porta do worker":     {"DATABASE_URL": "x", "WORKER_HEALTH_PORT": "70000"},
		"concorrência zero":   {"DATABASE_URL": "x", "WORKER_CONCURRENCY": "0"},
		"tentativas demais":   {"DATABASE_URL": "x", "JOB_MAX_TENTATIVAS": "21"},
		"espera zero":         {"DATABASE_URL": "x", "JOB_POLL_SECONDS": "0"},
		"posse curta":         {"DATABASE_URL": "x", "JOB_LEASE_SECONDS": "5"},
		"litellm sem esquema": {"DATABASE_URL": "x", "LITELLM_BASE_URL": "litellm:4000"},
		"litellm timeout":     {"DATABASE_URL": "x", "LITELLM_TIMEOUT_SECONDS": "0"},
		"modelo sem provedor": {"DATABASE_URL": "x", "TRIADOR_MODEL": "gpt-6-luna"},
		"modelo com chave":    {"DATABASE_URL": "x", "EXTRATOR_MODEL": "openai/sk-proj-abc"},
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
