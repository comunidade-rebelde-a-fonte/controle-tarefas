// Package config carrega e valida a configuração da aplicação a partir de
// variáveis de ambiente [PRD §45.3].
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"
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
}

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
