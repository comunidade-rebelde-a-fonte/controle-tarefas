# Prompts de execução do projeto

Prompts para conduzir a implementação do MVP com um agente (Claude Code ou equivalente). Todos partem do mesmo contrato, o `AGENTS.md`, e das mesmas fontes: `docs/PRD.md`, `docs/plan.md` e `docs/tasks.md`.

## Quais prompts existem

| Arquivo | Quando usar | Frequência |
|---|---|---|
| `01-iniciar-projeto.md` | Primeira sessão: prepara o repositório e executa o marco M0 (infraestrutura) | 1 vez |
| `02-executar-tarefas.md` | Implementar as próximas tarefas do `tasks.md`. Também serve para retomar uma sessão interrompida | Repetidamente, é o prompt principal |
| `03-fechar-marco.md` | Ao terminar todas as tarefas de um marco (M0…M7): verificação completa, revisão e relatório | 1 vez por marco |
| `04-registrar-decisoes.md` | Quando o escritório responder às perguntas Q1–Q8 (T-000) | Sempre que chegarem respostas |
| `05-destravar.md` | Quando algo quebrar: teste ou CI vermelho, bug encontrado, erro de ambiente, tarefa bloqueada | Sob demanda |

## Fluxo recomendado

```text
01-iniciar-projeto  (M0)
        │
        ▼
02-executar-tarefas ◄──────────┐   repetir até acabar o marco
        │                      │
        ├── algo quebrou? ──► 05-destravar
        │
        ▼
03-fechar-marco ───────────────┘   próximo marco (M1 … M7)

04-registrar-decisoes: a qualquer momento em que chegarem respostas do escritório
```

## Como usar

**No Claude Code**, na raiz do repositório:
- referencie o arquivo, por exemplo `Execute @prompts/02-executar-tarefas.md`;
- ou cole o conteúdo;
- parâmetros opcionais vão depois, na mesma mensagem:

  ```
  Execute @prompts/02-executar-tarefas.md
  TAREFAS: até o fim do marco M1
  ```

**Como slash command (opcional):** copie os arquivos para `.claude/commands/` (ex.: `.claude/commands/proxima-tarefa.md`) e use `/proxima-tarefa`.

**Contexto automático:** o Claude Code carrega o `CLAUDE.md`, não o `AGENTS.md`. Crie um `CLAUDE.md` na raiz contendo apenas a linha `@AGENTS.md` para que o contrato valha em toda sessão. Mesmo assim, cada prompt manda ler o `AGENTS.md` explicitamente.

## Modelo de branches usado pelos prompts

| Branch | Papel |
|---|---|
| `main` | Versões aprovadas. Merge **somente com autorização humana**, ao fim de um marco (prompt 03) |
| `develop` | Branch de integração. Recebe o merge local de cada tarefa concluída e verificada |
| `task/T-Mx-NN-slug` | Uma por tarefa, criada a partir de `develop` |

Push para remoto, PRs e merge em `main` nunca são feitos sem pedido explícito seu [AGENTS.md §6.7].

## Ambiente local (Docker + banco)

Definido nas tarefas T-M0-02, 03, 05 e 07 e no AGENTS.md §8.1. Existe a partir do fim do M0.

| Comando | O que faz |
|---|---|
| `make dev` | Sobe `db` (Postgres 16) + `api` (Go com hot reload) + `web` (Vite em http://localhost:5173) |
| `make db-up` / `make db-down` | Sobe ou para só o banco |
| `make db-reset` | **Apaga** e recria os bancos locais `controle`, `controle_e2e` e `controle_perf` |
| `make db-psql` | Abre o `psql` no banco de dev |
| `make migrate` / `make migrate-down` | Aplica ou reverte migrations (`DB=e2e` ou `DB=perf` para os outros bancos) |
| `make seed-demo` | Carrega as 20 empresas fictícias e o histórico de demonstração |
| `make test` | Testes; os de integração sobem Postgres descartável via testcontainers |
| `make e2e` | Playwright contra o banco `controle_e2e`, resetado a cada execução |

Três regras que os prompts reforçam:
- os dados do banco `controle` (dev) só são apagados por `make db-reset`, sempre com aviso no relatório;
- E2E e testes de carga nunca tocam no banco de dev;
- o agente nunca roda comandos Docker globais (`docker system prune`, `docker volume prune`), que afetariam outros projetos da sua máquina.

Pré-requisitos na máquina: Docker Desktop (ou Colima) rodando, Go 1.23+, Node 22+. Se a porta 5432 já estiver ocupada por outro Postgres, defina `POSTGRES_PORT` no `.env`.

## Dicas de sessão

- **Uma sessão por marco, ou por bloco de 3–6 tarefas.** Sessões longas demais perdem contexto; o prompt 02 retoma de onde parou lendo `tasks.md` e o `git`.
- **Revise o relatório de cada tarefa** (modelo do AGENTS.md §11). Ele informa desvios e valores provisórios.
- **Responda às escalações rapidamente.** O agente para quando encontra conflito entre documentos ou precisa de algo fora do contrato [AGENTS.md §9].
