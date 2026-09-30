# Prompt 01 — Iniciar o projeto (preparação + marco M0)

Você vai iniciar a implementação do **Sistema de Controle de Obrigações para Escritório Contábil**. Esta é a primeira sessão de execução. Até agora o repositório contém apenas documentação.

## 1. Leitura obrigatória (antes de qualquer ação)

Leia **por inteiro**:
1. `AGENTS.md`: o contrato. Ele vale acima de qualquer instinto seu sobre como fazer as coisas.
2. `docs/tasks.md`: seções 1 a 3 e todas as tarefas de M0.
3. `docs/plan.md`: seções 1 a 4 (visão geral, decisões, estrutura do repositório, modelo de dados).
4. `docs/PRD.md`: seções 1 a 6 e 42 a 45 (objetivo, escopo, stack, organização, requisitos não funcionais). As demais seções serão consultadas por tarefa.

Ao terminar, escreva em no máximo 10 linhas o que entendeu:
- objetivo;
- stack;
- regra de ouro de cada documento;
- o que você **não** pode fazer.

Depois siga em frente sem esperar resposta.

## 2. Verificar o ambiente (somente leitura)

Rode e registre as versões:

```bash
git status
git branch -a
go version          # exigido: 1.23+
node --version      # exigido: 22+
npm --version
docker --version
docker compose version
docker info >/dev/null && echo "docker ok"
lsof -nP -iTCP:5432 -sTCP:LISTEN   # porta do Postgres já ocupada?
lsof -nP -iTCP:5173 -sTCP:LISTEN   # porta do Vite já ocupada?
```

- Se a porta 5432 estiver ocupada (outro Postgres local), use outra no `.env` (ex.: `POSTGRES_PORT=5433`) em vez de parar o serviço existente.

- Se Go, Node ou Docker estiverem **ausentes ou abaixo da versão**, **pare** e informe o que falta e como instalar. Não instale software de sistema (brew, apt etc.) sem autorização.
- Ferramentas Go do projeto (`sqlc`, `goose`, `golangci-lint`) podem ser instaladas com `go install` e versão fixada, dentro da tarefa T-M0-04 (alvo `make tools`).

## 3. Preparar o repositório

1. **Commit inicial.** Se a branch atual não tiver commits, crie o primeiro commit com a documentação existente:
   - arquivos: `AGENTS.md`, `docs/`, `prompts/` (e `CLAUDE.md`, se existir);
   - mensagem: `chore: add project documentation`.
2. **Branch principal e de integração.** Se a branch principal se chamar `master`, renomeie-a localmente para `main` (`git branch -m master main`). Depois crie `develop` a partir de `main`. Todo o trabalho parte de `develop`.
3. **Documentos de acompanhamento**, criados em `develop`:
   - `docs/pendencias.md`: tabela `| Pergunta | Tarefa | Valor provisório | Onde está no código |`, inicialmente vazia;
   - `docs/decisoes.md`: uma seção por pergunta Q1–Q8 (texto copiado de `docs/plan.md` §9), cada uma com `**Status:** sem resposta`. **Não preencha respostas**: elas vêm do escritório (prompt 04).

   Commit: `chore: add tracking documents`.

## 4. Executar o marco M0

Execute **T-M0-01 até T-M0-08**, na ordem do `tasks.md`, seguindo **exatamente** o ciclo do AGENTS.md §4 para cada uma:

- branch `task/T-M0-NN-slug` a partir de `develop`;
- implementar só o escopo da tarefa;
- verificar os critérios de "Aceite". Enquanto `make lint`/`make test` não existirem, use os comandos equivalentes que a própria tarefa criar;
- marcar `[x]` no `tasks.md` no mesmo commit;
- commit `T-M0-NN: <descrição em inglês>`;
- merge local em `develop` (`git merge --no-ff`);
- relatório no modelo do AGENTS.md §11.

### Cuidados específicos do M0

- **Versões:** use versões estáveis atuais das dependências da lista aprovada (AGENTS.md §6.1). Fixe-as (`go.mod`, `package-lock.json`, versões de ferramentas no `Makefile`). Na dúvida sobre uma API de biblioteca, consulte a documentação oficial.
- **Docker e banco:** siga à risca T-M0-02, T-M0-03, T-M0-05 e T-M0-07. Em resumo:
  - imagem `postgres:16.x` Debian, com ICU pt-BR e UTC;
  - bancos `controle`, `controle_e2e` e `controle_perf`;
  - `make dev` sobe db + api + web;
  - o E2E nunca usa o banco de dev;
  - depois da T-M0-02, crie o `.env` local com `cp .env.example .env`, ajustando `POSTGRES_PORT` se a 5432 estiver ocupada. O `.env` **nunca** é commitado;
  - comprove cada bullet de "Aceite" da T-M0-02 executando os comandos (listagem dos 3 bancos, `SHOW timezone`, teste de ordenação com acentos) e inclua as saídas no relatório;
  - nunca rode `docker system prune` nem `docker volume prune`: a máquina tem outros projetos. Para recomeçar, use só `make db-reset`.
- **`Makefile`:** precisa terminar M0 com os alvos `help`, `tools`, `dev`, `db-up`, `db-down`, `db-reset`, `db-psql`, `lint`, `test`, `e2e`, `migrate`, `migrate-down`, `seed`, `seed-demo`, `gen` funcionando ou, quando dependerem de tarefas futuras, com uma mensagem clara de "disponível a partir de T-…".
- **CI (T-M0-08):** o workflow deve ser criado e validado localmente, com os mesmos comandos. **Não faça push** para testá-lo; informe que a validação no GitHub depende de push autorizado.
- **Nada de domínio no M0:** nenhuma tabela de negócio, nenhuma tela real. Só o esqueleto.

## 5. Encerramento da sessão

Ao concluir T-M0-08 (ou se precisar parar antes):
1. Rode a verificação completa do marco conforme `prompts/03-fechar-marco.md`, com `MARCO: M0`.
2. Entregue:
   - lista de tarefas concluídas;
   - versões instaladas;
   - comandos que o humano pode rodar para ver o projeto de pé:
     - `cp .env.example .env`;
     - `make tools`;
     - `make dev`, com a SPA em http://localhost:5173 e a API em http://localhost:<HTTP_PORT>/api/v1/health;
     - `make db-psql`;
     - `make test`;
   - estado do Docker (`docker compose ps`) com os 3 serviços healthy;
   - pendências;
   - próximo passo: `prompts/02-executar-tarefas.md` para iniciar o M1.

Se qualquer situação do AGENTS.md §9 ocorrer, **pare e escale** no formato indicado lá.
