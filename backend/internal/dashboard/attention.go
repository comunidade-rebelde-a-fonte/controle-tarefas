package dashboard

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"controle/internal/credential"
	"controle/internal/platform/civil"
	"controle/internal/platform/clock"
	comp "controle/internal/platform/competence"
	"controle/internal/store"
)

// Categorias do "Exige atenção", na ordem de exibição [PRD §24.6].
const (
	AttentionOverdue                = "OVERDUE"
	AttentionPending                = "PENDING"
	AttentionMissingDocuments       = "MISSING_DOCUMENTS"
	AttentionErrors                 = "ERROR"
	AttentionCertificateExpired     = "CERTIFICATE_EXPIRED"
	AttentionCertificateDueSoon     = "CERTIFICATE_DUE_SOON"
	AttentionPowerOfAttorneyExpired = "POWER_OF_ATTORNEY_EXPIRED"
	AttentionPowerOfAttorneyDueSoon = "POWER_OF_ATTORNEY_DUE_SOON"
	AttentionPreviousOpen           = "PREVIOUS_OPEN"
	AttentionDctfwebBlocked         = "DCTFWEB_BLOCKED"
	AttentionAccountingBlocked      = "ACCOUNTING_BLOCKED"
)

const (
	attentionSample        = 5
	statusMissingDocuments = "MISSING_DOCUMENTS"
	statusError            = "ERROR"
)

// AttentionCompany identifica a empresa do item.
type AttentionCompany struct {
	ID           string `json:"id"`
	InternalCode string `json:"internal_code"`
	LegalName    string `json:"legal_name"`
}

// AttentionItem é um exemplo da categoria.
type AttentionItem struct {
	Company AttentionCompany `json:"company"`
	// Obrigação ou item (ex.: "ICMS", "Certificado digital", "Declarações").
	Title  string      `json:"title"`
	Detail string      `json:"detail"`
	Date   *civil.Date `json:"date"`
	// Rota do item (grade do módulo filtrada na empresa, ou a tela de credenciais).
	Drilldown *Drilldown `json:"drilldown"`
}

// AttentionCategory é uma categoria com contagem, amostra e o "ver todos".
type AttentionCategory struct {
	Key       string          `json:"key"`
	Label     string          `json:"label"`
	Count     int             `json:"count"`
	Sample    []AttentionItem `json:"sample"`
	Drilldown *Drilldown      `json:"drilldown"`
}

// Attention é a resposta de GET /dashboard/attention.
type Attention struct {
	Competence comp.Competence     `json:"competence"`
	Categories []AttentionCategory `json:"categories"`
}

var attentionLabels = map[string]string{
	AttentionOverdue:                "Obrigações atrasadas",
	AttentionPending:                "Pendências",
	AttentionMissingDocuments:       "Documentos faltantes",
	AttentionErrors:                 "Erros",
	AttentionCertificateExpired:     "Certificados vencidos",
	AttentionCertificateDueSoon:     "Certificados próximos do vencimento",
	AttentionPowerOfAttorneyExpired: "Procurações vencidas",
	AttentionPowerOfAttorneyDueSoon: "Procurações próximas do vencimento",
	AttentionPreviousOpen:           "Empresas com competências anteriores abertas",
	AttentionDctfwebBlocked:         "Dependências bloqueando DCTFWeb",
	AttentionAccountingBlocked:      "Dependências bloqueando Contábil",
}

var attentionOrder = []string{
	AttentionOverdue, AttentionPending, AttentionMissingDocuments, AttentionErrors,
	AttentionCertificateExpired, AttentionCertificateDueSoon, AttentionPowerOfAttorneyExpired, AttentionPowerOfAttorneyDueSoon,
	AttentionPreviousOpen, AttentionDctfwebBlocked, AttentionAccountingBlocked,
}

// CompanyGrid leva à grade do módulo na competência, com a busca na empresa; no Fiscal, no
// agrupamento (Normais ou Simples) do regime da empresa naquela competência.
func CompanyGrid(module, code, regimeGroup string, c comp.Competence) *Drilldown {
	q := map[string]string{"competence": c.String(), "q": code}
	if module == "FISCAL" {
		q["group"] = "normal"
		if regimeGroup == "SIMPLES" {
			q["group"] = "simples"
		}
	}
	return &Drilldown{Path: modulePaths[module], Query: q}
}

// Attention monta as 11 categorias do §24.6 com contagem, amostra e drill-down.
func (s *Service) Attention(ctx context.Context, c comp.Competence, f Filters) (Attention, error) {
	return s.attention(ctx, c, f, nil)
}

