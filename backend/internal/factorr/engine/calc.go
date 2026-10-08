package engine

import (
	"controle/internal/platform/civil"
	"controle/internal/platform/competence"
	"controle/internal/platform/decimal"
)

// LegalCut é o corte legal (LC 123/2006, art. 18 §§ 5º-J e 5º-M): Fator R ≥ 28% → Anexo III.
// A meta operacional nunca muda o anexo (CORTE_LEGAL).
var LegalCut = decimal.MustParse("0.28")

var twelve = decimal.MustParse("12") // DOZE

// Status do cálculo.
type Status string

// Status possíveis.
const (
	StatusOK           Status = "ok"
	StatusInsufficient Status = "dados_insuficientes"
)

// Reason explica um cálculo com dados insuficientes (Motivo).
type Reason string

// Motivos, em ordem de precedência.
const (
	// ReasonNewCompany: menos de 12 meses na janela. A proporcionalização (Res. CGSN 140/2018,
	// arts. 22 e 26) está pendente de conferência (P-05 da origem).
	ReasonNewCompany       Reason = "empresa_nova_regra_pendente"
	ReasonRBT12Zero        Reason = "rbt12_zero"
	ReasonAboveSimplesCeil Reason = "rbt12_acima_do_limite_do_simples"
)

// Light é o semáforo operacional (Semaforo).
type Light string

// Semáforos.
const (
	LightRed    Light = "vermelho" // < 28%: Anexo V
	LightYellow Light = "amarelo"  // 28% até a meta
	LightGreen  Light = "verde"    // na meta ou acima
)

// Movement é um mês da série (MovimentoMes). Os valores vêm do banco com a escala da coluna.
type Movement struct {
	Competence   competence.Competence
	GrossRevenue decimal.Decimal // receita_bruta
	ProLabore    decimal.Decimal // pro_labore
	Salaries     decimal.Decimal // salarios_13_ferias (salarios na origem)
	CPP          decimal.Decimal // cpp_recolhida (cpp)
	FGTS         decimal.Decimal // fgts_recolhido (fgts)
	// DeclaredPayroll é a folha total declarada no PGDAS-D, sem divisão. Na origem ela ia
	// inteira em salarios, com o resto zerado; aqui tem coluna própria (05 §4) e o resultado é
	// o mesmo.
	DeclaredPayroll *decimal.Decimal
	Origin          string
}

// Policy é a política do escritório usada no cálculo (Politica).
type Policy struct {
	CPPInFS12 bool            // cpp_das_integra_fs12
	Target    decimal.Decimal // meta_operacional
}

// Result é o resultado do Fator R num PA (ResultadoFatorR). Os campos opcionais ficam nulos
// com dados insuficientes: nunca se inventa anexo, alíquota ou economia.
type Result struct {
	PA                 competence.Competence   `json:"pa"`
	WindowStart        competence.Competence   `json:"janela_inicio"`
	WindowEnd          competence.Competence   `json:"janela_fim"`
	FilledMonths       []competence.Competence `json:"meses_preenchidos"`
	MissingMonths      []competence.Competence `json:"meses_faltantes"`
	Status             Status                  `json:"status"`
	Reason             *Reason                 `json:"motivo"`
	RBT12              decimal.Decimal         `json:"rbt12"`
	FS12               decimal.Decimal         `json:"fs12"`
	FactorR            *decimal.Decimal        `json:"fator_r"`
	Annex              *Annex                  `json:"anexo"`
	Light              *Light                  `json:"semaforo"`
	MinPayroll28       *decimal.Decimal        `json:"folha_minima_28"`
	MinPayrollTarget   *decimal.Decimal        `json:"folha_minima_meta"`
	Gap28              *decimal.Decimal        `json:"gap_12m_28"`
	MonthlyTopUp28     *decimal.Decimal        `json:"reforco_mensal_28"`
	GapTarget          *decimal.Decimal        `json:"gap_12m_meta"`
	MonthlyTopUpTarget *decimal.Decimal        `json:"reforco_mensal_meta"`
	EffectiveRateIII   *decimal.Decimal        `json:"aliquota_efetiva_iii"`
	EffectiveRateV     *decimal.Decimal        `json:"aliquota_efetiva_v"`
	Savings12m         *decimal.Decimal        `json:"economia_12m"`
	Target             decimal.Decimal         `json:"meta_operacional"`
	CPPInFS12          bool                    `json:"cpp_integra_fs12"`
	TableValidFrom     civil.Date              `json:"tabela_vigencia_inicio"`
}

