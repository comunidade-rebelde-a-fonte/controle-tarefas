package config

import (
	"strings"
	"testing"
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
