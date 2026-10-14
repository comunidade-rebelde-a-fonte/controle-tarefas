# Golden dos extratores da Conciliação

Porte do `tests/golden/` e do `scripts/avaliar_extrator.py` do Conciliador (commit `2bc6e26`; CC-25,
eval-plan §3), estendido ao comprovante como a M3.9 da origem pede (acurácia por campo). Pares PDF +
JSON conferidos, **anonimizados**, que medem os extratores (os agentes): o relatório pula os leitores
determinísticos.

## Convenção (a da origem)

```
extratos/<banco>/<AAAA-MM>.pdf
extratos/<banco>/<AAAA-MM>.json
comprovantes/gabarito.json
```

JSON gabarito do extrato: `saldo_inicial`, `saldo_final` e `transacoes[]` no schema do PRD §7
(`ordem`, `data`, `descricao`, `valor` em texto decimal, `sentido`), mais `banco`, `agencia`,
`conta` e o período.

Gabarito do comprovante: a empresa do cenário e, por arquivo, `data`, `valor` (texto decimal),
`sentido` em relação à empresa, `contraparte`, `contraparte_doc` (só os dígitos, nulo quando o
comprovante não o mostra por inteiro) e `tipo_operacao`.

## O que está aqui

| Arquivo | Origem |
| --- | --- |
| `extratos/a_fonte/2026-08.pdf` | `conciliacao/extratos/extrato_construtora_horizonte_2026-08.pdf` (cenário de aula, fictício) |
| `extratos/beta/2026-08.pdf` | `conciliacao/extratos/extrato_banco_beta_2026-08.pdf` |
| `extratos/*/*.json` | o CSV do mesmo extrato lido pelo `extrato_de_csv` da origem (`gabarito_gen.py`) |
| `comprovantes/gabarito.json` | os dados de cada um dos 13 comprovantes do cenário no gerador da origem (`conciliacao/scripts/gerar_cenario.py`); os PDFs ficam em `../conciliacao/comprovantes` |

O gate M0 da origem pede extratos reais do banco-alvo (mínimo 3, meses distintos), e a M3.8, ao
menos 5 comprovantes de cada tipo (PIX, boleto, TED, DARF, guia), em PDF e imagem. Eles entram aqui
só anonimizados, como a origem exige:

- substituir nomes de pessoas e razões sociais;
- CPF/CNPJ com dígitos mascarados ou fictícios consistentes;
- agência e conta no mesmo formato, com outros números;
- nada de print de app com QR code de PIX real.

Se restar dado pessoal, o PDF não entra no repositório.

## Relatório

- `make test` roda os relatórios com o gateway simulado (`TestGoldenReport_*` e
  `TestGoldenReceiptReport_*`), para conferir as contas.
- `make eval-extrator` roda contra o LiteLLM configurado (`LITELLM_BASE_URL`, `LITELLM_API_KEY` e os
  `<ALIAS>_MODEL`), **com custo de provedor real**: só com a sua autorização. Grava
  `backend/eval-extrator.json` e `backend/eval-extrator-comprovantes.json`.

No extrato, o relatório traz, por documento e no total: aprovação na 1ª tentativa, tentativas,
custo, latência e a diferença das transações importadas para o gabarito (saldos, quantidade e, linha
a linha, data, valor, sentido e descrição normalizada). O portão continua soberano: o saldo tem de
fechar em R$ 0,00, qualquer que seja a confiança do modelo.

No comprovante, traz a acurácia por campo (data, valor, sentido, contraparte normalizada, documento
da contraparte e tipo), os lançamentos extraídos e parciais, as chamadas, o custo e a latência.
