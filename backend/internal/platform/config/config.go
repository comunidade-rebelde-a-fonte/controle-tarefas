// Package config carrega e valida a configuração da aplicação a partir de
// variáveis de ambiente [PRD §45.3].
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"time"

	"controle/internal/ai"
)

// Environment identifica o ambiente de execução.
type Environment string

const (
	EnvDevelopment Environment = "development"
	EnvTest        Environment = "test"
	EnvProduction  Environment = "production"
)

// Config reúne a configuração validada da aplicação.
type Config struct {
	Env            Environment
	HTTPPort       int
	DatabaseURL    string
	DBMaxConns     int32
	TimezoneName   string
	Location       *time.Location
	CORSOrigins    []string
	FileStorageDir string
	// StaticDir é o build da SPA servido pelo binário (imagem de produção); vazio = só a API.
	StaticDir     string
	MigrateOnBoot bool
	LogLevel      slog.Level
	// FixedTime congela o relógio da aplicação (somente fora de produção).
	// Usado pelos testes E2E para datas determinísticas.
	FixedTime *time.Time
	// WorkerID identifica o processo worker (subcomando `worker`) nos logs.
	WorkerID string
	// WorkerHealthPort é a porta do healthcheck do worker, que roda separado da API.
	WorkerHealthPort int
	// WorkerConcurrency é quantos jobs o worker executa ao mesmo tempo (padrão 1, como a origem).
	WorkerConcurrency int
	// JobMaxAttempts é o padrão de tentativas de um job (JOB_MAX_TENTATIVAS; origem: 3).
	JobMaxAttempts int
	// JobPollInterval é a espera do worker com a fila vazia (JOB_POLL_SECONDS; origem: 1 s).
	JobPollInterval time.Duration
	// JobLease é o prazo da posse de um job; o worker a renova enquanto executa (TASK-005).
	JobLease time.Duration
	// LiteLLMBaseURL é o gateway de modelos dos agentes [ADR-004]; vazio = IA não configurada.
	LiteLLMBaseURL string
	// LiteLLMAPIKey autentica a aplicação no gateway. As chaves dos provedores (OpenAI etc.)
	// ficam só no ambiente do LiteLLM e nunca passam pela aplicação [spec 09 §4].
	LiteLLMAPIKey Secret
	// LiteLLMTimeout limita cada chamada ao gateway.
	LiteLLMTimeout time.Duration
	// AIDefaults são os modelos padrão dos aliases (<ALIAS>_MODEL). A escolha feita na tela
	// Configurações → IA vence o padrão (TASK-008).
	AIDefaults ai.Defaults
}

// Secret guarda um segredo que nunca aparece em log, print ou JSON: String, GoString,
// LogValue e MarshalJSON devolvem "[oculto]". O valor só sai por Reveal [spec 09 §4].
type Secret string

const redacted = "[oculto]"

// Reveal devolve o valor real, para uso direto na chamada que precisa dele.
func (s Secret) Reveal() string { return string(s) }

func (s Secret) String() string {
	if s == "" {
		return ""
	}
	return redacted
}

func (s Secret) GoString() string { return s.String() }

func (s Secret) LogValue() slog.Value { return slog.StringValue(s.String()) }

func (s Secret) MarshalJSON() ([]byte, error) { return []byte(`"` + s.String() + `"`), nil }

// IsProduction indica se a aplicação roda em produção.
func (c Config) IsProduction() bool { return c.Env == EnvProduction }

