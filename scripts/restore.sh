#!/usr/bin/env bash
# Restauração de produção [T-M7-06, docs/operacao.md]: confere as somas, para a aplicação,
# restaura o banco (pg_restore --clean) e os arquivos, e sobe a aplicação de novo.
#
#   scripts/restore.sh <diretório-do-backup>
#
# ATENÇÃO: substitui o banco e os arquivos atuais pelos do backup.
set -euo pipefail

cd "$(dirname "$0")/.."
dir=${1:?informe o diretório do backup (ex.: backups/controle-20261001-020000)}
ENV_FILE=${ENV_FILE:-.env.prod}
COMPOSE_FILE=${COMPOSE_FILE:-docker-compose.prod.yml}
compose() { docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" "$@"; }

[ -f "$dir/db.dump" ] && [ -f "$dir/storage.tar.gz" ] || { echo "backup incompleto em $dir"; exit 1; }
(cd "$dir" && shasum -a 256 -c SHA256SUMS)

echo ">>> parando a aplicação"
compose up -d db
compose stop app || true

echo ">>> restaurando o banco"
compose exec -T db sh -c 'pg_restore --clean --if-exists --no-owner -U "$POSTGRES_USER" -d "$POSTGRES_DB"' < "$dir/db.dump"

echo ">>> restaurando os arquivos"
compose run --rm --no-deps -T --entrypoint sh app -c 'find /data/storage -mindepth 1 -delete && tar -C /data -xzf -' < "$dir/storage.tar.gz"

echo ">>> subindo a aplicação"
compose up -d app
echo ">>> restauração concluída a partir de $dir"
