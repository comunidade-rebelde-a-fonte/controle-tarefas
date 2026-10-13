#!/usr/bin/env bash
# TEST-031 (TASK-031, D-044): a instalação completa do docker-compose.prod.yml numa cópia
# descartável, com dados em todos os armazenamentos; o backup; a destruição dela (down -v); uma
# instalação nova e vazia; a restauração; e a conferência, com a instalação original, das
# contagens e do conteúdo de cada armazenamento.
#
#   scripts/testar-restore.sh            (ou make test-restore; uns 15 minutos)
#
# Isolamento: projetos compose próprios (controle-t031a e controle-t031b), com volumes próprios,
# sem porta no host e sem saída para a internet (docker/testar-restore.yml): nenhum dado sai da
# máquina e nenhum provedor de IA é chamado. Os segredos são gerados na hora, num diretório
# temporário apagado no fim, junto com os volumes das duas instalações. O ambiente de
# desenvolvimento não é tocado. Constrói a imagem controle-app:t031 a partir do Dockerfile.
set -euo pipefail
shopt -s inherit_errexit

cd "$(dirname "$0")/.."
. scripts/backup-comum.sh

A=controle-t031a
B=controle-t031b
tmp=$(mktemp -d)
ENV_FILE="$tmp/env.prod"
COMPOSE_FILE=docker-compose.prod.yml
COMPOSE_OVERRIDE=docker/testar-restore.yml
export ENV_FILE COMPOSE_FILE COMPOSE_OVERRIDE

# Arquivos dos testes do próprio repositório: uma procuração (um PDF qualquer), um extrato do
# PGDAS-D e um extrato bancário do cenário da Conciliação.
FIX_PROCURACAO=backend/internal/factorr/pgdas/testdata/fixtures/pdf_com_texto/documento.pdf
FIX_PGDAS=backend/internal/factorr/pgdas/testdata/fixtures/pdf_declaratorio_fator_r_acima_28/documento.pdf
FIX_EXTRATO=backend/internal/reconciliation/testdata/conciliacao/extratos/extrato_construtora_horizonte_2026-08.pdf
# Imagem já presente na máquina (a do frontend de dev), só para o curl dentro da rede do teste.
CURL_IMAGE=node:22.23.3-bookworm

limpa() {
  local rc=$?
  log "limpando: as duas instalações descartáveis (com os volumes) e o diretório temporário"
  for p in "$A" "$B"; do
    COMPOSE_PROJECT_NAME=$p compose --profile backup down -v --remove-orphans >/dev/null 2>&1 || true
  done
  rm -rf "$tmp"
  if [ "$rc" -eq 0 ]; then log "TEST-031 OK"; else log "TEST-031 FALHOU (saída $rc)"; fi
  exit "$rc"
}
trap limpa EXIT

# --- Segredos descartáveis: o modelo do .env.prod com os campos obrigatórios preenchidos ---
segredo() { openssl rand -hex "${1:-24}"; }
cp .env.prod.example "$ENV_FILE"
preenche() { sed -i "s|^$1=.*|$1=$2|" "$ENV_FILE"; }
for v in POSTGRES_PASSWORD S3_ROOT_PASSWORD S3_SECRET_KEY NEXTAUTH_SECRET SALT LF_POSTGRES_PASSWORD \
  CLICKHOUSE_PASSWORD MINIO_ROOT_PASSWORD REDIS_AUTH LANGFUSE_ADMIN_PASSWORD; do
  preenche "$v" "$(segredo)"
done
preenche LITELLM_MASTER_KEY "sk-$(segredo)"
preenche ENCRYPTION_KEY "$(segredo 32)"
preenche LANGFUSE_PUBLIC_KEY "pk-lf-$(segredo 16)"
preenche LANGFUSE_SECRET_KEY "sk-lf-$(segredo 24)"
preenche APP_VERSION t031
# O worker reenvia logo ao Langfuse uma corrida que falhou no envio (o padrão é 5 minutos).
preenche LANGFUSE_RETRY_SECONDS 30

# --- Esperas e contagens ---

# espera <segundos> <descrição> <comando...>: repete o comando, a cada 3 s, até ele dar certo.
espera() {
  local limite=$((SECONDS + $1)) desc=$2
  shift 2
  until "$@"; do
    [ "$SECONDS" -lt "$limite" ] || falha "tempo esgotado esperando $desc"
    sleep 3
  done
}
sql() { printf '%s\n' "$1" | psql_em db | tr -d '\r'; }
fila_vazia() { [ "$(sql "SELECT count(*) FROM jobs WHERE status IN ('pending', 'running', 'retry');")" = 0 ]; }
pgdas_lido() { [ "$(sql "SELECT count(*) FROM factor_r_pgdas_documents WHERE status <> 'received';")" -gt 0 ]; }
# O Langfuse v4 grava os traces nas tabelas de eventos: basta haver linha fora das migrations.
trace_no_langfuse() {
  [ "$(ch "SELECT sum(total_rows) FROM system.tables WHERE database = 'default' AND engine LIKE '%MergeTree' AND name <> 'schema_migrations'" | tr -d '\r')" -gt 0 ]
}