// PayrollForFS12 é a folha do mês na FS12: remunerações com INSS + FGTS + CPP, se a política
// mandar (folha_para_fs12). Com folha total declarada no PGDAS-D, ela é a folha do mês.
func PayrollForFS12(m Movement, p Policy) decimal.Decimal {
	if m.DeclaredPayroll != nil {
		return *m.DeclaredPayroll
	}
	payroll := m.ProLabore.Add(m.Salaries).Add(m.FGTS)
	if p.CPPInFS12 {
		return payroll.Add(m.CPP)
	}
	return payroll
}

// EffectiveRate é a alíquota efetiva do anexo para o RBT12: (RBT12 × nominal − parcela a
// deduzir) ÷ RBT12; nil acima do limite do Simples ou sem receita (aliquota_efetiva).
func EffectiveRate(t AnnexTable, rbt12 decimal.Decimal) *decimal.Decimal {
	b := t.BracketFor(rbt12)
	if b == nil || rbt12.Sign() <= 0 {
		return nil
	}
	r := rbt12.Mul(b.NominalRate).Sub(b.Deduction).Quo(rbt12)
	return &r
}

// LightFor classifica o fator: vermelho abaixo do corte legal, amarelo abaixo da meta, verde
// a partir dela (semaforo_para).
func LightFor(factor, target decimal.Decimal) Light {
	if factor.Cmp(LegalCut) < 0 {
		return LightRed
	}
	if factor.Cmp(target) < 0 {
		return LightYellow
	}
	return LightGreen
}

func ptr[T any](v T) *T { return &v }

// Calculate calcula o Fator R do PA (calcular). Só os movimentos dos meses válidos da janela
// contam; mês faltante soma zero e aparece em MissingMonths.
func Calculate(pa competence.Competence, activityStart *competence.Competence, movements []Movement, tables Tables, p Policy) Result {
	w := NewWindow(pa)
	valid := ValidMonths(w, activityStart)
	isValid := make(map[competence.Competence]bool, len(valid))
	for _, m := range valid {
		isValid[m] = true
	}
	// Um movimento por mês; repetido, vale o último (dicionário da origem), na posição do primeiro.
	byMonth := map[competence.Competence]Movement{}
	var order []competence.Competence
	for _, m := range movements {
		if !isValid[m.Competence] {
			continue
		}
		if _, seen := byMonth[m.Competence]; !seen {
			order = append(order, m.Competence)
		}
		byMonth[m.Competence] = m
	}
	posted := make(map[competence.Competence]bool, len(byMonth))
	for c := range byMonth {
		posted[c] = true
	}
	filled, missing := Classify(valid, posted)

	rbt12, fs12 := decimal.Zero, decimal.Zero
	for _, c := range order {
		rbt12 = rbt12.Add(byMonth[c].GrossRevenue)
	}
	for _, c := range order {
		fs12 = fs12.Add(PayrollForFS12(byMonth[c], p))
	}

	r := Result{
		PA: w.PA, WindowStart: w.Start, WindowEnd: w.End,
		FilledMonths: filled, MissingMonths: missing,
		RBT12: rbt12, FS12: fs12,
		Target: p.Target, CPPInFS12: p.CPPInFS12,
		TableValidFrom: civil.Of(tables.III.ValidFrom),
	}

	var reason *Reason
	var rateIII, rateV *decimal.Decimal
	switch {
	case NewCompanyInWindow(w, activityStart):
		reason = ptr(ReasonNewCompany)
	case rbt12.Sign() <= 0:
		reason = ptr(ReasonRBT12Zero)
	default:
		rateIII, rateV = EffectiveRate(tables.III, rbt12), EffectiveRate(tables.V, rbt12)
		if rateIII == nil || rateV == nil {
			reason = ptr(ReasonAboveSimplesCeil)
		}
	}
	if reason != nil {
		r.Status, r.Reason = StatusInsufficient, reason
		return r
	}

	factor := fs12.Quo(rbt12)
	min28 := LegalCut.Mul(rbt12)
	minTarget := p.Target.Mul(rbt12)
	gap28 := decimal.Max(decimal.Zero, min28.Sub(fs12))
	gapTarget := decimal.Max(decimal.Zero, minTarget.Sub(fs12))
	annex := AnnexV
	if factor.Cmp(LegalCut) >= 0 {
		annex = AnnexIII
	}
	r.Status = StatusOK
	r.FactorR = &factor
	r.Annex = &annex
	r.Light = ptr(LightFor(factor, p.Target))
	r.MinPayroll28 = &min28
	r.MinPayrollTarget = &minTarget
	r.Gap28 = &gap28
	r.MonthlyTopUp28 = ptr(gap28.Quo(twelve))
	r.GapTarget = &gapTarget
	r.MonthlyTopUpTarget = ptr(gapTarget.Quo(twelve))
	r.EffectiveRateIII = rateIII
	r.EffectiveRateV = rateV
	r.Savings12m = ptr(rateV.Sub(*rateIII).Mul(rbt12))
	return r
}
