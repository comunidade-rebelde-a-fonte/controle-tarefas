//go:build golden

package pipeline_test

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"controle/internal/ai"
	"controle/internal/ai/litellm"
	"controle/internal/platform/config"
	"controle/internal/platform/decimal"
	"controle/internal/reconciliation/pipeline"
)

// Avaliação do extrator contra o golden com o LiteLLM real (make eval-extrator): custo de provedor
// real, só por ação explícita e autorizada [eval-plan §6]. O relatório vai para o log do teste e,
// com EVAL_REPORT, para esse arquivo.
func TestGoldenReport_RealGateway(t *testing.T) {
	base := os.Getenv("LITELLM_BASE_URL")
	if base == "" {
		t.Skip("defina LITELLM_BASE_URL (o LiteLLM do ambiente) para avaliar o extrator")
	}
	defaults, err := ai.DefaultsFromEnv(os.Getenv)
	if err != nil {
		t.Fatal(err)
	}
	pairs := loadGoldenPairs(t)
	if len(pairs) == 0 {
		t.Fatal("golden vazio")
	}
	gw := litellm.New(base, config.Secret(os.Getenv("LITELLM_API_KEY")), 180*time.Second)
	e := setupWith(t, gw, defaults, pipeline.Config{TriageThreshold: decimal.MustParse("0.70"), AgentOnly: true})
	rep := runGolden(t, e, pairs)
	b, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("relatório do extrator:\n%s", b)
	if path := os.Getenv("EVAL_REPORT"); path != "" {
		if err := os.WriteFile(path, b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
