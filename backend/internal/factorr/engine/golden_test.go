package engine

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"slices"
	"testing"

	"controle/internal/platform/competence"
	"controle/internal/platform/decimal"
)

// golden.json é gerado por testdata/golden_gen.py rodando o domínio Python da origem
// (fator_r/domain) sobre casos determinísticos: fronteiras de 28% e da meta, limites das
// faixas, RBT12 zero, acima do limite, empresa nova, meses faltantes e fora da janela, folha do
// PGDAS-D, políticas de CPP e metas variadas, carteiras com empates e simulações.
//
// TEST-010: o motor portado tem de dar exatamente o mesmo resultado, dígito a dígito — mesmo
// valor e mesmo expoente (str() do Python contra String() do decimal).
//
//go:embed testdata/golden.json
var goldenJSON []byte

type goldenMovement [7]any // [competência, receita, pró-labore, salários, CPP, FGTS, pgdas]

type goldenPolicy struct {
	CPP  bool   `json:"cpp"`
	Meta string `json:"meta"`
}

type goldenCalc struct {
	Name          string           `json:"nome"`
	PA            string           `json:"pa"`
	ActivityStart *string          `json:"inicio_atividade"`
	Policy        goldenPolicy     `json:"politica"`
	Movements     []goldenMovement `json:"movimentos"`
	Expected      map[string]any   `json:"esperado"`
}

type goldenCompany struct {
	ID            string           `json:"id"`
	Name          string           `json:"nome"`
	Fee           string           `json:"honorario"`
	ActivityStart *string          `json:"inicio_atividade"`
	Movements     []goldenMovement `json:"movimentos"`
}

type goldenPortfolio struct {
	PA        string          `json:"pa"`
	Policy    goldenPolicy    `json:"politica"`
	Companies []goldenCompany `json:"empresas"`
	Expected  struct {
		Order   []string       `json:"ordem"`
		Actions []string       `json:"acoes"`
		KPIs    map[string]any `json:"kpis"`
	} `json:"esperado"`
}

type goldenSimulation struct {
	Calc   int `json:"calculo"`
	Params struct {
		Target   string `json:"meta"`
		Fraction string `json:"fracao_pro_labore"`
		INSS     string `json:"inss"`
		IRRF     string `json:"irrf"`
		Horizon  int    `json:"horizonte_meses"`
		MinSave  string `json:"piso_economia_anual"`
	} `json:"parametros"`
	Expected map[string]any `json:"esperado"`
}

type golden struct {
	Source      string             `json:"fonte"`
	Calcs       []goldenCalc       `json:"calculos"`
	Portfolios  []goldenPortfolio  `json:"carteiras"`
	Simulations []goldenSimulation `json:"simulacoes"`
}

func loadGolden(t *testing.T) golden {
	t.Helper()
	var g golden
	if err := json.Unmarshal(goldenJSON, &g); err != nil {
		t.Fatalf("golden.json: %v", err)
	}
	if len(g.Calcs) < 200 || len(g.Portfolios) < 10 || len(g.Simulations) < 100 {
		t.Fatalf("golden.json incompleto: %d cálculos, %d carteiras, %d simulações", len(g.Calcs), len(g.Portfolios), len(g.Simulations))
	}
	return g
}

func (m goldenMovement) toEngine() Movement {
	s := func(i int) decimal.Decimal { return decimal.MustParse(m[i].(string)) }
	mv := Movement{
		Competence: competence.MustParse(m[0].(string)), GrossRevenue: s(1),
		ProLabore: s(2), Salaries: s(3), CPP: s(4), FGTS: s(5), Origin: "manual",
	}
	// Na origem a folha do PGDAS-D vem em salarios; aqui, no campo próprio (05 §4).
	if m[6].(float64) == 1 {
		total := mv.Salaries
		mv.Salaries, mv.DeclaredPayroll, mv.Origin = decimal.MustParse("0.00"), &total, "pgdas"
	}
	return mv
}

func movements(gs []goldenMovement) []Movement {
	out := make([]Movement, len(gs))
	for i, g := range gs {
		out[i] = g.toEngine()
	}
	return out
}

func compPtr(s *string) *competence.Competence {
	if s == nil {
		return nil
	}
	return ptr(competence.MustParse(*s))
}

func (p goldenPolicy) toEngine() Policy {
	return Policy{CPPInFS12: p.CPP, Target: decimal.MustParse(p.Meta)}
}

// text converte um campo do resultado Go na forma do golden (str() do Python ou null).
func text(v any) any {
	switch x := v.(type) {
	case decimal.Decimal:
		return x.String()
	case *decimal.Decimal:
		if x == nil {
			return nil
		}
		return x.String()
	case []competence.Competence:
		out := make([]any, len(x))
		for i, c := range x {
			out[i] = c.String()
		}
		return out
	case competence.Competence:
		return x.String()
	case *Reason:
		if x == nil {
			return nil
		}
		return string(*x)
	case *Annex:
		if x == nil {
			return nil
		}
		return string(*x)
	case *Light:
		if x == nil {
			return nil
		}
		return string(*x)
	case *Verdict:
		if x == nil {
			return nil
		}
		return string(*x)
	case Status:
		return string(x)
	case int:
		return float64(x)
	}
	return v
}

