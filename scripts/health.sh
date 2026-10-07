#!/usr/bin/env bash
# Saúde dos serviços locais [spec 11 §8, TASK-003]: o healthcheck de cada container do
# docker-compose.yml e do docker-compose.langfuse.yml, mais o /api/v1/health da API.
#
#   scripts/health.sh            (ou make health)
#
# Sai com erro se algum serviço obrigatório não estiver saudável. n8n e Langfuse são
# informativos: indisponibilidade deles não bloqueia o domínio [spec 11 §8, AC-030].
set -uo pipefail

cd "$(dirname "$0")/.."
REQUIRED=(db api web worker minio litellm)
HTTP_PORT=${HTTP_PORT:-$(sed -n 's/^HTTP_PORT=\([0-9][0-9]*\)$/\1/p' .env 2>/dev/null | tail -1)}
HTTP_PORT=${HTTP_PORT:-8080}
failed=0

# report <título> <comando compose...>: uma linha por serviço com o estado do healthcheck.
report() {
  local title=$1
  shift
  echo "$title"
  local lines
  if ! lines=$("$@" ps --all --format '{{.Service}}|{{.State}}|{{.Health}}|{{.ExitCode}}' 2>/dev/null); then
    echo "  (não configurado: confira o .env)"
    return
  fi
  if [ -z "$lines" ]; then
    echo "  (parado)"
    return
  fi
  local service state health code status
  while IFS='|' read -r service state health code; do
    if [ "$state" != "running" ]; then
      status="parado ($state, saída $code)"
    elif [ -n "$health" ]; then
      status=$health
    else
      status=$state
    fi
    printf '  %-18s %s\n' "$service" "$status"
  done <<<"$lines"
}

report "Aplicação (docker-compose.yml)" docker compose
for service in "${REQUIRED[@]}"; do
  health=$(docker compose ps --format '{{.Health}}' "$service" 2>/dev/null)
  if [ "$health" != "healthy" ]; then
    echo "  !! obrigatório fora do ar ou não saudável: $service"
    failed=1
  fi
done

echo
report "Langfuse (docker-compose.langfuse.yml, informativo)" docker compose -f docker-compose.langfuse.yml

echo
echo "API: GET http://localhost:$HTTP_PORT/api/v1/health"
if body=$(curl -fsS --max-time 5 "http://localhost:$HTTP_PORT/api/v1/health" 2>&1); then
  echo "  $body"
else
  echo "  !! sem resposta: $body"
  failed=1
fi

exit "$failed"
