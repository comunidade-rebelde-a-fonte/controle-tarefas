package credential

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"controle/internal/platform/validate"
)

// Situação de validade, calculada pelas faixas configuráveis [PRD §19.3].
const (
	SituationExpired = "EXPIRED"
	SituationDueSoon = "DUE_SOON" // vence em até N dias (N = ThresholdDays)
	SituationCurrent = "CURRENT"
	SituationNoDate  = "NO_DATE"
)

// Situation é a situação de validade de uma procuração ou certificado.
type Situation struct {
	Code  string `json:"code"`
	Label string `json:"label"`
	// Faixa "vence em até N dias" em que o item está (só em DUE_SOON).
	ThresholdDays *int `json:"threshold_days"`
	// Dias até o vencimento (negativo se vencido; nil sem data).
	DaysLeft *int `json:"days_left"`
}

func civilDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// SituationOf classifica a validade em relação a hoje (data civil no fuso de negócio):
// vence hoje ainda não é vencido; cada item cai na menor faixa que o contém; além da
// maior faixa é Vigente.
func SituationOf(expiresAt *time.Time, today time.Time, thresholds []int) Situation {
	if expiresAt == nil {
		return Situation{Code: SituationNoDate, Label: "Sem data cadastrada"}
	}
	days := int(civilDay(*expiresAt).Sub(civilDay(today)).Hours() / 24)
	if days < 0 {
		return Situation{Code: SituationExpired, Label: "Vencido", DaysLeft: &days}
	}
	sorted := slices.Clone(thresholds)
	slices.Sort(sorted)
	for _, n := range sorted {
		if days <= n {
			return Situation{Code: SituationDueSoon, Label: "Vence em até " + strconv.Itoa(n) + " dias", ThresholdDays: &n, DaysLeft: &days}
		}
	}
	return Situation{Code: SituationCurrent, Label: "Vigente", DaysLeft: &days}
}

// Filtros rápidos da tela [PRD §33].
const (
	FilterExpired        = "expired"
	FilterAwaitingClient = "awaiting_client"
	FilterNoDate         = "no_date"
	filterDuePrefix      = "due_" // due_N: não vencido e vence em até N dias (acumulado)
)

// Filter é um filtro rápido já validado; o vazio aceita tudo.
type Filter struct {
	kind    string
	dueDays int
}

// ParseFilter valida o filtro: expired, awaiting_client, no_date ou due_N (1 ≤ N ≤ 365).
func ParseFilter(s string) (Filter, error) {
	switch s {
	case "", FilterExpired, FilterAwaitingClient, FilterNoDate:
		return Filter{kind: s}, nil
	}
	if n, ok := strings.CutPrefix(s, filterDuePrefix); ok {
		if d, err := strconv.Atoi(n); err == nil && d >= 1 && d <= 365 && strconv.Itoa(d) == n {
			return Filter{kind: filterDuePrefix, dueDays: d}, nil
		}
	}
	return Filter{}, validate.Field("filter", fmt.Sprintf("Filtro inválido %q: use expired, due_N (1 a 365), awaiting_client ou no_date.", s))
}

// Match indica se o item atende ao filtro.
func (f Filter) Match(c Control) bool {
	switch f.kind {
	case "":
		return true
	case FilterExpired:
		return c.Situation.Code == SituationExpired
	case FilterAwaitingClient:
		return c.RenewalStatus == RenewalAwaitingClient
	case FilterNoDate:
		return c.Situation.Code == SituationNoDate
	case filterDuePrefix:
		d := c.Situation.DaysLeft
		return d != nil && *d >= 0 && *d <= f.dueDays
	}
	return false
}
