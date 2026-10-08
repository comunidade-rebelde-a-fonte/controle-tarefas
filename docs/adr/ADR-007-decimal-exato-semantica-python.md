# ADR-007 — Decimal exato com a semântica do decimal do Python

Status: Accepted (07/10/2026, decisão técnica da TASK-010 dentro da autonomia da spec; registrada em D-018)

## Contexto

O motor do Fator R de origem (`fator_r/domain`) faz todas as contas com o `Decimal` do Python, no contexto padrão: 28 dígitos significativos, arredondamento half-even e as regras da especificação General Decimal Arithmetic (GDA). A quantização acontece só na apresentação. A TASK-010 exige "dinheiro em precisão exata" e o golden do projeto de origem; o `CLAUDE.md` proíbe `float` para dinheiro.

O projeto-base não tinha coluna `numeric` nem tipo decimal. Em Go:

- `math/big` não implementa contexto, arredondamento nem expoente ideal;
- `github.com/cockroachdb/apd/v3` implementa a GDA e é a biblioteca decimal do CockroachDB. Mesmo assim, difere do Python em dois pontos, encontrados no porte:
  1. **Divisão exata.** O apd devolve o quociente com 28 dígitos (`0.2799000…0`); o Python reduz ao expoente ideal (`0.2799`). O valor é o mesmo, mas a representação não. Os testes GDA do próprio apd ignoram zeros à direita, por isso a diferença não aparece neles.
  2. **Zero de expoente muito negativo.** O apd escreve `0.000000000`, como o Postgres; o Python escreve `0E-9`.
- O pgx devolve o zero de uma coluna `numeric(14,2)` com expoente 0 (`0`, e não `0.00`).

## Decisão

1. Tipo `decimal.Decimal` (`backend/internal/platform/decimal`), imutável, sobre o apd v3, com contexto de 28 dígitos e half-even. As condições que o Python transforma em exceção (divisão por zero, operação inválida, estouro) causam pânico; o domínio garante, como na origem, que não ocorrem.
2. `Quo` reduz o resultado ao expoente ideal quando a divisão é exata, como o Python. `String()` reproduz o `str()` do Python, inclusive o zero. `Max` segue o `max()` do Python, que devolve o primeiro argumento no empate.
3. Na API, valor decimal é string em ponto fixo (`Text()`), nunca número JSON [06 §12]. A entrada também é string. A interface formata com arredondamento half-even feito por string, sem `float` (`frontend/src/lib/decimal.ts`).
4. Ao montar o motor, o serviço reaplica a escala da coluna a cada valor lido (dinheiro com 2 casas, percentuais com 6). Isso é exato, porque a coluna já tem essa escala, e reproduz o que a origem lia do banco.
5. O sqlc mapeia `numeric` para `decimal.Decimal` (`sqlc.yaml`).

## Evidência

O golden (`backend/internal/factorr/engine/testdata/golden.json`) foi gerado pelo próprio código Python da origem (`golden_gen.py`, semente fixa). Ele contém 233 cálculos, 18 carteiras e 160 simulações, cobrindo as fronteiras de 28% e da meta, os limites das faixas, RBT12 zero e acima do limite, empresa nova e folha do PGDAS-D.

O motor em Go dá resultados **idênticos dígito a dígito**: mesmo valor e mesmo expoente, comparando `str()` com `String()`. A primeira execução do golden revelou a diferença 2 (zero), corrigida no `String()`.

## Consequências

- Uma dependência nova (`cockroachdb/apd/v3`), pequena e mantida, isolada no pacote `decimal`.
- Qualquer conta nova com dinheiro (Conciliação, simulador) usa este tipo; o golden do Fator R serve de trava de regressão para a aritmética.
- Os valores da API podem ter muitas casas (`43740.00000000`). É a representação exata do cálculo; quem arredonda é a interface, e só para exibir.
