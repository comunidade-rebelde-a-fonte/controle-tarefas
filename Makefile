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

.PHONY: help tools dev db-up db-down db-reset db-psql db-logs test lint e2e migrate migrate-down seed seed-demo gen

help: ## Lista os alvos disponíveis
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z0-9_-]+:.*?## / {printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

tools: ## Instala as ferramentas de desenvolvimento (sqlc, goose, golangci-lint, air)
	@echo "disponível a partir de T-M0-04"

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
	@echo "disponível a partir de T-M0-04"

migrate-down: ## Reverte a última migration (DB=e2e|perf para os outros bancos)
	@echo "disponível a partir de T-M0-04"

seed: ## Carrega os dados de referência (status, catálogo, feriados)
	@echo "disponível a partir de T-M1-05"

seed-demo: ## Carrega as 20 empresas fictícias e o histórico de demonstração
	@echo "disponível a partir de T-M1-12"

gen: ## Gera código (sqlc) e tipos TypeScript (openapi-typescript)
	@echo "disponível a partir de T-M0-04"

lint: ## Lint do backend e do frontend
	@echo "disponível a partir de T-M0-03"

test: ## Testes do backend e do frontend
	@echo "disponível a partir de T-M0-03"

e2e: ## Testes E2E (Playwright) contra o banco controle_e2e
	@echo "disponível a partir de T-M0-07"