// Load lê a configuração das variáveis de ambiente usando getenv.
func Load(getenv func(string) string) (Config, error) {
	var errs []error
	cfg := Config{}

	cfg.Env = Environment(strings.ToLower(orDefault(getenv("APP_ENV"), string(EnvDevelopment))))
	switch cfg.Env {
	case EnvDevelopment, EnvTest, EnvProduction:
	default:
		errs = append(errs, fmt.Errorf("APP_ENV inválido: %q (use development, test ou production)", cfg.Env))
	}

	port, err := strconv.Atoi(orDefault(getenv("HTTP_PORT"), "8080"))
	if err != nil || port <= 0 || port > 65535 {
		errs = append(errs, fmt.Errorf("HTTP_PORT inválida: %q", getenv("HTTP_PORT")))
	}
	cfg.HTTPPort = port

	cfg.DatabaseURL = strings.TrimSpace(getenv("DATABASE_URL"))
	if cfg.DatabaseURL == "" {
		errs = append(errs, errors.New("DATABASE_URL é obrigatória"))
	}

	maxConns, err := strconv.ParseInt(orDefault(getenv("DB_MAX_CONNS"), "10"), 10, 32)
	if err != nil || maxConns < 1 || maxConns > 1000 {
		errs = append(errs, fmt.Errorf("DB_MAX_CONNS inválido: %q", getenv("DB_MAX_CONNS")))
	}
	cfg.DBMaxConns = int32(maxConns)

	cfg.TimezoneName = orDefault(getenv("APP_TIMEZONE"), "America/Sao_Paulo")
	loc, err := time.LoadLocation(cfg.TimezoneName)
	if err != nil {
		errs = append(errs, fmt.Errorf("APP_TIMEZONE inválido: %q", cfg.TimezoneName))
	}
	cfg.Location = loc

	for _, o := range strings.Split(getenv("CORS_ORIGINS"), ",") {
		o = strings.TrimSpace(o)
		if o == "" {
			continue
		}
		if o == "*" {
			errs = append(errs, errors.New("CORS_ORIGINS não pode conter \"*\": liste as origens explicitamente"))
			continue
		}
		cfg.CORSOrigins = append(cfg.CORSOrigins, o)
	}

	cfg.FileStorageDir = orDefault(getenv("FILE_STORAGE_DIR"), "./storage")
	cfg.StaticDir = strings.TrimSpace(getenv("STATIC_DIR"))

	defaultMigrate := "false"
	if cfg.Env == EnvProduction {
		defaultMigrate = "true"
	}
	migrate, err := strconv.ParseBool(orDefault(getenv("MIGRATE_ON_BOOT"), defaultMigrate))
	if err != nil {
		errs = append(errs, fmt.Errorf("MIGRATE_ON_BOOT inválido: %q", getenv("MIGRATE_ON_BOOT")))
	}
	cfg.MigrateOnBoot = migrate

	switch strings.ToLower(orDefault(getenv("LOG_LEVEL"), "info")) {
	case "debug":
		cfg.LogLevel = slog.LevelDebug
	case "info":
		cfg.LogLevel = slog.LevelInfo
	case "warn":
		cfg.LogLevel = slog.LevelWarn
	case "error":
		cfg.LogLevel = slog.LevelError
	default:
		errs = append(errs, fmt.Errorf("LOG_LEVEL inválido: %q", getenv("LOG_LEVEL")))
	}

	if raw := strings.TrimSpace(getenv("APP_FIXED_TIME")); raw != "" {
		if cfg.Env == EnvProduction {
			errs = append(errs, errors.New("APP_FIXED_TIME não é permitido em produção"))
		} else {
			t, err := time.Parse(time.RFC3339, raw)
			if err != nil {
				errs = append(errs, fmt.Errorf("APP_FIXED_TIME inválido (use RFC 3339): %q", raw))
			} else {
				cfg.FixedTime = &t
			}
		}
	}

	cfg.WorkerID = orDefault(getenv("WORKER_ID"), "worker-local")
	workerPort, err := strconv.Atoi(orDefault(getenv("WORKER_HEALTH_PORT"), "8090"))
	if err != nil || workerPort <= 0 || workerPort > 65535 {
		errs = append(errs, fmt.Errorf("WORKER_HEALTH_PORT inválida: %q", getenv("WORKER_HEALTH_PORT")))
	}
	cfg.WorkerHealthPort = workerPort

	cfg.WorkerConcurrency, err = strconv.Atoi(orDefault(getenv("WORKER_CONCURRENCY"), "1"))
	if err != nil || cfg.WorkerConcurrency < 1 || cfg.WorkerConcurrency > 16 {
		errs = append(errs, fmt.Errorf("WORKER_CONCURRENCY inválido: %q (1 a 16)", getenv("WORKER_CONCURRENCY")))
	}
	cfg.JobMaxAttempts, err = strconv.Atoi(orDefault(getenv("JOB_MAX_TENTATIVAS"), "3"))
	if err != nil || cfg.JobMaxAttempts < 1 || cfg.JobMaxAttempts > 20 {
		errs = append(errs, fmt.Errorf("JOB_MAX_TENTATIVAS inválido: %q (1 a 20)", getenv("JOB_MAX_TENTATIVAS")))
	}
	poll, err := strconv.ParseFloat(orDefault(getenv("JOB_POLL_SECONDS"), "1"), 64)
	if err != nil || poll <= 0 || poll > 60 {
		errs = append(errs, fmt.Errorf("JOB_POLL_SECONDS inválido: %q (maior que 0, até 60)", getenv("JOB_POLL_SECONDS")))
	}
	cfg.JobPollInterval = time.Duration(poll * float64(time.Second))
	lease, err := strconv.Atoi(orDefault(getenv("JOB_LEASE_SECONDS"), "300"))
	if err != nil || lease < 10 || lease > 86400 {
		errs = append(errs, fmt.Errorf("JOB_LEASE_SECONDS inválido: %q (10 a 86400)", getenv("JOB_LEASE_SECONDS")))
	}
	cfg.JobLease = time.Duration(lease) * time.Second

	cfg.LiteLLMBaseURL = strings.TrimRight(strings.TrimSpace(getenv("LITELLM_BASE_URL")), "/")
	if cfg.LiteLLMBaseURL != "" {
		u, err := url.Parse(cfg.LiteLLMBaseURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			errs = append(errs, fmt.Errorf("LITELLM_BASE_URL inválida: %q (use http://host:porta)", cfg.LiteLLMBaseURL))
		}
	}
	cfg.LiteLLMAPIKey = Secret(strings.TrimSpace(getenv("LITELLM_API_KEY")))
	timeout, err := strconv.Atoi(orDefault(getenv("LITELLM_TIMEOUT_SECONDS"), "120"))
	if err != nil || timeout < 1 || timeout > 3600 {
		errs = append(errs, fmt.Errorf("LITELLM_TIMEOUT_SECONDS inválido: %q (1 a 3600)", getenv("LITELLM_TIMEOUT_SECONDS")))
	}
	cfg.LiteLLMTimeout = time.Duration(timeout) * time.Second
	if cfg.AIDefaults, err = ai.DefaultsFromEnv(getenv); err != nil {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return Config{}, errors.Join(errs...)
	}
	return cfg, nil
}

func orDefault(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return strings.TrimSpace(v)
}
