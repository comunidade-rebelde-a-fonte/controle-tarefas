# Operação — implantação, atualização e backup

Guia da execução local completa, fora do modo dev [T-M7-06; TASK-031, D-044]. O `docker-compose.prod.yml` sobe o produto inteiro:

| Serviço | O que faz | Porta no host |
|---|---|---|
| `app` | API `/api/v1` e interface (SPA) numa imagem única, como usuário não-root (uid 10001); aplica as migrations no boot | `APP_BIND:APP_PORT` (padrão `127.0.0.1:8080`) |
| `worker` | jobs assíncronos: PGDAS-D, Conciliação, agentes e envio ao Langfuse; mesma imagem do `app` | — |
| `db` | PostgreSQL 16 (Debian) da aplicação | — |
| `minio` e `minio-init` | originais da Conciliação, num bucket versionado; o usuário da aplicação não apaga | — |
| `litellm` | gateway dos modelos dos agentes; as chaves dos provedores existem só nele | — |
| `n8n` | ingestão automatizada pela pasta monitorada | `127.0.0.1:5678` |
| `langfuse-web`, `langfuse-worker`, `langfuse-postgres`, `langfuse-clickhouse`, `langfuse-minio`, `langfuse-redis` | observabilidade e evals dos agentes | `127.0.0.1:3100` (só a interface) |

O banco, os dois MinIO, o LiteLLM e o resto do Langfuse só existem na rede interna do compose. Nenhum segredo tem valor padrão: sem eles, o compose recusa subir.

Os dados ficam em volumes nomeados:

| Volume | Conteúdo | No backup |
|---|---|---|
| `pgdata` | o banco da aplicação | `app.dump` |
| `storage` | procurações, certificados (só a parte pública, [RN-018]) e extratos do PGDAS-D | `storage.tar.gz` |
| `minio_data` | originais da Conciliação, com todas as versões | `minio.tar.gz` |
| `n8n_data` | workflows, credenciais e execuções do n8n | `n8n.tar.gz` |
| `langfuse_postgres_data` | Langfuse: usuários, projetos, chaves e configurações | `langfuse-postgres.dump` |
| `langfuse_clickhouse_data` | Langfuse: traces, observações e notas | `langfuse-clickhouse.zip` |
| `langfuse_minio_data` | Langfuse: eventos recebidos e mídia | `langfuse-minio.tar.gz` |
| `langfuse_redis_data`, `langfuse_clickhouse_logs`, `langfuse_clickhouse_backups` | fila e cache do Langfuse, logs e a área temporária do backup do ClickHouse | fora (descartáveis) |

Nos comandos abaixo, `dcp` é um atalho para o compose de execução:

```bash
alias dcp='docker compose -f docker-compose.prod.yml --env-file .env.prod'
```

## 1. Requisitos

- Docker Engine 24+ com o plugin `docker compose` 2.20 ou mais novo.
- Acesso à internet no build e na primeira subida, para baixar as imagens fixadas no `Dockerfile` e no compose.
- Memória para o Langfuse, a parte mais pesada (ClickHouse, Postgres, Redis e MinIO próprios).
- Um proxy reverso com HTTPS na frente da aplicação, se ela for acessada por outras máquinas. Por padrão, as portas só são publicadas em `127.0.0.1`.

## 2. Primeira implantação

```bash
git clone <repositório> /opt/controle && cd /opt/controle
cp .env.prod.example .env.prod
chmod 600 .env.prod
```

Preencha os campos vazios do `.env.prod`, todos obrigatórios:

- as senhas e chaves, cada uma com `openssl rand -hex 24`;
- a `ENCRYPTION_KEY` com `openssl rand -hex 32` (64 caracteres hexadecimais);
- a `LANGFUSE_PUBLIC_KEY` como `pk-lf-` seguido de `openssl rand -hex 16`, e a `LANGFUSE_SECRET_KEY` como `sk-lf-` seguido de `openssl rand -hex 24`.

