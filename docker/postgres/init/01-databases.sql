-- Cria os bancos locais do projeto. Executado pelo entrypoint do Postgres
-- SOMENTE quando o volume de dados está vazio (primeira inicialização).
-- Para recriar os bancos: `make db-reset` (apaga os dados locais).
--
--   controle       desenvolvimento (make dev)
--   controle_e2e   testes E2E (make e2e) — nunca usar o banco de dev
--   controle_perf  testes de carga (loadgen)

SELECT 'CREATE DATABASE controle'
WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'controle')\gexec

SELECT 'CREATE DATABASE controle_e2e'
WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'controle_e2e')\gexec

SELECT 'CREATE DATABASE controle_perf'
WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'controle_perf')\gexec
