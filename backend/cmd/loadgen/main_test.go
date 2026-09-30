package main

import "testing"

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
