# Prompt 02 — Executar as próximas tarefas

Você vai implementar tarefas do `docs/tasks.md` seguindo o contrato `AGENTS.md`.

## Parâmetros (opcionais, informados pelo humano junto com este prompt)

| Parâmetro | Exemplo | Padrão |
|---|---|---|
| `TAREFAS` | `T-M1-08` · `3 tarefas` · `até o fim do marco M1` | a próxima tarefa disponível |
| `TRILHA` | `backend` · `frontend` | ambas |

Sem parâmetros: execute **uma** tarefa e pare.

---

## Passo 0 — Situar-se (sempre, inclusive ao retomar uma sessão)

1. Releia o `AGENTS.md` inteiro. Não confie na memória de sessões anteriores.
2. Rode e analise:
   ```bash
   git status
   git branch --show-current
   git log --oneline -10 develop
   ```
3. Classifique a situação:
   - **Trabalho não commitado em uma `task/*`:** é uma tarefa interrompida. Leia o diff, confira o que falta do "Fazer" e do "Aceite" e **continue essa tarefa** antes de qualquer outra.
   - **Branch `task/*` com commits não mergeados em `develop`:** verifique se o aceite está completo. Se estiver, faça o merge; se não, continue a tarefa.
   - **Estado limpo em `develop`:** siga para o passo 1.
4. Confira o ambiente local (a partir do fim do M0):
   ```bash
   docker info >/dev/null && echo "docker ok"
   test -f .env && echo ".env ok"
   docker compose ps
   ```
   - Se o banco não estiver rodando, use `make db-up`.
   - Sem `.env`, crie-o com `cp .env.example .env`.
   - Com o Docker parado, **pare** e peça ao humano para iniciá-lo. Não tente iniciar ou instalar o Docker.
5. Confira se `docs/decisoes.md` tem respostas novas que ainda não foram aplicadas (itens respondidos que continuam em `docs/pendencias.md`). Se houver, **avise** o humano e sugira o `prompts/04-registrar-decisoes.md`, mas continue.

## Passo 1 — Escolher a tarefa

- Com `TAREFAS` contendo um ID: use esse ID e confirme que todas as dependências estão `[x]`. Se não estiverem, **pare** e liste as pendentes.
- Sem ID: pegue a **primeira tarefa `[ ]` na ordem do arquivo** cuja lista "Depende de" esteja toda `[x]`, respeitando `TRILHA` se informada. Em empate, prefira a do caminho crítico (`tasks.md` §3).
- Anuncie em uma linha: `Tarefa escolhida: T-Mx-NN — <título> (dependências OK: …)`.

## Passo 2 — Entender antes de codar

1. Leia a tarefa inteira no `tasks.md`.
2. Abra **cada referência** do campo "Ref":
   - seções do PRD;
   - decisões do `plan.md` §2 (D*/R*);
   - CAs (PRD §47) e RNs (PRD §46);
   - `docs/decisoes.md` para as perguntas Q* envolvidas.
3. Leia o código já existente que a tarefa vai tocar ou reutilizar. **Reuse** helpers, componentes e padrões existentes: o envelope de erro, `Clock`, `competence`, `cnpj`, `testutil`, `DataGrid`, `StatusCell` etc. Não duplique.
4. Escreva um **mini-plano** (5–10 linhas):
   - arquivos a criar ou alterar;
   - testes que vai escrever (com os IDs de CA);
   - pontos de atenção;
   - valores provisórios envolvidos.

## Passo 3 — Implementar

1. `git checkout develop && git checkout -b task/T-Mx-NN-slug`.
2. **Testes primeiro** quando houver regra de negócio (AGENTS.md §4.4). Nome do teste com o ID do CA: `TestCA0xx_…` / `it("CA-0xx …")`.
3. Implemente **apenas** o escopo da tarefa. Se enxergar melhoria fora do escopo, anote-a para o relatório; não a implemente.
4. **Contrato HTTP:** se ele mudou, atualize `backend/api/openapi.yaml` e rode `make gen`.
5. **Valores provisórios** (itens dependentes de Q1–Q8): marque com `-- A VALIDAR (Qn)` ou `// A VALIDAR (Qn)` e adicione a linha correspondente em `docs/pendencias.md`.
6. Lembretes que mais causam retrabalho:
   - regra de negócio **só no backend**; o frontend exibe derivados da API;
   - **nunca** editar migration commitada;
   - nada de DELETE físico em dados históricos;
   - **tempo sempre via `Clock`**;
   - SQL sempre parametrizado; nada de `any`, `@ts-ignore` ou `//nolint` sem justificativa;
   - textos de UI e mensagens ao usuário em **português**; código em **inglês**.
