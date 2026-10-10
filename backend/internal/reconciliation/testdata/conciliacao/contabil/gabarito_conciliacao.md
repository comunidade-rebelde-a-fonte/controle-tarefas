# Gabarito da conciliação bancária - Agosto/2026

Construtora Horizonte Ltda - conta Banco A Fonte (Ag 1234 CC 56789-0)

## Saldos

| Item | Valor (R$) |
|---|---:|
| Saldo contábil em 31/08 (razão) | 227.390,77 |
| Saldo bancário em 31/08 (extrato A Fonte) | 213.142,37 |
| Diferença a explicar | -14.248,40 |

## Conciliação: do saldo contábil ao saldo bancário

| Ajuste | Valor (R$) |
|---|---:|
| Saldo contábil | 227.390,77 |
| (-) Tarifa de manutenção 01/08 não contabilizada | -89,90 |
| (-) Tarifas TED/PIX 29/08 não contabilizadas | -45,00 |
| (-) Multa e juros do boleto Cerâmica Santa Rita não contabilizados (despesa financeira) | -396,50 |
| (-) PIX Hidráulica Total pago em duplicidade em 21/08 (registrar como adiantamento/crédito a recuperar do fornecedor) | -7.960,00 |
| (+) Boleto Madeireira Pinheiro contabilizado no A Fonte, mas pago pelo Banco Beta (reclassificar entre contas) | 12.640,00 |
| (-) ISS retido pela Prefeitura: NF 1044 contabilizada por 100.000, crédito foi de 95.000 (registrar ISS retido a compensar) | -5.000,00 |
| (+) Ana Lúcia Ferreira: NF 1042 baixada por 10.000, banco recebeu 4 PIX = 16.000 (6.000 a maior: adiantamento de cliente / devolução) | 6.000,00 |
| (+) Multa e juros recebidos no boleto Mercado Bom Preço não contabilizados (receita financeira) | 603,00 |
| (-) PIX Escola Crescer contabilizado no A Fonte, mas creditado no Banco Beta (reclassificar entre contas) | -20.000,00 |
| **Saldo bancário conciliado** | **213.142,37** |

## Casos e como aparecem em cada fonte

| # | Caso | Contas a pagar/receber | Razão contábil | Extrato A Fonte | Extrato Beta | Comprovante |
|---|---|---|---|---|---|---|
| 1 | Boleto pago com multa e juros | CP-010 Cerâmica, doc 18.300,00 venc 08/08 | 18.300,00 em 13/08 | 18.696,50 em 13/08 | - | 13_boleto_ceramica... |
| 2 | Boleto recebido com multa e juros | NF 1047 Mercado Bom Preço 27.000,00 venc 15/08 | 27.000,00 em 22/08 | 27.603,00 em 22/08 | - | 22_boleto_mercado... |
| 3 | Pagamento dividido em dois PIX | CP-004 Aço Brasil 31.850,00 | 31.850,00 em 06/08 | 20.000,00 (06/08) + 11.850,00 (09/08) | - | 06_ e 09_pix_enviado_aco_brasil |
| 4 | Cliente pagou em 4 PIX e a maior | NF 1042 Ana Lúcia 10.000,00 | 10.000,00 em 21/08 | 2.000 + 6.000 + 3.000 + 5.000 = 16.000,00 | - | 05_, 08_, 14_, 21_pix_recebido_ana_lucia |
| 5 | Pagamento por outra conta | CP-007 Madeireira 12.640,00 | 12.640,00 em 10/08 (conta errada) | não aparece | 12.640,00 em 10/08 | 10_boleto_madeireira... |
| 6 | Recebimento em outra conta | NF 1048 Escola Crescer 20.000,00 | 20.000,00 em 26/08 (conta errada) | não aparece | 20.000,00 em 26/08 | 26_pix_recebido_escola_crescer... |
| 7 | Pagamento em duplicidade | CP-016 Hidráulica 7.960,00 | 7.960,00 em 20/08 | 7.960,00 em 20/08 e 21/08 | - | 20_ e 21_pix_enviado_hidraulica_total |
| 8 | Recebimento líquido de retenção | NF 1044 Prefeitura 100.000,00 | 100.000,00 em 12/08 | 95.000,00 em 12/08 | - | 12_ordem_bancaria_prefeitura... |
| 9 | Tarifas não contabilizadas | - | não lançadas | 89,90 (01/08) e 45,00 (29/08) | - | - |

