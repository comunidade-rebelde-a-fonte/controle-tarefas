# Sistema de Controle de Obrigações — comandos de desenvolvimento.
# Execute `make help` para a lista de alvos.

SHELL := /bin/bash
.DEFAULT_GOAL := help

.PHONY: help tools dev db-up db-down db-reset db-psql db-logs test lint e2e migrate migrate-down seed seed-demo gen

help: ## Lista os alvos disponíveis
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z0-9_-]+:.*?## / {printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

tools: ## Instala as ferramentas de desenvolvimento (sqlc, goose, golangci-lint, air)
	@echo "disponível a partir de T-M0-04"

dev: ## Sobe o ambiente completo (db + api + web)
	@echo "disponível a partir de T-M0-05"

db-up: ## Sobe somente o banco e espera ficar healthy
	@echo "disponível a partir de T-M0-02"

db-down: ## Para o banco
	@echo "disponível a partir de T-M0-02"

db-reset: ## APAGA e recria os bancos locais (controle, controle_e2e, controle_perf)
	@echo "disponível a partir de T-M0-02"

db-psql: ## Abre o psql no banco de desenvolvimento
	@echo "disponível a partir de T-M0-02"

db-logs: ## Mostra os logs do Postgres
	@echo "disponível a partir de T-M0-02"

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
