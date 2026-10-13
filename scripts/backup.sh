#!/usr/bin/env bash
# Backup completo da instalação (TASK-031, D-044; base: T-M7-06 e o T-701 do Fator R): todos os
# armazenamentos do docker-compose.prod.yml num diretório com data e hora, com o MANIFEST das
# contagens (conferido pelo scripts/restore.sh), as somas SHA-256 e a rotação da origem.
#
#   scripts/backup.sh [destino]            (padrão: ./backups)
#
# Gera <destino>/diario/controle-AAAAMMDD-HHMMSS/ com:
#   app.dump                 o banco da aplicação (pg_dump -Fc, com a aplicação no ar)
#   storage.tar.gz           os arquivos locais: procurações, certificados e extratos do PGDAS-D
#   minio.tar.gz             os originais da Conciliação, com todas as versões (o volume do MinIO,
#                            copiado com o MinIO pausado)
#   n8n.tar.gz               o n8n: workflows, credenciais e execuções (com o n8n pausado)
#   langfuse-postgres.dump   o Postgres do Langfuse (pg_dump -Fc)
#   langfuse-clickhouse.zip  o ClickHouse do Langfuse (BACKUP DATABASE nativo)
#   langfuse-minio.tar.gz    os eventos e a mídia do Langfuse (o volume do MinIO dele, pausado)
#   MANIFEST                 as contagens de cada armazenamento no momento da cópia
#   SHA256SUMS               as somas de todos os arquivos acima
#
# Uma cópia que falha não impede as outras: o diretório ganha o sufixo "-incompleto", o MANIFEST
# diz o que faltou e o script sai com erro. Ficam de fora, por serem descartáveis: a fila do
# Langfuse (Redis), os logs do ClickHouse e a área temporária do BACKUP dele.
#
# Rotação (a da origem): 7 diários, 4 semanais (o de domingo) e 12 mensais (o do dia 1), ou o que
# estiver em BACKUP_KEEP_DAILY, BACKUP_KEEP_WEEKLY e BACKUP_KEEP_MONTHLY no .env.prod.
# Outras variáveis: ver scripts/backup-comum.sh.
set -euo pipefail
shopt -s inherit_errexit

cd "$(dirname "$0")/.."
. scripts/backup-comum.sh

DEST=${1:-./backups}
[ -f "$ENV_FILE" ] || falha "não achei o $ENV_FILE"
KEEP_DAILY=$(env_valor BACKUP_KEEP_DAILY 7)
KEEP_WEEKLY=$(env_valor BACKUP_KEEP_WEEKLY 4)
KEEP_MONTHLY=$(env_valor BACKUP_KEEP_MONTHLY 12)

umask 077
stamp=$(date +%Y%m%d-%H%M%S)
dir="$DEST/diario/controle-$stamp"
mkdir -p "$dir"

# --- Uma função por armazenamento: grava o arquivo em $dir e escreve as contagens na saída ---

copia_app_db() {
  compose exec -T db sh -c 'exec pg_dump -Fc -U "$POSTGRES_USER" -d "$POSTGRES_DB"' >"$dir/app.dump"
  pg_contagens_dump db app_db "$dir/app.dump"
}

# Os arquivos locais são gravados de uma vez (arquivo temporário + rename): a cópia com o app no ar
# não pega arquivo pela metade.
copia_storage() {
  ferramenta "tar --numeric-owner -C /volumes -czf - storage" >"$dir/storage.tar.gz"
  printf 'storage.arquivos %s\n' "$(arquivos_tar "$dir/storage.tar.gz")"
}

# copia_pausada <componente> <contagem...>: copia o volume com o serviço dono pausado, contando
# antes e depois. Se a contagem mudou no meio (alguém gravou), copia de novo, até três vezes: o
# arquivo guardado é o de uma janela sem escrita, e a contagem do MANIFEST é a dele.
copia_pausada() {
  local comp=$1 svc=${DONO[$1]} antes depois tentativa
  shift
  for tentativa in 1 2 3; do
    antes=$("$@")
    compose pause "$svc" >/dev/null 2>&1 || falha "não consegui pausar o $svc (está no ar?)"
    trap 'compose unpause "'"$svc"'" >/dev/null 2>&1 || true' EXIT
    ferramenta "tar --numeric-owner -C /volumes -czf - ${PASTA[$comp]}" >"$dir/${ARQUIVO[$comp]}"
    compose unpause "$svc" >/dev/null 2>&1
    trap - EXIT
    depois=$("$@")
    if [ "$antes" = "$depois" ]; then
      printf '%s\n%s.arquivos %s\n' "$depois" "$comp" "$(arquivos_tar "$dir/${ARQUIVO[$comp]}")"
      return 0
    fi
    log "$comp mudou durante a cópia; copiando de novo ($tentativa de 3)"
  done
  falha "$comp mudou durante as três cópias"
}

