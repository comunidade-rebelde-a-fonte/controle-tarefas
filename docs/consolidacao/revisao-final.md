# Revisão final da consolidação (TASK-030, TEST-030)

13/10/2026. Escopo enxuto, por pedido do usuário no mesmo dia: o produto é uma demonstração para alunos, e o foco é ele funcionar de ponta a ponta. A revisão percorre o checklist da spec (`docs/ai-spec/14-review-checklist.md`) por bloco, apontando a evidência. As TASKs já validadas uma a uma não foram reauditadas.

## 1. Rodada final de validação

| Validação | Resultado |
|---|---|
| `make lint` | verde: golangci-lint sem nenhum apontamento, ESLint, Prettier e a checagem de tipos do frontend e do E2E |
| `make test` | verde: 66 pacotes Go, com os goldens das origens, e 253 testes do frontend em 63 arquivos |
| `make e2e` | verde: 51 de 51 cenários, com o axe nas telas |
| `make test-langfuse` | verde: corrida enviada ao Langfuse local e conferida pela API pública |
| `make test-restore` (TEST-031) | verde: a instalação completa sobe, o backup é restaurado numa instalação vazia e tudo confere (D-044) |
| `make audit` | `npm audit` sem nenhuma vulnerabilidade (frontend e E2E). O govulncheck aponta as 11 da biblioteca padrão do Go 1.26, já conhecidas (seção 3) |
| Jornada da demonstração, no navegador | Dashboard, ficha da 111, Fator R (carteira) e a fila da Conciliação de 08/2026, com os dados do `make seed-demo` e do `make demo-fixtures` |

Os goldens do Fator R e da Conciliação rodam dentro do `make test`. Eles são gerados pelo próprio Python das origens (D-016 a D-031).

## 2. Checklist por bloco

- **Produto e escopo:** atendido.
  - Os REQ-001 a REQ-052 estão na matriz (`docs/ai-spec/16`), com a TASK-031.
  - Nenhuma capacidade foi removida: o inventário da TASK-002 entrou no escopo (D-011), com uma exceção aprovada, o agente `echo` (D-012).
  - O cadastro de empresas é único (D-039) e, por decisão, não há login.
- **Controle de Tarefas:** atendido. Os cenários do baseline continuam no `make e2e`. As pendências Q1–Q8 seguem pendentes, sem resolução por suposição.
- **Fator R:** atendido.
  - O motor é um porte fiel, travado por golden, com o corte de 28% e a meta de 30% (D-016, D-018).
  - O PGDAS-D só grava em competência vazia (D-023), e o simulador é código, sem LLM (D-024).
  - Nada do Fator R conclui obrigação do Fiscal. Corridas, traces e notas estão preservados (D-022, D-025).
- **Conciliação:** atendido.
  - Os originais ficam no MinIO versionado, e o usuário da aplicação não apaga (D-026).
  - O portão do saldo é de R$ 0,00, com as tentativas e os playbooks da origem (D-027, D-034).
  - O matching não concilia ambiguidade (D-029, ADR-008), e o resolvedor só propõe (D-030).
  - A classificação segue regra → histórico → agente, e o razão é uma projeção (D-031). Os casos de borda vão sinalizados para a fila.
- **Integração:** atendido.
  - A conclusão e a reabertura passam pelo serviço de obrigações, com histórico (D-036).
  - O fechamento do Contábil é a única autoridade e trava a Conciliação (D-037).
  - O Dashboard calcula cada número a partir das mesmas fontes das listas (D-038).
- **IA:** atendido.
  - O LiteLLM funciona com aliases, e o modelo padrão é da OpenAI (D-014, D-015).
  - O modelo é trocado pela tela, sem segredo, e a corrida guarda o modelo que resolveu (D-021).
  - O texto do LLM passa por guarda (D-024). Os testes não chamam provedor real.
