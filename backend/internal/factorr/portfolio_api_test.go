package factorr_test

import (
	"context"
	"net/http"
	"slices"
	"testing"
	"time"

	"controle/internal/testutil"
	"controle/internal/testutil/seeded"
	"controle/internal/user"
)

type portfolioOut struct {
	PA              string `json:"pa"`
	MetaOperacional string `json:"meta_operacional"`
	KPIs            struct {
		Monitoradas        int    `json:"monitoradas"`
		NoV                int    `json:"no_v"`
		NoLimite           int    `json:"no_limite"`
		Seguras            int    `json:"seguras"`
		DadosInsuficientes int    `json:"dados_insuficientes"`
		EconomiaEmJogo     string `json:"economia_em_jogo"`
		HonorariosPacotes  string `json:"honorarios_pacotes"`
	} `json:"kpis"`
	Linhas []struct {
		Company struct {
			ID           string `json:"id"`
			InternalCode string `json:"internal_code"`
			LegalName    string `json:"legal_name"`
		} `json:"company"`
		PacoteComercial *string  `json:"pacote_comercial"`
		HonorarioMensal string   `json:"honorario_mensal"`
		Acao            string   `json:"acao"`
		AcaoTexto       string   `json:"acao_texto"`
		Semaforo        *string  `json:"semaforo"`
		Anexo           *string  `json:"anexo"`
		Economia12m     *string  `json:"economia_12m"`
		MesesFaltantes  []string `json:"meses_faltantes"`
	} `json:"linhas"`
}

func portfolio(t *testing.T, srv *testutil.Server, pa string) portfolioOut {
	t.Helper()
	var p portfolioOut
	srv.Get("/api/v1/factor-r/portfolio?pa="+pa).MustStatus(t, http.StatusOK).JSON(t, &p)
	return p
}

func codesOf(p portfolioOut) []string {
	out := make([]string, len(p.Linhas))
	for i, l := range p.Linhas {
		out[i] = l.Company.InternalCode
	}
	return out
}

// AC-007 e REQ-011: carteira ordenada por semáforo e economia, com ação sugerida, KPIs e o
// filtro da carteira (ativa, sujeita ao Fator R e no Simples no PA — D-017).
func TestAC007_PortfolioOrderActionsAndKPIs(t *testing.T) {
	srv := server(t)
	id := func(code string) string { return companyID(t, srv, code) }
	putWindow(t, srv, id("208"), "2026-09", map[string]any{"receita_bruta": "50000.00", "pro_labore": "10000.00"}) // 20%: vermelho
	putWindow(t, srv, id("205"), "2026-09", map[string]any{"receita_bruta": "15000.00", "pro_labore": "1000.00"})  // vermelho, economia menor
	putWindow(t, srv, id("206"), "2026-09", map[string]any{"receita_bruta": "50000.00", "pro_labore": "14000.00"}) // 28%: amarelo
	putWindow(t, srv, id("202"), "2026-09", map[string]any{"receita_bruta": "50000.00", "pro_labore": "15000.00"}) // 30%: verde
	// 210 sem lançamentos: dados insuficientes.
	srv.Patch("/api/v1/companies/"+id("208")+"/factor-r/settings", map[string]any{"version": 0, "pacote_comercial": "correcao", "honorario_mensal": "1500.50"}).MustStatus(t, http.StatusOK)
	srv.Patch("/api/v1/companies/"+id("202")+"/factor-r/settings", map[string]any{"version": 0, "pacote_comercial": "monitoramento", "honorario_mensal": "300"}).MustStatus(t, http.StatusOK)

	p := portfolio(t, srv, "2026-09")
	if want := []string{"208", "205", "206", "202", "210"}; !slices.Equal(codesOf(p), want) {
		t.Fatalf("ordem = %v; esperado %v", codesOf(p), want)
	}
	var texts []string
	for _, l := range p.Linhas {
		texts = append(texts, l.AcaoTexto)
	}
	if want := []string{"reunião de correção", "reunião de correção", "vigiar / simular", "manter", "completar lançamentos"}; !slices.Equal(texts, want) {
		t.Fatalf("ações = %v", texts)
	}
	k := p.KPIs
	if k.Monitoradas != 5 || k.NoV != 2 || k.NoLimite != 1 || k.Seguras != 1 || k.DadosInsuficientes != 1 {
		t.Fatalf("KPIs = %+v", k)
	}
	// Economia em jogo: 43.740 (208) + 17.100 (205) + 43.740 (206, amarelo).
	if dec(k.EconomiaEmJogo).Cmp(dec("104580")) != 0 || dec(k.HonorariosPacotes).Cmp(dec("1800.50")) != 0 {
		t.Fatalf("economia em jogo %s, honorários %s", k.EconomiaEmJogo, k.HonorariosPacotes)
	}
	last := p.Linhas[4]
	if last.Semaforo != nil || len(last.MesesFaltantes) != 12 || p.Linhas[0].HonorarioMensal != "1500.50" || str(p.Linhas[0].PacoteComercial) != "correcao" {
		t.Fatalf("linhas = %+v", p.Linhas)
	}
	if p.PA != "2026-09" || p.MetaOperacional != "0.300000" {
		t.Fatalf("PA %s, meta %s", p.PA, p.MetaOperacional)
	}
}