MC='export MC_HOST_local="http://$MINIO_ROOT_USER:$MINIO_ROOT_PASSWORD@127.0.0.1:9000";'

# contagens: o retrato de cada armazenamento, medido por um caminho diferente do MANIFEST — todas
# as tabelas pelo catálogo, o SHA-256 de cada arquivo local, a lista completa das versões dos
# MinIO, os workflows exportados do n8n e um hash do conteúdo de cada tabela do ClickHouse (a soma
# do hash de cada linha, que não depende da ordem).
contagens() {
  local tabelas="SELECT format('SELECT %L, count(*) FROM %I.%I', n.nspname || '.' || c.relname, n.nspname, c.relname)
    FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
    WHERE c.relkind = 'r' AND n.nspname NOT IN ('pg_catalog', 'information_schema') AND n.nspname NOT LIKE 'pg_toast%'
    ORDER BY 1 \\gexec"
  printf '%s\n' "$tabelas" | psql_em db | sed 's/^/app_db /'
  printf '%s\n' "$tabelas" | psql_em langfuse-postgres | sed 's/^/langfuse_db /'
  ferramenta "cd /volumes/storage && find . -type f -print0 | sort -z | xargs -0 -r sha256sum" | sed 's/^/storage /'
  compose exec -T minio sh -c "$MC mc ls --recursive --versions local" | sed 's/^/minio /'
  compose exec -T langfuse-minio sh -c "$MC mc ls --recursive --versions local" | sed 's/^/langfuse_minio /'
  compose exec -T n8n n8n export:workflow --all 2>/dev/null | soma_sha256 | sed 's/^/n8n workflows /'
  clickhouse_contagens
  clickhouse_por_tabela clickhouse_conteudo "sum(cityHash64(formatRow('TSV', *)))"
}
contagens_estaveis() {
  local c1 c2
  c1=$(contagens)
  sleep 10
  c2=$(contagens)
  [ "$c1" = "$c2" ]
}

# curl_em <projeto> <args...>: o curl dentro da rede da instalação (que não tem porta no host).
curl_em() {
  local projeto=$1
  shift
  docker run --rm -i --network "${projeto}_default" "$CURL_IMAGE" curl -fsS "$@"
}

servicos_no_ar() {
  local estado
  estado=$(compose ps --all --format '{{.Service}} {{.State}} {{.Health}} {{.ExitCode}}')
  printf '%s\n' "$estado" | sed 's/^/    /' >&2
  # Todos rodando e saudáveis, menos o minio-init, que termina com sucesso.
  ! printf '%s\n' "$estado" | grep -v '^minio-init exited  0$' | grep -vqE '^[a-z0-9-]+ running healthy 0$'
}

# --- 1. Instalação A, completa, com dados em todos os armazenamentos ---
export COMPOSE_PROJECT_NAME=$A
log "construindo a imagem controle-app:t031 (Dockerfile)"
compose build app
log "subindo a instalação A ($A) com todos os serviços"
compose up -d --wait --wait-timeout 900
servicos_no_ar || falha "a instalação A não subiu com todos os serviços saudáveis"

log "A: seed de referência e de demonstração (dados fictícios: só com APP_ENV=test, num container avulso)"
compose run --rm --no-deps -e APP_ENV=test app seed --demo >/dev/null
empresa=$(sql "SELECT id FROM companies WHERE internal_code = '111';")
[ -n "$empresa" ] || falha "a empresa 111 do seed não apareceu"

log "A: procuração e PGDAS-D (arquivos locais) e extrato bancário da Conciliação (MinIO), pela API"
curl_em "$A" -X PUT -F "file=@-;filename=procuracao.pdf;type=application/pdf" \
  "http://app:8080/api/v1/companies/$empresa/credential-controls/RFB_POWER_OF_ATTORNEY/file" <"$FIX_PROCURACAO" >/dev/null
curl_em "$A" -F "file=@-;filename=pgdas.pdf;type=application/pdf" http://app:8080/api/v1/factor-r/pgdas <"$FIX_PGDAS" >/dev/null
curl_em "$A" -F "file=@-;filename=extrato.pdf;type=application/pdf" -F "company_id=$empresa" -F "competence=2026-08" \
  http://app:8080/api/v1/reconciliation/documents <"$FIX_EXTRATO" >/dev/null

log "A: versões e uma marca de exclusão no bucket (o histórico do bucket versionado vai no backup)"
compose exec -T minio sh -c "$MC"' b=local/conciliacao-originais/teste-restauracao
  for v in 1 2 3; do echo "versao $v" | mc pipe "$b/versionado.txt" >/dev/null; done
  echo removido | mc pipe "$b/removido.txt" >/dev/null
  mc rm "$b/removido.txt" >/dev/null'