A `OPENAI_API_KEY` pode ficar vazia: sem ela, os agentes que usam modelo ficam indisponíveis e o resto do sistema funciona. Para ativá-la depois, preencha e rode `dcp up -d litellm`.

```bash
dcp up -d --build --wait                    # todos "healthy"; o minio-init termina com "exited (0)"
dcp ps
curl -fsS http://127.0.0.1:8080/api/v1/health      # {"status":"ok","database":"ok"}

# Dados de referência: status, catálogo de obrigações e feriados nacionais. Rodar UMA vez,
# na instalação; depois, o catálogo é mantido pela tela de Configurações.
dcp exec app /app/api seed
```

No boot, a aplicação:

1. aplica as migrations pendentes (`MIGRATE_ON_BOOT=true`);
2. garante a competência corrente (UC-01);
3. agenda a verificação diária de virada de mês, às 00:05 no fuso `APP_TIMEZONE`.

O worker sobe depois do app (que aplicou as migrations) e o `minio-init` cria o bucket versionado e o usuário da aplicação sem permissão de apagar. Ele roda a cada `up` e termina. Com o MinIO fora, o app sobe assim mesmo e só o envio da Conciliação falha.

Depois do seed, cadastre as empresas pela tela Empresas ou pela importação. As empresas cadastradas depois da geração entram na competência corrente na próxima execução do `ensureCurrentCompetence` [PRD §10.2]. Isso acontece na verificação diária (00:05), ao reiniciar a aplicação ou por `curl -fsS -X POST http://127.0.0.1:8080/api/v1/competencies/ensure-current`.

> Nunca rode `api seed --demo` nem `api seed --fixtures` em produção. O comando recusa carregar dados fictícios quando `APP_ENV=production`.

**Langfuse:** a interface fica em `http://127.0.0.1:3100`, com o usuário `LANGFUSE_ADMIN_EMAIL` e a senha `LANGFUSE_ADMIN_PASSWORD` (o cadastro aberto fica desligado). O projeto e as chaves do worker são criados na primeira subida. Não troque a `SALT` nem a `ENCRYPTION_KEY` depois disso: o Langfuse usa a primeira nas chaves de API e a segunda nos dados cifrados.

**n8n (opcional):** o caminho obrigatório é o envio pela tela [REQ-045]. Para a ingestão automática da pasta `N8N_INBOX_DIR`, importe e ative o workflow:

```bash
dcp exec -T n8n n8n import:workflow --input=/workflows/ingestao.json
dcp exec -T n8n n8n update:workflow --id=ConcilIngestInbx --active=true
dcp restart n8n
```

### Competência inicial de produção (Q8)

**Valor provisório — A VALIDAR (Q8):** o sistema entra em produção na competência corrente do primeiro boot. Competências retroativas não são geradas. Pendências de meses anteriores continuam nas planilhas até a resposta do escritório. Registrado em `docs/pendencias.md`.

## 3. Atualizar a versão

```bash
cd /opt/controle
scripts/backup.sh /var/backups/controle          # backup antes de qualquer atualização
git fetch --tags && git checkout <nova-versão>
sed -i 's/^APP_VERSION=.*/APP_VERSION=<nova-versão>/' .env.prod
dcp up -d --build --wait
dcp logs --tail 50 app worker
```

O app e o worker usam a mesma imagem, então sobem juntos na versão nova. As migrations novas rodam sozinhas no boot do app. As já aplicadas nunca são editadas, só acrescentadas, então atualizar não reescreve dados. Para voltar à versão anterior, restaure o backup feito antes da atualização (seção 5) e suba a versão antiga.

## 4. Backup

O `scripts/backup.sh` copia todos os armazenamentos com o sistema no ar. Ele gera `<destino>/diario/controle-AAAAMMDD-HHMMSS/` com:

