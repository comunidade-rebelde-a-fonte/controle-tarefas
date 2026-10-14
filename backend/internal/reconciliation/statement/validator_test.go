package statement

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"math/rand/v2"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"controle/internal/platform/decimal"
)

// Testes de propriedade do portão aritmético (TEST-016), no lugar do hypothesis da origem
// (test_propriedade_saldo): sementes fixas, para o teste ser determinístico.

func cents(r *rand.Rand, lo, hi int64) decimal.Decimal {
	v := lo + r.Int64N(hi-lo+1)
	return decimal.FromInt(v).Quo(decimal.FromInt(100)).Quantize(-2)
}

func randomStatement(r *rand.Rand) Statement {
	s := Statement{
		Bank: "itau", Branch: "1", Account: "1",
		PeriodStart: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), PeriodEnd: time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC),
		OpeningBalance: cents(r, -1_000_000, 1_000_000),
	}
	closing := s.OpeningBalance
	order := 1
	for _, dir := range []string{Credit, Debit} {
		for range r.IntN(9) {
			v := cents(r, 1, 100_000)
			s.Transactions = append(s.Transactions, Transaction{
				Order: order, Date: time.Date(2026, 3, min(28, order), 0, 0, 0, 0, time.UTC), Amount: v, Direction: dir,
			})
			if dir == Credit {
				closing = closing.Add(v)
			} else {
				closing = closing.Sub(v)
			}
			order++
		}
	}
	s.ClosingBalance = closing.Quantize(-2)
	return s
}

// saldo_inicial + ΣC − ΣD = saldo_final fecha com diferença exatamente zero.
func TestValidate_PropertyBalanceCloses(t *testing.T) {
	r := rand.New(rand.NewPCG(16, 2026))
	for range 2000 {
		s := randomStatement(r)
		res := Validate(s)
		if !res.OK || !res.Difference.IsZero() {
			b, _ := json.Marshal(res)
			t.Fatalf("extrato que fecha reprovado: %s", b)
		}
		// A ordem das linhas não muda as somas.
		shuffled := s
		shuffled.Transactions = slices.Clone(s.Transactions)
		r.Shuffle(len(shuffled.Transactions), func(i, j int) {
			shuffled.Transactions[i], shuffled.Transactions[j] = shuffled.Transactions[j], shuffled.Transactions[i]
		})
		if again := Validate(shuffled); again.Credits.Cmp(res.Credits) != 0 || again.Debits.Cmp(res.Debits) != 0 || !again.OK {
			t.Fatal("a ordem das linhas mudou as somas")
		}
	}
}

// Qualquer diferença diferente de R$ 0,00, por menor que seja, reprova [AC-016].
func TestValidate_PropertyZeroTolerance(t *testing.T) {
	r := rand.New(rand.NewPCG(21, 2026))
	for range 2000 {
		s := randomStatement(r)
		delta := cents(r, 1, 50_000)
		if r.IntN(2) == 0 {
			delta = delta.Neg()
		}
		s.ClosingBalance = s.ClosingBalance.Add(delta).Quantize(-2)
		res := Validate(s)
		if res.OK {
			t.Fatalf("diferença de %s aprovada", delta)
		}
		if res.Difference.Cmp(delta.Neg()) != 0 {
			t.Fatalf("diferença = %s; esperado %s", res.Difference, delta.Neg())
		}
	}
	// O centavo da origem (test_um_centavo_reprova).
	one := Statement{
		PeriodStart: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), PeriodEnd: time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC),
		OpeningBalance: decimal.MustParse("100.00"), ClosingBalance: decimal.MustParse("150.01"),
		Transactions: []Transaction{{Order: 1, Date: time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC), Amount: decimal.MustParse("50.00"), Direction: Credit}},
	}
	if res := Validate(one); res.OK || res.Difference.String() != "-0.01" {
		t.Fatalf("um centavo: ok=%v diferença=%s", res.OK, res.Difference)
	}
}

// Linha pulada: a diferença é o valor dela e a dica aponta pulo ou duplicata.
func TestValidate_PropertySkippedLineHint(t *testing.T) {
	r := rand.New(rand.NewPCG(7, 2026))
	checked := 0
	for range 2000 {
		s := randomStatement(r)
		if len(s.Transactions) < 2 {
			continue
		}
		i := r.IntN(len(s.Transactions))
		skipped := s.Transactions[i]
		s.Transactions = slices.Delete(slices.Clone(s.Transactions), i, i+1)
		res := Validate(s)
		if res.OK || res.Difference.Abs().Cmp(skipped.Amount) != 0 {
			t.Fatalf("linha pulada de %s: ok=%v diferença=%s", skipped.Amount, res.OK, res.Difference)
		}
		hints := Hints(s, res)
		has := slices.Contains(hints, HintSkippedOrDuplicated)
		// A dica só aparece se outra linha tiver o mesmo valor da diferença (a origem compara com
		// as linhas extraídas).
		want := slices.ContainsFunc(s.Transactions, func(t Transaction) bool { return t.Amount.Cmp(skipped.Amount) == 0 })
		if has != want {
			t.Fatalf("dica de linha pulada = %v; esperado %v (%v)", has, want, hints)
		}
		checked++
	}
	if checked < 1000 {
		t.Fatalf("poucos casos: %d", checked)
	}
}

// Datas fora do período e lacunas na ordem só sinalizam; o saldo decide (a origem:
// test_data_fora_periodo_sinaliza_mas_saldo_pode_ok e test_lacuna_ordem).
func TestValidate_FlagsDoNotRejectAndRejectedLinesDo(t *testing.T) {
	d := func(m, day int) time.Time { return time.Date(2026, time.Month(m), day, 0, 0, 0, 0, time.UTC) }
	s := Statement{
		PeriodStart: d(3, 1), PeriodEnd: d(3, 31), OpeningBalance: decimal.MustParse("0.00"), ClosingBalance: decimal.MustParse("20.00"),
		Transactions: []Transaction{
			{Order: 1, Date: d(4, 2), Amount: decimal.MustParse("10.00"), Direction: Credit},
			{Order: 3, Date: d(3, 5), Amount: decimal.MustParse("10.00"), Direction: Credit},
		},
	}
	res := Validate(s)
	if !res.OK || !slices.Equal(res.DatesOutside, []int{1}) || !slices.Equal(res.OrderGaps, []int{2}) {
		t.Fatalf("sinais = %+v", res)
	}
	// Linha com valor zero ou sentido inválido reprova o lote mesmo com o saldo fechando.
	s.Transactions = append(s.Transactions, Transaction{Order: 2, Date: d(3, 6), Amount: decimal.MustParse("0.00"), Direction: "X"})
	if res := Validate(s); res.OK || len(res.RejectedLines) != 2 {
		t.Fatalf("linhas rejeitadas = %+v", res.RejectedLines)
	}
}

// Nenhum float no dinheiro: os arquivos do domínio não usam float32/float64 nem literal real
// (test_validador_nao_usa_float_em_dinheiro da origem). O repr do float na confiança fica no
// pacote decimal.
func TestDomain_NoFloat(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") || f == "pyjson.go" {
			continue // pyjson.go lê o número do JSON como o Python (int ou float) antes do Decimal
		}
		node, err := parser.ParseFile(token.NewFileSet(), f, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(node, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.BasicLit:
				if x.Kind == token.FLOAT {
					t.Errorf("%s: literal real %s", f, x.Value)
				}
			case *ast.Ident:
				if x.Name == "float64" || x.Name == "float32" {
					t.Errorf("%s: uso de %s", f, x.Name)
				}
			}
			return true
		})
	}
}
