// Package config carrega e valida a configuração da aplicação a partir de
// variáveis de ambiente [PRD §45.3].
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"controle/internal/ai"
	"controle/internal/platform/decimal"
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
	JobMaxAttempts int32
	// JobPollInterval é a espera do worker com a fila vazia (JOB_POLL_SECONDS; origem: 1 s).
	JobPollInterval time.Duration
	// JobLease é o prazo da posse de um job; o worker a renova enquanto executa (TASK-005).
	JobLease time.Duration
	// RoutinePriorizadorHour é a hora (APP_TIMEZONE) a partir da qual, aos domingos, o worker
	// roda a fila semanal do priorizador (ROTINA_PRIORIZADOR_HORA; origem: 7).
	RoutinePriorizadorHour int
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
	// Langfuse (TASK-009): espelho dos traces e notas dos agentes, sincronizado pelo worker.
	// LangfuseHost vazio = sem sincronização; as corridas ficam pending na trilha local.
	LangfuseHost      string
	LangfusePublicKey string
	// LangfuseSecretKey só vai para o worker, que envia ao Langfuse.
	LangfuseSecretKey   Secret
	LangfuseEnvironment string
	LangfuseTimeout     time.Duration
	// LangfuseRetry é o intervalo do reenvio periódico das corridas com falha (origem: 5 min).
	LangfuseRetry time.Duration
	// TracePayloadMaxKB limita cada atributo enviado ao Langfuse (origem: 32 KB).
	TracePayloadMaxKB int
	// LangfusePublicURL e LangfuseProjectID montam o link do trace para o navegador.
	LangfusePublicURL string
	LangfuseProjectID string
	// Originais da Conciliação no MinIO (S3, bucket versionado; REQ-043, TASK-015). S3Endpoint
	// vazio = armazenamento não configurado: o envio responde 503 STORAGE_UNAVAILABLE. A
	// credencial é a do usuário da aplicação, sem permissão de apagar (docker/minio/init.sh).
	S3Endpoint  string
	S3Region    string
	S3AccessKey string
	S3SecretKey Secret
	S3Bucket    string
	// TriageThreshold é o limiar de confiança da triagem da Conciliação (LIMIAR_TRIAGEM, padrão
	// 0,70 como na origem; TASK-016): abaixo dele, o documento vai para revisao_triagem.
	TriageThreshold decimal.Decimal
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

// LangfuseTraceURL é o link do trace no Langfuse, para o navegador; nil sem LANGFUSE_PUBLIC_URL
// e LANGFUSE_PROJECT_ID.
func (c Config) LangfuseTraceURL(traceID string) *string {
	if c.LangfusePublicURL == "" || c.LangfuseProjectID == "" {
		return nil
	}
	u := c.LangfusePublicURL + "/project/" + url.PathEscape(c.LangfuseProjectID) + "/traces/" + url.PathEscape(traceID)
	return &u
}

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

	cfg.RoutinePriorizadorHour, err = strconv.Atoi(orDefault(getenv("ROTINA_PRIORIZADOR_HORA"), "7"))
	if err != nil || cfg.RoutinePriorizadorHour < 0 || cfg.RoutinePriorizadorHour > 23 {
		errs = append(errs, fmt.Errorf("ROTINA_PRIORIZADOR_HORA inválida: %q (0 a 23)", getenv("ROTINA_PRIORIZADOR_HORA")))
	}
	cfg.WorkerConcurrency, err = strconv.Atoi(orDefault(getenv("WORKER_CONCURRENCY"), "1"))
	if err != nil || cfg.WorkerConcurrency < 1 || cfg.WorkerConcurrency > 16 {
		errs = append(errs, fmt.Errorf("WORKER_CONCURRENCY inválido: %q (1 a 16)", getenv("WORKER_CONCURRENCY")))
	}
	maxAttempts, err := strconv.ParseInt(orDefault(getenv("JOB_MAX_TENTATIVAS"), "3"), 10, 32)
	if err != nil || maxAttempts < 1 || maxAttempts > 20 {
		errs = append(errs, fmt.Errorf("JOB_MAX_TENTATIVAS inválido: %q (1 a 20)", getenv("JOB_MAX_TENTATIVAS")))
	}
	cfg.JobMaxAttempts = int32(maxAttempts)
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
	errs = append(errs, loadLangfuse(&cfg, getenv)...)
	errs = append(errs, loadS3(&cfg, getenv)...)
	threshold, terr := decimal.Parse(orDefault(strings.TrimSpace(getenv("LIMIAR_TRIAGEM")), "0.70"))
	if terr != nil || threshold.Sign() < 0 || threshold.Cmp(decimal.FromInt(1)) > 0 {
		errs = append(errs, fmt.Errorf("LIMIAR_TRIAGEM inválido: %q (decimal de 0 a 1, ex.: 0.70)", getenv("LIMIAR_TRIAGEM")))
	}
	cfg.TriageThreshold = threshold

	if len(errs) > 0 {
		return Config{}, errors.Join(errs...)
	}
	return cfg, nil
}

