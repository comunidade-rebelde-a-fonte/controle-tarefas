package engine

import (
	"slices"
	"testing"

	"controle/internal/platform/decimal"
)

// test_carteira.py

var policyPortfolio = Policy{CPPInFS12: false, Target: d("0.30")}

func company(name, fee string) PortfolioCompany {
	return PortfolioCompany{ID: "id-" + name, Name: name, MonthlyFee: d(fee)}
}

func revenueSeries(revenue, proLabore string) []Movement {
	return series(mv{revenue: revenue, proLabore: proLabore})
}

func names(p Portfolio) []string {
	out := make([]string, len(p.Lines))
	for i, l := range p.Lines {
		out[i] = l.Company.Name
	}
	return out
}

// AC-007 e REQ-011: ordenação por semáforo e economia, ação sugerida e KPIs.
func TestPortfolio_OrderByLightAndSavingsAndKPIs(t *testing.T) {
	green, yellow := company("Verde", "300"), company("Amarelo", "200")
	redSmall, redLarge := company("Vermelho pequeno", "100"), company("Vermelho grande", "150")
	noData := company("Sem dados", "0")
	movements := map[string][]Movement{
		green.ID:    revenueSeries("50000", "15000"), // 30%
		yellow.ID:   revenueSeries("50000", "14500"), // 29%
		redSmall.ID: revenueSeries("15000", "1000"),  // RBT12 180 mil: economia menor
		redLarge.ID: revenueSeries("50000", "1000"),  // RBT12 600 mil: economia maior
	}
	p := BuildPortfolio(testPA, []PortfolioCompany{green, noData, redSmall, yellow, redLarge}, movements, tables2018(t), policyPortfolio)

	if got, want := names(p), []string{"Vermelho grande", "Vermelho pequeno", "Amarelo", "Verde", "Sem dados"}; !slices.Equal(got, want) {
		t.Fatalf("ordem = %v; esperado %v", got, want)
	}
	var texts []string
	for _, l := range p.Lines {
		texts = append(texts, ActionTexts[l.Action])
	}
	if want := []string{"reunião de correção", "reunião de correção", "vigiar / simular", "manter", "completar lançamentos"}; !slices.Equal(texts, want) {
		t.Fatalf("ações = %v; esperado %v", texts, want)
	}
	k := p.KPIs
	if k.Monitored != 5 || k.InAnnexV != 2 || k.AtLimit != 1 || k.Safe != 1 || k.Insufficient != 1 {
		t.Fatalf("KPIs = %+v", k)
	}
	eq(t, "honorarios_pacotes", &k.PackageFees, "750")
	want := decimal.Zero
	for _, l := range p.Lines {
		if l.Result.Light != nil && (*l.Result.Light == LightRed || *l.Result.Light == LightYellow) && l.Result.Savings12m != nil {
			want = want.Add(*l.Result.Savings12m)
		}
	}
	if k.SavingsAtPlay.Cmp(want) != 0 || k.SavingsAtPlay.Sign() <= 0 {
		t.Fatalf("economia em jogo = %s; esperado %s (> 0)", k.SavingsAtPlay, want)
	}
}

func TestPortfolio_SavingsTieBreaksByName(t *testing.T) {
	b, a := company("Beta", "0"), company("Alfa", "0")
	movements := map[string][]Movement{b.ID: revenueSeries("50000", "1000"), a.ID: revenueSeries("50000", "1000")}
	p := BuildPortfolio(testPA, []PortfolioCompany{b, a}, movements, tables2018(t), policyPortfolio)
	if got := names(p); !slices.Equal(got, []string{"Alfa", "Beta"}) {
		t.Fatalf("ordem = %v", got)
	}
}

func TestPortfolio_Empty(t *testing.T) {
	p := BuildPortfolio(testPA, nil, nil, tables2018(t), policyPortfolio)
	if len(p.Lines) != 0 || p.KPIs.Monitored != 0 || !p.KPIs.SavingsAtPlay.IsZero() {
		t.Fatalf("carteira vazia = %+v", p)
	}
}

func TestPortfolio_ActionTextsAreSingleSource(t *testing.T) {
	var got []string
	for _, v := range ActionTexts {
		got = append(got, v)
	}
	slices.Sort(got)
	if want := []string{"completar lançamentos", "manter", "reunião de correção", "vigiar / simular"}; !slices.Equal(got, want) {
		t.Fatalf("textos = %v", got)
	}
}
