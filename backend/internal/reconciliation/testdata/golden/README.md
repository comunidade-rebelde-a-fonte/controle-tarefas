# Golden do extrator de extrato

Porte do `tests/golden/` e do `scripts/avaliar_extrator.py` do Conciliador (commit `2bc6e26`; CC-25,
eval-plan §3). Pares PDF + JSON conferidos, **anonimizados**, que medem o extrator (o agente): o
relatório pula os leitores determinísticos.

## Convenção (a da origem)

```
extratos/<banco>/<AAAA-MM>.pdf
extratos/<banco>/<AAAA-MM>.json
```

JSON gabarito: `saldo_inicial`, `saldo_final` e `transacoes[]` no schema do PRD §7 (`ordem`,
`data`, `descricao`, `valor` em texto decimal, `sentido`), mais `banco`, `agencia`, `conta` e o
período.

## O que está aqui

| Arquivo | Origem |
| --- | --- |
| `extratos/a_fonte/2026-08.pdf` | `conciliacao/extratos/extrato_construtora_horizonte_2026-08.pdf` (cenário de aula, fictício) |
| `extratos/beta/2026-08.pdf` | `conciliacao/extratos/extrato_banco_beta_2026-08.pdf` |
| `*.json` | o CSV do mesmo extrato lido pelo `extrato_de_csv` da origem (`gabarito_gen.py`) |

O gate M0 da origem pede extratos reais do banco-alvo (mínimo 3, meses distintos). Eles entram aqui
só anonimizados, como a origem exige:

- substituir nomes de pessoas e razões sociais;
- CPF/CNPJ com dígitos mascarados ou fictícios consistentes;
- agência e conta no mesmo formato, com outros números;
- nada de print de app com QR code de PIX real.

Se restar dado pessoal, o PDF não entra no repositório.

## Relatório

- `make test` roda o relatório com o gateway simulado (`TestGoldenReport_*`), para conferir as contas
  do relatório.
- `make eval-extrator` roda contra o LiteLLM configurado (`LITELLM_BASE_URL`, `LITELLM_API_KEY` e os
  `<ALIAS>_MODEL`), **com custo de provedor real**: só com a sua autorização.

O relatório traz, por documento e no total: aprovação na 1ª tentativa, tentativas, custo, latência e
a diferença das transações importadas para o gabarito (saldos, quantidade e, linha a linha, data,
valor, sentido e descrição normalizada). O portão continua soberano: o saldo tem de fechar em
R$ 0,00, qualquer que seja a confiança do modelo.
