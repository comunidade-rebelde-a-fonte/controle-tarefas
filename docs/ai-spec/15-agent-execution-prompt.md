# 15 — Claude Code Execution Prompt

> Alvo único: Claude Code. Codex está explicitamente fora do escopo desta execução e não deve orientar a estrutura operacional.

## Prompt mestre

Leia primeiro `CLAUDE.md` e depois `docs/ai-spec/00-index.md`.

Use modo de planejamento antes de editar.

### Objetivo

Consolidar integralmente os projetos `controle-tarefas`, `fator-r` e `conciliador-contabil` dentro do repositório `controle-tarefas`, mantendo o Controle de Tarefas como base Go/React/PostgreSQL e preservando os comportamentos e serviços auxiliares dos outros dois sistemas.

### Regra soberana

Não simplifique removendo capacidade. “Juntar” significa portar e integrar, não redesenhar funcionalidades.

### Antes de editar

1. Leia:
   - `docs/ai-spec/01-intake.md`
   - `docs/ai-spec/02-prd.md`
   - `docs/ai-spec/03-acceptance-criteria.md`
   - `docs/ai-spec/04-system-design.md`
   - `docs/ai-spec/05-data-model.md`
   - `docs/ai-spec/06-api-contract.md`
   - `docs/ai-spec/07-flows.md`
   - `docs/ai-spec/08-ui-ux-spec.md`
   - `docs/ai-spec/09-security-privacy.md`
   - `docs/ai-spec/10-env-and-secrets.md`
   - `docs/ai-spec/11-observability.md`
   - `docs/ai-spec/13-implementation-plan.md`
2. Leia os PRDs/planos/contratos dos três repositórios fonte indicados no intake.
3. Explore o código real antes de propor caminhos/arquivos.
4. Selecione apenas a próxima TASK cuja dependência esteja satisfeita.
5. Liste critérios de aceite e testes dessa TASK.

### Durante

- Trabalhe por uma vertical slice de cada vez.
- Preserve a suíte do `controle-tarefas` verde.
- Porte regra crítica com teste de equivalência/golden.
- Use Go/React para a aplicação, mas mantenha MinIO, LiteLLM, n8n e Langfuse conforme a spec.
- Não crie cadastro de empresa paralelo.
- Não crie fechamento Contábil paralelo.
- Não use LLM para cálculo/validação final.
- Atualize OpenAPI e tipos TypeScript quando contrato mudar.
- Atualize docs/ADR quando decisão estrutural aprovada mudar.

### Autonomia autorizada

Você pode, sem pedir aprovação prévia:
- criar migrations aditivas;
- adicionar dependências necessárias e justificadas;
- alterar Docker Compose/Makefile;
- criar módulos Go e features React;
- modificar OpenAPI;
- gerar tipos/código;
- executar lint, testes, build e E2E;
- criar/adaptar fixtures fictícias;
- refatorar internamente quando não muda comportamento contratado.

### Pare e peça aprovação se

- precisar executar `db-reset` em base não explicitamente descartável;
- precisar apagar dados, arquivos ou volumes;
- precisar criar migration destrutiva/irreversível;
- precisar fazer qualquer operação Git remota;
- precisar mudar fórmula, limiar ou regra tributária;
- precisar remover MinIO, LiteLLM, n8n ou Langfuse;
- precisar relaxar portão de validação ou human-in-the-loop;
- o Design System InovaSala não estiver disponível no momento de concluir styling visual;
- houver conflito material entre documentos que esta spec não resolve.

### Validação por tarefa

1. Rode os testes mais próximos da mudança.
2. Rode lint/typecheck apropriados.
3. Rode integração com Postgres real quando houver data model/API.
4. Para regra portada, rode golden/equivalência.
5. Antes de fechar marco, rode a suíte completa aplicável.

### Ao final de cada TASK

Relate:
- TASK executada;
- requisitos/AC cobertos;
- arquivos principais alterados;
- migrations adicionadas;
- comandos executados;
- testes que passaram/falharam;
- evidências/golden;
- riscos/pendências;
- próxima TASK elegível.

Não faça commit, push, PR, merge ou tag remota salvo instrução explícita do usuário.
