package seed_test

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"

	"controle/internal/testutil"
	"controle/seed"
)

// Transcrição das tabelas do PRD §11.1-§11.5: código|label|progresso|final.
// Progresso "fora" = fora do denominador (Fiscal "Não se aplica").
var prdStatus = map[string][]string{
	"FISCAL_CLOSING": {
		"NOT_STARTED|A iniciar|não|não", "DOWNLOADED|Baixado|sim|não", "POSTED|Lançado|sim|não",
		"CHECKED|Conferido|sim|não", "MISSING_DOCUMENTS|Faltando documentos|não|não", "ERROR|Erro|não|não",
		"CLOSED|Encerrado|sim|sim",
	},
	"FISCAL_ITEM": {
		"NOT_STARTED|A iniciar|não|não", "DOWNLOADED|Baixadas|sim|não", "POSTED|Lançadas|sim|não",
		"CLOSED|Fechadas|sim|sim", "MISSING_DOCUMENTS|Documentos faltantes|não|não", "NO_MOVEMENT|Sem movimento|sim|sim",
		"NOT_APPLICABLE|Não se aplica|fora|sim",
	},
	"DECLARATION": {
		"NOT_STARTED|A iniciar|não|não", "TRANSMITTED|Transmitida|sim|sim", "CLOSED|Fechado|sim|sim",
		"PENDING|Pendente|não|não", "EXEMPT|Dispensado|sim|sim", "NO_MOVEMENT|Sem movimento|sim|sim",
	},
	"ACCOUNTING": {
		"NOT_STARTED|A iniciar|não|não", "PENDING|Pendente|não|não", "NO_MOVEMENT|Sem movimento|sim|sim", "OK|OK|sim|sim",
	},
	"PAYROLL": {
		"NOT_STARTED|A iniciar|não|não", "IN_PROGRESS|Em andamento|sim|não", "SENT_TO_CLIENT|Enviada ao Cliente|sim|não",
		"CLOSED|Fechada|sim|sim", "ARCHIVED|Salvo na pasta|sim|sim", "PENDING|Pendências|não|não", "NO_MOVEMENT|Sem movimento|sim|sim",
	},
}

var blocking = map[string]bool{"MISSING_DOCUMENTS": true, "ERROR": true, "PENDING": true}

func yesNo(b bool) string {
	if b {
		return "sim"
	}
	return "não"
}

func TestStatusSeed_MatchesPRDTablesAndIsIdempotent(t *testing.T) {
	pool := testutil.NewDB(t)
	ctx := context.Background()
	clk := testutil.FixedClock(t, "2026-10-01T12:00:00Z")
	for range 2 {
		if err := seed.Run(ctx, pool, clk, seed.ReferenceSteps(), io.Discard); err != nil {
			t.Fatal(err)
		}
	}

	rows, err := pool.Query(ctx, `
		SELECT s.code, d.code, d.label, d.counts_as_operational_progress, d.is_final, d.is_blocking,
		       d.excluded_from_denominator, d.counts_as_started
		FROM status_definitions d JOIN status_sets s ON s.id = d.status_set_id
		ORDER BY s.code, d.sort_order`)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string][]string{}
	total := 0
	for rows.Next() {
		var set, code, label string
		var progress, final, isBlocking, excluded, started bool
		if err := rows.Scan(&set, &code, &label, &progress, &final, &isBlocking, &excluded, &started); err != nil {
			t.Fatal(err)
		}
		p := yesNo(progress)
		if excluded {
			p = "fora"
		}
		got[set] = append(got[set], fmt.Sprintf("%s|%s|%s|%s", code, label, p, yesNo(final)))
		if isBlocking != blocking[code] {
			t.Errorf("%s/%s: is_blocking = %v", set, code, isBlocking)
		}
		if started != (code != "NOT_STARTED") {
			t.Errorf("%s/%s: counts_as_started = %v", set, code, started)
		}
		total++
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	for set, want := range prdStatus {
		if strings.Join(got[set], "\n") != strings.Join(want, "\n") {
			t.Errorf("conjunto %s difere do PRD:\n got: %v\nwant: %v", set, got[set], want)
		}
	}
	if len(got) != 5 || total != 31 {
		t.Fatalf("seed duplicou ou faltou: %d conjuntos, %d status (want 5, 31)", len(got), total)
	}
}