## Lançamentos contábeis de ajuste sugeridos

1. D Despesas bancárias / C Banco A Fonte: 134,90 (tarifas).
2. D Juros e multas passivos / C Banco A Fonte: 396,50 (Cerâmica).
3. D Adiantamento a fornecedores (Hidráulica Total) / C Banco A Fonte: 7.960,00 (duplicidade; pedir estorno).
4. D Banco A Fonte / C Banco Beta: 12.640,00 (Madeireira foi paga pelo Beta).
5. D ISS retido a compensar / C Banco A Fonte: 5.000,00 (Prefeitura).
6. D Banco A Fonte / C Adiantamento de clientes (Ana Lúcia): 6.000,00 (pago a maior; devolver ou abater em próxima NF).
7. D Banco A Fonte / C Juros e multas ativos: 603,00 (Mercado Bom Preço).
8. D Banco Beta / C Banco A Fonte: 20.000,00 (Escola Crescer entrou no Beta).

Após os ajustes o saldo contábil do Banco A Fonte passa a 213.142,37, igual ao extrato.
Saldo do Banco Beta em 31/08: 22.325,00 (contabilidade deve refletir 15.000,00 - 12.640,00 + 20.000,00 - 35,00 de tarifa).

## Comprovantes gerados

- `05_pix_recebido_ana_lucia_1de4.pdf`: PIX Ana Lucia Ferreira 1/4 - R$ 2.000,00
- `08_pix_recebido_ana_lucia_2de4.pdf`: PIX Ana Lucia Ferreira 2/4 - R$ 6.000,00
- `14_pix_recebido_ana_lucia_3de4.pdf`: PIX Ana Lucia Ferreira 3/4 - R$ 3.000,00
- `21_pix_recebido_ana_lucia_4de4.pdf`: PIX Ana Lucia Ferreira 4/4 - R$ 5.000,00
- `06_pix_enviado_aco_brasil_1de2.pdf`: PIX Aco Brasil 1/2 - R$ 20.000,00
- `09_pix_enviado_aco_brasil_2de2.pdf`: PIX Aco Brasil 2/2 - R$ 11.850,00
- `13_boleto_ceramica_santa_rita_com_juros.pdf`: Boleto Ceramica Santa Rita - R$ 18.696,50 (documento 18.300,00 + multa e juros)
- `10_boleto_madeireira_pinheiro_banco_beta.pdf`: Boleto Madeireira Pinheiro - R$ 12.640,00 (pago pelo Banco Beta)
- `20_pix_enviado_hidraulica_total_1.pdf`: PIX Hidraulica Total (original) - R$ 7.960,00
- `21_pix_enviado_hidraulica_total_2.pdf`: PIX Hidraulica Total (DUPLICADO) - R$ 7.960,00
- `22_boleto_mercado_bom_preco_pago_com_juros.pdf`: Boleto Mercado Bom Preco - R$ 27.603,00 (documento 27.000,00 + multa e juros)
- `12_ordem_bancaria_prefeitura_rio_claro.pdf`: TED Prefeitura Rio Claro - R$ 95.000,00 liquido (bruto 100.000,00, ISS retido 5.000,00)
- `26_pix_recebido_escola_crescer_banco_beta.pdf`: PIX Escola Crescer - R$ 20.000,00 (recebido no Banco Beta)