var (
	langfuseEnvRe     = regexp.MustCompile(`^[a-z0-9_-]{1,40}$`)
	langfuseProjectRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,100}$`)
)

// loadLangfuse lê LANGFUSE_* e TRACE_PAYLOAD_MAX_KB (TASK-009).
// loadS3 lê o armazenamento dos originais da Conciliação. Com endpoint, credencial e bucket são
// obrigatórios.
func loadS3(cfg *Config, getenv func(string) string) []error {
	var errs []error
	cfg.S3Endpoint = strings.TrimRight(strings.TrimSpace(getenv("S3_ENDPOINT")), "/")
	if cfg.S3Endpoint != "" {
		if u, err := url.Parse(cfg.S3Endpoint); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || (u.Path != "" && u.Path != "/") {
			errs = append(errs, fmt.Errorf("S3_ENDPOINT inválido: %q (use http://host:porta)", cfg.S3Endpoint))
		}
	}
	cfg.S3Region = orDefault(strings.TrimSpace(getenv("S3_REGION")), "us-east-1")
	cfg.S3AccessKey = strings.TrimSpace(getenv("S3_ACCESS_KEY"))
	cfg.S3SecretKey = Secret(strings.TrimSpace(getenv("S3_SECRET_KEY")))
	cfg.S3Bucket = strings.TrimSpace(getenv("S3_BUCKET"))
	if cfg.S3Endpoint != "" && (cfg.S3AccessKey == "" || cfg.S3SecretKey == "" || cfg.S3Bucket == "") {
		errs = append(errs, errors.New("S3_ENDPOINT exige S3_ACCESS_KEY, S3_SECRET_KEY e S3_BUCKET"))
	}
	if cfg.S3Bucket != "" && !s3BucketRe.MatchString(cfg.S3Bucket) {
		errs = append(errs, fmt.Errorf("S3_BUCKET inválido: %q (3 a 63 caracteres: minúsculas, dígitos, ponto e hífen)", cfg.S3Bucket))
	}
	return errs
}

// s3BucketRe são os nomes de bucket aceitos pelo S3/MinIO (forma simplificada).
var s3BucketRe = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$`)

func loadLangfuse(cfg *Config, getenv func(string) string) []error {
	var errs []error
	httpURL := func(name, v string) string {
		v = strings.TrimRight(strings.TrimSpace(v), "/")
		if v == "" {
			return ""
		}
		if u, err := url.Parse(v); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			errs = append(errs, fmt.Errorf("%s inválida: %q (use http://host:porta)", name, v))
		}
		return v
	}
	cfg.LangfuseHost = httpURL("LANGFUSE_HOST", getenv("LANGFUSE_HOST"))
	cfg.LangfusePublicURL = httpURL("LANGFUSE_PUBLIC_URL", getenv("LANGFUSE_PUBLIC_URL"))
	cfg.LangfusePublicKey = strings.TrimSpace(getenv("LANGFUSE_PUBLIC_KEY"))
	cfg.LangfuseSecretKey = Secret(strings.TrimSpace(getenv("LANGFUSE_SECRET_KEY")))
	if cfg.LangfuseHost != "" && (cfg.LangfusePublicKey == "" || cfg.LangfuseSecretKey == "") {
		errs = append(errs, errors.New("LANGFUSE_HOST exige LANGFUSE_PUBLIC_KEY e LANGFUSE_SECRET_KEY"))
	}
	cfg.LangfuseProjectID = strings.TrimSpace(getenv("LANGFUSE_PROJECT_ID"))
	if cfg.LangfuseProjectID != "" && !langfuseProjectRe.MatchString(cfg.LangfuseProjectID) {
		errs = append(errs, fmt.Errorf("LANGFUSE_PROJECT_ID inválido: %q", cfg.LangfuseProjectID))
	}
	cfg.LangfuseEnvironment = strings.ToLower(orDefault(getenv("LANGFUSE_ENVIRONMENT"), string(cfg.Env)))
	if !langfuseEnvRe.MatchString(cfg.LangfuseEnvironment) || strings.HasPrefix(cfg.LangfuseEnvironment, "langfuse") {
		errs = append(errs, fmt.Errorf("LANGFUSE_ENVIRONMENT inválido: %q (minúsculas, dígitos, - e _; sem o prefixo langfuse)", cfg.LangfuseEnvironment))
	}
	timeout, err := strconv.Atoi(orDefault(getenv("LANGFUSE_TIMEOUT_SECONDS"), "5"))
	if err != nil || timeout < 1 || timeout > 60 {
		errs = append(errs, fmt.Errorf("LANGFUSE_TIMEOUT_SECONDS inválido: %q (1 a 60)", getenv("LANGFUSE_TIMEOUT_SECONDS")))
	}
	cfg.LangfuseTimeout = time.Duration(timeout) * time.Second
	retry, err := strconv.Atoi(orDefault(getenv("LANGFUSE_RETRY_SECONDS"), "300"))
	if err != nil || retry < 30 || retry > 86400 {
		errs = append(errs, fmt.Errorf("LANGFUSE_RETRY_SECONDS inválido: %q (30 a 86400)", getenv("LANGFUSE_RETRY_SECONDS")))
	}
	cfg.LangfuseRetry = time.Duration(retry) * time.Second
	cfg.TracePayloadMaxKB, err = strconv.Atoi(orDefault(getenv("TRACE_PAYLOAD_MAX_KB"), "32"))
	if err != nil || cfg.TracePayloadMaxKB < 1 || cfg.TracePayloadMaxKB > 1024 {
		errs = append(errs, fmt.Errorf("TRACE_PAYLOAD_MAX_KB inválido: %q (1 a 1024)", getenv("TRACE_PAYLOAD_MAX_KB")))
	}
	return errs
}

func orDefault(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return strings.TrimSpace(v)
}
