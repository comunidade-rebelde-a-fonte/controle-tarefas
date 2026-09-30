# Medições de desempenho

Metas do PRD §45.1 acompanhadas pelo plano (M3 e M7):
- a grade com cerca de 300 linhas abre em até ~2 s ponta a ponta;
- o PATCH de status tem p95 abaixo de 200 ms;
- o `ensure` de uma competência nova leva menos de 10 s (T-M7-01).

## Como reproduzir

Todas as medições usam só o banco local `controle_perf`. O `loadgen` recusa qualquer outro banco.

```bash
make perf-generate                    # 300 empresas, 3 competências, semente 42 (recria o schema do controle_perf)
make perf-generate COMPETENCES=12     # massa do T-M7-01: 300 empresas × 12 competências; imprime o tempo do ensure de cada competência
make perf-generate COMPANIES=530      # pior caso: grade do Simples com mais de 300 linhas
make perf-bench                       # API em processo (HTTP real em 127.0.0.1), gzip como no navegador
make perf-bench BENCH_ARGS="-iterations 30 -patches 200 -bulks 10 -bulk-size 300"
```

Para medir no navegador, com o build de produção do frontend:

```bash
cd frontend && npx vite build --outDir /tmp/perf-dist
# API na porta 8092 apontando para o controle_perf
cd backend && DATABASE_URL=postgres://…/controle_perf APP_ENV=test HTTP_PORT=8092 CORS_ORIGINS=http://localhost:5175 go run ./cmd/api serve
# SPA de produção com proxy /api → 8092
cd frontend && VITE_API_PROXY_TARGET=http://localhost:8092 npx vite preview --outDir /tmp/perf-dist --port 5175
cd e2e && PERF_RUNS=5 node perf/grid-render.mjs   # as seis grades (Fiscal simples/normal, Declarações, Folha, Contábil, DCTFWeb)
```

O `perf-bench` mede, na competência mais recente:
- as seis grades;
- dashboard, attention e history;
- backlog (páginas de 100 e 500, e atrasadas da competência);
- as seis exportações XLSX;
- `POST /competencies/ensure-current` (idempotente);
- PATCH de status;
- `bulk-status` com 300 instâncias do Fiscal.

O `ensure` de uma competência **nova** é medido pelo `perf-generate`, que cronometra o motor em cada competência gerada. Ao final da carga, o `perf-generate` roda `ANALYZE`, como o autovacuum faria num banco em uso. Sem isso, as estimativas do planejador logo após a carga em massa ficam erradas: o backlog chegou a 1,7 s no banco antes do `ANALYZE` e caiu para 0,6 s depois, sob a mesma carga da máquina.

**Concorrência com outras sessões:** o `db-reset` de outra sessão apaga também o `controle_perf`. Por isso, gere e meça com o mutex compartilhado do E2E:

```bash
mkdir /tmp/controle-e2e.lock   # antes
rmdir /tmp/controle-e2e.lock   # depois
```

**Massa sintética** (`backend/cmd/loadgen`), determinística pela semente:
- cerca de 60% das empresas no Simples; as demais em Presumido, Real, Arbitrado e Imune/Isenta;
- cerca de 80% com Contábil; Folha conforme o tipo de folha;
- cerca de 15% de overrides "não aplicável" por obrigação (IPI, ICMS, ISSQN, SINTEGRA, Retidos);
- CNPJs com dígitos válidos na faixa sintética 99xxxxxx;
- status por hash de empresa, obrigação, competência e semente: competências anteriores majoritariamente concluídas e cerca de 70% dos períodos fechados; a corrente, em andamento.

## Resultados — 30/09/2026 (T-M3-10)

**Ambiente:** Apple M4 Pro com 24 GB de memória; PostgreSQL 16.15 (Debian) no Docker Desktop; Go 1.26; Chromium do Playwright. Competência corrente: 09/2026.

### 300 empresas (massa padrão: 23.385 instâncias em 3 competências)

| Medição | Linhas | Tamanho (gzip) | n | p50 | p95 | máx |
|---|---|---|---|---|---|---|
| GET grade Fiscal simples | 179 | 79 KB | 20 | 44 ms | 68 ms | 68 ms |
| GET grade Fiscal normal | 121 | 73 KB | 20 | 47 ms | 67 ms | 67 ms |
| PATCH status (com agregado da linha) | — | — | 100 | 5 ms | 8 ms | 16 ms |