| Arquivo | Conteúdo | Como é copiado |
|---|---|---|
| `app.dump` | o banco da aplicação | `pg_dump -Fc` |
| `storage.tar.gz` | os arquivos locais | tar do volume |
| `minio.tar.gz` | os originais da Conciliação, com todas as versões, as marcas de exclusão, os usuários e a configuração do bucket | tar do volume, com o MinIO pausado por alguns segundos |
| `n8n.tar.gz` | o n8n | tar do volume, com o n8n pausado |
| `langfuse-postgres.dump` | o Postgres do Langfuse | `pg_dump -Fc` |
| `langfuse-clickhouse.zip` | o ClickHouse do Langfuse | `BACKUP DATABASE` nativo |
| `langfuse-minio.tar.gz` | os eventos e a mídia do Langfuse | tar do volume, com o MinIO dele pausado |
| `MANIFEST` | as contagens de cada armazenamento no momento da cópia | — |
| `SHA256SUMS` | as somas de verificação de todos os arquivos acima | — |

```bash
scripts/backup.sh /var/backups/controle
```

As contagens do `MANIFEST` são as do conteúdo copiado:

- nos bancos, as linhas de cada tabela, lidas do próprio dump;
- nos volumes de arquivos, quantos arquivos o tar guarda;
- no MinIO, os objetos e as versões; no n8n, os workflows; no ClickHouse, as linhas de cada tabela.
  - Essas três são medidas antes e depois da cópia. Se mudaram no meio (alguém gravou), a cópia se repete, até três vezes.

Uma cópia que falha não impede as outras: o diretório sai com o sufixo `-incompleto`, o `MANIFEST` diz o que faltou e o script termina com erro.

**Rotação** (a do Fator R): ficam os 7 backups diários mais recentes, os 4 semanais (o de domingo, copiado em `semanal/`) e os 12 mensais (o do dia 1, em `mensal/`). Os números vêm de `BACKUP_KEEP_DAILY`, `BACKUP_KEEP_WEEKLY` e `BACKUP_KEEP_MONTHLY` no `.env.prod`. Os backups do formato anterior (`controle-*` direto no destino) ficam fora da rotação: apague-os quando não precisar mais deles.

Copie o destino para outra máquina ou mídia: um backup no mesmo disco não protege contra falha do disco. Os arquivos têm dados de clientes; o script cria tudo só com permissão do dono.

**Agendamento no cron do host** (todo dia às 02:00):

```cron
0 2 * * * cd /opt/controle && scripts/backup.sh /var/backups/controle >> /var/log/controle-backup.log 2>&1
```

O backup é operacional, fora da interface [PRD §3.2].

## 5. Restauração

```bash
scripts/restore.sh /var/backups/controle/diario/controle-20261001-020000
```

O script executa, em ordem:

1. confere as somas SHA-256;
2. pede confirmação (digite `sim`; para rodar sem a pergunta, `RESTORE_CONFIRM=sim`);
3. para os serviços que escrevem: app, worker, n8n e Langfuse;
4. restaura cada armazenamento presente no backup:
   - os bancos com `pg_restore --clean` numa transação só (com erro, o banco fica como estava);
   - os volumes de arquivos, trocando o conteúdo inteiro pelo do backup;
   - o ClickHouse com `RESTORE DATABASE`;
5. confere cada contagem com a do `MANIFEST`;
6. sobe todos os serviços de novo e espera ficarem saudáveis.

**Todos os dados atuais são substituídos pelos do backup.** Com alguma contagem diferente, o app, o worker, o n8n e o Langfuse ficam parados e o script termina com erro, mostrando a diferença.

Em servidor novo, faça a primeira implantação (seção 2, sem o `seed`) e depois rode o `restore.sh`, com o mesmo `.env.prod` da instalação original. Um backup do formato anterior (só `db.dump` e `storage.tar.gz`, sem `MANIFEST`) também é restaurado, sem a conferência das contagens.

**Ensaio (T-M7-06, 30/09/2026, com as T-M7-02 e M6 já na develop):** subida do zero com o UC-01; relatório PDF e exportação XLSX funcionando na imagem final; backup, `down -v` (volumes apagados), stack recriada vazia e backup restaurado. Empresas, competências, histórico e o documento enviado conferiram (SHA-256 idêntico).

