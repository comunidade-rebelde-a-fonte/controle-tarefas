# Operação — implantação, atualização e backup

Guia de produção do sistema de controle de obrigações [T-M7-06]. A aplicação roda em dois containers:

- **`app`:** imagem única com o binário Go, que serve a interface (SPA) e a API `/api/v1`, e roda como usuário não-root;
- **`db`:** PostgreSQL 16 (Debian), acessível só pela rede interna do compose, sem porta publicada no host.

Os dados ficam em dois volumes nomeados:

- **`pgdata`:** o banco;
- **`storage`:** os documentos de procurações e certificados. Só documentos públicos; chaves privadas nunca são armazenadas [RN-018].

## 1. Requisitos

- Docker Engine 24+ com o plugin `docker compose`.
- Acesso à internet no build, para baixar as imagens oficiais fixadas no `Dockerfile` e as dependências.
- Um proxy reverso com HTTPS na frente da aplicação, se ela for acessada por outras máquinas. Por padrão a porta só é publicada em `127.0.0.1`.

## 2. Primeira implantação

```bash
git clone <repositório> /opt/controle && cd /opt/controle
cp .env.prod.example .env.prod
# Edite .env.prod: gere a senha do banco com, por exemplo, `openssl rand -hex 24`.
chmod 600 .env.prod

docker compose -f docker-compose.prod.yml --env-file .env.prod up -d --build
docker compose -f docker-compose.prod.yml --env-file .env.prod ps     # app e db "healthy"
curl -fsS http://127.0.0.1:8080/api/v1/health                           # {"status":"ok","database":"ok"}

# Dados de referência: status, catálogo de obrigações e feriados nacionais. Rodar UMA vez,
# na instalação; depois, o catálogo é mantido pela tela de Configurações.
docker compose -f docker-compose.prod.yml --env-file .env.prod exec app /app/api seed
```

No boot, a aplicação:

1. aplica as migrations pendentes (`MIGRATE_ON_BOOT=true`);
2. garante a competência corrente (UC-01);
3. agenda a verificação diária de virada de mês, às 00:05 no fuso `APP_TIMEZONE`.

Depois do seed, cadastre as empresas pela tela Empresas ou pela importação. As empresas cadastradas depois da geração entram na competência corrente na próxima execução do `ensureCurrentCompetence` [PRD §10.2]. Isso acontece na verificação diária (00:05), ao reiniciar a aplicação ou por `curl -fsS -X POST http://127.0.0.1:8080/api/v1/competencies/ensure-current`.

> Nunca rode `api seed --demo` nem `api seed --fixtures` em produção. O comando recusa carregar dados fictícios quando `APP_ENV=production`.

### Competência inicial de produção (Q8)

**Valor provisório — A VALIDAR (Q8):** o sistema entra em produção na competência corrente do primeiro boot. Competências retroativas não são geradas. Pendências de meses anteriores continuam nas planilhas até a resposta do escritório. Registrado em `docs/pendencias.md`.

## 3. Atualizar a versão

```bash
cd /opt/controle
scripts/backup.sh /var/backups/controle          # backup antes de qualquer atualização
git fetch --tags && git checkout <nova-versão>
sed -i 's/^APP_VERSION=.*/APP_VERSION=<nova-versão>/' .env.prod
docker compose -f docker-compose.prod.yml --env-file .env.prod up -d --build
docker compose -f docker-compose.prod.yml --env-file .env.prod logs --tail 50 app
```

As migrations novas rodam sozinhas no boot. As já aplicadas nunca são editadas, só acrescentadas, então atualizar não reescreve dados. Para voltar à versão anterior, restaure o backup feito antes da atualização (seção 5) e suba a versão antiga.

## 4. Backup

O `scripts/backup.sh` gera, num diretório com data e hora:

- **`db.dump`:** o banco completo, gerado com `pg_dump -Fc`;
- **`storage.tar.gz`:** os documentos de procurações e certificados;
- **`SHA256SUMS`:** as somas de verificação dos dois arquivos.

```bash
scripts/backup.sh /var/backups/controle
```

A retenção vem de `BACKUP_RETENTION_DAYS` (padrão: 14 dias). Backups mais antigos no mesmo destino são removidos a cada execução. Copie o destino para outra máquina ou mídia; um backup no mesmo disco não protege contra falha do disco.

**Agendamento no cron do host** (todo dia às 02:00):

```cron
0 2 * * * cd /opt/controle && scripts/backup.sh /var/backups/controle >> /var/log/controle-backup.log 2>&1
```

O backup é operacional, fora da interface [PRD §3.2].

## 5. Restauração

```bash
scripts/restore.sh /var/backups/controle/controle-20261001-020000
```

O script executa, em ordem:

1. confere as somas SHA-256;
2. para a aplicação;
3. restaura o banco com `pg_restore --clean`;
4. substitui os documentos pelos do backup;
5. sobe a aplicação de novo.

**Todos os dados atuais são substituídos pelos do backup.**

Em servidor novo, faça a primeira implantação (seção 2, sem o `seed`) e depois rode o `restore.sh`.

**Ensaio (T-M7-06, 30/09/2026, com as T-M7-02 e M6 já na develop):** subida do zero com o UC-01; relatório PDF e exportação XLSX funcionando na imagem final; backup, `down -v` (volumes apagados), stack recriada vazia e backup restaurado. Empresas, competências, histórico e o documento enviado conferiram (SHA-256 idêntico).

## 6. Rotação da senha do banco

```bash
NOVA=$(openssl rand -hex 24)
docker compose -f docker-compose.prod.yml --env-file .env.prod exec db \
  psql -U "$(grep ^POSTGRES_USER= .env.prod | cut -d= -f2)" -d postgres \
  -c "ALTER USER \"$(grep ^POSTGRES_USER= .env.prod | cut -d= -f2)\" PASSWORD '$NOVA'"
sed -i "s/^POSTGRES_PASSWORD=.*/POSTGRES_PASSWORD=$NOVA/" .env.prod
docker compose -f docker-compose.prod.yml --env-file .env.prod up -d app   # recria o app com a nova URL
```

O `POSTGRES_PASSWORD` do container `db` só é usado na criação do volume. Depois disso vale a senha alterada pelo `ALTER USER`.

## 7. Rotina e diagnóstico

| Ação | Comando |
|---|---|
| Estado dos serviços | `docker compose -f docker-compose.prod.yml --env-file .env.prod ps` |
| Saúde da API e do banco | `curl -fsS http://127.0.0.1:8080/api/v1/health` |
| Logs da aplicação (JSON) | `docker compose -f docker-compose.prod.yml --env-file .env.prod logs -f app` |
| Status das migrations | `docker compose -f docker-compose.prod.yml --env-file .env.prod exec app /app/api migrate status` |
| Reiniciar a aplicação | `docker compose -f docker-compose.prod.yml --env-file .env.prod restart app` |

**Garantias de segurança conferidas no ensaio:**

- o banco não tem porta publicada no host: `docker ps` mostra só `5432/tcp`, sem `0.0.0.0:5432->`, e `docker compose port db 5432` não devolve endereço;
- o container `app` roda com UID 10001 (`docker compose exec app id -u`);
- `.env.prod` e `backups/` estão fora do git;
- as respostas têm os cabeçalhos `X-Content-Type-Options: nosniff`, `Referrer-Policy: same-origin` e `X-Frame-Options: DENY`.

## 8. Desenvolvimento: dump e restauração do banco local

```bash
make db-dump                                   # backups/dev-<data>.dump (banco controle)
make db-restore FILE=backups/dev-<data>.dump   # APAGA e substitui os dados de dev
```
