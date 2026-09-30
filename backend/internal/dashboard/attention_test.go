package dashboard_test

import (
	"context"
	"net/http"
	"slices"
	"testing"
)

type attentionCategory struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	Count  int    `json:"count"`
	Sample []struct {
		Company struct {
			ID           string `json:"id"`
			InternalCode string `json:"internal_code"`
		} `json:"company"`
		Title  string  `json:"title"`
		Detail string  `json:"detail"`
		Date   *string `json:"date"`
	} `json:"sample"`
	Drilldown *struct {
		Path  string            `json:"path"`
		Query map[string]string `json:"query"`
	} `json:"drilldown"`
}

type attentionBody struct {
	Competence string              `json:"competence"`
	Categories []attentionCategory `json:"categories"`
}

func TestAttention_EveryCategoryHasSeedCase(t *testing.T) {
	srv := server(t)
	var a attentionBody
	srv.Get("/api/v1/dashboard/attention?competence=2026-10").MustStatus(t, http.StatusOK).JSON(t, &a)
	want := []string{"OVERDUE", "PENDING", "MISSING_DOCUMENTS", "ERROR", "CERTIFICATE_EXPIRED", "CERTIFICATE_DUE_SOON",
		"POWER_OF_ATTORNEY_EXPIRED", "POWER_OF_ATTORNEY_DUE_SOON", "PREVIOUS_OPEN", "DCTFWEB_BLOCKED", "ACCOUNTING_BLOCKED"}
	var keys []string
	for _, c := range a.Categories {
		keys = append(keys, c.Key)
		if c.Count == 0 || len(c.Sample) == 0 || len(c.Sample) > 5 || len(c.Sample) > c.Count || c.Drilldown == nil || c.Label == "" {
			t.Errorf("categoria %s: count %d, amostra %d, drill-down %v", c.Key, c.Count, len(c.Sample), c.Drilldown)
		}
	}
	if !slices.Equal(keys, want) {
		t.Fatalf("categorias = %v", keys)
	}

	// Atrasadas: a contagem bate com o KPI do dashboard e a amostra vem pelo vencimento.
	d := get(t, srv, "competence=2026-10")
	overdue := a.Categories[0]
	if overdue.Count != d.KPIs.Overdue.Value || overdue.Drilldown.Query["state"] != "overdue" {
		t.Fatalf("atrasadas = %d (KPI %d), drill-down %+v", overdue.Count, d.KPIs.Overdue.Value, overdue.Drilldown)
	}
	// Anteriores em aberto: igual ao KPI.
	if prev := a.Categories[8]; prev.Count != d.KPIs.PreviousOpen.Value || prev.Drilldown.Path != "/backlog" {
		t.Fatalf("anteriores em aberto = %d (KPI %d)", prev.Count, d.KPIs.PreviousOpen.Value)
	}
}

func TestCA019_ExpiringCertificateInAttention(t *testing.T) {
	srv := server(t)
	var companyID string
	if err := srv.Pool.QueryRow(context.Background(), `SELECT id FROM companies WHERE internal_code = '206'`).Scan(&companyID); err != nil {
		t.Fatal(err)
	}
	var row struct {
		DigitalCertificate struct {
			Version int `json:"version"`
		} `json:"digital_certificate"`
	}
	srv.Get("/api/v1/companies/"+companyID+"/credential-controls").MustStatus(t, http.StatusOK).JSON(t, &row)
	ctl := row.DigitalCertificate
	// Certificado vencendo amanhã (dentro da menor faixa configurada) [CA-019].
	srv.Put("/api/v1/companies/"+companyID+"/credential-controls/DIGITAL_CERTIFICATE",
		map[string]any{"version": ctl.Version, "expires_at": "2026-10-02", "renewal_status": "CURRENT"}).MustStatus(t, http.StatusOK)

	var a attentionBody
	srv.Get("/api/v1/dashboard/attention?competence=2026-10").MustStatus(t, http.StatusOK).JSON(t, &a)
	idx := slices.IndexFunc(a.Categories, func(c attentionCategory) bool {
		return c.Key == "CERTIFICATE_DUE_SOON"
	})
	cat := a.Categories[idx]
	first := cat.Sample[0]
	if first.Company.InternalCode != "206" || first.Detail != "Vence em 1 dia" || first.Date == nil || *first.Date != "2026-10-02" {
		t.Fatalf("mais urgente = %+v", first)
	}
	// E no filtro correspondente da tela de credenciais.
	var filtered struct {
		Items []struct {
			Company struct {
				InternalCode string `json:"internal_code"`
			} `json:"company"`
		} `json:"items"`
	}
	srv.Get("/api/v1/credential-controls?type=DIGITAL_CERTIFICATE&filter="+cat.Drilldown.Query["filter"]).MustStatus(t, http.StatusOK).JSON(t, &filtered)
	if !slices.ContainsFunc(filtered.Items, func(it struct {
		Company struct {
			InternalCode string `json:"internal_code"`
		} `json:"company"`
	}) bool {
		return it.Company.InternalCode == "206"
	}) || len(filtered.Items) != cat.Count {
		t.Fatalf("filtro %s: %d itens (atenção: %d)", cat.Drilldown.Query["filter"], len(filtered.Items), cat.Count)
	}
}
