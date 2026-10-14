#!/bin/sh
# Sobe o binário do hot reload (make dev) com uma instância só por container [D-035]. Uso, no
# build.full_bin do .air.toml e do .air.worker.toml:
#
#   exec sh .air-run.sh <espera_s> <binário> [argumentos...]
#
# O air v1.67.4 perde a referência do processo quando dois rebuilds terminam juntos: sobem dois
# binários, o segundo morre com "address already in use" e o primeiro fica órfão, servindo o código
# antigo até o container reiniciar. Por isso, antes do exec, toda instância anterior do mesmo binário
# recebe SIGTERM e tem até <espera_s> segundos para encerrar de forma ordenada (a API conclui as
# requisições; o worker, o job em andamento); depois, SIGKILL. Com o exec, o processo que o air
# acompanha é o próprio binário, sem sh no meio.
set -eu

espera=$1
shift
bin=$(readlink -f "$1")
shift
set -- "$bin" "$@"

# Linha de comando exata das instâncias (o exec do fim a reproduz), escapada para o pgrep -x.
padrao=$(printf '%s' "$*" | sed 's/[][\.*^$+?(){}|]/\\&/g')
pidfile=/tmp/air-run.pid

# PIDs vivos (zumbi não conta) das instâncias anteriores: as do mesmo comando e a última que este
# script subiu, que pode estar no meio do exec. O PID do arquivo só vale se ainda for deste binário.
anteriores() {
  { pgrep -f -x -- "$padrao" || :; cat "$pidfile" 2>/dev/null || :; } | sort -u |
    while read -r pid; do
      [ -n "$pid" ] && [ "$pid" != "$$" ] || continue
      estado=$(sed -n 's/^.*) \(.\).*$/\1/p' "/proc/$pid/stat" 2>/dev/null) || continue
      [ -n "$estado" ] && [ "$estado" != Z ] || continue
      tr '\0' ' ' 2>/dev/null <"/proc/$pid/cmdline" | grep -qF -- "$bin" || continue
      echo "$pid"
    done
}

# aguarda <segundos>: espera as instâncias anteriores terminarem; falha se sobrar alguma.
aguarda() {
  fim=$(($(date +%s) + $1))
  while [ -n "$(anteriores)" ]; do
    [ "$(date +%s)" -lt "$fim" ] || return 1
    sleep 0.2
  done
}

# Um início por vez: rebuilds sobrepostos chegam aqui juntos. O lock é liberado no exec; quem vier
# depois acha este processo pelo pidfile, mesmo antes de o exec terminar.
exec 9>/tmp/air-run.lock
flock 9
pids=$(anteriores)
if [ -n "$pids" ]; then
  echo "air-run: encerrando a instância anterior (pid $(echo $pids))"
  kill -TERM $pids 2>/dev/null || :
  if ! aguarda "$espera"; then
    pids=$(anteriores)
    echo "air-run: a instância anterior não encerrou em ${espera}s; SIGKILL (pid $(echo $pids))"
    kill -KILL $pids 2>/dev/null || :
    aguarda 10 || echo "air-run: pid $(echo $(anteriores)) ainda vivo; subindo assim mesmo"
  fi
fi
echo $$ >"$pidfile"
exec "$@" 9>&-
