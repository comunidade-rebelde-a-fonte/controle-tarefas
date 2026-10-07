# Baseline do Controle de Tarefas — TASK-001

Registro do estado do sistema-base **antes** de qualquer alteração da consolidação. É a referência de regressão do AC-004: a partir daqui, toda TASK compara o resultado das suítes com esta tabela.

| Item | Valor |
|---|---|
| Data da execução | 07/10/2026 |
| Repositório | `comunidade-rebelde-a-fonte/controle-tarefas` |
| Commit medido | `a8b46802fcd1f88b7908de317ed34da97696999e` (`develop` = `main`) |
| Branch de trabalho | `feat/consolidacao`, local, criada a partir de `develop` |
| Código alterado antes da medição | nenhum (só `.env` local, não versionado) |

## 1. Ambiente

Primeira execução completa do projeto em Windows. O projeto-base foi desenvolvido e validado em macOS (`docs/perf.md`), e o CI do GitHub nunca rodou (seção 6).

| Componente | Versão usada | Referência do projeto |
|---|---|---|
| Sistema | Windows 11 Home 10.0.26200, Git Bash | CI: `ubuntu-latest` |
| GNU Make | 4.4.1 (ezwinports, instalado via winget nesta sessão) | `make` exigido pelo README |
| Go | 1.27.0 | `go.mod`: 1.26.1; CI usa `go-version-file` |
| Node / npm | 24.14.0 / 11.9.0 | `engines: >=22`; CI usa Node 22 |
| Docker | 29.8.1, Docker Desktop (WSL2, kernel 6.6.87.2), 20 CPUs, 15,5 GiB para a VM | Docker Desktop ou Colima |
| Postgres (compose e testcontainers) | `postgres:16.15-bookworm` | igual |
| Ferramentas (`make tools`) | sqlc v1.31.1, goose v3.28.0, golangci-lint v2.14.0, air v1.67.4, govulncheck v1.8.0 | igual (versões fixadas) |
| Playwright / axe | @playwright/test 1.63.0 + Chromium, @axe-core/playwright 4.13.0 | igual |

Configuração local:

- `.env` copiado de `.env.example`, com uma única mudança: `HTTP_PORT=8088`, porque a 8080 está ocupada por outro processo do usuário. O README orienta trocar a porta no `.env` sem parar o outro serviço.
- O clone usa `core.autocrlf=false`, para manter LF nos scripts montados nos containers.

## 2. Resultados

Mesmos comandos do `Makefile` e do `.github/workflows/ci.yml`.

| Etapa | Comando | Resultado | Duração |
|---|---|---|---|
| Lint | `make lint` | **verde**: golangci-lint com 0 issues; ESLint; `tsc` do frontend e do e2e | 2m48s |
| Código gerado em dia | `sqlc generate` + `npm run gen:api` + `git diff --exit-code` | **sem diferenças** | — |
| Testes Go (unidade + integração) | `go test -count=1 ./...` (testcontainers) | **verde**: 25 pacotes `ok`, 0 falhas; 200 funções `Test*`, 28 delas `TestCA*` | 1m41s |
| Testes do frontend | `npm test` (Vitest) | **verde**: 31 arquivos, 142/142 testes | 1m27s |
| Build do frontend | `npx vite build` | **verde** (aviso: chunk maior que 500 kB) | 5s |
| E2E | `make e2e` | **28 de 29**: os 11 fluxos do PRD §53.3 passam; 1 falha de acessibilidade pré-existente (BL-001) | 3m09s |
| Auditoria | `make audit` | govulncheck: nenhuma vulnerabilidade alcançável; npm audit do frontend: **1 alta** (BL-002); npm audit do e2e: 0 | 19s |
| Ambiente de desenvolvimento | `make dev` + `make migrate` + `make seed` + `make seed-demo` | **verde**: `GET /api/v1/health` → `{"status":"ok","database":"ok"}`; SPA responde 200; 20 empresas fictícias e 3 competências anteriores + a corrente | 1m26s (`make dev`) |

### 2.1 Os 11 fluxos E2E do PRD §53.3 (AC-004)

| Fluxo | Spec | Resultado |
|---|---|---|
| §53.3-1 cadastrar empresa (CNPJ inválido recusado, cópia sem máscara) | `company-create.spec.ts` | ok |
| §53.3-2 configurar regime e módulos (UC-04, R6) | `company-config.spec.ts` | ok |
| §53.3-2 troca de regime com vigência futura (UC-04) | `company-regime.spec.ts` | ok |
| §53.3-3 abrir competência (UC-01, idempotência) | `competence-open.spec.ts` | ok |
| §53.3-4 filtrar, alterar status, progresso e fechar (UC-02, CA-014) | `fiscal-uc02.spec.ts` | ok |
| §53.3-5 eSocial na Folha e DCTFWeb na mesma instância | `payroll.spec.ts` | ok |
| §53.3-6 DCTFWeb com eSocial pendente exige exceção (CA-011) | `dctfweb.spec.ts` | ok |
| §53.3-7 Contábil em sequência (CA-010) | `accounting.spec.ts` | ok |
| §53.3-8 fechar módulos de forma independente (RN-006, 423) | `module-periods.spec.ts` | ok |
| §53.3-9 reabrir com motivo (RN-007) | `module-periods.spec.ts` | ok |
| §53.3-10 Backlog (R8) | `backlog.spec.ts` | ok |
| §53.3-10 Dashboard → Atrasadas → grade (UC-03) | `dashboard.spec.ts` | ok |
| §53.3-11 exportar XLSX com filtros | `grid-export.spec.ts` | ok |

