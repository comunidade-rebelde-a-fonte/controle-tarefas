package seed_test

import (
	"context"
	"io"
	"net/http"
	"testing"

	"controle/internal/testutil"
	"controle/seed"
)

type credentialList struct {
	Total int `json:"total"`
	Items []struct {
		PowerOfAttorney struct {
			RenewalStatus string `json:"renewal_status"`
			Situation     struct {
				Label string `json:"label"`
			} `json:"situation"`
		} `json:"rfb_power_of_attorney"`
		Certificate struct {
			RenewalStatus string `json:"renewal_status"`
			Situation     struct {
				Label string `json:"label"`
			} `json:"situation"`
		} `json:"digital_certificate"`
	} `json:"items"`
}

// T-M4-09: o seed cobre todas as faixas e cada filtro rápido do §33 encontra ao menos uma empresa.
func TestDemoSeed_CredentialsCoverEveryQuickFilter(t *testing.T) {
	pool := testutil.NewDB(t)
	ctx := context.Background()
	clk := testutil.FixedClock(t, "2026-10-01T12:00:00Z")
	for range 2 {
		if err := seed.Reference(ctx, pool, clk, io.Discard); err != nil {
			t.Fatal(err)
		}
		if err := seed.Demo(ctx, pool, clk, io.Discard); err != nil {
			t.Fatal(err)
		}
	}
	// Idempotente: rodar de novo não duplica nem altera os registros.
	if n := count(t, pool, `SELECT count(*) FROM credential_controls`); n != 36 {
		t.Fatalf("registros de credenciais = %d, want 36 (40 menos 4 sem data)", n)
	}
	if n := count(t, pool, `SELECT count(*) FROM credential_controls WHERE file_storage_key IS NOT NULL`); n != 0 {
		t.Fatal("o seed não grava arquivos")
	}

	srv := testutil.NewServer(t, pool, clk)
	for _, f := range []string{"expired", "due_7", "due_15", "due_30", "due_60", "awaiting_client", "no_date"} {
		var l credentialList
		srv.Get("/api/v1/credential-controls?filter="+f).MustStatus(t, http.StatusOK).JSON(t, &l)
		if l.Total == 0 {
			t.Errorf("filtro %s: nenhuma empresa no seed", f)
		}
	}

	var all credentialList
	srv.Get("/api/v1/credential-controls").MustStatus(t, http.StatusOK).JSON(t, &all)
	labels, renewals := map[string]int{}, map[string]int{}
	for _, it := range all.Items {
		labels[it.PowerOfAttorney.Situation.Label]++
		labels[it.Certificate.Situation.Label]++
		renewals[it.PowerOfAttorney.RenewalStatus]++
		renewals[it.Certificate.RenewalStatus]++
	}
	for _, want := range []string{"Vencido", "Vence em até 7 dias", "Vence em até 15 dias", "Vence em até 30 dias",
		"Vence em até 60 dias", "Vigente", "Sem data cadastrada"} {
		if labels[want] == 0 {
			t.Errorf("faixa %q sem nenhum item no seed (%v)", want, labels)
		}
	}
	for _, want := range []string{"CURRENT", "RENEWAL_REQUESTED", "AWAITING_CLIENT"} {
		if renewals[want] == 0 {
			t.Errorf("etapa %s sem nenhum item no seed (%v)", want, renewals)
		}
	}
}
