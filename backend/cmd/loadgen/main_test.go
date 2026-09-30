package main

import (
	"encoding/json"
	"testing"
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

func TestListBodyCount(t *testing.T) {
	cases := []struct {
		body string
		want string
	}{
		{`{"rows":[{},{},{}]}`, "3"},
		{`{"rows":[]}`, "0"},
		{`{"items":[{},{}],"total":250}`, "2 de 250"},
		{`{"categories":[{}]}`, "1 categorias"},
		{`{"kpis":{}}`, "—"},
	}
	for _, c := range cases {
		var b listBody
		if err := json.Unmarshal([]byte(c.body), &b); err != nil {
			t.Fatal(err)
		}
		if got := b.count(); got != c.want {
			t.Errorf("%s: got %q, want %q", c.body, got, c.want)
		}
	}
}
