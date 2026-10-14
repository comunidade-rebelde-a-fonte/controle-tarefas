package agents_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"controle/internal/ai"
	"controle/internal/ai/tracer"
	"controle/internal/factorr"
	"controle/internal/factorr/agents"
	"controle/internal/jobs"
	"controle/internal/platform/clock"
	"controle/internal/platform/config"
	"controle/internal/platform/decimal"
	"controle/internal/testutil"
	"controle/internal/testutil/seeded"
)

// TEST-013: simulador persistido e agentes consultor e priorizador contra Postgres real e um
// gateway LiteLLM simulado (nunca provedor real) [AC-010, AC-029, AC-031]. Porte de
// tests/api/test_simulations_api.py, tests/agents/test_agentes_api.py, test_llm_guardas.py e
// test_disclaimer_agentes.py da origem. PA 09/2026: a série vai de 09/2025 a 08/2026.

const disclaimer = factorr.Disclaimer

var (
	quiet   = slog.New(slog.NewTextHandler(io.Discard, nil))
	codeSeq atomic.Int32
)

// fakeGateway imita o /v1/chat/completions do LiteLLM: a chamada com response_format é o plan
// (classificação) e a sem é o render (texto). Status diferente de 200 simula a falha.
type fakeGateway struct {
	mu           sync.Mutex
	intent       string
	text         string
	planStatus   int
	renderStatus int
	requests     []map[string]any
}

func (f *fakeGateway) set(intent, text string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.intent, f.text, f.planStatus, f.renderStatus, f.requests = intent, text, http.StatusOK, http.StatusOK, nil
	if intent == "" {
		f.planStatus = http.StatusBadRequest
	}
	if text == "" {
		f.renderStatus = http.StatusBadRequest
	}
}

