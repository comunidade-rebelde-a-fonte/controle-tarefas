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

.PHONY: help tools dev db-up db-down db-reset db-psql db-logs test test-backend lint lint-backend e2e migrate migrate-down migrate-status seed seed-demo gen

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

dev: ## Sobe o ambiente completo (db + api + web)
	@echo "disponível a partir de T-M0-05"

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

seed: ## Carrega os dados de referência (status, catálogo, feriados)
	@echo "disponível a partir de T-M1-05"

seed-demo: ## Carrega as 20 empresas fictícias e o histórico de demonstração
	@echo "disponível a partir de T-M1-12"

gen: ## Gera código (sqlc) e tipos TypeScript (openapi-typescript)
	cd backend && $(BIN)/sqlc generate

lint: lint-backend ## Lint do backend e do frontend

lint-backend:
	@test -x $(BIN)/golangci-lint || { echo "golangci-lint ausente: rode 'make tools'"; exit 1; }
	cd backend && $(BIN)/golangci-lint run ./...

test: test-backend ## Testes do backend e do frontend

test-backend:
	cd backend && go test ./...

e2e: ## Testes E2E (Playwright) contra o banco controle_e2e
	@echo "disponível a partir de T-M0-07"
