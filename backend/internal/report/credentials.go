package report

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"strings"

	"controle/internal/credential"
	"controle/internal/platform/validate"
)

var credentialLabels = map[string]string{
	credential.TypePowerOfAttorney: "Procuração RFB",
	credential.TypeCertificate:     "Certificado digital",
}

// credentialLine é um documento (procuração ou certificado) de uma empresa.
type credentialLine struct {
	row credential.Row
	typ string
	c   credential.Control
}

// daysLeft é o prazo em dias até o vencimento; negativo quando já venceu.
func daysLeft(c credential.Control) string {
	if c.Situation.DaysLeft == nil {
		return "—"
	}
	return itoa(*c.Situation.DaysLeft)
}

// credentials lista procurações e certificados vencidos ou a vencer [PRD §19, §33, §36.3].
// Sem filtro: vencidos e os que vencem dentro da maior faixa configurada. filter e type
// seguem a tela de procurações e certificados (expired, due_N, awaiting_client, no_date).
func (h *Handler) credentials(ctx context.Context, q url.Values) (built, error) {
	var filters []Field
	file := competenceTime(h.clock.Now().In(h.clock.Location()))
	doc := Document{Title: "Procurações e certificados a vencer", Landscape: true}
	if v := strings.TrimSpace(q.Get("competence")); v != "" {
		c, err := requiredCompetence(q)
		if err != nil {
			return built{}, err
		}
		doc.Competence, file = &c, c.String()
	}
	typ := strings.TrimSpace(q.Get("type"))
	if typ != "" && credentialLabels[typ] == "" {
		return built{}, validate.Field("type", "Tipo inválido: use RFB_POWER_OF_ATTORNEY ou DIGITAL_CERTIFICATE.")
	}
	filter := strings.TrimSpace(q.Get("filter"))
	f, err := credential.ParseFilter(filter)
	if err != nil {
		return built{}, err
	}
	list, err := h.creds.List(ctx, credential.ListParams{})
	if err != nil {
		return built{}, err
	}
	horizon := 0
	for _, t := range list.Thresholds {
		horizon = max(horizon, t)
	}

	match := func(c credential.Control) bool {
		if filter != "" {
			return f.Match(c)
		}
		return c.Situation.Code == credential.SituationExpired || c.Situation.Code == credential.SituationDueSoon
	}
	var lines []credentialLine
	for _, r := range list.Items {
		for _, t := range credential.Types {
			c := r.RFBPowerOfAttorney
			if t == credential.TypeCertificate {
				c = r.DigitalCertificate
			}
			if (typ == "" || typ == t) && match(c) {
				lines = append(lines, credentialLine{r, t, c})
			}
		}
	}
	// Mais urgentes primeiro (vencidos há mais tempo); sem data por último.
	slices.SortStableFunc(lines, func(a, b credentialLine) int {
		da, db := a.c.Situation.DaysLeft, b.c.Situation.DaysLeft
		switch {
		case da == nil && db == nil:
			return strings.Compare(a.row.Company.InternalCode, b.row.Company.InternalCode)
		case da == nil:
			return 1
		case db == nil:
			return -1
		case *da != *db:
			return *da - *db
		}
		return strings.Compare(a.row.Company.InternalCode, b.row.Company.InternalCode)
	})

	n := map[string]map[string]int{}
	for _, t := range credential.Types {
		n[t] = map[string]int{}
	}
	for _, l := range lines {
		n[l.typ][l.c.Situation.Code]++
		doc.Rows = append(doc.Rows, []string{
			l.row.Company.InternalCode, clip(l.row.Company.LegalName, 30), formatCnpj(l.row.Company.Cnpj), credentialLabels[l.typ],
			date(l.c.ExpiresAt), l.c.Situation.Label, daysLeft(l.c), clip(l.c.RenewalLabel, 24), clip(deref(l.c.Notes), 20),
		})
	}

	if filter == "" {
		filters = append(filters, Field{"Situação", fmt.Sprintf("vencidos ou a vencer em até %d dias", horizon)})
	} else {
		filters = append(filters, Field{"Filtro", filterLabel(filter)})
	}
	if typ != "" {
		filters = append(filters, Field{"Documento", credentialLabels[typ]})
	}
	doc.Filters = filters
	doc.Summary = []Field{
		{"Documentos listados", itoa(len(lines))},
		{"Procurações vencidas", itoa(n[credential.TypePowerOfAttorney][credential.SituationExpired])},
		{"Procurações a vencer", itoa(n[credential.TypePowerOfAttorney][credential.SituationDueSoon])},
		{"Certificados vencidos", itoa(n[credential.TypeCertificate][credential.SituationExpired])},
		{"Certificados a vencer", itoa(n[credential.TypeCertificate][credential.SituationDueSoon])},
		{"Faixa de alerta (dias)", itoa(horizon)},
	}
	doc.Columns = []Column{
		{"Código", 2}, {"Empresa", 7}, {"CNPJ", 4}, {"Documento", 4}, {"Vencimento", 3}, {"Situação", 6},
		{"Prazo (dias)", 3}, {"Renovação", 5}, {"Observação", 4},
	}
	return built{doc: doc, file: file}, nil
}

func filterLabel(f string) string {
	switch f {
	case credential.FilterExpired:
		return "Vencidos"
	case credential.FilterAwaitingClient:
		return "Aguardando cliente"
	case credential.FilterNoDate:
		return "Sem data de vencimento"
	}
	if n, ok := strings.CutPrefix(f, "due_"); ok {
		return "Vencem em até " + n + " dias"
	}
	return f
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
