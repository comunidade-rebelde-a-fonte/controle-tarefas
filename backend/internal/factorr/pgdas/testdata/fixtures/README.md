# Fixtures do parser PGDAS-D

Cada pasta tem `documento.(pdf|txt)` e `expected.json` (campos esperados, faixa de confiança e,
se houver, o motivo esperado).

## Sintéticas (`"sintetica": true`)

Geradas por `scripts/gerar_fixtures_pgdas.py`. Imitam o layout do "Extrato do Simples Nacional"
e cobrem variações de rótulo, espaçamento, encoding, campos ausentes, CNPJ inválido e PDF sem
camada de texto. **Não substituem extratos reais.**

## Layout declaratório (`pdf_declaratorio_*`, M8)

Quatro PDFs fictícios no layout da declaração do PGDAS-D (PA em intervalo, tabelas de receita e
folha), com CNPJ fictício de dígitos verificadores válidos, gerados a partir de `pdf/` por
`scripts/preencher_cnpj_ficticio.py`. Cobrem fator r abaixo e acima de 28%, comércio (Anexo I) e
serviço não sujeito ao fator r.

Chaves extras no `expected.json`:

- `campos`: o valor **verdadeiro** do documento, mesmo quando o parser ainda não o lê;
- `lacunas_conhecidas`: campo → tarefa que vai ensiná-lo a ler (hoje só o DAS → T-504).
  Para esses campos o teste exige `None` (nunca um valor inventado), a métrica de acerto conta erro
  e o experimento no Langfuse mostra a lacuna nos metadados. Quando o parser passar a ler o campo,
  o teste quebra e a lacuna tem de sair do arquivo;
- `identificacao`: nome empresarial, sugestão de `sujeita_fator_r` e início de atividade
  esperados (fora da confiança);
- `series_anteriores`: quantidade de meses das tabelas 2.2/2.3 e se conferem com RBT12 e FS12.

## Reais anonimizadas (P-03)

Só entram depois da revisão do usuário (CLAUDE.md §5.3). Nunca commitar extrato real sem
anonimização. Extratos brutos ficam fora do repositório (`fixtures/pgdas/real/` está no
`.gitignore`).

Como anonimizar:
1. Trocar CNPJ por um CNPJ válido fictício (manter os dígitos verificadores corretos).
2. Trocar nome empresarial, endereço e dados de sócios.
3. Manter os valores (RPA, RBT12, FS12, Fator r, DAS), que são o que o parser precisa ler, ou
   aplicar o mesmo fator multiplicativo em todos para preservar a consistência Fator r = FS12/RBT12.
4. Criar a pasta `real_<n>/` com `documento.pdf` e `expected.json` com `"sintetica": false`.
