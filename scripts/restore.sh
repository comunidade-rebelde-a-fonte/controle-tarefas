#!/usr/bin/env bash
# Restauração completa (TASK-031, D-044; base: T-M7-06): confere as somas, para os serviços que
# escrevem, restaura cada armazenamento presente no backup, confere as contagens com o MANIFEST e
# sobe tudo de novo.
#
#   scripts/restore.sh <diretório-do-backup>
#
# ATENÇÃO: substitui os dados atuais da instalação pelos do backup. Pede confirmação (digite
# "sim"); para rodar sem a pergunta, RESTORE_CONFIRM=sim.
#
# A instalação precisa existir (a primeira implantação feita, sem o seed: docs/operacao.md §2).
# Com alguma contagem diferente da do MANIFEST, os serviços que escrevem (app, worker, n8n e
# Langfuse) ficam parados e o script sai com erro. Um backup sem MANIFEST (o formato anterior à
# TASK-031: db.dump e storage.tar.gz) é restaurado sem essa conferência.
# Variáveis: ver scripts/backup-comum.sh.
set -euo pipefail
shopt -s inherit_errexit

cd "$(dirname "$0")/.."
. scripts/backup-comum.sh

dir=${1:?informe o diretório do backup (ex.: backups/diario/controle-20261001-020000)}
[ -d "$dir" ] || falha "não achei o diretório $dir"
[ -f "$dir/SHA256SUMS" ] || falha "backup sem SHA256SUMS em $dir"
[ -f "$ENV_FILE" ] || falha "não achei o $ENV_FILE"

log "conferindo as somas SHA-256"
(cd "$dir" && soma_sha256 -c SHA256SUMS >/dev/null) || falha "somas SHA-256 divergentes: o backup em $dir está corrompido"

manifest="$dir/MANIFEST"
if [ ! -f "$manifest" ]; then
  manifest=""
  # Formato anterior à TASK-031: o banco era db.dump.
  if [ -f "$dir/db.dump" ]; then ARQUIVO[app_db]=db.dump; fi
  log "backup sem MANIFEST (formato anterior à TASK-031): restaura sem conferir as contagens"
fi
presentes=()
for comp in "${COMPONENTES[@]}"; do
  if [ -f "$dir/${ARQUIVO[$comp]}" ]; then
    presentes+=("$comp")
  else
    log "sem ${ARQUIVO[$comp]} no backup: $comp fica como está"
  fi
done
[ ${#presentes[@]} -gt 0 ] || falha "nenhum armazenamento para restaurar em $dir"

if [ "${RESTORE_CONFIRM:-}" != sim ]; then
  [ -t 0 ] || falha "confirme com RESTORE_CONFIRM=sim (ou rode num terminal)"
  printf 'Isto SUBSTITUI os dados atuais (%s) pelos do backup %s.\nDigite sim para continuar: ' "${presentes[*]}" "$dir" >&2
  read -r resposta
  [ "$resposta" = sim ] || falha "restauração cancelada"
fi

divergencias=()
# confere <componente> <regex das chaves> <contagens atuais>: compara com as linhas do MANIFEST.
confere() {
  local comp=$1 chaves=$2 atual esperado
  [ -n "$manifest" ] || return 0
  esperado=$(grep -E "^($chaves) " "$manifest" | sort)
  atual=$(printf '%s\n' "$3" | sed '/^$/d' | sort)
  if [ "$esperado" = "$atual" ]; then
    local n
    n=$(printf '%s\n' "$esperado" | grep -c .)
    if [ "$n" -le 3 ]; then
      log "$comp: confere com o MANIFEST ($(printf '%s\n' "$esperado" | paste -sd ',' - | sed 's/,/, /g'))"
    else
      log "$comp: as $n contagens conferem com o MANIFEST"
    fi
  else
    log "$comp: contagens DIFERENTES das do MANIFEST (< backup, > restaurado):"
    diff <(printf '%s\n' "$esperado") <(printf '%s\n' "$atual") >&2 || true
    divergencias+=("$comp")
  fi
}

# restaura_volume <componente>: troca o conteúdo do volume pelo do tar (dono e permissões numéricos).
restaura_volume() {
  ferramenta "find /volumes/${PASTA[$1]} -mindepth 1 -delete && tar --numeric-owner -C /volumes -xzf -" <"$dir/${ARQUIVO[$1]}"
  confere "$1" "$1\\.arquivos" "$1.arquivos $(arquivos_volume "$1")"
}

# restaura_pg <serviço> <componente> <comando pg_restore>: substitui o banco numa transação só (com
# erro, o banco fica como estava) e conta as tabelas do MANIFEST.
restaura_pg() {
  compose up -d --wait "$1" >/dev/null
  compose exec -T "$1" sh -c "$3" <"$dir/${ARQUIVO[$2]}"
  if [ -n "$manifest" ]; then
    confere "$2" "$2\\.[^ ]+" "$(grep "^$2\\." "$manifest" | pg_contagens_vivo "$1" "$2")"
  fi
}

log "parando os serviços que escrevem (app, worker, n8n e Langfuse)"
compose stop app worker n8n langfuse-web langfuse-worker >/dev/null

for comp in "${presentes[@]}"; do
  log "restaurando $comp ← ${ARQUIVO[$comp]}"
  case $comp in
    app_db)
      restaura_pg db app_db 'exec pg_restore --clean --if-exists --no-owner --single-transaction --exit-on-error -U "$POSTGRES_USER" -d "$POSTGRES_DB"'
      ;;
    storage)
      restaura_volume storage
      ;;
    minio | langfuse_minio)
      compose stop "${DONO[$comp]}" >/dev/null
      restaura_volume "$comp"
      compose up -d --wait "${DONO[$comp]}" >/dev/null
      confere "$comp" "$comp\\.(objetos|versoes)" "$(minio_contagens "${DONO[$comp]}" "$comp")"
      ;;
    n8n)
      restaura_volume n8n
      confere n8n 'n8n\.workflows' "$(n8n_contagens run)"
      ;;
    langfuse_db)
      restaura_pg langfuse-postgres langfuse_db 'exec pg_restore --clean --if-exists --no-owner --single-transaction --exit-on-error -U postgres -d postgres'
      ;;
    clickhouse)
      compose up -d --wait langfuse-clickhouse >/dev/null
      compose exec -T -u 0 langfuse-clickhouse chown 101:101 /backups
      compose exec -T langfuse-clickhouse sh -c 'cat >/backups/restauracao.zip' <"$dir/${ARQUIVO[clickhouse]}"
      # O banco default inteiro (tabelas e o controle das migrations do Langfuse) vem do backup.
      ch "DROP DATABASE IF EXISTS default SYNC" >/dev/null
      ch "RESTORE DATABASE default FROM File('/backups/restauracao.zip')" >/dev/null
      compose exec -T langfuse-clickhouse rm -f /backups/restauracao.zip
      confere clickhouse 'clickhouse\.[^ ]+' "$(clickhouse_contagens)"
      ;;
  esac
done

if [ ${#divergencias[@]} -gt 0 ]; then
  falha "contagens diferentes em: ${divergencias[*]}. O app, o worker, o n8n e o Langfuse continuam parados; confira antes de subir com 'docker compose -f $COMPOSE_FILE --env-file $ENV_FILE up -d'"
fi

log "subindo todos os serviços"
compose up -d --wait >/dev/null || falha "os serviços não ficaram saudáveis: veja 'docker compose -f $COMPOSE_FILE --env-file $ENV_FILE ps'"
log "restauração concluída a partir de $dir${manifest:+ (todas as contagens conferem)}"
