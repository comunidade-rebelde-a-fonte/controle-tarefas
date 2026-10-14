# Cenário de conciliação bancária x contábil - Construtora Horizonte (agosto/2026)

Material fictício para aula de conciliação. A empresa faturou R$ 500.000,00 em agosto (9 notas fiscais)
e movimenta duas contas bancárias: Banco A Fonte (principal) e Banco Beta (secundária).

## Arquivos

| Pasta | Conteúdo |
|---|---|
| `extratos/` | Extrato do Banco A Fonte (CSV, TXT e PDF) e do Banco Beta (CSV e PDF) |
| `contabil/contas_a_receber.csv` | Notas fiscais emitidas no mês (o que a empresa tem a receber) |
| `contabil/contas_a_pagar.csv` | Títulos de fornecedores e impostos, com data, valor e conta em que foram baixados |
| `contabil/razao_banco_a_fonte.csv` | Razão contábil da conta Banco A Fonte, como a contabilidade lançou (contém as divergências) |
| `comprovantes/` | 13 comprovantes em PDF (PIX, boletos e ordem bancária) dos casos difíceis |
| `contabil/gabarito_conciliacao.md` | Resposta: diferenças explicadas, conciliação de saldos e lançamentos de ajuste |

## Casos embutidos

1. Boleto de fornecedor pago em atraso com multa e juros (Cerâmica Santa Rita).
2. Boleto de cliente recebido em atraso com multa e juros (Mercado Bom Preço).
3. Pagamento a fornecedor dividido em dois PIX (Aço Brasil).
4. Cliente que comprou R$ 10.000 e pagou em quatro PIX de 2.000, 6.000, 3.000 e 5.000, ou seja, pagou R$ 6.000 a maior (Ana Lúcia Ferreira).
5. Pagamento feito pela outra conta bancária (Madeireira Pinheiro, via Banco Beta).
6. Recebimento na outra conta bancária (Escola Crescer, via Banco Beta).
7. Pagamento em duplicidade (Hidráulica Total).
8. Recebimento líquido de retenção de ISS (Prefeitura de Rio Claro).
9. Tarifas bancárias não contabilizadas.

## Exercício sugerido

Entregar aos alunos os extratos, os CSVs de contas a pagar/receber, o razão e os comprovantes.
Pedir que identifiquem cada diferença entre razão e extrato, classifiquem a causa e proponham o
lançamento de ajuste. Conferir com o gabarito ao final.

## Regerar os arquivos

```bash
python3 -m venv .venv && .venv/bin/pip install reportlab   # uma vez
python3 scripts/gerar_extrato.py        # extrato A Fonte (CSV/TXT)
.venv/bin/python scripts/gerar_pdf.py   # extrato A Fonte (PDF)
.venv/bin/python scripts/gerar_cenario.py   # CSVs contábeis, Banco Beta, comprovantes e gabarito
```

Para alterar valores, edite as listas em `scripts/gerar_extrato.py` (extrato) e
`scripts/gerar_cenario.py` (títulos, razão, comprovantes). O script de cenário verifica que
saldo contábil + ajustes = saldo do extrato e falha se algo não fechar.
