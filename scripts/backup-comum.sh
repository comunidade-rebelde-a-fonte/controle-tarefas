# Funções comuns do scripts/backup.sh, do scripts/restore.sh e do scripts/testar-restore.sh
# (TASK-031, D-044). Não é executável: os scripts o leem com ". scripts/backup-comum.sh", a partir
# da raiz do repositório.
#
# Variáveis: ENV_FILE (padrão .env.prod), COMPOSE_FILE (padrão docker-compose.prod.yml),
# COMPOSE_OVERRIDE (um arquivo compose adicional, opcional) e COMPOSE_PROJECT_NAME (opcional, lida
# pelo próprio docker compose).
#
# Credenciais: nenhuma passa pela linha de comando do host. Cada comando roda dentro do container
# do serviço e usa as variáveis de ambiente dele.

# Git Bash no Windows (desenvolvimento e o teste da TASK-031): os caminhos dos containers
# (/volumes, /backups...) vão como estão; os do host passam pelo hostpath.
export MSYS_NO_PATHCONV=1 MSYS2_ARG_CONV_EXCL='*'

ENV_FILE=${ENV_FILE:-.env.prod}
COMPOSE_FILE=${COMPOSE_FILE:-docker-compose.prod.yml}
COMPOSE_OVERRIDE=${COMPOSE_OVERRIDE:-}

# O ENV_FILE é a única fonte da configuração: no compose, uma variável de mesmo nome no ambiente
# (um shell com o .env de dev carregado, o make) passaria na frente dele. As variáveis do modelo
# saem do ambiente destes scripts (o shell de quem chamou não muda).
while IFS= read -r nome; do unset "$nome"; done < <(sed -n 's/^\([A-Z][A-Z0-9_]*\)=.*/\1/p' .env.prod.example)

# Os armazenamentos, na ordem do backup e da restauração. O prefixo de cada um é o das chaves do
# MANIFEST; o arquivo é o que ele gera no diretório do backup.
COMPONENTES=(app_db storage minio n8n langfuse_db clickhouse langfuse_minio)
declare -A ARQUIVO=(
  [app_db]=app.dump
  [storage]=storage.tar.gz
  [minio]=minio.tar.gz
  [n8n]=n8n.tar.gz
  [langfuse_db]=langfuse-postgres.dump
  [clickhouse]=langfuse-clickhouse.zip
  [langfuse_minio]=langfuse-minio.tar.gz
)
# Volumes copiados com tar (montados no serviço "backup" em /volumes/<pasta>) e o serviço dono de
# cada um, pausado durante a cópia e parado durante a restauração.
declare -A PASTA=([storage]=storage [minio]=minio [n8n]=n8n [langfuse_minio]=langfuse-minio)
declare -A DONO=([minio]=minio [n8n]=n8n [langfuse_minio]=langfuse-minio)

log() {
  local nome=${0##*/}
  printf '[%s %s] %s\n' "${nome##*\\}" "$(date +%H:%M:%S)" "$*" >&2
}
falha() {
  log "ERRO: $*"
  exit 1
}

# hostpath converte um caminho do host para o docker do Windows; no Linux, devolve o mesmo.
hostpath() {
  if command -v cygpath >/dev/null 2>&1; then cygpath -m "$1"; else printf '%s' "$1"; fi
}

# compose: o docker compose da instalação, sem as barras de progresso (os erros continuam saindo).
compose() {
  local args=(-f "$(hostpath "$COMPOSE_FILE")")
  if [ -n "$COMPOSE_OVERRIDE" ]; then args+=(-f "$(hostpath "$COMPOSE_OVERRIDE")"); fi
  docker compose --progress quiet "${args[@]}" --env-file "$(hostpath "$ENV_FILE")" "$@"
}

# env_valor <NOME> <padrão>: o valor de uma variável do ENV_FILE, lido sem executar o arquivo.
env_valor() {
  local v
  v=$(sed -n "s/^$1=\\(.*\\)$/\\1/p" "$ENV_FILE" 2>/dev/null | tr -d '\r' | tail -1)
  printf '%s' "${v:-$2}"
}

soma_sha256() {
  if command -v sha256sum >/dev/null 2>&1; then sha256sum "$@"; else shasum -a 256 "$@"; fi
}

# ferramenta <comando>: roda um comando no serviço "backup" (bash do Debian, tar GNU, sem rede),
# com os volumes de arquivos em /volumes.
ferramenta() { compose run --rm --no-deps -T backup "$1"; }

# --- Contagens: uma linha "chave valor" por item, com a chave prefixada pelo armazenamento ---

# psql_em <serviço>: psql no banco do serviço, lendo o SQL da entrada; saída "a b" sem cabeçalho.
psql_em() {
  case $1 in
    db) compose exec -T db sh -c 'exec psql -X -At -F " " -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB"' ;;
    langfuse-postgres) compose exec -T langfuse-postgres psql -X -At -F " " -v ON_ERROR_STOP=1 -U postgres -d postgres ;;
    *) falha "psql_em: serviço desconhecido $1" ;;
  esac
}