### Pior caso: 530 empresas (grade do Simples com 315 linhas; 41.541 instâncias)

API (`make perf-bench BENCH_ARGS="-iterations 50 -patches 200"`):

| Medição | Linhas | Tamanho (gzip) | n | p50 | p95 | máx |
|---|---|---|---|---|---|---|
| GET grade Fiscal simples | 315 | 138 KB | 50 | 83 ms | 231 ms | 282 ms |
| GET grade Fiscal normal | 215 | 128 KB | 50 | 66 ms | 76 ms | 86 ms |
| PATCH status (com agregado da linha) | — | — | 200 | 5 ms | 7 ms | 19 ms |

No navegador, cache frio a cada rodada: da navegação até a grade exibir as linhas, incluindo o carregamento da SPA, `/competencies/latest`, a grade e a renderização virtualizada.

| Grade no navegador (cold) | Linhas | n | mín | mediana | máx |
|---|---|---|---|---|---|
| Fiscal simples | 315 | 5 | 489 ms | 516 ms | 696 ms |
| Fiscal normal | 215 | 5 | 439 ms | 494 ms | 508 ms |

**Conclusão:** as metas foram atingidas com folga.
- Grade com 315 linhas: mediana de 0,52 s ponta a ponta (máximo de 0,70 s), contra a meta de ~2 s.
- PATCH de status: p95 de 7 ms, contra a meta de 200 ms.
- Não foi preciso abrir tarefa de otimização antes do M4.

**Observações:**
- Aparecem picos isolados de 0,2 a 0,6 s em requisições de grade, variando entre rodadas e sem padrão. É ruído do ambiente (VM do Docker Desktop no macOS, outras cargas na máquina).
- O build do frontend gera um único chunk JS acima de 500 kB (aviso do Vite). Isso ainda não pesa na meta. Se o bundle crescer, a divisão por rota é a otimização natural, a avaliar no M7 (T-M7-01).

## Resultados — 30/09/2026 (T-M7-01)

**Ambiente:**
- hardware: Apple M4 Pro (12 núcleos) com 24 GB de memória;
- PostgreSQL 16.15 (Debian) no Docker Desktop;
- Go 1.27.1 no host;
- navegador: Chromium headless do Playwright;
- build de produção do frontend (`vite build` + `vite preview`), develop em `48146a7`, já com a exportação XLSX (T-M6-05);
- competência corrente: 09/2026.

**Massa:** 300 empresas × 12 competências (10/2025 a 09/2026), com 93.540 instâncias no total, 7.795 por competência. Aparecem 179 linhas no Fiscal simples e 121 no Fiscal normal. Declarações e DCTFWeb têm 300 linhas, Folha 257 e Contábil 227. O backlog tem 10.602 pendências anteriores.

**Ruído do ambiente:** outras três sessões de desenvolvimento rodavam testes na mesma máquina (Playwright, testcontainers e `make test`), com load average entre 25 e 55 em 12 núcleos. As medições nessas condições oscilaram muito:
- grades: p95 de até 0,5 s;
- backlog: p95 de até 5 s;
- PATCH: p95 de 192 ms, perto do limite;
- bulk de 300: p95 de até 7 s.

Como pede a tarefa, as medições foram repetidas numa janela mais calma (load average de 8 a 11), com o mutex `/tmp/controle-e2e.lock` retido para que nenhum E2E nem `db-reset` rodasse no meio. Em duas rodadas consecutivas nessa janela, os números ficaram estáveis. Os valores abaixo são da última rodada.

### `ensure` de competência nova (`make perf-generate COMPETENCES=12`)

| Medição | Empresas | Instâncias criadas | n | mín | máx |
|---|---|---|---|---|---|
| Ensure de competência nova (motor) | 300 | 7.795 | 12 | 276 ms | 313 ms |

A competência corrente (09/2026) levou 298 ms. Com a máquina carregada (load average acima de 25), o pior caso observado foi de 855 ms, ainda longe da meta de 10 s.

### API (`make perf-bench BENCH_ARGS="-iterations 30 -patches 200 -bulks 10"`)

