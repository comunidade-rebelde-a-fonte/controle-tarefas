package agents

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"

	"controle/internal/ai/tracer"
	"controle/internal/factorr"
	"controle/internal/platform/apperr"
	"controle/internal/platform/httpx"
	"controle/internal/store"
)

// Spans plan, decide e render compartilhados pelo consultor e pelo priorizador (etapas.py).

// plan classifica o pedido: pelo LLM e, se ele falhar, por palavras-chave. A empresa citada é
// resolvida localmente contra a lista de empresas, nunca pelo LLM.
func plan(ctx context.Context, run *tracer.Run, llm *LLM, message string, names []string) (Intent, error) {
	var in Intent
	err := run.Span(tracer.SpanPlan, map[string]any{"mensagem": message, "total_empresas": len(names)}, func(s tracer.SpanHandle) error {
		s.Meta("prompt_version", planPrompt.Ref())
		fallback, reason := false, ""
		var err error
		if llm == nil {
			err = unavailable("llm_nao_configurado", nil)
		} else {
			in, err = llm.Classify(ctx, run, message)
		}
		switch {
		case err == nil:
			if local := CitedCompany(message, names); local != nil {
				in.CompanyRef = local
			}
		case errors.Is(err, ErrLLMUnavailable):
			in = ClassifyByKeywords(message, names)
			fallback, reason = true, err.Error()
		default:
			return err
		}
		out := map[string]any{
			"intencao": in.Kind, "company_ref": in.CompanyRef, "pa": in.PA, "meta": nil,
			"fallback": fallback, "motivo_fallback": nilIfEmpty(reason),
		}
		if in.Meta != nil {
			out["meta"] = in.Meta.String()
		}
		s.Output(out)
		return nil
	})
	return in, err
}

// ErrRecommendationWithoutSimulation é o "corrigir" sem simulação persistida que o justifique
// (RecomendacaoSemSimulacao da origem, PRD §7.9): a corrida termina em erro.
func errRecommendationWithoutSimulation(msg string) error {
	return apperr.Conflict("RECOMMENDATION_WITHOUT_SIMULATION", msg)
}

// ensureSimulationForCorrect é a guarda do decide: recomendação "corrigir" só com o
// simulation_id de uma simulação gravada com esse veredito.
func ensureSimulationForCorrect(ctx context.Context, q *store.Queries, decision map[string]any) error {
	pm, _ := plain(decision).(map[string]any)
	recommends := false
	for _, k := range []string{"veredito", "recomendacao", "acao"} {
		if v, _ := pm[k].(string); v == "corrigir" {
			recommends = true
		}
	}
	if !recommends {
		return nil
	}
	id, _ := pm["simulation_id"].(string)
	if !httpx.IsUUID(strings.TrimSpace(id)) {
		return errRecommendationWithoutSimulation("Recomendação 'corrigir' sem simulation_id.")
	}
	verdict, err := q.GetFactorRSimulationVerdict(ctx, strings.TrimSpace(id))
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && (verdict == nil || *verdict != "corrigir")) {
		return errRecommendationWithoutSimulation("Recomendação 'corrigir' sem simulação correspondente.")
	}
	return err
}

// decide grava o passo de decisão com a guarda do "corrigir".
func decide(ctx context.Context, run *tracer.Run, q *store.Queries, decision map[string]any) error {
	return run.Span(tracer.SpanDecide, plain(decision), func(s tracer.SpanHandle) error {
		if err := ensureSimulationForCorrect(ctx, q, decision); err != nil {
			return err
		}
		s.Output(plain(decision))
		return nil
	})
}

// render redige o texto: o do LLM se passar na validação contra a decisão; senão, o template.
// Sempre com o disclaimer do PGDAS-D.
func render(ctx context.Context, run *tracer.Run, llm *LLM, decision map[string]any, template string, useLLM bool) (string, error) {
	var final string
	err := run.Span(tracer.SpanRender, nil, func(s tracer.SpanHandle) error {
		s.Meta("prompt_version", renderPrompt.Ref())
		text := template
		rejected, usedTemplate := false, true
		var orphans []string
		reason := ""
		if useLLM && llm != nil {
			candidate, err := llm.Write(ctx, run, decision)
			switch {
			case err == nil:
				if ok, o := ValidateText(candidate, decision); ok {
					text, usedTemplate = candidate, false
				} else {
					rejected, orphans = true, o
				}
			case errors.Is(err, ErrLLMUnavailable):
				reason = err.Error()
			default:
				return err
			}
		} else if useLLM {
			reason = "llm_nao_configurado"
		}
		final = factorr.WithDisclaimer(text)
		s.Output(map[string]any{"texto": final})
		s.Meta("render_rejeitado", boolText(rejected))
		s.Meta("numeros_orfaos", strings.Join(orphans, ", "))
		s.Meta("llm_indisponivel", reason)
		s.Meta("usou_template", boolText(usedTemplate))
		return nil
	})
	return final, err
}

func boolText(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