# pg_contagens_dump <serviço> <prefixo> <arquivo.dump>: as linhas de cada tabela, lidas do próprio
# dump. O pg_restore do serviço converte o dump em SQL; cada linha de um COPY é uma linha da tabela.
pg_contagens_dump() {
  compose exec -T "$1" pg_restore --data-only -f - <"$3" |
    awk -v p="$2" '/^COPY / { t = $2; n = 0; dentro = 1; next }
      dentro && $0 == "\\." { print p "." t, n; dentro = 0; next }
      dentro { n++ }'
}

# pg_contagens_vivo <serviço> <prefixo>: conta, no banco em uso, as tabelas das chaves
# "<prefixo>.<tabela>" lidas da entrada (as linhas do MANIFEST).
pg_contagens_vivo() {
  local svc=$1 prefixo=$2 t sql=""
  for t in $(sed -n "s/^${prefixo}\\.\\([^ ]*\\) [0-9]*$/\\1/p"); do
    sql+="${sql:+ UNION ALL }SELECT '$t', count(*) FROM $t"
  done
  [ -n "$sql" ] || return 0
  printf '%s;\n' "$sql" | psql_em "$svc" | tr -d '\r' | sed "s/^/${prefixo}./" | sort
}

# minio_contagens <serviço> <prefixo>: os objetos (versão atual) e as versões (todas, inclusive as
# marcas de exclusão) de todos os buckets, pelo mc do container com o administrador dele.
minio_contagens() {
  local mc='MC_HOST_local="http://$MINIO_ROOT_USER:$MINIO_ROOT_PASSWORD@127.0.0.1:9000" mc ls --recursive --json'
  local atuais todas
  atuais=$(compose exec -T "$1" sh -c "$mc local") || return 1
  todas=$(compose exec -T "$1" sh -c "$mc --versions local") || return 1
  printf '%s.objetos %s\n%s.versoes %s\n' "$2" "$(conta_linhas '"type":"file"' "$atuais")" \
    "$2" "$(conta_linhas '"type":"file"' "$todas")"
}

# n8n_contagens [run]: os workflows do n8n (lista do próprio CLI). Com "run", num container avulso
# do serviço, sem subir o n8n (para conferir a restauração com ele parado).
n8n_contagens() {
  local lista
  if [ "${1:-}" = run ]; then
    lista=$(compose run --rm --no-deps -T n8n list:workflow 2>/dev/null) || return 1
  else
    lista=$(compose exec -T n8n n8n list:workflow 2>/dev/null) || return 1
  fi
  printf 'n8n.workflows %s\n' "$(conta_linhas '^[A-Za-z0-9]+\|' "$lista")"
}

# ch <consulta>: clickhouse-client no ClickHouse do Langfuse, com a senha da variável do container.
# As tabelas de eventos do Langfuse têm índice de texto: o RESTORE só as recria com ele ligado
# (como no testar_restore.sh da origem).
ch() {
  compose exec -T langfuse-clickhouse sh -c 'exec clickhouse-client --user "$CLICKHOUSE_USER" --database system --enable_full_text_index=1 --query "$1"' _ "$1"
}

# clickhouse_por_tabela <prefixo> <expressão>: a expressão (count(), um hash...) em cada tabela
# MergeTree do banco default. As tabelas que juntam versões de uma linha (Replacing,
# Aggregating...) são lidas com FINAL, para o resultado não mudar quando o ClickHouse funde as
# partes. As tabelas internas das views materializadas ficam de fora (o nome delas muda a cada
# restauração).
clickhouse_por_tabela() {
  local prefixo=$1 expr=$2 nome engine sql=""
  while IFS=$'\t' read -r nome engine; do
    [ -n "$nome" ] || continue
    local final=""
    case $engine in
      *Replacing* | *Collapsing* | *Aggregating* | *Summing* | *Coalescing* | *Graphite*) final=" FINAL" ;;
    esac
    sql+="${sql:+ UNION ALL }SELECT '$prefixo.$nome', $expr FROM default.\`$nome\`$final"
  done < <(ch "SELECT name, engine FROM system.tables WHERE database = 'default' AND engine LIKE '%MergeTree' AND NOT startsWith(name, '.inner') ORDER BY name FORMAT TSV" | tr -d '\r')
  [ -n "$sql" ] || return 0
  # O UNION ALL do ClickHouse não garante a ordem das linhas.
  ch "$sql FORMAT TSV" | tr -d '\r' | tr '\t' ' ' | sort
}

# clickhouse_contagens: as linhas de cada tabela do ClickHouse do Langfuse.
clickhouse_contagens() { clickhouse_por_tabela clickhouse 'count()'; }

# conta_linhas <regex> <texto>: quantas linhas do texto casam com a regex (zero sem nenhuma).
conta_linhas() { printf '%s\n' "$2" | grep -cE -- "$1" || true; }

# arquivos_tar <arquivo.tar.gz>: quantos arquivos comuns o tar guarda.
arquivos_tar() {
  local lista
  lista=$(tar -tvzf "$1") || return 1
  conta_linhas '^-' "$lista"
}

# arquivos_volume <componente>: quantos arquivos comuns o volume do componente tem agora.
arquivos_volume() { ferramenta "find /volumes/${PASTA[$1]} -type f | wc -l" | tr -d '\r '; }
