package dashboard

import (
	"context"
	"strings"

	co "controle/internal/company"
	"controle/internal/factorr"
	"controle/internal/factorr/engine"
	comp "controle/internal/platform/competence"
	"controle/internal/reconciliation/overview"
	"controle/internal/reconciliation/review"
	"controle/internal/store"
)

// Seções Fator R e Conciliação do Dashboard (PRD §8, 08 §3; TASK-024, D-038). Cada número sai
// das mesmas funções das listas do drill-down (a carteira, a fila, os documentos e a Conciliação
// por empresa), com os filtros de empresa do Dashboard na URL: o número e a lista batem.

// MoneyMetric é um valor em reais clicável, em texto decimal.
type MoneyMetric struct {
	Value     string     `json:"value"`
	Drilldown *Drilldown `json:"drilldown"`
}

// FactorRPriority é uma empresa que exige ação, na ordem da carteira (semáforo e economia).
type FactorRPriority struct {
	Company    factorr.CompanyRef `json:"company"`
	Light      *engine.Light      `json:"semaforo"`
	FactorR    *string            `json:"fator_r"`
	Savings12m *string            `json:"economia_12m"`
	Action     engine.Action      `json:"acao"`
	ActionText string             `json:"acao_texto"`
	Drilldown  *Drilldown         `json:"drilldown"`
}

// FactorRSection é a carteira do Fator R no PA da competência do Dashboard, com o responsável do
// Fiscal no filtro (decisão do usuário).
type FactorRSection struct {
	PA comp.Competence `json:"pa"`
	// Unavailable é o motivo de a carteira não poder ser calculada (sem tabela do Simples vigente
	// no PA); os números ficam zerados.
	Unavailable     *string `json:"unavailable"`
	MetaOperacional string  `json:"meta_operacional"`
	Monitored       Metric  `json:"monitored"`
	Red             Metric  `json:"red"`
	Yellow          Metric  `json:"yellow"`
	Green           Metric  `json:"green"`
	Insufficient    Metric  `json:"insufficient"`
	// SavingsAtPlay é a economia positiva em 12 meses de quem está no vermelho ou no amarelo.
	SavingsAtPlay MoneyMetric `json:"savings_at_play"`
	// ActionRequired são as empresas com ação sugerida diferente de manter: vermelho, amarelo e
	// dados insuficientes.
	ActionRequired Metric            `json:"action_required"`
	Priorities     []FactorRPriority `json:"priorities"`
}

// ReconciliationSection é a Conciliação, com o responsável do Contábil no filtro. Os documentos
// contam todas as competências (decisão do usuário); a fila e as empresas, a do Dashboard.
type ReconciliationSection struct {
	Processing       Metric `json:"processing"`
	ExtractionReview Metric `json:"extraction_review"`
	ClosedPeriod     Metric `json:"closed_period"`
	Queue            Metric `json:"queue"`
	Divergent        Metric `json:"divergent"`
	WithoutReceipt   Metric `json:"without_receipt"`
	WithoutStatement Metric `json:"without_statement"`
	// Ready são as empresas com a conciliação pronta para concluir (decisão do usuário).
	Ready Metric `json:"ready"`
	// BlocksAccounting são as empresas com o Contábil aberto que não fecha pela Conciliação.
	BlocksAccounting Metric `json:"blocks_accounting"`
}

// Os grupos de status dos documentos na seção Conciliação.
var (
	DocsProcessing       = []string{"pendente", "triado"}
	DocsExtractionReview = []string{"revisao_triagem", "revisao_extracao"}
	DocsClosedPeriod     = []string{"pendencia_periodo_fechado"}
)

// maxPriorities é o tamanho da lista de prioridades do Fator R.
const maxPriorities = 5

func companyQuery(f co.Filter, kv ...string) map[string]string {
	return with(f.Query(), kv...)
}

