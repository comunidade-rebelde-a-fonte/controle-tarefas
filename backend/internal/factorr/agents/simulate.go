package agents

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"controle/internal/ai/tracer"
	"controle/internal/factorr"
	"controle/internal/factorr/engine"
	"controle/internal/platform/competence"
	"controle/internal/platform/decimal"
	"controle/internal/store"
)

// Padrões do simulador (services/simulacao.py da origem).
var (
	defaultFraction = decimal.MustParse("1")
	defaultINSS     = decimal.MustParse("0.11")
	defaultIRRF     = decimal.MustParse("0.275")
)

const defaultHorizon = 12

// verdictTexts explica cada veredito (VEREDITOS_TEXTO).
var verdictTexts = map[engine.Verdict]string{
	engine.VerdictOnTarget: "A empresa já está na meta; não é preciso corrigir.",
	engine.VerdictCorrect:  "Vale corrigir: a economia de DAS supera o custo de INSS e IRRF do pró-labore extra.",
	engine.VerdictDontPush: "Não forçar: a economia não compensa o custo ou fica abaixo do piso do escritório.",
}

// SimulationParams é a entrada da simulação (EntradaSimulacao): meta nula = a da política.
type SimulationParams struct {
	PA       competence.Competence
	Meta     *decimal.Decimal
	Fraction decimal.Decimal
	INSS     decimal.Decimal
	IRRF     decimal.Decimal
	Horizon  int
}

func defaultParams(pa competence.Competence, meta *decimal.Decimal) SimulationParams {
	return SimulationParams{PA: pa, Meta: meta, Fraction: defaultFraction, INSS: defaultINSS, IRRF: defaultIRRF, Horizon: defaultHorizon}
}

// simulated é a simulação gravada.
type simulated struct {
	ID     string
	Result engine.SimulationResult
	Params engine.SimulationParams
}

func jsonObject(v any) ([]byte, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("agents: serializar: %w", err)
	}
	return b, nil
}

// simulateAndRecord roda o motor e o simulador nos spans tool e decide da corrida e grava a
// simulação no trace (simular_e_registrar): é ela que autoriza um "corrigir".
func simulateAndRecord(ctx context.Context, run *tracer.Run, q *store.Queries, companyID string, p SimulationParams, userID *string, now time.Time) (simulated, error) {
	pol, err := factorr.Policy(ctx, q)
	if err != nil {
		return simulated{}, err
	}
	params := engine.SimulationParams{
		Target: pol.MetaOperacional, ProLaboreFraction: p.Fraction, INSS: p.INSS, IRRF: p.IRRF,
		HorizonMonths: p.Horizon, MinAnnualSavings: pol.PisoEconomiaAnual,
	}
	if p.Meta != nil {
		params.Target = *p.Meta
	}
	var res engine.SimulationResult
	err = run.Span(tracer.SpanTool, map[string]any{"pa": p.PA.String(), "parametros": params}, func(s tracer.SpanHandle) error {
		calc, err := factorr.CalcWith(ctx, q, companyID, p.PA)
		if err != nil {
			return err
		}
		res = engine.Simulate(calc.Result, params)
		s.Output(res)
		return nil
	})
	if err != nil {
		return simulated{}, err
	}
	out := simulated{Result: res, Params: params}
	err = run.Span(tracer.SpanDecide, nil, func(s tracer.SpanHandle) error {
		paramsJSON, err := jsonObject(params)
		if err != nil {
			return err
		}
		resultJSON, err := jsonObject(res)
		if err != nil {
			return err
		}
		var verdict *string
		if res.Verdict != nil {
			v := string(*res.Verdict)
			verdict = &v
		}
		out.ID, err = q.InsertFactorRSimulation(ctx, store.InsertFactorRSimulationParams{
			CompanyID: companyID, Pa: p.PA.FirstDay(), Meta: params.Target, FracaoProLabore: params.ProLaboreFraction,
			Inss: params.INSS, Irrf: params.IRRF, HorizonteMeses: int32Of(params.HorizonMonths),
			PisoEconomiaAnual: params.MinAnnualSavings, Status: string(res.Status), Veredito: verdict,
			Reforco12m: res.TopUp12m, ReforcoMensal: res.MonthlyTopUp, CustoTotal: res.TotalCost,
			Economia12m: res.Savings12m, Liquido: res.Net, ParametrosJson: paramsJSON, ResultadoJson: resultJSON,
			TraceID: run.TraceID(), CreatedBy: userID, CreatedAt: now,
		})
		if err != nil {
			return err
		}
		decision := map[string]any{"simulation_id": out.ID, "veredito": verdict}
		if err := run.RecordDecision(ctx, "simulacao", decision); err != nil {
			return err
		}
		s.Output(decision)
		return nil
	})
	return out, err
}

// simulationText é o texto do simulador (texto_simulacao, sem LLM).
func simulationText(companyName string, pa competence.Competence, r engine.SimulationResult) string {
	if r.Verdict == nil {
		reason := "None"
		if r.Reason != nil {
			reason = string(*r.Reason)
		}
		return factorr.WithDisclaimer(fmt.Sprintf("%s, PA %s: dados insuficientes para simular (%s).", companyName, pa, reason))
	}
	return factorr.WithDisclaimer(fmt.Sprintf("%s, PA %s: %s", companyName, pa, verdictTexts[*r.Verdict]))
}

// Campos em reais do resultado, arredondados só na exibição (para_exibicao).
var moneyFields = []string{
	"reforco_12m", "reforco_mensal", "pro_labore_extra_mensal", "pro_labore_extra_horizonte",
	"custo_inss", "custo_irrf", "custo_total", "economia_12m", "economia_horizonte", "liquido",
}

// forDisplay arredonda os valores em reais a centavos e o fator atual a 6 casas; o banco guarda o
// valor completo.
func forDisplay(raw []byte) (map[string]any, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var out map[string]any
	if err := dec.Decode(&out); err != nil {
		return nil, err
	}
	round := func(field string, exp int32) {
		s, ok := out[field].(string)
		if !ok {
			return
		}
		if d, err := decimal.Parse(s); err == nil {
			out[field] = d.Quantize(exp).Text()
		}
	}
	for _, f := range moneyFields {
		round(f, -2)
	}
	round("fator_atual", -6)
	return out, nil
}

// int32Of converte o horizonte (1 a 60, validado na entrada) para a coluna integer.
func int32Of(n int) int32 {
	switch {
	case n < 0:
		return 0
	case n > math.MaxInt32:
		return math.MaxInt32
	}
	return int32(n)
}
