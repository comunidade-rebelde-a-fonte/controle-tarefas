# 01 — Intake: Consolidação Controle de Tarefas + Fator R + Conciliação Contábil

**Status:** pronto para especificação, com uma pendência visual não bloqueante para domínio/backend  
**Data:** 2026-10-07  
**Repositório-alvo:** `comunidade-rebelde-a-fonte/controle-tarefas`  
**Agente-alvo:** Claude Code  
**Usuário operacional:** único usuário local

## Solicitação original

Consolidar três projetos existentes em um único sistema:

1. `controle-tarefas` — base do produto;
2. `fator-r` — módulo integral de monitoramento e consultoria de Fator R;
3. `conciliador-contabil` — módulo integral de conciliação contábil agentic.

A aplicação final deve viver no repositório `controle-tarefas`. Quando houver tecnologias conflitantes de aplicação, prevalece a stack do `controle-tarefas`. A consolidação não deve remover ou simplificar capacidades dos sistemas de origem: deve preservar seus comportamentos, regras, fluxos, validações, filas, auditoria e serviços auxiliares, adaptando somente o necessário para coexistirem como um único produto.

## Fontes de verdade analisadas

- `https://github.com/comunidade-rebelde-a-fonte/controle-tarefas`
  - `README.md`
  - `docs/PRD.md`
  - `docs/plan.md`
  - `docs/decisoes.md`
  - `AGENTS.md`
  - `backend/go.mod`
  - `frontend/package.json`
  - `docker-compose.yml`
- `https://github.com/comunidade-rebelde-a-fonte/fator-r`
  - `README.md`
  - `docs/prd.md`
  - `docs/plan.md`
  - `CLAUDE.md`
  - `docker-compose.yml`
  - `.env.example`
  - `infra/langfuse/*`
- `https://github.com/comunidade-rebelde-a-fonte/conciliador-contabil`
  - `README.md`
  - `docs/prd.md`
  - `docs/plan.md`
  - `AGENTS.md`
  - `docker-compose.yml`
  - `.env.example`
  - `litellm/config.yaml`

## Classificação da demanda

**Tipo principal:**
- Sistema completo por consolidação de sistemas existentes.

**Tipos secundários:**
- Refactor arquitetural.
- Migração de módulos para nova stack de aplicação.
- Módulo de dados.
- Módulo de UI.
- Automação com IA.
- Infraestrutura/deploy local.
- Teste/QA.
- Análise de código existente.

**Justificativa:** o produto final preserva três domínios existentes, unifica cadastro, navegação e fechamento, migra dois módulos para Go/React e mantém componentes auxiliares de IA/storage/orquestração já presentes.

## Entendimento confirmado

- [CONFIRMED] `controle-tarefas` é o produto e repositório-base.
- [CONFIRMED] Stack principal: Go 1.26+, React 19 + TypeScript + Vite, PostgreSQL 16, OpenAPI 3.1, sqlc/goose, Docker Compose.
- [CONFIRMED] O Fator R entra com 100% da v1 funcional especificada no seu projeto de origem.
- [CONFIRMED] O Conciliador entra com 100% das fases/capacidades já especificadas e implementadas no seu projeto de origem.
- [CONFIRMED] O cadastro de empresas do `controle-tarefas` é a entidade canônica para todos os módulos.
- [CONFIRMED] Não haverá autenticação/login na aplicação principal; existe um único operador local.
- [CONFIRMED] O sistema roda somente na máquina do usuário.
- [CONFIRMED] Os três módulos devem se comunicar, não apenas coexistir.
- [CONFIRMED] Dashboard será consolidado.
- [CONFIRMED] Ficha da empresa será o hub central.
- [CONFIRMED] Não há migração de dados reais dos bancos dos projetos auxiliares; os dados atuais são fictícios.
- [CONFIRMED] A demonstração parte das empresas do seed do `controle-tarefas`.
- [CONFIRMED] IA interpreta/sugere; código determinístico valida, calcula e persiste.
- [CONFIRMED] Upload operacional da demonstração é manual pela UI.
- [CONFIRMED] Processamento demorado de documentos continua assíncrono, com fila/worker como já previsto.
- [CONFIRMED] Serviços auxiliares existentes não devem ser removidos apenas para simplificar a consolidação.
- [CONFIRMED] LiteLLM permanece e será a camada configurável de modelos.
- [CONFIRMED] Provedor inicial de IA: OpenAI; os modelos devem ser configuráveis sem alterar código.
- [CONFIRMED] Langfuse self-hosted permanece como observabilidade de IA.
- [CONFIRMED] MinIO permanece para o fluxo do Conciliador conforme seu funcionamento atual.
- [CONFIRMED] n8n permanece disponível conforme o sistema atual, ainda que o fluxo demonstrado seja upload manual.
- [CONFIRMED] O storage local já usado pelo `controle-tarefas` permanece para os documentos que já usam esse mecanismo.
- [CONFIRMED] Exportação do razão do Conciliador será genérica XLSX/CSV; adaptador de ERP específico fica fora da entrega.
- [CONFIRMED] Fator R não muda automaticamente status de obrigação fiscal.
- [CONFIRMED] Fator R em vermelho/amarelo entra no dashboard e ficha da empresa.
- [CONFIRMED] Ao concluir/reabrir a conciliação, a obrigação canônica `Conciliação` do módulo Contábil deve refletir esse estado automaticamente.
- [CONFIRMED] O fechamento canônico é o fechamento do módulo Contábil do `controle-tarefas`; ele trava a conciliação daquela competência.
- [CONFIRMED] O alvo de execução é somente Claude Code.
- [CONFIRMED] Claude Code pode criar migrations, adicionar dependências necessárias, alterar Docker Compose, criar módulos Go/React, modificar OpenAPI, rodar geração/lint/testes/E2E e adaptar fixtures.
- [CONFIRMED] `db-reset`, exclusões, migrations destrutivas e operações Git remotas exigem aprovação humana explícita.