**Ensaio completo (TASK-031, TEST-031):** `make test-restore` (`scripts/testar-restore.sh`) repete o ensaio com todos os serviços. Ele roda numa instalação descartável, sem porta no host e sem saída para a internet:

1. sobe a instalação com dados em todos os armazenamentos:
   - o seed de demonstração;
   - uma procuração, um extrato do PGDAS-D lido pelo worker e um extrato bancário da Conciliação, enviados pela API;
   - versões e uma marca de exclusão no bucket;
   - um workflow no n8n;
   - o trace do parser no Langfuse;
2. faz o backup e destrói a instalação (`down -v`);
3. sobe outra instalação, vazia, e restaura o backup;
4. confere, com a instalação original, cada tabela dos dois bancos, o SHA-256 de cada arquivo local, a lista de versões dos dois MinIO, os workflows do n8n e os traces do ClickHouse, além dos originais baixados pela API.

**Resultado (13/10/2026):**

- a instalação subiu com os 13 serviços saudáveis em menos de 1 minuto;
- o backup levou cerca de 30 segundos (1,3 MB e 149 contagens no `MANIFEST`);
- a restauração levou cerca de 1 minuto, com todas as contagens conferidas antes de os serviços subirem;
- tudo o que existia na instalação original estava igual na restaurada, inclusive o SHA-256 dos originais baixados pela API.

Ficam fora da comparação só as tabelas de referência que o Langfuse completa sozinho ao subir: o catálogo de modelos e preços e os painéis padrão. O `restore.sh` também as confere, antes de os serviços subirem.

## 6. Rotação da senha do banco

```bash
NOVA=$(openssl rand -hex 24)
dcp exec db psql -U "$(grep ^POSTGRES_USER= .env.prod | cut -d= -f2)" -d postgres \
  -c "ALTER USER \"$(grep ^POSTGRES_USER= .env.prod | cut -d= -f2)\" PASSWORD '$NOVA'"
sed -i "s/^POSTGRES_PASSWORD=.*/POSTGRES_PASSWORD=$NOVA/" .env.prod
dcp up -d --wait app worker   # recria o app e o worker com a nova URL
```

O `POSTGRES_PASSWORD` do container `db` só é usado na criação do volume. Depois disso, vale a senha alterada pelo `ALTER USER`. O mesmo vale para a `LF_POSTGRES_PASSWORD` do Postgres do Langfuse.

## 7. Rotina e diagnóstico

| Ação | Comando |
|---|---|
| Estado dos serviços | `dcp ps` |
| Saúde da API e do banco | `curl -fsS http://127.0.0.1:8080/api/v1/health` |
| Logs da aplicação e do worker (JSON) | `dcp logs -f app worker` |
| Status das migrations | `dcp exec app /app/api migrate status` |
| Reiniciar a aplicação | `dcp restart app worker` |
| Interface do Langfuse e do n8n | `http://127.0.0.1:3100` e `http://127.0.0.1:5678` |

**Garantias de segurança conferidas:**

- só a aplicação, o n8n e a interface do Langfuse têm porta no host, em `127.0.0.1` por padrão. O banco, os MinIO, o LiteLLM e o resto do Langfuse não têm: `dcp port db 5432` não devolve endereço;
- o `app` e o `worker` rodam com UID 10001 (`dcp exec app id -u`);
- nenhum segredo tem valor padrão no compose, e as chaves dos provedores só chegam ao LiteLLM (testes em `backend/cmd/api/deploy_test.go` e `backend/internal/ai/models_test.go`);
- `.env.prod` e `backups/` estão fora do git;
- as respostas têm os cabeçalhos `X-Content-Type-Options: nosniff`, `Referrer-Policy: same-origin` e `X-Frame-Options: DENY`.

## 8. Desenvolvimento: dump e restauração do banco local

```bash
make db-dump                                   # backups/dev-<data>.dump (banco controle)
make db-restore FILE=backups/dev-<data>.dump   # APAGA e substitui os dados de dev
```
