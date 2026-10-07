# Sistema de Controle de Obrigações — comandos de desenvolvimento.
# Execute `make help` para a lista de alvos.

SHELL := /bin/bash
.DEFAULT_GOAL := help

# Variáveis locais (.env não é versionado; ver .env.example).
-include .env
export

POSTGRES_USER ?= controle
POSTGRES_PASSWORD ?= dev-only-change-me
POSTGRES_PORT ?= 5432

# DB=dev (padrão) | e2e | perf — seleciona o banco local alvo.
DB ?= dev
ifeq ($(DB),dev)
DB_NAME := controle
else ifeq ($(DB),e2e)
DB_NAME := controle_e2e
else ifeq ($(DB),perf)
DB_NAME := controle_perf
else
$(error DB inválido: "$(DB)". Use dev, e2e ou perf)
endif

COMPOSE := docker compose
# Langfuse self-hosted em compose dedicado (pesado: Postgres, ClickHouse, Redis e MinIO próprios).
COMPOSE_LF := docker compose -f docker-compose.langfuse.yml

# Ferramentas de desenvolvimento, instaladas em .bin/ com versões fixadas (make tools).
BIN := $(CURDIR)/.bin
SQLC_VERSION := v1.31.1
GOOSE_VERSION := v3.28.0
GOLANGCI_LINT_VERSION := v2.14.0
AIR_VERSION := v1.67.4
GOVULNCHECK_VERSION := v1.8.0

# URL do banco local selecionado por DB=, acessado a partir do host.
DB_URL := postgres://$(POSTGRES_USER):$(POSTGRES_PASSWORD)@localhost:$(POSTGRES_PORT)/$(DB_NAME)?sslmode=disable
API := cd backend && DATABASE_URL='$(DB_URL)' go run ./cmd/api

# Banco e portas exclusivos do E2E (nunca o banco de dev).
E2E_DB_URL := postgres://$(POSTGRES_USER):$(POSTGRES_PASSWORD)@localhost:$(POSTGRES_PORT)/controle_e2e?sslmode=disable
PERF_DB_URL := postgres://$(POSTGRES_USER):$(POSTGRES_PASSWORD)@localhost:$(POSTGRES_PORT)/controle_perf?sslmode=disable
LOADGEN := cd backend && DATABASE_URL='$(PERF_DB_URL)' go run ./cmd/loadgen
COMPANIES ?= 300
COMPETENCES ?= 3
SEED ?= 42
E2E_API_PORT ?= 8091
E2E_WEB_PORT ?= 5174
# Relógio fixo do E2E (API e seed): datas e "competência atual" não dependem do dia real.
E2E_FIXED_TIME ?= 2026-09-15T12:00:00-03:00

.PHONY: help tools dev dev-all dev-logs dev-down langfuse-up langfuse-down langfuse-logs health db-up db-down db-reset db-psql db-logs test test-backend test-frontend lint lint-backend lint-frontend e2e migrate migrate-down migrate-status db-clean seed seed-demo gen perf-generate perf-bench db-dump db-restore audit

help: ## Lista os alvos disponíveis
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z0-9_-]+:.*?## / {printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

tools: ## Instala as ferramentas de desenvolvimento (sqlc, goose, golangci-lint, air, govulncheck)
	@mkdir -p $(BIN)
	GOBIN=$(BIN) go install github.com/sqlc-dev/sqlc/cmd/sqlc@$(SQLC_VERSION)
	GOBIN=$(BIN) go install github.com/pressly/goose/v3/cmd/goose@$(GOOSE_VERSION)
	GOBIN=$(BIN) go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
	GOBIN=$(BIN) go install github.com/air-verse/air@$(AIR_VERSION)
	GOBIN=$(BIN) go install golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)
	@echo "ferramentas instaladas em $(BIN)"

dev: ## Sobe o ambiente (db, api, web, worker, MinIO, LiteLLM, n8n) e espera ficar healthy
	$(COMPOSE) up -d --wait db api web worker minio litellm n8n
	$(COMPOSE) run --rm minio-init
	@echo ">>> SPA: http://localhost:5173  |  API: http://localhost:$${HTTP_PORT:-8080}/api/v1/health"
	@echo ">>> MinIO: http://localhost:$${MINIO_CONSOLE_PORT:-9001}  |  LiteLLM: http://localhost:$${LITELLM_PORT:-4000}  |  n8n: http://localhost:$${N8N_PORT:-5678}"
	@echo ">>> Langfuse (opcional, pesado): make langfuse-up  |  saúde de tudo: make health"

dev-all: dev langfuse-up ## Sobe o ambiente de desenvolvimento e o Langfuse

dev-logs: ## Acompanha os logs do ambiente de desenvolvimento
	$(COMPOSE) logs -f --tail=100

dev-down: ## Para o ambiente de desenvolvimento e o Langfuse (dados preservados)
	$(COMPOSE) stop
	-@$(COMPOSE_LF) stop

langfuse-up: ## Sobe o Langfuse self-hosted (traces e evals dos agentes) e espera ficar healthy
	$(COMPOSE_LF) up -d --wait
	@echo ">>> Langfuse: http://localhost:$${LANGFUSE_WEB_PORT:-3100} (usuário: LANGFUSE_ADMIN_EMAIL do .env)"

langfuse-down: ## Para o Langfuse (dados preservados)
	$(COMPOSE_LF) stop

langfuse-logs: ## Acompanha os logs do Langfuse
	$(COMPOSE_LF) logs -f --tail=100

health: ## Mostra a saúde de cada serviço local (API, banco, worker, MinIO, LiteLLM, n8n, Langfuse)
	@scripts/health.sh