## Regra soberana de consolidação

[DECISION] **Preservar comportamento antes de simplificar arquitetura.**

Ao portar Fator R e Conciliação para a stack da aplicação-base:

1. não remover regra de negócio;
2. não reduzir estados;
3. não eliminar auditoria;
4. não eliminar fila/worker;
5. não eliminar storage ou serviço auxiliar cujo comportamento atual dependa dele;
6. não substituir validação determinística por LLM;
7. não considerar a migração concluída sem teste de equivalência funcional.

## Escopo

### Incluído

- Todo o `controle-tarefas` atual, sem regressões.
- Todo o Fator R v1 especificado.
- Todo o Conciliador especificado até seu fechamento/exportação.
- Cadastro canônico único de empresas.
- Dashboard e ficha de empresa integrados.
- Menu único e navegação única.
- Configuração de modelos de IA pela interface.
- LiteLLM, Langfuse, MinIO e n8n preservados conforme aplicabilidade.
- Workers e jobs assíncronos.
- Seeds/fixtures fictícios integrados.
- Exportações XLSX/CSV do razão.
- Testes unitários, integração, E2E, golden/evals e regressão.
- Documentação e contrato operacional para Claude Code.

### Fora de escopo

- [OUT-OF-SCOPE] SaaS/multi-tenant real.
- [OUT-OF-SCOPE] Login da aplicação principal e múltiplos usuários.
- [OUT-OF-SCOPE] Portal do cliente.
- [OUT-OF-SCOPE] App mobile dedicado.
- [OUT-OF-SCOPE] Migração de dados reais dos bancos legados.
- [OUT-OF-SCOPE] Adaptador de ERP específico.
- [OUT-OF-SCOPE] Integração nativa com eSocial/NFS-e/PGDAS para transmissão.
- [OUT-OF-SCOPE] Automação de ingestão como requisito da demonstração; upload manual é suficiente. O n8n continua disponível.
- [OUT-OF-SCOPE] Remoção de serviços auxiliares apenas por simplificação.

## Pendências e lacunas

### Q-001 — Design System InovaSala

**Nível:** importante para UI final, não bloqueante para domínio/backend.  
**Status:** `[PENDING]`.

O usuário informou que o design system InovaSala estava anexado, mas nenhum arquivo está disponível no contexto da conversa. A especificação não inventa cores, tipografia, tokens, grids ou componentes.

**Regra para execução:** antes de finalizar styling/visual QA, Claude Code deve localizar o material InovaSala fornecido pelo usuário. Se ele não estiver no repositório ou contexto, parar somente a frente visual e pedir o arquivo. Estrutura de informação, rotas e comportamento podem avançar.

### Q-002 — Decisões pendentes herdadas do Controle de Tarefas

O arquivo `docs/decisoes.md` do projeto-base ainda contém perguntas operacionais sobre vencimentos, fechamento Fiscal, ICMS Simples, edição de período fechado, folha sem movimento, matriz de aplicabilidade, feriados e competência inicial.

**Status:** `[PENDING]` herdado.  
**Regra:** a consolidação não deve inventar respostas. Manter os valores/provisões e mecanismos atuais, preservando as pendências como tais.

### Q-003 — Modelo OpenAI padrão por alias

**Status:** `[PENDING]` não bloqueante.  
Os aliases e a capacidade de configuração são requisitos. O nome exato do modelo inicial não deve ser hardcoded nesta especificação; deve vir de `.env`/configuração e poder ser alterado pela tela de IA.

## Riscos iniciais

- RISK-001 — regressão no `controle-tarefas` ao expandir o monólito modular.
- RISK-002 — perda silenciosa de regra ao portar Python para Go.
- RISK-003 — unificação indevida de conceitos parecidos, mas semanticamente distintos, entre os três projetos.
- RISK-004 — tentativa de remover serviços auxiliares por “simplificação”.
- RISK-005 — divergência entre rastreio local e Langfuse.
- RISK-006 — execução de LLM em testes não determinísticos/caros.
- RISK-007 — configuração visual incompleta enquanto o design system InovaSala não estiver disponível.
- RISK-008 — Docker Compose local ficar pesado devido a Langfuse/ClickHouse/Redis/MinIO; mitigar com perfis/comandos, sem remover capacidades.

## Critério de saída da descoberta

A descoberta está encerrada para produto, arquitetura, dados, IA e operação. A única pendência material é o arquivo visual InovaSala, tratado como gate específico da camada de styling.