log "A: um workflow no n8n (inativo)"
printf '%s' '{"id":"TesteRestaur0031","name":"Teste da restauração (TEST-031)","active":false,"nodes":[{"parameters":{},"id":"8f2d3c1e-1a2b-4c3d-9e8f-031031031031","name":"Início manual","type":"n8n-nodes-base.manualTrigger","typeVersion":1,"position":[0,0]}],"connections":{},"settings":{"executionOrder":"v1"},"pinData":{}}' |
  compose exec -T n8n sh -c 'cat >/tmp/teste.json && n8n import:workflow --input=/tmp/teste.json && rm -f /tmp/teste.json' >/dev/null

log "A: esperando o worker (parser do PGDAS-D e Conciliação) e o envio do trace ao Langfuse"
espera 300 "o parser do PGDAS-D" pgdas_lido
espera 300 "a fila de jobs vazia" fila_vazia
espera 300 "o trace no ClickHouse do Langfuse" trace_no_langfuse
espera 300 "as contagens pararem de mudar" contagens_estaveis

# --- 2. Backup de A ---
log "A: backup completo"
scripts/backup.sh "$tmp/backups"
backup=$(find "$tmp/backups/diario" -mindepth 1 -maxdepth 1 -type d -name 'controle-*' | head -1)
[ -n "$backup" ] && [ -f "$backup/MANIFEST" ] || falha "o backup não gerou o diretório com o MANIFEST"
awk '$1 ~ /^[a-z0-9_]+\./ { split($1, k, "."); n[k[1]]++ } END { for (c in n) printf "    %-15s %d contagens\n", c, n[c] }' "$backup/MANIFEST" | sort >&2
antes=$(contagens)

# --- 3. Destruição de A e instalação B, nova e vazia ---
log "destruindo a instalação A (down -v: containers e volumes)"
compose --profile backup down -v --remove-orphans >/dev/null
[ -z "$(docker volume ls -q --filter "label=com.docker.compose.project=$A")" ] || falha "sobrou volume da instalação A"

export COMPOSE_PROJECT_NAME=$B
log "subindo a instalação B ($B), vazia"
compose up -d --wait --wait-timeout 900
servicos_no_ar || falha "a instalação B não subiu com todos os serviços saudáveis"
[ "$(sql "SELECT count(*) FROM companies;")" = 0 ] || falha "a instalação B deveria nascer vazia"

# --- 4. Restauração em B e conferência ---
log "B: restaurando o backup de A"
RESTORE_CONFIRM=sim scripts/restore.sh "$backup"
servicos_no_ar || falha "a instalação B não voltou com todos os serviços saudáveis"

log "B: conferindo cada armazenamento com o retrato tirado de A"
depois=$(contagens)
# Tudo o que existia em A tem de estar igual em B. Ao subir, o Langfuse completa sozinho os dados
# de referência dele (catálogo de modelos e preços, painéis padrão) e as migrações de fundo, que
# podem criar tabelas novas: essas tabelas de referência saem da comparação. O restore.sh já as
# conferiu com o MANIFEST antes de subir os serviços.
REFERENCIA_LANGFUSE='^langfuse_db public\.(models|prices|pricing_tiers|dashboards|dashboard_widgets) '
retrato() { printf '%s\n' "$1" | grep -vE "$REFERENCIA_LANGFUSE" | sort; }
faltando=$(comm -23 <(retrato "$antes") <(retrato "$depois"))
if [ -n "$faltando" ]; then
  log "itens de A que não estão iguais em B:"
  printf '%s\n' "$faltando" >&2
  falha "a instalação restaurada difere da original"
fi
log "B: $(retrato "$antes" | wc -l | tr -d ' ') itens de A iguais em B (tabelas, arquivos, versões, workflows e traces)"

log "B: a API responde e devolve os originais com o mesmo SHA-256"
curl_em "$B" http://app:8080/api/v1/health | grep -q '"status":"ok"' || falha "a API de B não está saudável"
confere_original() { # <rota> <arquivo de origem>
  local h
  h=$(docker run --rm --network "${B}_default" "$CURL_IMAGE" sh -c "curl -fsS 'http://app:8080$1' | sha256sum" | cut -d' ' -f1)
  [ "$h" = "$(soma_sha256 <"$2" | cut -d' ' -f1)" ] || falha "o original de $1 mudou na restauração"
}
confere_original "/api/v1/factor-r/pgdas/$(sql "SELECT id FROM factor_r_pgdas_documents LIMIT 1;")/file" "$FIX_PGDAS"
confere_original "/api/v1/reconciliation/documents/$(sql "SELECT id FROM reconciliation_documents LIMIT 1;")/file?download=1" "$FIX_EXTRATO"