Os outros 16 testes da suíte (acessibilidade, credenciais, importação, relatórios, configurações, smoke, declarações) também passam, com exceção do BL-001.

## 3. Falhas e achados pré-existentes

Nenhum foi corrigido na TASK-001: a spec manda registrar falhas pré-existentes sem corrigi-las fora do escopo (13-implementation-plan, TASK-001).

### BL-001 — Acessibilidade: `target-size` no Dashboard

- **Teste:** `e2e/specs/a11y.spec.ts:27`, "T-M7-04 axe: todas as telas sem violações serious/critical".
- **Violação:** `target-size [serious]` em `/dashboard?competence=2026-09`. Quatro links de empresa na amostra das categorias do painel "Exige atenção" ficam abaixo de 24 px e sem o espaçamento mínimo.
- **Origem no código:** `frontend/src/features/dashboard/AttentionPanel.tsx:29`. Os links estão em `text-xs`, dentro de linhas `flex-wrap` com 2 px entre elas (`gap-0.5`).
- **Reprodução:** determinística, os mesmos 4 nós em 3 execuções:
  - `make e2e` completo no Windows;
  - `make e2e ARGS=specs/a11y.spec.ts` no Windows;
  - a mesma suíte com o Chromium **Linux** da imagem oficial `mcr.microsoft.com/playwright:v1.63.0-noble`, por conexão remota. O trace confirma que os contextos do navegador rodaram em `linux`.
- **Causa provável:** o projeto foi validado em macOS. A fonte da aplicação (`"Inter", "Segoe UI", system-ui, -apple-system…`, sem a Inter carregada) cai em fontes diferentes em cada sistema, o que muda a quebra das linhas e o tamanho dos links. No Linux, ambiente do CI, a falha também acontece.
- **Por que importa:** o teste varre todas as telas com `expect.soft` dentro de um único caso. Enquanto ele falhar, uma violação nova em qualquer outra tela fica escondida dentro da mesma falha.
- **Os outros 4 testes de acessibilidade passam:** diálogos, menus, teclado na grade e status com texto ou ícone.

### BL-002 — Dependência com vulnerabilidade alta (npm audit)

- `source-map-js` 1.0.0–1.2.1, GHSA-68fv-2mgg-jv7q: negação de serviço ao processar source maps.
- Entra só por dependências de build e teste: `@tailwindcss/vite` → `@tailwindcss/node`; `vite` → `postcss`; `jsdom` → `css-tree`.
- `npm audit fix` resolve. Como o código não mudou, é um alerta publicado depois da última verificação do projeto-base.
- `make audit` (e o job `audit` do CI) falha por causa dele.

## 4. Observações

- **O CI do GitHub nunca executou:** `gh run list` não retorna nenhum run. O `docs/tasks.md` (T-M7-07) já registra que o E2E do CI depende de push, não autorizado (DT-02). Este baseline é a primeira execução fora do macOS.
- **`make tools` levou 9m19s** na primeira vez, porque o `air` compila dependências nativas. Depois, as ferramentas ficam em `.bin/`.
- **Git Bash e Docker:** um caminho Linux em `docker run -w` é convertido pelo MSYS. Para comandos `docker` com caminho absoluto de container, use `MSYS_NO_PATHCONV=1`.
- **Banco de dev:** `make migrate`, `make seed` e `make seed-demo` aplicados num volume novo (`controle_pgdata`), que antes não existia. Nenhum dado pré-existente foi tocado.

## 5. Critério de regressão para as próximas TASKs (AC-004)

Uma TASK não pode piorar nenhum destes números. Teste novo soma; teste removido ou quebrado é regressão.

| Suíte | Baseline |
|---|---|
| `make lint` | 0 issues; ESLint e `tsc` limpos |
| Código gerado | sem diferenças após `make gen` |
| `go test -count=1 ./...` | 25 pacotes `ok`, 0 falhas |
| Vitest | 142 testes passando |
| `make e2e` | 28 passando; 11 fluxos §53.3 verdes; única falha permitida: BL-001, até ser tratada |
| `make audit` | govulncheck limpo; npm audit: só o BL-002, até ser tratado |

## 6. Como reproduzir

```bash
cp .env.example .env    # ajuste HTTP_PORT se a 8080 estiver ocupada
make tools
(cd frontend && npm ci) && (cd e2e && npm ci)
make lint
make test
(cd backend && ../.bin/sqlc generate) && (cd frontend && npm run gen:api) && git diff --exit-code -- backend/internal/store frontend/src/types
(cd frontend && npx vite build)
make e2e
make audit
make dev && make migrate && make seed && make seed-demo
```

Para rodar o E2E com o Chromium Linux (mesmo ambiente do CI), com API e SPA no host:

```bash
MSYS_NO_PATHCONV=1 docker run -d --rm --init --name pw-linux -p 127.0.0.1:3000:3000 --workdir /home/pwuser --user pwuser mcr.microsoft.com/playwright:v1.63.0-noble /bin/sh -c "npx -y playwright@1.63.0 run-server --port 3000 --host 0.0.0.0"
PW_TEST_CONNECT_WS_ENDPOINT=ws://127.0.0.1:3000/ PW_TEST_CONNECT_EXPOSE_NETWORK='<loopback>' make e2e
```