- **Observabilidade:** atendido.
  - A trilha local fica no banco, e o Langfuse é self-hosted.
  - Com o Langfuse fora, o domínio segue funcionando, e o reenvio é idempotente.
  - Nenhum documento inteiro vai para o Langfuse (D-022).
- **Dados:** atendido.
  - As migrations 0001 a 0030 são aditivas.
  - O `companies.id` é a referência dos módulos (D-039).
  - O dinheiro é decimal (ADR-007), e as identidades naturais têm unicidade (D-020).
- **API:** atendido. O OpenAPI 3.1 é o contrato e os tipos do frontend são gerados dele. Os erros são estruturados, e nenhuma resposta traz segredo (D-021).
- **UI/UX:** atendido.
  - O menu é o consolidado, e a ficha da empresa é o hub (D-039, D-043).
  - A fila de revisão está pronta (D-032), e o axe roda no E2E.
  - O Design System InovaSala está aplicado, com evidência visual (AC-035, D-043).
- **Segurança:** atendido.
  - Não há segredo no repositório.
  - Os uploads são validados pelo conteúdo, e os originais não se apagam.
  - As chaves dos arquivos são o SHA-256, nunca o nome enviado.
  - Nenhuma operação destrutiva foi feita sem aprovação.
  - O compose de execução não tem segredo com valor padrão (D-044). O govulncheck tem uma pendência (seção 3).
- **Ambiente:** atendido.
  - O `make dev` sobe o Postgres, o MinIO com o bucket versionado, o LiteLLM, o n8n e o worker.
  - O Langfuse sobe pelo `make langfuse-up`.
  - A execução completa, fora do modo dev, está no `docker-compose.prod.yml` (D-044).
- **Testes:** atendidos. Veja a seção 1. As migrations rodam do zero em cada teste de integração e na instalação nova do TEST-031. O E2E usa um LiteLLM simulado.
- **Rastreabilidade:** atendida.
  - A matriz está atualizada.
  - Cada TASK tem a sua decisão (D-0xx) e a nota "Como foi implementado" no plano (`docs/ai-spec/13`).
  - As divergências aprovadas têm ADR ou decisão registrada (ADR-005 a ADR-008, D-017).

## 3. Pendências que continuam (não bloqueiam a demonstração)

- **P-007:** a ferramenta `consultar_cnpj` aguarda decisão.
- **Go 1.27.2:** o govulncheck aponta 11 vulnerabilidades da biblioteca padrão do Go 1.26. Corrigi-las exige o Go 1.27.2 e o `x/net` v0.60.0. A atualização aguarda autorização. O `npm audit` está limpo.
- **P-006:** o fator aparece com 2 casas, como na origem. É uma observação, não muda regra.
- **Herdadas das origens:**
  - Q1–Q8 do Controle (valores provisórios em `docs/pendencias.md`);
  - P-04, P-05 e P-07/T-707 do Fator R;
  - o golden com extratos reais e as questões 1, 4, 5 e 6 do PRD do Conciliador.

## 4. Veredito

Proposta: aprovado para a demonstração, com as pendências da seção 3. A decisão é do usuário.

## 5. Como rodar a demonstração

Com o Docker Desktop aberto (ele não abre sozinho com o Windows) e o PC sem entrar em suspensão durante a aula:

```bash
make dev            # sobe tudo; a primeira tela demora alguns segundos a mais
make migrate        # aplica as migrations (o ambiente de dev não aplica sozinho)
make seed-demo      # as 21 empresas fictícias, com a 111 Construtora Horizonte
make demo-fixtures  # o cenário da Conciliação na 111 e os extratos do PGDAS-D em demo/pgdas
```

A interface fica em `http://localhost:5173`. O roteiro sugerido (F-012) é:

1. Dashboard;
2. ficha da 111;
3. Fator R: a carteira e o inbox, com os arquivos de `demo/pgdas`;
4. Conciliação: a fila de 08/2026;
5. o fechamento do Contábil, recusado até a Conciliação ser concluída;
6. o histórico.