| Medição | Linhas/itens | Tamanho (gzip) | n | p50 | p95 | máx |
|---|---|---|---|---|---|---|
| GET grade Fiscal simples | 179 | 80 KB | 30 | 17 ms | 21 ms | 21 ms |
| GET grade Fiscal normal | 121 | 74 KB | 30 | 15 ms | 17 ms | 17 ms |
| GET grade Declarações | 300 | 91 KB | 30 | 20 ms | 20 ms | 21 ms |
| GET grade Folha | 257 | 59 KB | 30 | 15 ms | 28 ms | 28 ms |
| GET grade Contábil | 227 | 80 KB | 30 | 16 ms | 17 ms | 17 ms |
| GET grade DCTFWeb | 300 | 80 KB | 30 | 18 ms | 22 ms | 22 ms |
| GET dashboard | — | 1 KB | 30 | 47 ms | 61 ms | 81 ms |
| GET dashboard/attention | 11 categorias | 1 KB | 30 | 39 ms | 51 ms | 66 ms |
| GET dashboard/history | — | < 1 KB | 30 | 93 ms | 109 ms | 154 ms |
| GET backlog (página de 100) | 100 de 10.602 | 7 KB | 30 | 226 ms | 254 ms | 270 ms |
| GET backlog (página de 500) | 500 de 10.602 | 33 KB | 30 | 239 ms | 253 ms | 257 ms |
| GET backlog atrasadas da competência | 0 | < 1 KB | 30 | 20 ms | 21 ms | 22 ms |
| GET exportação XLSX Fiscal simples | — | 35 KB (xlsx) | 30 | 19 ms | 21 ms | 22 ms |
| GET exportação XLSX Fiscal normal | — | 31 KB (xlsx) | 30 | 17 ms | 18 ms | 18 ms |
| GET exportação XLSX Declarações | — | 51 KB (xlsx) | 30 | 26 ms | 26 ms | 28 ms |
| GET exportação XLSX Folha | — | 33 KB (xlsx) | 30 | 16 ms | 17 ms | 17 ms |
| GET exportação XLSX Contábil | — | 33 KB (xlsx) | 30 | 18 ms | 23 ms | 25 ms |
| GET exportação XLSX DCTFWeb | — | 41 KB (xlsx) | 30 | 22 ms | 23 ms | 23 ms |
| POST ensure-current (já gerada) | — | — | 30 | 1 ms | 1 ms | 2 ms |
| PATCH status (com agregado da linha) | — | — | 200 | 2 ms | 2 ms | 6 ms |
| POST bulk-status (300 itens) | 300 alteradas | — | 10 | 106 ms | 122 ms | 122 ms |

"Atrasadas da competência" retorna 0 porque, em 30/09, nenhum vencimento de 09/2026 passou.

### Grades no navegador (cold)

Cache frio a cada rodada, da navegação até a grade exibir as linhas, incluindo o carregamento da SPA, `/competencies/latest`, a grade e a renderização virtualizada.

| Grade no navegador (cold) | Linhas | n | mín | mediana | máx |
|---|---|---|---|---|---|
| Fiscal simples | 179 | 5 | 227 ms | 230 ms | 345 ms |
| Fiscal normal | 121 | 5 | 223 ms | 233 ms | 239 ms |
| Declarações | 300 | 5 | 187 ms | 193 ms | 197 ms |
| Folha | 257 | 5 | 176 ms | 179 ms | 188 ms |
| Contábil | 227 | 5 | 184 ms | 190 ms | 205 ms |
| DCTFWeb | 300 | 5 | 172 ms | 176 ms | 182 ms |

### Conclusão (T-M7-01)

As três metas do aceite foram atingidas com folga, com 300 empresas e 12 competências:

| Meta | Resultado | Situação |
|---|---|---|
| Grades ≤ ~2 s | Máximo de 0,35 s ponta a ponta no navegador; API com p95 ≤ 28 ms | Atingida |
| PATCH p95 < 200 ms | p95 de 2 ms (192 ms no pior momento da máquina carregada) | Atingida |
| `ensure` < 10 s | 276 a 313 ms por competência nova (máximo de 855 ms com a máquina carregada) | Atingida |

Pontos de atenção para a T-M7-02 (otimização de consultas), sem meta violada:
- **Backlog:** é a leitura mais cara, com cerca de 230 ms. O plano faz *nested loop* sobre `v_obligation_instances` com todas as competências anteriores e até ~300 ms de *planning* (medido com a máquina carregada). É o mais sensível a estatísticas desatualizadas e à carga da máquina.
- **bulk-status:** faz duas instruções por item (UPDATE + histórico) em sequência na transação. São cerca de 110 ms para 300 itens em condições normais, mas escala com a latência do banco: chegou a 4 a 7 s com a máquina saturada. Agrupar em `pgx.Batch` ou num UPDATE/INSERT por conjunto é a otimização natural.
- **Bundle do frontend:** o chunk único de JS acima de 500 kB (aviso do Vite) continua. Não pesa na meta: a carga fria da SPA mais a grade fica abaixo de 0,35 s.