db-up: ## Sobe somente o banco e espera ficar healthy
	$(COMPOSE) up -d --wait db

db-down: ## Para o banco (os dados são preservados)
	$(COMPOSE) stop db

db-reset: ## APAGA e recria os bancos locais (controle, controle_e2e, controle_perf)
	@echo ">>> ATENÇÃO: apagando os dados locais dos bancos controle, controle_e2e e controle_perf."
	$(COMPOSE) rm --stop --force db
	docker volume rm --force controle_pgdata
	$(COMPOSE) up -d --wait db
	@echo ">>> Bancos recriados vazios. Rode 'make migrate' e 'make seed'."

db-psql: ## Abre o psql no banco local (DB=e2e|perf para os outros bancos)
	$(COMPOSE) exec db psql -U $(POSTGRES_USER) -d $(DB_NAME)

db-logs: ## Mostra os logs do Postgres
	$(COMPOSE) logs --tail=200 db

migrate: ## Aplica as migrations (DB=e2e|perf para os outros bancos)
	@$(API) migrate up

migrate-down: ## Reverte a última migration (DB=e2e|perf para os outros bancos)
	@$(API) migrate down

migrate-status: ## Mostra o estado das migrations (DB=e2e|perf para os outros bancos)
	@$(API) migrate status

seed: ## Carrega os dados de referência (status, catálogo, feriados) — idempotente
	@$(API) seed

seed-demo: ## Carrega as 20 empresas fictícias e o histórico de demonstração — idempotente
	@$(API) seed --demo

gen: ## Gera código (sqlc) e tipos TypeScript (openapi-typescript)
	cd backend && $(BIN)/sqlc generate
	cd frontend && npm run --silent gen:api

lint: lint-backend lint-frontend ## Lint do backend e do frontend

lint-backend:
	@test -x $(BIN)/golangci-lint || { echo "golangci-lint ausente: rode 'make tools'"; exit 1; }
	cd backend && $(BIN)/golangci-lint run ./...

lint-frontend:
	cd frontend && npm run lint && npm run typecheck
	@test -d e2e/node_modules || (cd e2e && npm ci --no-audit --no-fund)
	cd e2e && npm run typecheck

test: test-backend test-frontend ## Testes do backend e do frontend

test-backend:
	cd backend && go test ./...

test-frontend:
	cd frontend && npm test

audit: ## Vulnerabilidades conhecidas: govulncheck (Go) e npm audit (frontend e e2e) [docs/seguranca.md]
	cd backend && $(BIN)/govulncheck ./...
	cd frontend && npm audit --audit-level=high
	cd e2e && npm audit --audit-level=high

db-clean: ## Recria vazio o schema de controle_e2e ou controle_perf (DB=e2e|perf) e aplica as migrations
	@if [ "$(DB)" = "dev" ]; then echo "db-clean não se aplica ao banco de dev; use make db-reset"; exit 1; fi
	@echo ">>> recriando o schema do banco $(DB_NAME)"
	@$(COMPOSE) exec -T db psql -q -U $(POSTGRES_USER) -d $(DB_NAME) -c "SET client_min_messages TO warning; DROP SCHEMA public CASCADE; CREATE SCHEMA public;"
	@$(API) migrate up
	@if [ "$(DB)" = "e2e" ]; then $(API) seed --demo; else $(API) seed; fi

db-dump: ## Dump do banco de dev (pg_dump -Fc) em backups/dev-<data>.dump
	@mkdir -p backups
	@f=backups/dev-$$(date +%Y%m%d-%H%M%S).dump; \
	$(COMPOSE) exec -T db pg_dump -Fc -U $(POSTGRES_USER) -d $(DB_NAME) > $$f && echo ">>> $$f"

db-restore: ## Restaura um dump no banco de dev (FILE=backups/dev-....dump; APAGA os dados atuais)
	@test -n "$(FILE)" || { echo "informe FILE=backups/...dump"; exit 1; }
	@$(COMPOSE) exec -T db pg_restore --clean --if-exists --no-owner -U $(POSTGRES_USER) -d $(DB_NAME) < $(FILE)
	@echo ">>> $(FILE) restaurado em $(DB_NAME)"

perf-generate: ## Massa sintética no controle_perf (COMPANIES=300 COMPETENCES=3 SEED=42; recria o schema)
	@$(MAKE) --no-print-directory db-clean DB=perf
	@$(LOADGEN) generate -companies $(COMPANIES) -competences $(COMPETENCES) -seed $(SEED)

perf-bench: ## Mede grades, dashboard, backlog, exportações, ensure, PATCH e bulk no controle_perf (BENCH_ARGS="-iterations 50")
	@$(LOADGEN) bench $(BENCH_ARGS)

e2e: ## Testes E2E (Playwright) contra o banco controle_e2e (recriado a cada execução)
	$(COMPOSE) up -d --wait db
	@APP_FIXED_TIME='$(E2E_FIXED_TIME)' $(MAKE) --no-print-directory db-clean DB=e2e
	@test -d e2e/node_modules || (cd e2e && npm ci --no-audit --no-fund)
	@cd e2e && npx playwright install chromium >/dev/null
	@echo ">>> playwright (API :$(E2E_API_PORT) e SPA :$(E2E_WEB_PORT) sobre controle_e2e, relógio fixo em $(E2E_FIXED_TIME))"
	@cd e2e && E2E_DATABASE_URL='$(E2E_DB_URL)' E2E_FIXED_TIME='$(E2E_FIXED_TIME)' E2E_API_PORT=$(E2E_API_PORT) E2E_WEB_PORT=$(E2E_WEB_PORT) npx playwright test $(ARGS)
