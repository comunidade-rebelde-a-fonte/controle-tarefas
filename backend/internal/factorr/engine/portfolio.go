package engine

import (
	"slices"
	"strings"

	"controle/internal/platform/competence"
	"controle/internal/platform/decimal"
)

// Action é a ação sugerida de uma linha da carteira (Acao).
type Action string

// Ações sugeridas.
const (
	ActionCorrectionMeeting Action = "reuniao_correcao"
	ActionWatchSimulate     Action = "vigiar_simular"
	ActionKeep              Action = "manter"
	ActionCompleteEntries   Action = "completar_lancamentos"
)

// ActionTexts são os textos das ações sugeridas: fonte única (ACOES).
var ActionTexts = map[Action]string{
	ActionCorrectionMeeting: "reunião de correção",
	ActionWatchSimulate:     "vigiar / simular",
	ActionKeep:              "manter",
	ActionCompleteEntries:   "completar lançamentos",
}

// SuggestedAction é a ação de acordo com o semáforo; sem semáforo, completar os lançamentos
// (acao_sugerida).
func SuggestedAction(r Result) Action {
	if r.Light == nil {
		return ActionCompleteEntries
	}
	switch *r.Light {
	case LightRed:
		return ActionCorrectionMeeting
	case LightYellow:
		return ActionWatchSimulate
	default:
		return ActionKeep
	}
}

// PortfolioCompany é uma empresa monitorada (EmpresaCarteira).
type PortfolioCompany struct {
	ID            string
	Name          string // nome: desempate da ordenação
	Package       *string
	MonthlyFee    decimal.Decimal // honorario_mensal
	ActivityStart *competence.Competence
}

// PortfolioLine é uma linha da carteira (LinhaCarteira).
type PortfolioLine struct {
	Company PortfolioCompany
	Result  Result
	Action  Action
}

// KPIs são os indicadores da carteira (KpisCarteira).
type KPIs struct {
	Monitored     int             `json:"monitoradas"`
	InAnnexV      int             `json:"no_v"`
	AtLimit       int             `json:"no_limite"`
	Safe          int             `json:"seguras"`
	Insufficient  int             `json:"dados_insuficientes"`
	SavingsAtPlay decimal.Decimal `json:"economia_em_jogo"`
	PackageFees   decimal.Decimal `json:"honorarios_pacotes"`
}

// Portfolio é a carteira do escritório num PA (Carteira).
type Portfolio struct {
	PA    competence.Competence
	Lines []PortfolioLine
	KPIs  KPIs
}

func lightRank(l *Light) int {
	if l == nil {
		return 3
	}
	switch *l {
	case LightRed:
		return 0
	case LightYellow:
		return 1
	default:
		return 2
	}
}

// comparePortfolioLines ordena por semáforo (vermelho, amarelo, verde, sem dados), maior
// economia primeiro (sem economia por último no grupo) e nome (_chave_ordem). A comparação de
// nomes é por ponto de código, como no Python.
func comparePortfolioLines(a, b PortfolioLine) int {
	if ra, rb := lightRank(a.Result.Light), lightRank(b.Result.Light); ra != rb {
		return ra - rb
	}
	sa, sb := a.Result.Savings12m, b.Result.Savings12m
	switch {
	case sa != nil && sb != nil:
		if c := sb.Cmp(*sa); c != 0 {
			return c
		}
	case sa != nil:
		return -1
	case sb != nil:
		return 1
	}
	return strings.Compare(a.Company.Name, b.Company.Name)
}

// BuildPortfolio calcula cada empresa com o mesmo motor, ordena e totaliza (montar_carteira).
// movements traz os lançamentos da janela por empresa.
func BuildPortfolio(pa competence.Competence, companies []PortfolioCompany, movements map[string][]Movement, tables Tables, p Policy) Portfolio {
	lines := make([]PortfolioLine, 0, len(companies))
	for _, c := range companies {
		r := Calculate(pa, c.ActivityStart, movements[c.ID], tables, p)
		lines = append(lines, PortfolioLine{Company: c, Result: r, Action: SuggestedAction(r)})
	}
	slices.SortStableFunc(lines, comparePortfolioLines)

	k := KPIs{Monitored: len(lines), SavingsAtPlay: decimal.Zero, PackageFees: decimal.Zero}
	for _, l := range lines {
		r := l.Result
		if r.Annex != nil && *r.Annex == AnnexV {
			k.InAnnexV++
		}
		switch {
		case r.Light == nil:
			k.Insufficient++
		case *r.Light == LightYellow:
			k.AtLimit++
		case *r.Light == LightGreen:
			k.Safe++
		}
		// Economia em jogo: a economia positiva de quem está no V ou no limite.
		if r.Light != nil && (*r.Light == LightRed || *r.Light == LightYellow) &&
			r.Savings12m != nil && r.Savings12m.Sign() > 0 {
			k.SavingsAtPlay = k.SavingsAtPlay.Add(*r.Savings12m)
		}
	}
	for _, c := range companies {
		k.PackageFees = k.PackageFees.Add(c.MonthlyFee)
	}
	return Portfolio{PA: pa, Lines: lines, KPIs: k}
}
