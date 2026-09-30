package main

import (
	"slices"
	"testing"

	"controle/internal/platform/cnpj"
)

func TestRequirePerfDatabase(t *testing.T) {
	ok := []string{
		"postgres://u:p@localhost:5432/controle_perf?sslmode=disable",
		"postgres://u:p@127.0.0.1/controle_perf",
		"postgres://u:p@db:5432/controle_perf",
	}
	for _, u := range ok {
		if err := requirePerfDatabase(u); err != nil {
			t.Errorf("%s deveria ser aceito: %v", u, err)
		}
	}
	bad := []string{
		"",
		"postgres://u:p@localhost:5432/controle?sslmode=disable",
		"postgres://u:p@localhost:5432/controle_e2e",
		"postgres://u:p@prod.example.com:5432/controle_perf",
	}
	for _, u := range bad {
		if err := requirePerfDatabase(u); err == nil {
			t.Errorf("%q deveria ser recusado", u)
		}
	}
}

func TestSyntheticCompanies_DeterministicAndRealistic(t *testing.T) {
	a, b := syntheticCompanies(300, 42), syntheticCompanies(300, 42)
	if len(a) != 300 {
		t.Fatalf("len = %d", len(a))
	}
	simples := 0
	seen := map[string]bool{}
	for i := range a {
		if a[i].Code != b[i].Code || a[i].Regimes[0].Regime != b[i].Regimes[0].Regime || !slices.Equal(a[i].Modules, b[i].Modules) ||
			!slices.Equal(a[i].NotApplicable, b[i].NotApplicable) {
			t.Fatalf("a mesma semente deveria gerar a mesma massa (empresa %d)", i)
		}
		if !cnpj.Valid(a[i].Cnpj) || seen[a[i].Cnpj] {
			t.Fatalf("CNPJ inválido ou repetido: %s", a[i].Cnpj)
		}
		seen[a[i].Cnpj] = true
		if !slices.Contains(a[i].Modules, "FISCAL") {
			t.Fatalf("empresa %s sem Fiscal", a[i].Code)
		}
		if a[i].Regimes[0].Regime == "SIMPLES_NACIONAL" {
			simples++
		}
	}
	if simples < 150 || simples > 210 {
		t.Fatalf("Simples = %d de 300 (esperado ~60%%)", simples)
	}
	if other := syntheticCompanies(300, 7); other[0].Regimes[0].Regime == a[0].Regimes[0].Regime && slices.Equal(other[0].Modules, a[0].Modules) &&
		other[1].Regimes[0].Regime == a[1].Regimes[0].Regime && slices.Equal(other[1].Modules, a[1].Modules) &&
		other[2].Regimes[0].Regime == a[2].Regimes[0].Regime && slices.Equal(other[2].Modules, a[2].Modules) {
		t.Fatal("sementes diferentes deveriam variar a massa")
	}
}
