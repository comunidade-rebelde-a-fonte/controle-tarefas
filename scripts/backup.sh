#!/usr/bin/env bash
# Backup de produção [T-M7-06, docs/operacao.md]: banco (pg_dump -Fc) + arquivos de
# procurações/certificados (tar do volume), num diretório com data e somas SHA-256.
#
#   scripts/backup.sh [diretório-destino]      (padrão: ./backups)
#
# Variáveis: ENV_FILE (padrão .env.prod), COMPOSE_FILE (padrão docker-compose.prod.yml),
# BACKUP_RETENTION_DAYS (padrão 14; backups mais antigos do destino são removidos).
set -euo pipefail

cd "$(dirname "$0")/.."
ENV_FILE=${ENV_FILE:-.env.prod}
COMPOSE_FILE=${COMPOSE_FILE:-docker-compose.prod.yml}
DEST=${1:-./backups}
# Retenção: variável de ambiente, senão o valor do .env.prod, senão 14 dias.
KEEP=${BACKUP_RETENTION_DAYS:-$(sed -n 's/^BACKUP_RETENTION_DAYS=\([0-9][0-9]*\)$/\1/p' "$ENV_FILE" 2>/dev/null | tail -1)}
KEEP=${KEEP:-14}
compose() { docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" "$@"; }

stamp=$(date +%Y%m%d-%H%M%S)
dir="$DEST/controle-$stamp"
mkdir -p "$dir"
chmod 700 "$dir"

echo ">>> banco → $dir/db.dump"
compose exec -T db sh -c 'pg_dump -Fc -U "$POSTGRES_USER" -d "$POSTGRES_DB"' > "$dir/db.dump"

echo ">>> arquivos → $dir/storage.tar.gz"
compose exec -T app tar -C /data -czf - storage > "$dir/storage.tar.gz"

(cd "$dir" && shasum -a 256 db.dump storage.tar.gz > SHA256SUMS)
echo ">>> backup concluído: $dir ($(du -sh "$dir" | cut -f1))"

# Retenção: remove backups do destino com mais de KEEP dias.
find "$DEST" -maxdepth 1 -type d -name 'controle-*' -mtime +"$KEEP" -print -exec rm -rf {} +
