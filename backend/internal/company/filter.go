package company

import (
	"context"
	"net/url"
	"strings"

	"controle/internal/platform/competence"
	"controle/internal/platform/httpx"
	"controle/internal/platform/validate"
	"controle/internal/store"
)

// Filter são os filtros de empresa do Dashboard: agrupamento, regime e responsável [PRD §24.4,
// §24.5]. Valem pelo snapshot da empresa na competência, como os números de obrigações. As listas
// que explicam os números das seções Fator R e Conciliação do Dashboard aceitam os mesmos filtros
// (TASK-024, D-038), com o responsável do módulo de cada uma.
type Filter struct {
	RegimeGroup   *string
	TaxRegime     *string
	ResponsibleID *string
}

// Active diz se algum filtro foi informado.
func (f Filter) Active() bool {
	return f.RegimeGroup != nil || f.TaxRegime != nil || f.ResponsibleID != nil
}

// ParseFilter lê regime_group, tax_regime e responsible_id da query.
func ParseFilter(q url.Values) (Filter, error) {
	var f Filter
	if v := strings.ToUpper(strings.TrimSpace(q.Get("regime_group"))); v != "" {
		if v != GroupNormal && v != GroupSimples {
			return f, validate.Field("regime_group", "Use NORMAL ou SIMPLES.")
		}
		f.RegimeGroup = &v
	}
	if v := strings.TrimSpace(q.Get("tax_regime")); v != "" {
		if RegimeGroup(v) == "" {
			return f, validate.Field("tax_regime", "Regime inválido.")
		}
		f.TaxRegime = &v
	}
	if v := strings.TrimSpace(q.Get("responsible_id")); v != "" {
		if !httpx.IsUUID(v) {
			return f, validate.Field("responsible_id", "Identificador inválido.")
		}
		f.ResponsibleID = &v
	}
	return f, nil
}

// CompanyIDs devolve as empresas que passam no filtro pelo snapshot da competência, com o
// responsável do módulo dado (FISCAL, DECLARATIONS, ACCOUNTING ou PAYROLL). Sem filtro, nil: todas
// as empresas. Com filtro e nenhuma empresa, uma lista vazia (não nil): nenhuma.
func (f Filter) CompanyIDs(ctx context.Context, q *store.Queries, c competence.Competence, module string) ([]string, error) {
	if !f.Active() {
		return nil, nil
	}
	ids, err := q.ListSnapshotCompanies(ctx, store.ListSnapshotCompaniesParams{
		Competence: c.FirstDay(), Module: module, RegimeGroup: f.RegimeGroup, TaxRegime: f.TaxRegime, ResponsibleID: f.ResponsibleID,
	})
	if err != nil {
		return nil, err
	}
	if ids == nil {
		ids = []string{}
	}
	return ids, nil
}

// Query devolve os filtros como parâmetros da URL (os drill-downs do Dashboard).
func (f Filter) Query() map[string]string {
	q := map[string]string{}
	if f.RegimeGroup != nil {
		q["regime_group"] = *f.RegimeGroup
	}
	if f.TaxRegime != nil {
		q["tax_regime"] = *f.TaxRegime
	}
	if f.ResponsibleID != nil {
		q["responsible_id"] = *f.ResponsibleID
	}
	return q
}