func TestPortfolio_OnlyActiveSubjectAndSimplesAtPA(t *testing.T) {
	srv := server(t)
	if got := codesOf(portfolio(t, srv, "2026-09")); !slices.Equal(sorted(got), []string{"202", "205", "206", "208", "210"}) {
		t.Fatalf("carteira 09/2026 = %v", got)
	}
	// 205 só está no Simples desde 01/2026.
	if got := codesOf(portfolio(t, srv, "2025-11")); slices.Contains(got, "205") || len(got) != 4 {
		t.Fatalf("carteira 11/2025 = %v", got)
	}
	ctx := context.Background()
	if _, err := srv.Pool.Exec(ctx, `UPDATE companies SET active = false WHERE internal_code = '210'`); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.Pool.Exec(ctx, `UPDATE companies SET factor_r = false WHERE internal_code = '202'`); err != nil {
		t.Fatal(err)
	}
	if got := codesOf(portfolio(t, srv, "2026-09")); !slices.Equal(sorted(got), []string{"205", "206", "208"}) {
		t.Fatalf("sem a inativa e a não sujeita = %v", got)
	}
}

func sorted(s []string) []string {
	out := slices.Clone(s)
	slices.Sort(out)
	return out
}

func TestPortfolio_ErrorsAndDefaultPA(t *testing.T) {
	srv := server(t)
	if p := portfolio(t, srv, ""); p.PA != "2026-10" {
		t.Fatalf("PA padrão = %s", p.PA)
	}
	if code := srv.Get("/api/v1/factor-r/portfolio?pa=2017-12").MustStatus(t, http.StatusUnprocessableEntity).ErrorCode(t); code != "SIMPLES_TABLE_NOT_IN_FORCE" {
		t.Fatalf("PA sem tabela: %s", code)
	}
	srv.Get("/api/v1/factor-r/portfolio?pa=10-2026").MustStatus(t, http.StatusBadRequest)
}

// AC-011: nenhuma operação do Fator R altera obrigação, status ou período do Controle.
func TestAC011_FactorRNeverTouchesObligations(t *testing.T) {
	pool := seeded.NewDemoDB(t)
	srv := testutil.NewServer(t, pool, seeded.DemoClock())
	ctx := context.Background()
	snapshot := func() string {
		var s string
		err := pool.QueryRow(ctx, `SELECT concat_ws('|',
			(SELECT count(*) FROM obligation_instances), (SELECT max(updated_at) FROM obligation_instances),
			(SELECT count(*) FROM obligation_status_history), (SELECT count(*) FROM company_module_periods),
			(SELECT max(updated_at) FROM company_module_periods), (SELECT max(updated_at) FROM companies))`).Scan(&s)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	before := snapshot()
	id := companyID(t, srv, "201") // Simples sem dados de demonstração do Fator R
	putWindow(t, srv, id, "2026-10", map[string]any{"receita_bruta": "50000.00", "pro_labore": "10000.00"})
	srv.Patch("/api/v1/companies/"+id+"/factor-r/settings", map[string]any{"version": 0, "pacote_comercial": "correcao"}).MustStatus(t, http.StatusOK)
	calc(t, srv, id, "2026-10")
	portfolio(t, srv, "2026-10")
	if after := snapshot(); after != before {
		t.Fatalf("o Fator R alterou dados do Controle:\nantes  %s\ndepois %s", before, after)
	}
}

// TEST-011, desempenho: carteira com 200 empresas em menos de 1 s (gate da origem, M3).
func TestTEST011_PortfolioWith200CompaniesUnderOneSecond(t *testing.T) {
	pool := seeded.NewDB(t)
	ctx := context.Background()
	stmts := []string{
		`INSERT INTO companies (internal_code, legal_name, cnpj, service_start_date, payroll_type, factor_r, created_by, updated_by)
		 SELECT 'P' || g, 'Empresa de carga ' || lpad(g::text, 3, '0'), lpad(g::text, 14, '0'), '2025-01-01', 'PAYROLL', true, $1, $1
		 FROM generate_series(1, 200) g`,
		`INSERT INTO company_tax_regime_history (company_id, tax_regime, effective_from_competence, created_by)
		 SELECT id, 'SIMPLES_NACIONAL', '2025-01-01', $1 FROM companies`,
		`INSERT INTO factor_r_movements (company_id, competence, receita_bruta, pro_labore, salarios_13_ferias, cpp_recolhida, fgts_recolhido, origem, updated_by)
		 SELECT c.id, m, 20000 + (abs(hashtext(c.id::text || m::text)) % 60000), 3000 + (abs(hashtext(c.legal_name || m::text)) % 9000), 2000, 600, 160, 'manual', $1
		 FROM companies c CROSS JOIN generate_series('2025-09-01'::date, '2026-08-01'::date, interval '1 month') m`,
	}
	for _, s := range stmts {
		if _, err := pool.Exec(ctx, s, user.AdminID); err != nil {
			t.Fatal(err)
		}
	}
	srv := testutil.NewServer(t, pool, testutil.FixedClock(t, "2026-10-01T12:00:00Z"))
	portfolio(t, srv, "2026-09") // aquece conexões e consultas preparadas
	start := time.Now()
	p := portfolio(t, srv, "2026-09")
	elapsed := time.Since(start)
	if p.KPIs.Monitoradas != 200 {
		t.Fatalf("monitoradas = %d", p.KPIs.Monitoradas)
	}
	if elapsed > time.Second {
		t.Fatalf("carteira com 200 empresas levou %s (limite 1 s)", elapsed)
	}
	t.Logf("carteira com 200 empresas e 2.400 movimentos: %s", elapsed)
}