func compare(t *testing.T, where string, got, want map[string]any) {
	t.Helper()
	for field, w := range want {
		g, ok := got[field]
		if !ok {
			t.Errorf("%s: campo %s sem correspondente no Go", where, field)
			continue
		}
		if fmt.Sprint(g) != fmt.Sprint(w) {
			t.Errorf("%s: %s = %v; a origem dá %v", where, field, g, w)
		}
	}
}

func resultFields(r Result) map[string]any {
	return map[string]any{
		"pa": text(r.PA), "janela_inicio": text(r.WindowStart), "janela_fim": text(r.WindowEnd),
		"meses_preenchidos": text(r.FilledMonths), "meses_faltantes": text(r.MissingMonths),
		"status": text(r.Status), "motivo": text(r.Reason), "rbt12": text(r.RBT12), "fs12": text(r.FS12),
		"fator_r": text(r.FactorR), "anexo": text(r.Annex), "semaforo": text(r.Light),
		"folha_minima_28": text(r.MinPayroll28), "folha_minima_meta": text(r.MinPayrollTarget),
		"gap_12m_28": text(r.Gap28), "reforco_mensal_28": text(r.MonthlyTopUp28),
		"gap_12m_meta": text(r.GapTarget), "reforco_mensal_meta": text(r.MonthlyTopUpTarget),
		"aliquota_efetiva_iii": text(r.EffectiveRateIII), "aliquota_efetiva_v": text(r.EffectiveRateV),
		"economia_12m": text(r.Savings12m), "meta_operacional": text(r.Target),
		"cpp_integra_fs12": r.CPPInFS12, "tabela_vigencia_inicio": r.TableValidFrom.String(),
	}
}

func TestGolden_CalculateMatchesOriginDigitByDigit(t *testing.T) {
	g := loadGolden(t)
	tables := tables2018(t)
	for i, gc := range g.Calcs {
		r := Calculate(competence.MustParse(gc.PA), compPtr(gc.ActivityStart), movements(gc.Movements), tables, gc.Policy.toEngine())
		compare(t, fmt.Sprintf("cálculo %d (%s)", i, gc.Name), resultFields(r), gc.Expected)
	}
	t.Logf("%d cálculos idênticos à origem (%s)", len(g.Calcs), g.Source)
}

func TestGolden_PortfolioMatchesOrigin(t *testing.T) {
	g := loadGolden(t)
	tables := tables2018(t)
	for i, gp := range g.Portfolios {
		companies := make([]PortfolioCompany, len(gp.Companies))
		byID := map[string][]Movement{}
		for k, c := range gp.Companies {
			companies[k] = PortfolioCompany{ID: c.ID, Name: c.Name, MonthlyFee: decimal.MustParse(c.Fee), ActivityStart: compPtr(c.ActivityStart)}
			byID[c.ID] = movements(c.Movements)
		}
		p := BuildPortfolio(competence.MustParse(gp.PA), companies, byID, tables, gp.Policy.toEngine())
		var order, actions []string
		for _, l := range p.Lines {
			order, actions = append(order, l.Company.ID), append(actions, string(l.Action))
		}
		where := fmt.Sprintf("carteira %d", i)
		if !slices.Equal(order, gp.Expected.Order) {
			t.Errorf("%s: ordem %v; a origem dá %v", where, order, gp.Expected.Order)
		}
		if !slices.Equal(actions, gp.Expected.Actions) {
			t.Errorf("%s: ações %v; a origem dá %v", where, actions, gp.Expected.Actions)
		}
		k := p.KPIs
		compare(t, where, map[string]any{
			"monitoradas": text(k.Monitored), "no_v": text(k.InAnnexV), "no_limite": text(k.AtLimit),
			"seguras": text(k.Safe), "dados_insuficientes": text(k.Insufficient),
			"economia_em_jogo": text(k.SavingsAtPlay), "honorarios_pacotes": text(k.PackageFees),
		}, gp.Expected.KPIs)
	}
}

func TestGolden_SimulateMatchesOrigin(t *testing.T) {
	g := loadGolden(t)
	tables := tables2018(t)
	for i, gs := range g.Simulations {
		gc := g.Calcs[gs.Calc]
		r := Calculate(competence.MustParse(gc.PA), compPtr(gc.ActivityStart), movements(gc.Movements), tables, gc.Policy.toEngine())
		s := Simulate(r, SimulationParams{
			Target: decimal.MustParse(gs.Params.Target), ProLaboreFraction: decimal.MustParse(gs.Params.Fraction),
			INSS: decimal.MustParse(gs.Params.INSS), IRRF: decimal.MustParse(gs.Params.IRRF),
			HorizonMonths: gs.Params.Horizon, MinAnnualSavings: decimal.MustParse(gs.Params.MinSave),
		})
		compare(t, fmt.Sprintf("simulação %d (cálculo %d)", i, gs.Calc), map[string]any{
			"status": text(s.Status), "motivo": text(s.Reason), "veredito": text(s.Verdict),
			"fator_atual": text(s.CurrentFactor), "reforco_12m": text(s.TopUp12m), "reforco_mensal": text(s.MonthlyTopUp),
			"pro_labore_extra_mensal": text(s.ExtraProLaboreMonthly), "pro_labore_extra_horizonte": text(s.ExtraProLaboreHorizon),
			"custo_inss": text(s.INSSCost), "custo_irrf": text(s.IRRFCost), "custo_total": text(s.TotalCost),
			"economia_12m": text(s.Savings12m), "economia_horizonte": text(s.SavingsHorizon), "liquido": text(s.Net),
		}, gs.Expected)
	}
}
