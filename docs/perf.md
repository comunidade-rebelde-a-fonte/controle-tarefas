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