// factorRSection monta a seção Fator R com a carteira do PA (factorr.PortfolioWith).
func factorRSection(ctx context.Context, q *store.Queries, c comp.Competence, f co.Filter) (FactorRSection, error) {
	ids, err := f.CompanyIDs(ctx, q, c, co.ModuleFiscal)
	if err != nil {
		return FactorRSection{}, err
	}
	p, err := factorr.PortfolioWith(ctx, q, c, ids)
	if err != nil {
		return FactorRSection{}, err
	}
	link := func(lights ...string) *Drilldown {
		kv := []string{"pa", c.String()}
		if len(lights) > 0 {
			kv = append(kv, "semaforo", strings.Join(lights, ","))
		}
		return &Drilldown{Path: "/factor-r", Query: companyQuery(f, kv...)}
	}
	s := FactorRSection{
		PA: c, MetaOperacional: p.MetaOperacional.Text(),
		Monitored:      Metric{p.KPIs.Monitored, link()},
		Red:            Metric{0, link(LightFilterRed)},
		Yellow:         Metric{p.KPIs.AtLimit, link(LightFilterYellow)},
		Green:          Metric{p.KPIs.Safe, link(LightFilterGreen)},
		Insufficient:   Metric{p.KPIs.Insufficient, link(LightFilterInsufficient)},
		SavingsAtPlay:  MoneyMetric{p.KPIs.SavingsAtPlay.Text(), link(LightFilterRed, LightFilterYellow)},
		ActionRequired: Metric{0, link(LightFilterRed, LightFilterYellow, LightFilterInsufficient)},
		Priorities:     []FactorRPriority{},
	}
	for _, l := range p.Linhas {
		if l.Semaforo != nil && *l.Semaforo == engine.LightRed {
			s.Red.Value++
		}
		if l.Acao == engine.ActionKeep {
			continue
		}
		s.ActionRequired.Value++
		if len(s.Priorities) < maxPriorities {
			pr := FactorRPriority{Company: l.Company, Light: l.Semaforo, Action: l.Acao, ActionText: l.AcaoTexto,
				Drilldown: &Drilldown{Path: "/companies/" + l.Company.ID, Query: map[string]string{"tab": "factor-r", "competence": c.String()}}}
			if l.FatorR != nil {
				v := l.FatorR.Text()
				pr.FactorR = &v
			}
			if l.Economia12m != nil {
				v := l.Economia12m.Text()
				pr.Savings12m = &v
			}
			s.Priorities = append(s.Priorities, pr)
		}
	}
	return s, nil
}

// Os valores do filtro de semáforo da carteira (o semáforo do motor e "sem dados").
const (
	LightFilterRed          = string(engine.LightRed)
	LightFilterYellow       = string(engine.LightYellow)
	LightFilterGreen        = string(engine.LightGreen)
	LightFilterInsufficient = "insuficiente"
)

// reconciliationSection monta a seção Conciliação com as contagens dos documentos, da fila e da
// Conciliação por empresa.
func reconciliationSection(ctx context.Context, q *store.Queries, c comp.Competence, f co.Filter) (ReconciliationSection, error) {
	ids, err := f.CompanyIDs(ctx, q, c, co.ModuleAccounting)
	if err != nil {
		return ReconciliationSection{}, err
	}
	var s ReconciliationSection
	docs, err := q.CountReconciliationDocumentsByStatus(ctx, store.CountReconciliationDocumentsByStatusParams{CompanyIds: ids})
	if err != nil {
		return s, err
	}
	byStatus := map[string]int{}
	for _, d := range docs {
		byStatus[d.Status] = int(d.Total)
	}
	docLink := func(statuses []string) Metric {
		n := 0
		for _, st := range statuses {
			n += byStatus[st]
		}
		kv := []string{"status", strings.Join(statuses, ",")}
		if f.Active() {
			kv = append(kv, "filter_competence", c.String())
		}
		return Metric{n, &Drilldown{Path: "/reconciliation/documents", Query: companyQuery(f, kv...)}}
	}
	s.Processing = docLink(DocsProcessing)
	s.ExtractionReview = docLink(DocsExtractionReview)
	s.ClosedPeriod = docLink(DocsClosedPeriod)

	day := c.FirstDay()
	counts, err := review.CountsFor(ctx, q, nil, &day, ids)
	if err != nil {
		return s, err
	}
	queueLink := func(bucket string) *Drilldown {
		kv := []string{"competence", c.String()}
		if bucket != "" {
			kv = append(kv, "bucket", bucket)
		}
		return &Drilldown{Path: "/reconciliation", Query: companyQuery(f, kv...)}
	}
	s.Queue = Metric{counts["total"], queueLink("")}
	s.Divergent = Metric{counts[review.BucketDivergent], queueLink(review.BucketDivergent)}
	s.WithoutReceipt = Metric{counts[review.BucketWithoutReceipt], queueLink(review.BucketWithoutReceipt)}
	s.WithoutStatement = Metric{counts[review.BucketWithoutStatement], queueLink(review.BucketWithoutStatement)}

	companies, err := overview.Load(ctx, q, c, ids)
	if err != nil {
		return s, err
	}
	k := overview.Count(companies)
	companiesLink := func(kv ...string) *Drilldown {
		return &Drilldown{Path: "/reconciliation/companies", Query: companyQuery(f, append([]string{"competence", c.String()}, kv...)...)}
	}
	s.Ready = Metric{k.Ready, companiesLink("state", "pronta")}
	s.BlocksAccounting = Metric{k.BlocksAccounting, companiesLink("blocked", "true")}
	return s, nil
}
