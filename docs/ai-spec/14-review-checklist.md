# 14 — Review Checklist

## Produto e escopo

- [ ] REQ-001 a REQ-052 foram revisados.
- [ ] Nenhuma funcionalidade dos três sistemas foi removida por simplificação.
- [ ] Fora de escopo continua respeitado.
- [ ] Cadastro de empresa é único.
- [ ] Sem login principal foi mantido por decisão explícita.

## Controle de Tarefas

- [ ] Fluxos e telas existentes continuam funcionais.
- [ ] Pendências de `docs/decisoes.md` não foram inventadas/resolvidas automaticamente.
- [ ] Competências, grades, fechamento, backlog, relatórios e credenciais não regrediram.

## Fator R

- [ ] Fórmula e janela são determinísticas.
- [ ] 28% legal e 30% operacional preservados.
- [ ] PGDAS não sobrescreve série indevidamente.
- [ ] Simulador é código determinístico.
- [ ] Fator R não altera obrigação Fiscal automaticamente.
- [ ] Evals/traces preservados.

## Conciliação

- [ ] Originais imutáveis em MinIO.
- [ ] Tolerância do saldo = R$ 0,00.
- [ ] Máximo/estratégia de tentativas preservados.
- [ ] Playbooks preservados.
- [ ] Matching não aceita ambiguidade.
- [ ] Resolvedor apenas propõe.
- [ ] Classificação segue precedência correta.
- [ ] Razão é projeção.
- [ ] Casos de borda são sinalizados.

## Integração

- [ ] Conciliação finaliza somente a obrigação canônica correta.
- [ ] Reabertura desfaz estado de forma auditada.
- [ ] Fechamento Contábil é autoridade única.
- [ ] Demais módulos não são fechados por efeito colateral.
- [ ] Dashboard calcula KPIs a partir das fontes corretas.

## IA

- [ ] LiteLLM permanece.
- [ ] OpenAI é provider inicial configurável.
- [ ] Modelos podem ser alterados pela UI sem secret.
- [ ] Alias/modelo ficam snapshotados na corrida.
- [ ] LLM não calcula/persiste domínio crítico sem guarda.
- [ ] Testes padrão não usam LLM real.

## Observabilidade

- [ ] Trace local existe para decisão persistente de agente.
- [ ] Langfuse permanece self-hosted.
- [ ] Falha do Langfuse não quebra domínio.
- [ ] Retry de sync é idempotente.
- [ ] Conteúdo sensível não é enviado indevidamente ao Langfuse.

## Dados

- [ ] Migrations são aditivas ou aprovadas explicitamente.
- [ ] `companies.id` é FK dos módulos.
- [ ] Valores financeiros não usam float.
- [ ] Constraints/uniques cobrem identidades naturais.
- [ ] Não existe segundo fechamento Contábil.

## API

- [ ] OpenAPI atualizado.
- [ ] Tipos TS regenerados.
- [ ] Erros de domínio são estruturados.
- [ ] Secrets nunca aparecem em responses/examples.
- [ ] Endpoints existentes não foram quebrados sem necessidade.

## UI/UX

- [ ] Menu consolidado completo.
- [ ] Ficha da empresa é hub.
- [ ] Fila de revisão mantém prioridade de UX.
- [ ] Estados loading/empty/error/processing/review/closed cobertos.
- [ ] Acessibilidade preservada.
- [ ] [PENDING] Design System InovaSala real foi localizado antes do styling final.
- [ ] Nenhum token visual foi inventado em substituição ao anexo ausente.

## Segurança

- [ ] Nenhum secret real no repositório.
- [ ] Uploads validados.
- [ ] Originais não deletados.
- [ ] Paths/keys não confiam no nome do usuário.
- [ ] Logs/traces sanitizados.
- [ ] Operações destrutivas não foram executadas sem aprovação.

## Ambiente

- [ ] Postgres principal sobe saudável.
- [ ] MinIO sobe e bucket versionado está pronto.
- [ ] LiteLLM sobe e resolve aliases.
- [ ] n8n continua disponível.
- [ ] Langfuse stack sobe separadamente/conforme Makefile.
- [ ] Worker processa jobs.

## Testes

- [ ] TEST-001 a TEST-030 revisados.
- [ ] `make lint` passa.
- [ ] `make test` passa.
- [ ] `make e2e` passa.
- [ ] Migrations do zero passam.
- [ ] Goldens do Fator R passam.
- [ ] Goldens do Conciliador passam.
- [ ] Integração Langfuse dedicada passa quando executada.
- [ ] E2E não chama LLM real salvo suíte explicitamente marcada.

## Rastreabilidade

- [ ] Cada REQ tem AC/tarefa/teste ou justificativa.
- [ ] Cada AC possui evidência.
- [ ] Cada TASK declara requisitos e validação.
- [ ] Divergências aprovadas possuem ADR.

## Veredito

- [ ] Aprovado.
- [ ] Aprovado com pendência visual AC-035.
- [ ] Reprovado — listar bloqueios antes de continuar.