copia_minio() { copia_pausada minio minio_contagens minio minio; }
copia_n8n() { copia_pausada n8n n8n_contagens; }
copia_langfuse_minio() { copia_pausada langfuse_minio minio_contagens langfuse-minio langfuse_minio; }

copia_langfuse_db() {
  compose exec -T langfuse-postgres pg_dump -Fc -U postgres -d postgres >"$dir/langfuse-postgres.dump"
  pg_contagens_dump langfuse-postgres langfuse_db "$dir/langfuse-postgres.dump"
}

# O BACKUP nativo do ClickHouse grava na área /backups do container (volume próprio); o arquivo
# vem para o diretório do backup e sai de lá. Mesma regra da cópia pausada para a contagem.
copia_clickhouse() {
  local nome="langfuse-$stamp.zip" antes depois tentativa
  # O volume da área de backup nasce do root; o ClickHouse roda como uid 101.
  compose exec -T -u 0 langfuse-clickhouse chown 101:101 /backups
  trap 'compose exec -T langfuse-clickhouse rm -f "/backups/'"$nome"'" >/dev/null 2>&1 || true' EXIT
  for tentativa in 1 2 3; do
    antes=$(clickhouse_contagens)
    ch "BACKUP DATABASE default TO File('/backups/$nome')" >/dev/null
    compose exec -T langfuse-clickhouse cat "/backups/$nome" >"$dir/langfuse-clickhouse.zip"
    compose exec -T langfuse-clickhouse rm -f "/backups/$nome"
    depois=$(clickhouse_contagens)
    if [ "$antes" = "$depois" ]; then
      printf '%s\n' "$depois"
      return 0
    fi
    log "o ClickHouse mudou durante a cópia; copiando de novo ($tentativa de 3)"
  done
  falha "o ClickHouse mudou durante as três cópias"
}

# --- Cópias ---

feitos=()
falhas=()
for comp in "${COMPONENTES[@]}"; do
  log "copiando $comp → ${ARQUIVO[$comp]}"
  set +e
  (
    set -e
    "copia_$comp" >"$dir/.contagens-$comp"
  )
  rc=$?
  set -e
  if [ "$rc" -eq 0 ]; then
    feitos+=("$comp")
  else
    falhas+=("$comp")
    rm -f "$dir/${ARQUIVO[$comp]}" "$dir/.contagens-$comp"
    log "FALHOU: $comp (os outros armazenamentos seguem)"
  fi
done

{
  echo "# MANIFEST do backup (scripts/backup.sh, TASK-031): as contagens de cada armazenamento no"
  echo "# momento da cópia, no formato \"chave valor\". O scripts/restore.sh confere cada uma."
  echo "criado_em $(date +%Y-%m-%dT%H:%M:%S%z)"
  echo "versao_app $(env_valor APP_VERSION local)"
  echo "componentes ${feitos[*]}"
  if [ ${#falhas[@]} -gt 0 ]; then echo "faltando ${falhas[*]}"; fi
  for comp in "${feitos[@]}"; do sort "$dir/.contagens-$comp"; done
} >"$dir/MANIFEST"
rm -f "$dir"/.contagens-*

arquivos=(MANIFEST)
for comp in "${feitos[@]}"; do arquivos+=("${ARQUIVO[$comp]}"); done
(cd "$dir" && soma_sha256 "${arquivos[@]}" >SHA256SUMS)

if [ ${#falhas[@]} -gt 0 ]; then
  mv "$dir" "$dir-incompleto"
  falha "backup incompleto em $dir-incompleto: faltou ${falhas[*]} (o resto foi copiado)"
fi
log "backup concluído: $dir ($(du -sh "$dir" | cut -f1); $(grep -c '^[a-z0-9_]*\.' "$dir/MANIFEST") contagens no MANIFEST)"

# Cópias semanal (domingo) e mensal (dia 1), e a rotação de cada pasta.
if [ "$(date +%u)" = 7 ]; then mkdir -p "$DEST/semanal" && cp -R "$dir" "$DEST/semanal/"; fi
if [ "$(date +%d)" = 01 ]; then mkdir -p "$DEST/mensal" && cp -R "$dir" "$DEST/mensal/"; fi

# rotaciona <pasta> <quantos manter>: apaga os backups mais antigos além dos N mais recentes.
rotaciona() {
  [ -d "$1" ] || return 0
  find "$1" -mindepth 1 -maxdepth 1 -type d -name 'controle-*' | sort |
    awk -v k="$2" '{ a[NR] = $0 } END { for (i = 1; i <= NR - k; i++) print a[i] }' |
    while read -r antigo; do
      log "rotação: removendo $antigo"
      rm -rf -- "$antigo"
    done
}
rotaciona "$DEST/diario" "$KEEP_DAILY"
rotaciona "$DEST/semanal" "$KEEP_WEEKLY"
rotaciona "$DEST/mensal" "$KEEP_MONTHLY"