// CompanyAttention são as pendências de uma empresa na competência, para a Visão geral da ficha
// (TASK-025, D-039): as mesmas categorias, só as que têm item, com o "ver todos" dentro da ficha
// (a aba Obrigações filtrada ou a de Procurações/Certificados) quando ele existe lá.
func (s *Service) CompanyAttention(ctx context.Context, c comp.Competence, companyID string) (Attention, error) {
	all, err := s.attention(ctx, c, Filters{}, &companyID)
	if err != nil {
		return all, err
	}
	ficha := func(kv ...string) *Drilldown {
		return &Drilldown{Path: "/companies/" + companyID, Query: with(map[string]string{"competence": c.String()}, kv...)}
	}
	inFicha := map[string]*Drilldown{
		AttentionOverdue:                ficha("tab", "obligations", "state", StateOverdue),
		AttentionPending:                ficha("tab", "obligations", "state", StatePending),
		AttentionMissingDocuments:       ficha("tab", "obligations", "status", statusMissingDocuments),
		AttentionErrors:                 ficha("tab", "obligations", "status", statusError),
		AttentionCertificateExpired:     ficha("tab", "credentials"),
		AttentionCertificateDueSoon:     ficha("tab", "credentials"),
		AttentionPowerOfAttorneyExpired: ficha("tab", "credentials"),
		AttentionPowerOfAttorneyDueSoon: ficha("tab", "credentials"),
	}
	out := Attention{Competence: c, Categories: []AttentionCategory{}}
	for _, cat := range all.Categories {
		if cat.Count == 0 {
			continue
		}
		if d, ok := inFicha[cat.Key]; ok {
			cat.Drilldown = d
		} else if len(cat.Sample) > 0 {
			cat.Drilldown = cat.Sample[0].Drilldown
		}
		out.Categories = append(out.Categories, cat)
	}
	return out, nil
}

// attention monta as categorias com os filtros do Dashboard ou só de uma empresa.
func (s *Service) attention(ctx context.Context, c comp.Competence, f Filters, companyID *string) (Attention, error) {
	today := clock.Today(s.clock)
	out := Attention{Competence: c, Categories: []AttentionCategory{}}
	q := store.New(s.pool)
	kid, err := competenceID(ctx, q, c)
	if err != nil {
		return out, err
	}
	base := f.baseQuery(c)
	cats := map[string]*AttentionCategory{}
	for _, key := range attentionOrder {
		cats[key] = &AttentionCategory{Key: key, Label: attentionLabels[key], Sample: []AttentionItem{}}
	}

	obligations, err := q.AttentionObligations(ctx, store.AttentionObligationsParams{
		Today: today, CompetenceID: kid, CompanyID: companyID, RegimeGroup: f.RegimeGroup, TaxRegime: f.TaxRegime, ResponsibleID: f.ResponsibleID,
	})
	if err != nil {
		return out, err
	}
	for _, r := range obligations {
		cat := cats[r.Category]
		cat.Count = int(r.Total)
		detail := r.StatusLabel
		if r.Category == AttentionOverdue && r.DueDateEffective != nil {
			days := int(today.Sub(*r.DueDateEffective).Hours() / 24)
			detail = fmt.Sprintf("%s · venceu em %s (%d %s)", r.StatusLabel, r.DueDateEffective.Format("02/01/2006"), days, plural(days, "dia", "dias"))
		}
		cat.Sample = append(cat.Sample, AttentionItem{
			Company: AttentionCompany{ID: r.CompanyID, InternalCode: r.InternalCode, LegalName: r.LegalName},
			Title:   fmt.Sprintf("%s — %s", moduleLabels[r.Module], r.TypeName), Detail: detail,
			Date: civil.Ptr(r.DueDateEffective), Drilldown: CompanyGrid(r.Module, r.InternalCode, r.RegimeGroup, c),
		})
	}
	cats[AttentionOverdue].Drilldown = obligationsLink(base, StateOverdue, "")
	cats[AttentionPending].Drilldown = obligationsLink(base, StatePending, "")
	cats[AttentionMissingDocuments].Drilldown = &Drilldown{Path: "/obligations", Query: with(base, "status", statusMissingDocuments)}
	cats[AttentionErrors].Drilldown = &Drilldown{Path: "/obligations", Query: with(base, "status", statusError)}

	if err := s.attentionCredentials(ctx, f, companyID, cats); err != nil {
		return out, err
	}

	prev, err := q.AttentionPreviousOpen(ctx, store.AttentionPreviousOpenParams{
		CompetenceKey: c.String(), CompanyID: companyID, RegimeGroup: f.RegimeGroup, TaxRegime: f.TaxRegime, ResponsibleID: f.ResponsibleID,
	})
	if err != nil {
		return out, err
	}
	backlog := &Drilldown{Path: "/backlog", Query: with(base, "before", c.String())}
	delete(backlog.Query, "competence")
	cats[AttentionPreviousOpen].Drilldown = backlog
	for _, r := range prev {
		cat := cats[AttentionPreviousOpen]
		cat.Count = int(r.Total)
		oldest, err := comp.Parse(r.OldestKey)
		if err != nil {
			return out, err
		}
		cat.Sample = append(cat.Sample, AttentionItem{
			Company:   AttentionCompany{ID: r.CompanyID, InternalCode: r.InternalCode, LegalName: r.LegalName},
			Title:     "Competência " + oldest.Display(),
			Detail:    fmt.Sprintf("%d %s em aberto", r.OpenPeriods, plural(int(r.OpenPeriods), "módulo", "módulos")),
			Drilldown: &Drilldown{Path: "/backlog", Query: map[string]string{"before": c.String(), "q": r.InternalCode}},
		})
	}

	blocked, err := q.AttentionBlockedDependencies(ctx, store.AttentionBlockedDependenciesParams{
		CompetenceID: kid, CompanyID: companyID, RegimeGroup: f.RegimeGroup, TaxRegime: f.TaxRegime, ResponsibleID: f.ResponsibleID,
	})
	if err != nil {
		return out, err
	}
	cats[AttentionDctfwebBlocked].Drilldown = &Drilldown{Path: "/dctfweb", Query: map[string]string{"competence": c.String()}}
	cats[AttentionAccountingBlocked].Drilldown = &Drilldown{Path: "/accounting", Query: map[string]string{"competence": c.String(), "pending": "true"}}
	for _, r := range blocked {
		key, module := AttentionDctfwebBlocked, "DECLARATIONS"
		if r.Mode == "HARD" {
			key, module = AttentionAccountingBlocked, "ACCOUNTING"
		}
		cat := cats[key]
		cat.Count = int(r.Total)
		dd := CompanyGrid(module, r.InternalCode, "", c)
		if key == AttentionDctfwebBlocked {
			dd = &Drilldown{Path: "/dctfweb", Query: map[string]string{"competence": c.String(), "q": r.InternalCode}}
		}
		cat.Sample = append(cat.Sample, AttentionItem{
			Company: AttentionCompany{ID: r.CompanyID, InternalCode: r.InternalCode, LegalName: r.LegalName},
			Title:   r.DependentName, Detail: "Pendente: " + r.PendingNames, Drilldown: dd,
		})
	}

	for _, key := range attentionOrder {
		out.Categories = append(out.Categories, *cats[key])
	}
	return out, nil
}