7. **Banco e Docker:**
   - **Migration nova:** o arquivo é `backend/migrations/NNNN_descricao.sql`, com Up e Down. Rode `make migrate` no banco de dev e depois `make migrate-down && make migrate`, para provar que o Down funciona.
   - **Seed:** precisa ser idempotente. Rode `make seed` (ou `make seed-demo`) **duas vezes** e confirme que nada duplica.
   - **Consultas manuais** só com `make db-psql` no banco local. Nunca contra outro host.
   - **E2E e carga** usam `controle_e2e` e `controle_perf`, nunca o `controle`.
   - **`docker-compose*.yml` e `Dockerfile`:**
     - imagens oficiais com versão fixada (nada de `latest`; Postgres Debian, não alpine);
     - Postgres sem porta publicada no compose de produção;
     - app de produção sem root.
   - **`make db-reset`** apaga os dados de dev: use só quando necessário e registre no relatório.
   - **Nunca** rode `docker system prune`, `docker volume prune` ou `docker rm` em containers de outros projetos.

## Passo 4 — Verificar

```bash
make lint
make test       # exige Docker rodando (testcontainers)
make e2e        # se a tarefa tocou UI ou fluxo ponta a ponta (usa o banco controle_e2e)
```

Conforme o que a tarefa tocou, rode também:

| Tocou em | Comandos extras |
|---|---|
| Migrations | `make migrate-down && make migrate` e `make migrate DB=e2e` |
| Seeds | `make seed-demo` duas vezes (idempotência) |
| Compose ou Dockerfile | `docker compose config -q` (valida o YAML) e `make dev`, com os 3 serviços healthy em `docker compose ps` |
| Dockerfile ou compose de produção | `docker compose -f docker-compose.prod.yml config -q` e build da imagem |

- Tudo precisa ficar verde. Se algo falhar e a causa não for óbvia, aplique o método do `prompts/05-destravar.md`. **Nunca** pule ou desabilite teste nem afrouxe lint.
- Para cada bullet de **"Aceite"**, registre a evidência: nome do teste, saída de comando ou passo manual reproduzível. Bullet sem evidência = tarefa não concluída.

## Passo 5 — Concluir

1. Marque `- [x] Concluída` da tarefa no `docs/tasks.md`.
2. Commit na branch da tarefa:
   ```
   T-Mx-NN: <imperative summary in English>

   - <principais mudanças>
   - tests: <TestCA…, …>
   ```
3. Integre:
   ```bash
   git checkout develop
   git merge --no-ff task/T-Mx-NN-slug
   ```
   Depois rode `make test` em `develop` e confirme que continua verde.
4. Entregue o relatório no **modelo do AGENTS.md §11**, sem omitir falhas, desvios ou valores provisórios. Informe também:
   - migrations criadas (arquivo e o que fazem);
   - se `make db-reset` foi executado, porque os dados de dev foram apagados.

## Passo 6 — Continuar ou parar

- Se `TAREFAS` pedir mais de uma, volte ao **Passo 1**.
- **Pare** quando:
  - a quantidade pedida foi atingida;
  - o marco terminou (todas as tarefas do marco `[x]`); nesse caso recomende `prompts/03-fechar-marco.md`;
  - ocorrer qualquer situação do **AGENTS.md §9**; nesse caso escale no formato indicado, **sem** marcar a tarefa como concluída;
  - o contexto da sessão estiver grande. É melhor parar após uma tarefa concluída e retomar numa nova sessão com este mesmo prompt.

Ao parar, termine com: tarefas concluídas nesta sessão, estado do git (branch atual e limpo ou não) e a próxima tarefa disponível.
