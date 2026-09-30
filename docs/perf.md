# Medições de desempenho

Metas do PRD §45.1 acompanhadas pelo plano (M3 e M7): a grade com cerca de 300 linhas abre em até ~2 s ponta a ponta, e o PATCH de status tem p95 abaixo de 200 ms.

## Como reproduzir

Todas as medições usam só o banco local `controle_perf`. O `loadgen` recusa qualquer outro banco.

```bash
make perf-generate                    # 300 empresas, 3 competências, semente 42 (recria o schema do controle_perf)
make perf-generate COMPANIES=530      # pior caso: grade do Simples com mais de 300 linhas
make perf-bench                       # API em processo (HTTP real em 127.0.0.1), gzip como no navegador
make perf-bench BENCH_ARGS="-iterations 50 -patches 200"
```

Para medir no navegador, com o build de produção do frontend:

```bash
cd frontend && npx vite build --outDir /tmp/perf-dist
# API na porta 8092 apontando para o controle_perf
cd backend && DATABASE_URL=postgres://…/controle_perf APP_ENV=test HTTP_PORT=8092 CORS_ORIGINS=http://localhost:5175 go run ./cmd/api serve
# SPA de produção com proxy /api → 8092
cd frontend && VITE_API_PROXY_TARGET=http://localhost:8092 npx vite preview --outDir /tmp/perf-dist --port 5175
cd e2e && PERF_RUNS=5 node perf/grid-render.mjs
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