// attentionCredentials preenche vencidos e próximos do vencimento (maior faixa configurada); com
// companyID, só os da empresa.
func (s *Service) attentionCredentials(ctx context.Context, f Filters, companyID *string, cats map[string]*AttentionCategory) error {
	list, err := s.credentials.List(ctx, credential.ListParams{})
	if err != nil {
		return err
	}
	maxDays := 0
	for _, t := range list.Thresholds {
		maxDays = max(maxDays, t)
	}
	keys := map[string][2]string{
		credential.TypeCertificate:     {AttentionCertificateExpired, AttentionCertificateDueSoon},
		credential.TypePowerOfAttorney: {AttentionPowerOfAttorneyExpired, AttentionPowerOfAttorneyDueSoon},
	}
	for typ, k := range keys {
		cats[k[0]].Drilldown = &Drilldown{Path: "/credentials", Query: map[string]string{"type": typ, "filter": credential.FilterExpired}}
		cats[k[1]].Drilldown = &Drilldown{Path: "/credentials", Query: map[string]string{"type": typ, "filter": fmt.Sprintf("due_%d", maxDays)}}
	}
	for _, row := range list.Items {
		if !regimeMatches(row.Company.TaxRegime, f) || (companyID != nil && row.Company.ID != *companyID) {
			continue
		}
		for typ, ctl := range map[string]credential.Control{credential.TypeCertificate: row.DigitalCertificate, credential.TypePowerOfAttorney: row.RFBPowerOfAttorney} {
			var key string
			switch ctl.Situation.Code {
			case credential.SituationExpired:
				key = keys[typ][0]
			case credential.SituationDueSoon:
				key = keys[typ][1]
			default:
				continue
			}
			cat := cats[key]
			cat.Count++
			detail := ctl.Situation.Label
			if ctl.Situation.DaysLeft != nil {
				if d := *ctl.Situation.DaysLeft; d < 0 {
					detail = fmt.Sprintf("Vencido há %d %s", -d, plural(-d, "dia", "dias"))
				} else {
					detail = fmt.Sprintf("Vence em %d %s", d, plural(d, "dia", "dias"))
				}
			}
			cat.Sample = append(cat.Sample, AttentionItem{
				Company: AttentionCompany{ID: row.Company.ID, InternalCode: row.Company.InternalCode, LegalName: row.Company.LegalName},
				Title:   credentialLabels[typ], Detail: detail, Date: ctl.ExpiresAt,
				Drilldown: &Drilldown{Path: "/companies/" + row.Company.ID, Query: map[string]string{"tab": "credentials"}},
			})
		}
	}
	for _, k := range keys {
		for _, key := range k {
			cats[key].Sample = firstByDate(cats[key].Sample)
		}
	}
	return nil
}

// firstByDate ordena a amostra pela data (a mais urgente primeiro) e limita o tamanho.
func firstByDate(items []AttentionItem) []AttentionItem {
	slices.SortStableFunc(items, func(a, b AttentionItem) int {
		switch {
		case a.Date == nil && b.Date == nil:
		case a.Date == nil:
			return 1
		case b.Date == nil:
			return -1
		default:
			if c := a.Date.Compare(b.Date.Time); c != 0 {
				return c
			}
		}
		return strings.Compare(a.Company.InternalCode, b.Company.InternalCode)
	})
	if len(items) > attentionSample {
		items = items[:attentionSample]
	}
	return items
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