## Otimização de consultas — 30/09/2026 (T-M7-02)

**Massa:** 300 empresas × 12 competências (`make perf-generate COMPETENCES=12`). O `loadgen` passou a gravar o histórico de cada status aplicado, como numa alteração real: são 85.883 linhas em `obligation_status_history`. Sem esse volume, os planos escondiam o custo das consultas que leem o histórico.

**Método:** `EXPLAIN (ANALYZE, BUFFERS)` de cada consulta crítica (grade, dashboard, atenção, histórico, backlog e histórico da empresa), com os parâmetros reais da competência 09/2026, antes e depois de cada mudança. Os tempos abaixo são do banco. As varreduras sequenciais que restam são em tabelas pequenas de catálogo (`obligation_types`, `status_definitions`, `users`, `competencies`) e nos snapshots (3.600 linhas), onde o hash join lendo tudo é a escolha certa do planejador. Nenhuma varredura sequencial resta em `obligation_instances` (93.540 linhas), `obligation_status_history` (85.883), `company_module_periods` (13.008) ou `company_module_period_events`.

| Consulta | Antes | Depois | Mudança |
|---|---|---|---|
| GridCells (Fiscal simples) | 19,9 ms, seq scan no histórico | 9,7 ms | `has_history` por `LEFT JOIN LATERAL … LIMIT 1` no índice do histórico; o `EXISTS` na lista de colunas virava uma "hashed SubPlan" que lia o histórico inteiro |
| DashboardPreviousOpenCompanies | 10,1 ms | 6,8 ms | índice parcial `company_module_periods_open_idx` (`WHERE state = 'OPEN'`, migration 0017) |
| AttentionPreviousOpen | 9,7 ms | 5,6 ms | idem |
| Backlog (competências anteriores) | 76 ms, seq scan nos períodos | 78 ms, sem seq scan nos períodos | idem; o custo restante é ler as competências anteriores, o que é inerente à consulta |

**Plano genérico × plano com os valores (a maior diferença):** pela API, o backlog levava 200 ms e o histórico 81 ms, contra 75 ms e 32 ms no banco. A causa: o pgx prepara e reutiliza as consultas, e o PostgreSQL passa a um plano genérico depois de algumas execuções. Nas consultas com filtros opcionais (`$n IS NULL OR …`), esse plano é 2 a 4 vezes mais lento; com `plan_cache_mode = force_generic_plan`, o backlog vai a 283 ms e o histórico a 104 ms. O pool agora abre as conexões com `plan_cache_mode = force_custom_plan` (`internal/platform/db`, com teste).

**Tentativa descartada:** pré-filtrar o backlog pelos códigos de status não finais, com um índice parcial `(status_code, competence_id)`. No mínimo de 5 execuções ficou pior (93 ms contra 71 ms), e a mudança foi revertida.

### API depois da otimização (`make perf-bench BENCH_ARGS="-iterations 30 -patches 200 -bulks 10"`, com o mutex)

| Medição | Linhas/itens | Tamanho (gzip) | n | p50 | p95 | máx | p50 da T-M7-01 |
|---|---|---|---|---|---|---|---|
| GET grade Fiscal simples | 179 | 81 KB | 30 | 22 ms | 24 ms | 25 ms | 17 ms |
| GET grade Fiscal normal | 121 | 76 KB | 30 | 19 ms | 20 ms | 22 ms | 15 ms |
| GET grade Declarações | 300 | 92 KB | 30 | 23 ms | 24 ms | 25 ms | 20 ms |
| GET grade Folha | 257 | 60 KB | 30 | 15 ms | 16 ms | 16 ms | 15 ms |
| GET grade Contábil | 227 | 81 KB | 30 | 19 ms | 21 ms | 21 ms | 16 ms |
| GET grade DCTFWeb | 300 | 81 KB | 30 | 22 ms | 24 ms | 25 ms | 18 ms |
| GET dashboard | — | 1 KB | 30 | 20 ms | 24 ms | 28 ms | 47 ms |
| GET dashboard/attention | 11 categorias | 1 KB | 30 | 45 ms | 50 ms | 52 ms | 39 ms |
| GET dashboard/history | — | < 1 KB | 30 | 19 ms | 20 ms | 20 ms | 93 ms |
| GET backlog (página de 100) | 100 de 10.602 | 7 KB | 30 | 59 ms | 68 ms | 101 ms | 226 ms |
| GET backlog (página de 500) | 500 de 10.602 | 33 KB | 30 | 60 ms | 73 ms | 77 ms | 239 ms |
| GET exportação XLSX (6 grades) | — | 31–51 KB | 30 | 18–28 ms | 19–29 ms | 19–30 ms | 16–26 ms |
| PATCH status (com agregado da linha) | — | — | 200 | 2 ms | 3 ms | 6 ms | 2 ms |
| POST bulk-status (300 itens) | 300 | — | 10 | 123 ms | 136 ms | 136 ms | 106 ms |

