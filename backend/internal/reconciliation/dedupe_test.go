package reconciliation

import (
	_ "embed"
	"encoding/json"
	"slices"
	"testing"
	"time"

	"controle/internal/platform/decimal"
)

// dedupe_golden.json é gerado por testdata/dedupe_gen.py rodando app/domain/dedupe.py da
// origem sobre descrições com casos difíceis de Unicode (ligaduras, ß, marcas combinantes,
// espaços exóticos, escritas com diacríticos). A chave é contrato congelado [05 §10]: tem de
// ser igual, caractere a caractere.
//
//go:embed testdata/dedupe_golden.json
var dedupeGolden []byte

type dedupeCase struct {
	Account     string `json:"conta"`
	Date        string `json:"data"`
	Amount      string `json:"valor"`
	Direction   string `json:"sentido"`
	Description string `json:"descricao"`
	Order       int    `json:"ordem_no_dia"`
	Normalized  string `json:"normalizada"`
	Key         string `json:"chave"`
}

func TestTEST007_DedupeKeyMatchesOrigin(t *testing.T) {
	var g struct {
		Cases  []dedupeCase `json:"casos"`
		Orders struct {
			Dates    []string `json:"datas"`
			Expected []int    `json:"esperado"`
		} `json:"ordens_no_dia"`
	}
	if err := json.Unmarshal(dedupeGolden, &g); err != nil {
		t.Fatal(err)
	}
	if len(g.Cases) < 40 {
		t.Fatalf("golden incompleto: %d casos", len(g.Cases))
	}
	for i, c := range g.Cases {
		if got := NormalizeDescription(c.Description); got != c.Normalized {
			t.Errorf("caso %d: NormalizeDescription(%q) = %q; a origem dá %q", i, c.Description, got, c.Normalized)
		}
		d, err := time.Parse(time.DateOnly, c.Date)
		if err != nil {
			t.Fatal(err)
		}
		if got := DedupeKey(c.Account, d, decimal.MustParse(c.Amount), c.Direction, c.Description, c.Order); got != c.Key {
			t.Errorf("caso %d (%q): chave %s; a origem dá %s", i, c.Description, got, c.Key)
		}
	}
	dates := make([]time.Time, len(g.Orders.Dates))
	for i, s := range g.Orders.Dates {
		dates[i], _ = time.Parse(time.DateOnly, s)
	}
	if got := OrdersInDay(dates); !slices.Equal(got, g.Orders.Expected) {
		t.Fatalf("OrdersInDay = %v; a origem dá %v", got, g.Orders.Expected)
	}
}

// tests/unit/test_dedupe.py da origem.
func TestDedupe_OriginUnitCases(t *testing.T) {
	if got := NormalizeDescription("  Pix  JOÃO  "); got != "pix joao" {
		t.Fatalf("normalizar = %q", got)
	}
	day := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	a := DedupeKey("123", day, decimal.MustParse("10.00"), "D", "PIX João", 1)
	b := DedupeKey("123", day, decimal.MustParse("10.00"), "D", "pix JOÃO", 1)
	c := DedupeKey("123", day, decimal.MustParse("10.00"), "D", "pix joao", 2)
	if a != b || a == c {
		t.Fatalf("chaves: a=%s b=%s c=%s", a, b, c)
	}
	// O valor entra com 2 casas: 10 e 10.00 são a mesma transação.
	if DedupeKey("123", day, decimal.MustParse("10"), "D", "x", 1) != DedupeKey("123", day, decimal.MustParse("10.00"), "D", "x", 1) {
		t.Fatal("o valor deve entrar quantizado em 2 casas")
	}
}