func (f *fakeGateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req map[string]any
	_ = json.NewDecoder(r.Body).Decode(&req)
	f.mu.Lock()
	f.requests = append(f.requests, req)
	isPlan := req["response_format"] != nil
	content, status := f.text, f.renderStatus
	if isPlan {
		content, status = f.intent, f.planStatus
	}
	f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	if status != http.StatusOK {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"error":{"message":"falha simulada"}}`))
		return
	}
	w.Header().Set("x-litellm-response-cost", "0.0000125")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"model":   req["model"],
		"choices": []any{map[string]any{"message": map[string]any{"content": content}, "finish_reason": "stop"}},
		"usage":   map[string]any{"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15},
	})
}

func (f *fakeGateway) calls() []map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]map[string]any(nil), f.requests...)
}

type env struct {
	srv *testutil.Server
	gw  *fakeGateway
}

func setup(t *testing.T) env {
	t.Helper()
	gw := &fakeGateway{}
	gw.set("", "")
	fake := httptest.NewServer(gw)
	t.Cleanup(fake.Close)
	model := ai.Model{Provider: "openai", Name: "gpt-teste"}
	srv := testutil.NewServer(t, seeded.NewDB(t), testutil.FixedClock(t, "2026-10-08T12:00:00Z"), func(c *config.Config) {
		c.LiteLLMBaseURL, c.LiteLLMAPIKey, c.LiteLLMTimeout = fake.URL, "chave-do-gateway-de-teste", 5*time.Second
		c.AIDefaults = ai.Defaults{ai.Consultor: model, ai.Priorizador: model}
	})
	return env{srv: srv, gw: gw}
}

// company cria a empresa (Simples Nacional, sujeita ao Fator R) com a série de 09/2025 a 08/2026:
// receita de 50.000,00 e o pró-labore dado em todos os meses.
func (e env) company(t *testing.T, name, cnpj, proLabore string) string {
	t.Helper()
	var c struct {
		ID string `json:"id"`
	}
	e.srv.Post("/api/v1/companies", map[string]any{
		"internal_code": fmt.Sprintf("AG%03d", codeSeq.Add(1)), "legal_name": name, "cnpj": cnpj,
		"service_start_date": "2024-01-01", "payroll_type": "PAYROLL", "tax_regime": "SIMPLES_NACIONAL", "factor_r": true,
	}).MustStatus(t, http.StatusCreated).JSON(t, &c)
	for m := 0; m < 12; m++ {
		comp := fmt.Sprintf("%04d-%02d", 2025+(8+m)/12, (8+m)%12+1)
		e.srv.Put("/api/v1/companies/"+c.ID+"/factor-r/movements/"+comp, map[string]any{"receita_bruta": "50000.00", "pro_labore": proLabore}).
			MustStatus(t, http.StatusOK)
	}
	return c.ID
}

func (e env) count(t *testing.T, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := e.srv.Pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

type agentOut struct {
	Texto   string         `json:"texto"`
	Decisao map[string]any `json:"decisao"`
	TraceID string         `json:"trace_id"`
}

func (e env) consult(t *testing.T, body map[string]any) agentOut {
	t.Helper()
	var out agentOut
	e.srv.Post("/api/v1/factor-r/agents/consult", body).MustStatus(t, http.StatusOK).JSON(t, &out)
	return out
}

// --- Simulador [AC-010] ---

type simOut struct {
	SimulationID string         `json:"simulation_id"`
	TraceID      string         `json:"trace_id"`
	PA           string         `json:"pa"`
	Veredito     *string        `json:"veredito"`
	Texto        string         `json:"texto"`
	Parametros   map[string]any `json:"parametros"`
	Resultado    map[string]any `json:"resultado"`
}

func TestSimulation_PersistedInTraceWithTheVerdicts(t *testing.T) {
	e := setup(t)
	id := e.company(t, "Beta Engenharia", "11.222.333/0001-81", "10000.00") // Fator R 20%
	var corrigir simOut
	e.srv.Post("/api/v1/companies/"+id+"/factor-r/simulations", map[string]any{"pa": "2026-09"}).MustStatus(t, http.StatusCreated).JSON(t, &corrigir)
	if corrigir.Veredito == nil || *corrigir.Veredito != "corrigir" || corrigir.Resultado["reforco_12m"] != "60000.00" ||
		!strings.Contains(corrigir.Texto, "Vale corrigir") || !strings.HasSuffix(corrigir.Texto, disclaimer) || corrigir.PA != "2026-09" {
		t.Fatalf("simulação = %+v", corrigir)
	}
	var naoForcar simOut
	e.srv.Post("/api/v1/companies/"+id+"/factor-r/simulations", map[string]any{"pa": "2026-09", "inss": "0.30", "irrf": "0.50"}).
		MustStatus(t, http.StatusCreated).JSON(t, &naoForcar)
	if naoForcar.Veredito == nil || *naoForcar.Veredito != "nao_forcar" || naoForcar.Parametros["inss"] != "0.30" {
		t.Fatalf("não forçar = %+v", naoForcar)
	}
	var meta28 simOut
	e.srv.Post("/api/v1/companies/"+id+"/factor-r/simulations", map[string]any{"pa": "2026-09", "meta": "0.28", "fracao_pro_labore": "1"}).
		MustStatus(t, http.StatusCreated).JSON(t, &meta28)
	if meta28.Veredito == nil || (*meta28.Veredito != "corrigir" && *meta28.Veredito != "nao_forcar") { // 20% < 28%
		t.Fatalf("meta 28%% = %+v", meta28)
	}
	// No trace: corrida do consultor (gatilho ficha), decisão da simulação e simulação ligada a ele.
	if n := e.count(t, `SELECT count(*) FROM ai_runs WHERE trace_id = $1 AND agent = 'consultor' AND trigger = 'ficha' AND status = 'ok' AND company_id = $2`,
		corrigir.TraceID, id); n != 1 {
		t.Fatal("corrida do simulador não encontrada")
	}
	if n := e.count(t, `SELECT count(*) FROM ai_decisions WHERE trace_id = $1 AND kind = 'simulacao' AND data ->> 'simulation_id' = $2`, corrigir.TraceID, corrigir.SimulationID); n != 1 {
		t.Fatal("decisão da simulação não encontrada")
	}
	if n := e.count(t, `SELECT count(*) FROM factor_r_simulations WHERE id = $1 AND trace_id = $2 AND veredito = 'corrigir' AND reforco_12m = 60000`, corrigir.SimulationID, corrigir.TraceID); n != 1 {
		t.Fatal("simulação gravada não encontrada")
	}
	// Sem LLM: nenhuma chamada ao gateway.
	if len(e.gw.calls()) != 0 {
		t.Fatal("o simulador chamou o LLM")
	}
	var hist struct {
		Items []simOut `json:"items"`
	}
	e.srv.Get("/api/v1/companies/"+id+"/factor-r/simulations").MustStatus(t, http.StatusOK).JSON(t, &hist)
	if len(hist.Items) != 3 || hist.Items[2].SimulationID != corrigir.SimulationID || !strings.Contains(hist.Items[2].Texto, "Vale corrigir") {
		t.Fatalf("histórico = %+v", hist.Items)
	}
}

func TestSimulation_OnTargetAndValidation(t *testing.T) {
	e := setup(t)
	id := e.company(t, "Gama Serviços", "45.723.174/0001-10", "15000.00") // 30%
	var ja simOut
	e.srv.Post("/api/v1/companies/"+id+"/factor-r/simulations", map[string]any{"pa": "2026-09"}).MustStatus(t, http.StatusCreated).JSON(t, &ja)
	if ja.Veredito == nil || *ja.Veredito != "ja_na_meta" {
		t.Fatalf("já na meta = %+v", ja)
	}
	for _, bad := range []map[string]any{
		{"pa": "2026-09", "meta": "0.20"}, {"pa": "2026-09", "inss": "1.5"}, {"pa": "2026-09", "horizonte_meses": 0},
		{"pa": "09/2026"}, {"pa": "2026-09", "irrf": "0.1234567"},
	} {
		e.srv.Post("/api/v1/companies/"+id+"/factor-r/simulations", bad).MustStatus(t, http.StatusBadRequest)
	}
	e.srv.Post("/api/v1/companies/00000000-0000-0000-0000-000000000000/factor-r/simulations", map[string]any{"pa": "2026-09"}).MustStatus(t, http.StatusNotFound)
	// Sem tabela do Simples vigente: 422 e a corrida de erro fica gravada.
	if code := e.srv.Post("/api/v1/companies/"+id+"/factor-r/simulations", map[string]any{"pa": "2017-12"}).MustStatus(t, http.StatusUnprocessableEntity).ErrorCode(t); code != "SIMPLES_TABLE_NOT_IN_FORCE" {
		t.Fatalf("tabela = %s", code)
	}
	if n := e.count(t, `SELECT count(*) FROM ai_runs WHERE trigger = 'ficha' AND status = 'error' AND error_code = 'SIMPLES_TABLE_NOT_IN_FORCE'`); n != 1 {
		t.Fatal("corrida de erro não registrada")
	}
}

// --- Consultor ---

func TestConsultor_StatusWithValidatedLLMText(t *testing.T) {
	e := setup(t)
	id := e.company(t, "Beta Engenharia", "11.222.333/0001-81", "12000.00") // 24%, Anexo V
	e.gw.set(`{"intencao":"status_empresa","company_ref":"Beta Engenharia","pa":"2026-09","meta":null}`,
		"A Beta Engenharia está com Fator R de 24,00% e está no Anexo V.")
	out := e.consult(t, map[string]any{"mensagem": "como está a Beta Engenharia?"})
	if out.Decisao["anexo"] != "V" || out.Decisao["empresa"] != "Beta Engenharia" || out.Decisao["fator_r"] != "0.240000" ||
		!strings.HasPrefix(out.Texto, "A Beta Engenharia está com Fator R de 24,00%") || !strings.HasSuffix(out.Texto, disclaimer) {
		t.Fatalf("resposta = %+v", out)
	}
	// Corrida com o snapshot do modelo, os prompts versionados e as duas gerações (plan e render).
	if n := e.count(t, `SELECT count(*) FROM ai_runs WHERE trace_id = $1 AND agent = 'consultor' AND trigger = 'chat' AND status = 'ok'
		AND alias = 'consultor' AND model = 'gpt-teste' AND prompt_version LIKE 'fatorr/plan@%' AND tokens_in = 20 AND cost_usd > 0`, out.TraceID); n != 1 {
		t.Fatal("corrida do consultor sem modelo, prompts ou consumo")
	}
	if n := e.count(t, `SELECT count(*) FROM ai_decisions WHERE trace_id = $1 AND kind = 'consultor_resposta'`, out.TraceID); n != 1 {
		t.Fatal("decisão do consultor ausente")
	}
	// O plan só leva o pedido; nada de lista de empresas, CNPJ ou valores (M3 da origem).
	calls := e.gw.calls()
	if len(calls) != 2 {
		t.Fatalf("chamadas ao gateway = %d", len(calls))
	}
	planBody, _ := json.Marshal(calls[0])
	for _, forbidden := range []string{"11222333000181", "600000", "rbt12", "economia"} {
		if strings.Contains(strings.ToLower(string(planBody)), forbidden) {
			t.Errorf("o plan levou %q ao LLM", forbidden)
		}
	}
	if calls[0]["model"] != "openai/gpt-teste" || !strings.Contains(string(planBody), "como está a Beta Engenharia?") {
		t.Errorf("plan = %s", planBody)
	}
	_ = id
}

func TestConsultor_InventedNumberFallsBackToTemplate(t *testing.T) {
	e := setup(t)
	id := e.company(t, "Beta Engenharia", "11.222.333/0001-81", "12000.00")
	e.gw.set(`{"intencao":"status_empresa","company_ref":null,"pa":"2026-09","meta":null}`, "Economia de R$ 1.234.567,00!")
	out := e.consult(t, map[string]any{"mensagem": "situação", "company_id": id})
	if strings.Contains(out.Texto, "1.234.567") || !strings.HasPrefix(out.Texto, "Beta Engenharia, PA 2026-09: Fator R 24,00% (Anexo V, semáforo vermelho).") {
		t.Fatalf("texto = %q", out.Texto)
	}
	var meta []byte
	if err := e.srv.Pool.QueryRow(context.Background(), `SELECT spans FROM ai_runs WHERE trace_id = $1`, out.TraceID).Scan(&meta); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(meta), `"render_rejeitado": "true"`) || !strings.Contains(string(meta), "1.234.567,00") {
		t.Fatalf("span render sem o registro da rejeição: %s", meta)
	}
}

func TestConsultor_SimulatePersistsAndPassesGuard(t *testing.T) {
	e := setup(t)
	id := e.company(t, "Beta Engenharia", "11.222.333/0001-81", "10000.00")
	e.gw.set(`{"intencao":"simular","company_ref":null,"pa":"2026-09","meta":null}`, "")
	out := e.consult(t, map[string]any{"mensagem": "simule a correção", "company_id": id})
	if out.Decisao["veredito"] != "corrigir" || !strings.HasSuffix(out.Texto, disclaimer) ||
		!strings.Contains(out.Texto, "Vale corrigir") || !strings.Contains(out.Texto, "Líquido estimado: R$") {
		t.Fatalf("resposta = %+v", out)
	}
	simID, _ := out.Decisao["simulation_id"].(string)
	if n := e.count(t, `SELECT count(*) FROM factor_r_simulations WHERE id = $1 AND trace_id = $2`, simID, out.TraceID); n != 1 {
		t.Fatal("simulação não ligada ao trace do consultor")
	}
	if n := e.count(t, `SELECT count(*) FROM ai_decisions WHERE trace_id = $1 AND kind IN ('simulacao', 'consultor_resposta')`, out.TraceID); n != 2 {
		t.Fatal("decisões da simulação e da resposta")
	}
}

func TestConsultor_KeywordFallbackWithoutLLM(t *testing.T) {
	e := setup(t)
	e.company(t, "Beta Engenharia", "11.222.333/0001-81", "12000.00")
	e.gw.set("", "") // gateway recusa: plan por palavras-chave e texto pelo template
	out := e.consult(t, map[string]any{"mensagem": "qual a situação da Beta Engenharia em 09/2026?"})
	if out.Decisao["intencao"] != "status_empresa" || out.Decisao["pa"] != "2026-09" || out.Decisao["empresa"] != "Beta Engenharia" ||
		!strings.HasPrefix(out.Texto, "Beta Engenharia, PA 2026-09:") {
		t.Fatalf("resposta = %+v", out)
	}
	// LLM desligado na tela de IA: nem chega a chamar o gateway.
	if _, err := e.srv.Pool.Exec(context.Background(), `UPDATE ai_model_aliases SET enabled = false WHERE alias = 'consultor'`); err != nil {
		t.Fatal(err)
	}
	e.gw.set(`{"intencao":"priorizar","company_ref":null,"pa":null,"meta":null}`, "x")
	out = e.consult(t, map[string]any{"mensagem": "o que priorizar esta semana em 09/2026?"})
	if out.Decisao["intencao"] != "priorizar" || len(e.gw.calls()) != 0 {
		t.Fatalf("sem LLM = %+v, chamadas %d", out, len(e.gw.calls()))
	}
	if n := e.count(t, `SELECT count(*) FROM ai_runs WHERE trace_id = $1 AND model IS NULL AND metadata ->> 'llm_indisponivel' = 'alias_desligado'`, out.TraceID); n != 1 {
		t.Fatal("corrida sem o registro do LLM desligado")
	}
}

func TestConsultor_UnknownCompanyAndValidation(t *testing.T) {
	e := setup(t)
	e.srv.Post("/api/v1/factor-r/agents/consult", map[string]any{"mensagem": "situação", "company_id": "00000000-0000-0000-0000-000000000000"}).MustStatus(t, http.StatusNotFound)
	e.srv.Post("/api/v1/factor-r/agents/consult", map[string]any{"mensagem": "situação", "company_id": "x"}).MustStatus(t, http.StatusBadRequest)
	e.srv.Post("/api/v1/factor-r/agents/consult", map[string]any{"mensagem": ""}).MustStatus(t, http.StatusBadRequest)
	e.srv.Post("/api/v1/factor-r/agents/consult", map[string]any{"mensagem": strings.Repeat("a", 1001)}).MustStatus(t, http.StatusBadRequest)
	// Empresa não identificada: resposta em revisão, com o template.
	e.gw.set("", "")
	out := e.consult(t, map[string]any{"mensagem": "como está a Empresa Que Não Existe?"})
	if out.Decisao["erro"] != "empresa_nao_identificada" || !strings.HasPrefix(out.Texto, "Não identifiquei a empresa.") {
		t.Fatalf("resposta = %+v", out)
	}
	if n := e.count(t, `SELECT count(*) FROM ai_runs WHERE trace_id = $1 AND status = 'needs_review'`, out.TraceID); n != 1 {
		t.Fatal("corrida sem empresa deveria ficar em revisão")
	}
}

// --- Priorizador ---

func TestPriorizador_QueueBySavingsWithTrace(t *testing.T) {
	e := setup(t)
	e.company(t, "Verde", "11.222.333/0001-81", "15000.00")
	e.company(t, "Amarela", "45.723.174/0001-10", "14500.00")
	e.company(t, "Vermelha", "04.252.011/0001-10", "5000.00")
	e.gw.set(`{"intencao":"priorizar","company_ref":null,"pa":null,"meta":null}`, "")
	var out agentOut
	e.srv.Post("/api/v1/factor-r/agents/prioritize", map[string]any{"mensagem": "o que priorizar?", "pa": "2026-09"}).MustStatus(t, http.StatusOK).JSON(t, &out)
	fila, _ := out.Decisao["fila"].([]any)
	var names []string
	var savings []decimal.Decimal
	for _, it := range fila {
		m := it.(map[string]any)
		names = append(names, m["empresa"].(string))
		savings = append(savings, decimal.MustParse(m["economia_12m"].(string)))
	}
	if len(names) != 2 || strings.Contains(strings.Join(names, ","), "Verde") || len(out.TraceID) != 32 || !strings.HasSuffix(out.Texto, disclaimer) ||
		!strings.Contains(out.Texto, "2 empresa(s) na fila") {
		t.Fatalf("fila = %v / %+v", names, out)
	}
	if savings[0].Cmp(savings[1]) < 0 {
		t.Errorf("fila fora da ordem de economia: %v", savings)
	}
	if n := e.count(t, `SELECT count(*) FROM ai_decisions WHERE trace_id = $1 AND kind = 'fila_priorizacao'`, out.TraceID); n != 1 {
		t.Fatal("decisão da fila ausente")
	}
}

func TestPriorizador_WeeklyRoutineRunsOncePerWeek(t *testing.T) {
	e := setup(t)
	e.company(t, "Vermelha", "04.252.011/0001-10", "5000.00")
	sys := clock.NewSystem(testutil.SaoPaulo)
	svc := agents.NewService(e.srv.Pool, sys, sys, tracer.New(sys, jobs.NewQueue(sys, 3)), nil, nil, quiet)
	// A rotina conta as corridas desde a segunda da semana pelo created_at real (relógio do
	// sistema), então a data de referência é a de hoje.
	now := sys.Now()
	if n, err := svc.WeeklyRoutine(context.Background(), now); err != nil || n != 1 {
		t.Fatalf("primeira rotina = %d, %v", n, err)
	}
	if n, err := svc.WeeklyRoutine(context.Background(), now); err != nil || n != 0 {
		t.Fatalf("segunda rotina na semana = %d, %v", n, err)
	}
	if n := e.count(t, `SELECT count(*) FROM ai_runs WHERE agent = 'priorizador' AND trigger = 'rotina' AND user_id IS NULL AND model IS NULL`); n != 1 {
		t.Fatalf("corridas da rotina = %d", n)
	}
	if n := e.count(t, `SELECT count(*) FROM ai_decisions WHERE kind = 'fila_priorizacao'`); n != 1 {
		t.Fatal("decisão da rotina ausente")
	}
	if len(e.gw.calls()) != 0 {
		t.Fatal("a rotina chamou o LLM")
	}
}

// Plan com saída inválida do LLM ou gateway fora do ar: cai no classificador por palavras-chave
// (test_plan_cai_no_fallback_por_erro_http_e_json_invalido da origem).
func TestPlan_FallbackOnInvalidIntentAndGatewayError(t *testing.T) {
	e := setup(t)
	e.company(t, "Vermelha", "04.252.011/0001-10", "5000.00")
	for _, intent := range []string{`{"intencao":"inventada","company_ref":null,"pa":null,"meta":null}`, `não é JSON`} {
		e.gw.set(intent, "")
		out := e.consult(t, map[string]any{"mensagem": "o que priorizar esta semana em 09/2026?"})
		if out.Decisao["intencao"] != "priorizar" || out.Decisao["pa"] != "2026-09" {
			t.Fatalf("fallback com %q = %+v", intent, out.Decisao)
		}
	}
	// Erro 500 do gateway (com as novas tentativas do cliente): também cai no fallback.
	e.gw.set(`{"intencao":"status_empresa","company_ref":null,"pa":null,"meta":null}`, "")
	e.gw.mu.Lock()
	e.gw.planStatus = http.StatusInternalServerError
	e.gw.mu.Unlock()
	out := e.consult(t, map[string]any{"mensagem": "o que priorizar esta semana em 09/2026?"})
	if out.Decisao["intencao"] != "priorizar" {
		t.Fatalf("fallback com erro 500 = %+v", out.Decisao)
	}
	var meta []byte
	if err := e.srv.Pool.QueryRow(context.Background(), `SELECT spans FROM ai_runs WHERE trace_id = $1`, out.TraceID).Scan(&meta); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(meta), `"fallback": true`) || !strings.Contains(string(meta), `"level": "ERROR"`) {
		t.Fatalf("plan sem o registro do fallback e da geração com erro: %s", meta)
	}
}

// Toda resposta de agente traz o disclaimer do PGDAS-D (T-704 da origem).
func TestAllAgentResponsesHaveDisclaimer(t *testing.T) {
	e := setup(t)
	id := e.company(t, "Beta Engenharia", "11.222.333/0001-81", "10000.00")
	e.gw.set("", "")
	texts := []string{
		e.consult(t, map[string]any{"mensagem": "situação em 09/2026", "company_id": id}).Texto,
		e.consult(t, map[string]any{"mensagem": "simule a correção em 09/2026", "company_id": id}).Texto,
		e.consult(t, map[string]any{"mensagem": "o que é fator r?"}).Texto,
	}
	var pr agentOut
	e.srv.Post("/api/v1/factor-r/agents/prioritize", map[string]any{"mensagem": "priorizar", "pa": "2026-09"}).MustStatus(t, http.StatusOK).JSON(t, &pr)
	var sim simOut
	e.srv.Post("/api/v1/companies/"+id+"/factor-r/simulations", map[string]any{"pa": "2026-09"}).MustStatus(t, http.StatusCreated).JSON(t, &sim)
	texts = append(texts, pr.Texto, sim.Texto)
	for _, text := range texts {
		if !strings.HasSuffix(strings.TrimRight(text, " \n"), disclaimer) {
			t.Errorf("sem disclaimer: %q", text)
		}
	}
}