As grades ficaram alguns milissegundos mais lentas porque agora há histórico real e cada consulta é planejada com os valores; seguem em cerca de 20 ms, contra a meta de ~2 s. O dashboard, o histórico e o backlog ficaram 2 a 4 vezes mais rápidos. Todas as metas (grades ≤ ~2 s, PATCH p95 < 200 ms, `ensure` < 10 s) continuam atendidas.


## Verificação final — 30/09/2026 (T-M7-07)

Massa padrão recriada depois do `make db-reset` (`make perf-generate`: 300 empresas, 23.385 instâncias em 3 competências; `ensure` de cada competência nova entre 501 e 516 ms), na `develop` com as T-M7-02 a T-M7-06. `make perf-bench` com os parâmetros padrão:

| Medição | Linhas/itens | Tamanho (gzip) | n | p50 | p95 | máx |
|---|---|---|---|---|---|---|
| GET grade Fiscal simples | 179 | 81 KB | 20 | 19 ms | 25 ms | 25 ms |
| GET grade Fiscal normal | 121 | 76 KB | 20 | 18 ms | 19 ms | 19 ms |
| GET grade Declarações | 300 | 92 KB | 20 | 22 ms | 39 ms | 39 ms |
| GET grade Folha | 257 | 60 KB | 20 | 15 ms | 16 ms | 16 ms |
| GET grade Contábil | 227 | 81 KB | 20 | 18 ms | 19 ms | 19 ms |
| GET grade DCTFWeb | 300 | 81 KB | 20 | 21 ms | 22 ms | 22 ms |
| GET dashboard | — | 1 KB | 20 | 16 ms | 21 ms | 21 ms |
| GET dashboard/attention | 11 categorias | 1 KB | 20 | 43 ms | 45 ms | 45 ms |
| GET dashboard/history | — | 0 KB | 20 | 11 ms | 12 ms | 12 ms |
| GET backlog (página de 100) | 100 de 1988 | 7 KB | 20 | 21 ms | 24 ms | 24 ms |
| GET backlog (página de 500) | 500 de 1988 | 33 KB | 20 | 26 ms | 30 ms | 30 ms |
| GET backlog atrasadas da competência | 0 de 0 | 0 KB | 20 | 9 ms | 12 ms | 12 ms |
| GET exportação XLSX Fiscal simples | — | 35 KB | 20 | 21 ms | 23 ms | 23 ms |
| GET exportação XLSX Fiscal normal | — | 31 KB | 20 | 19 ms | 19 ms | 19 ms |
| GET exportação XLSX Declarações | — | 51 KB | 20 | 27 ms | 35 ms | 35 ms |
| GET exportação XLSX Folha | — | 33 KB | 20 | 17 ms | 21 ms | 21 ms |
| GET exportação XLSX Contábil | — | 33 KB | 20 | 19 ms | 20 ms | 20 ms |
| GET exportação XLSX DCTFWeb | — | 41 KB | 20 | 24 ms | 24 ms | 24 ms |
| POST ensure-current (já gerada) | — | — | 20 | 1 ms | 2 ms | 2 ms |
| PATCH status (com agregado da linha) | — | — | 100 | 2 ms | 3 ms | 5 ms |
| POST bulk-status (300 itens) | 300 alteradas | — | 10 | 120 ms | 134 ms | 134 ms |

Todas as metas do PRD §45.1 continuam atendidas com folga:

- grades de até 300 linhas: p50 entre 15 e 22 ms na API, contra a meta de ~2 s;
- PATCH de status: p95 de 3 ms, contra a meta de 200 ms;
- `ensure` de competência nova: ~0,5 s, contra a meta de 10 s.
