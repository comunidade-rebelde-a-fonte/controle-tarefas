package factorr_test

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"

	"controle/internal/platform/competence"
	"controle/internal/testutil"
	"controle/internal/testutil/seeded"
)

// TEST-011: API do Fator R contra Postgres real, sobre as 20 empresas fictícias. Hoje é
// 01/10/2026 em São Paulo (PA corrente 10/2026). Empresas sujeitas ao Fator R no seed: 202,
// 205 (Simples só desde 01/2026), 206, 208 e 210.

func server(t *testing.T) *testutil.Server {
	t.Helper()
	return testutil.NewServer(t, seeded.NewCompaniesDB(t), testutil.FixedClock(t, "2026-10-01T12:00:00Z"))
}

func companyID(t *testing.T, srv *testutil.Server, code string) string {
	t.Helper()
	var id string
	if err := srv.Pool.QueryRow(context.Background(), `SELECT id FROM companies WHERE internal_code = $1`, code).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

type movementOut struct {
	Competence               string  `json:"competence"`
	ReceitaBruta             string  `json:"receita_bruta"`
	ProLabore                string  `json:"pro_labore"`
	Salarios13Ferias         string  `json:"salarios_13_ferias"`
	CppRecolhida             string  `json:"cpp_recolhida"`
	FgtsRecolhido            string  `json:"fgts_recolhido"`
	FolhaTotalDeclaradaPgdas *string `json:"folha_total_declarada_pgdas"`
	FolhaMes                 string  `json:"folha_mes"`
	Origem                   string  `json:"origem"`
	Observacao               *string `json:"observacao"`
	Version                  int     `json:"version"`
}

type calcOut struct {
	PA                 string   `json:"pa"`
	JanelaInicio       string   `json:"janela_inicio"`
	JanelaFim          string   `json:"janela_fim"`
	MesesPreenchidos   []string `json:"meses_preenchidos"`
	MesesFaltantes     []string `json:"meses_faltantes"`
	Status             string   `json:"status"`
	Motivo             *string  `json:"motivo"`
	RBT12              string   `json:"rbt12"`
	FS12               string   `json:"fs12"`
	FatorR             *string  `json:"fator_r"`
	Anexo              *string  `json:"anexo"`
	Semaforo           *string  `json:"semaforo"`
	FolhaMinima28      *string  `json:"folha_minima_28"`
	FolhaMinimaMeta    *string  `json:"folha_minima_meta"`
	Gap12m28           *string  `json:"gap_12m_28"`
	ReforcoMensal28    *string  `json:"reforco_mensal_28"`
	ReforcoMensalMeta  *string  `json:"reforco_mensal_meta"`
	AliquotaEfetivaIII *string  `json:"aliquota_efetiva_iii"`
	AliquotaEfetivaV   *string  `json:"aliquota_efetiva_v"`
	Economia12m        *string  `json:"economia_12m"`
	MetaOperacional    string   `json:"meta_operacional"`
	CPPIntegraFS12     bool     `json:"cpp_integra_fs12"`
	TabelaVigencia     string   `json:"tabela_vigencia_inicio"`
	RegimeNoPA         *string  `json:"regime_no_pa"`
	Monitorada         bool     `json:"monitorada"`
}

func put(t *testing.T, srv *testutil.Server, id, comp string, body map[string]any) movementOut {
	t.Helper()
	var m movementOut
	srv.Put("/api/v1/companies/"+id+"/factor-r/movements/"+comp, body).MustStatus(t, http.StatusOK).JSON(t, &m)
	return m
}

// putWindow lança os 12 meses da janela do PA, todos iguais.
func putWindow(t *testing.T, srv *testutil.Server, id, pa string, body map[string]any) {
	t.Helper()
	start := competence.MustParse(pa).AddMonths(-12)
	for i := range 12 {
		put(t, srv, id, start.AddMonths(i).String(), body)
	}
}

func calc(t *testing.T, srv *testutil.Server, id, pa string) calcOut {
	t.Helper()
	var c calcOut
	srv.Get("/api/v1/companies/"+id+"/factor-r/calc?pa="+pa).MustStatus(t, http.StatusOK).JSON(t, &c)
	return c
}

func str(p *string) string {
	if p == nil {
		return "<null>"
	}
	return *p
}

// AC-005: os valores da API são os do motor (caso tabelado da faixa 3 da origem) e saem como
// string decimal exata [06 §12].
func TestAC005_CalcEndToEnd(t *testing.T) {
	srv := server(t)
	id := companyID(t, srv, "202")
	putWindow(t, srv, id, "2026-09", map[string]any{"receita_bruta": "50000.00", "pro_labore": "10000.00"})

	c := calc(t, srv, id, "2026-09")
	if c.Status != "ok" || c.RBT12 != "600000.00" || c.FS12 != "120000.00" || str(c.FatorR) != "0.2" {
		t.Fatalf("cálculo = %+v", c)
	}
	checks := map[string]string{
		"anexo": str(c.Anexo), "semaforo": str(c.Semaforo),
		"folha_minima_28": str(c.FolhaMinima28), "folha_minima_meta": str(c.FolhaMinimaMeta),
		"gap_12m_28": str(c.Gap12m28), "reforco_mensal_28": str(c.ReforcoMensal28),
		"reforco_mensal_meta": str(c.ReforcoMensalMeta), "aliquota_efetiva_iii": str(c.AliquotaEfetivaIII),
		"aliquota_efetiva_v": str(c.AliquotaEfetivaV), "economia_12m": str(c.Economia12m),
		"meta_operacional": c.MetaOperacional, "tabela_vigencia_inicio": c.TabelaVigencia,
	}
	want := map[string]string{
		"anexo": "V", "semaforo": "vermelho",
		"folha_minima_28": "168000.0000", "folha_minima_meta": "180000.00000000",
		"gap_12m_28": "48000.0000", "reforco_mensal_28": "4000.0000",
		"reforco_mensal_meta": "5000.00000000", "aliquota_efetiva_iii": "0.105600",
		"aliquota_efetiva_v": "0.178500", "economia_12m": "43740.00000000",
		"meta_operacional": "0.300000", "tabela_vigencia_inicio": "2018-01-01",
	}
	for k, w := range want {
		if checks[k] != w {
			t.Errorf("%s = %s; esperado %s", k, checks[k], w)
		}
	}
	if !c.CPPIntegraFS12 || !c.Monitorada || str(c.RegimeNoPA) != "SIMPLES_NACIONAL" {
		t.Fatalf("política/situação = cpp %v, monitorada %v, regime %s", c.CPPIntegraFS12, c.Monitorada, str(c.RegimeNoPA))
	}
	// Valores decimais trafegam como string JSON, nunca como número.
	raw := srv.Get("/api/v1/companies/" + id + "/factor-r/calc?pa=2026-09").Body
	var generic map[string]any
	if err := json.Unmarshal(raw, &generic); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"rbt12", "fs12", "fator_r", "economia_12m", "meta_operacional"} {
		if _, ok := generic[f].(string); !ok {
			t.Errorf("%s deveria ser string, veio %T", f, generic[f])
		}
	}
}

// AC-006: trocar o PA move a janela um mês, sem edição manual.
func TestAC006_CalcWindowMovesWithPA(t *testing.T) {
	srv := server(t)
	id := companyID(t, srv, "206")
	// 09/2025 a 08/2026 lançados; 09/2026 ainda não.
	putWindow(t, srv, id, "2026-09", map[string]any{"receita_bruta": "50000.00", "pro_labore": "14000.00"})

	sep, oct := calc(t, srv, id, "2026-09"), calc(t, srv, id, "2026-10")
	if sep.JanelaInicio != "2025-09" || sep.JanelaFim != "2026-08" || len(sep.MesesFaltantes) != 0 {
		t.Fatalf("PA 09/2026: janela %s a %s, faltantes %v", sep.JanelaInicio, sep.JanelaFim, sep.MesesFaltantes)
	}
	if oct.JanelaInicio != "2025-10" || oct.JanelaFim != "2026-09" || !slices.Equal(oct.MesesFaltantes, []string{"2026-09"}) {
		t.Fatalf("PA 10/2026: janela %s a %s, faltantes %v", oct.JanelaInicio, oct.JanelaFim, oct.MesesFaltantes)
	}
	if slices.Contains(oct.MesesPreenchidos, "2025-09") || sep.RBT12 != "600000.00" || oct.RBT12 != "550000.00" {
		t.Fatalf("09/2025 deveria sair da janela: preenchidos %v, RBT12 %s → %s", oct.MesesPreenchidos, sep.RBT12, oct.RBT12)
	}
	// Lançar 09/2026 completa a janela de 10/2026.
	put(t, srv, id, "2026-09", map[string]any{"receita_bruta": "50000.00", "pro_labore": "14000.00"})
	if oct = calc(t, srv, id, "2026-10"); len(oct.MesesFaltantes) != 0 || str(oct.Semaforo) != "amarelo" {
		t.Fatalf("PA 10/2026 depois do lançamento: faltantes %v, semáforo %s", oct.MesesFaltantes, str(oct.Semaforo))
	}
}

func TestCalc_DefaultsAndErrors(t *testing.T) {
	srv := server(t)
	id := companyID(t, srv, "202")
	var c calcOut
	srv.Get("/api/v1/companies/"+id+"/factor-r/calc").MustStatus(t, http.StatusOK).JSON(t, &c)
	if c.PA != "2026-10" || c.Status != "dados_insuficientes" || str(c.Motivo) != "rbt12_zero" || len(c.MesesFaltantes) != 12 {
		t.Fatalf("sem PA = %+v", c)
	}
	if code := srv.Get("/api/v1/companies/"+id+"/factor-r/calc?pa=2017-12").MustStatus(t, http.StatusUnprocessableEntity).ErrorCode(t); code != "SIMPLES_TABLE_NOT_IN_FORCE" {
		t.Fatalf("PA sem tabela vigente: %s", code)
	}
	srv.Get("/api/v1/companies/"+id+"/factor-r/calc?pa=2026-13").MustStatus(t, http.StatusBadRequest)
	srv.Get("/api/v1/companies/00000000-0000-0000-0000-00000000abcd/factor-r/calc").MustStatus(t, http.StatusNotFound)
	srv.Get("/api/v1/companies/nao-e-uuid/factor-r/calc").MustStatus(t, http.StatusNotFound)
}

func TestCalc_MonitoredReflectsFlagRegimeAndActive(t *testing.T) {
	srv := server(t)
	cases := []struct {
		code, pa  string
		monitored bool
		regime    string
	}{
		{"202", "2026-09", true, "SIMPLES_NACIONAL"},
		{"201", "2026-09", false, "SIMPLES_NACIONAL"}, // Simples, mas não sujeita ao Fator R
		{"101", "2026-09", false, "LUCRO_PRESUMIDO"},
		{"205", "2025-11", false, "LUCRO_PRESUMIDO"}, // no Simples só desde 01/2026
		{"205", "2026-02", true, "SIMPLES_NACIONAL"},
	}
	for _, tc := range cases {
		c := calc(t, srv, companyID(t, srv, tc.code), tc.pa)
		if c.Monitorada != tc.monitored || str(c.RegimeNoPA) != tc.regime {
			t.Errorf("%s no PA %s: monitorada %v, regime %s; esperado %v, %s", tc.code, tc.pa, c.Monitorada, str(c.RegimeNoPA), tc.monitored, tc.regime)
		}
	}
}

type settingsOut struct {
	CompanyID        string  `json:"company_id"`
	Cnae             *string `json:"cnae"`
	Atividade        *string `json:"atividade"`
	QuantidadeSocios int     `json:"quantidade_socios"`
	Contato          *string `json:"contato"`
	PacoteComercial  *string `json:"pacote_comercial"`
	HonorarioMensal  string  `json:"honorario_mensal"`
	InicioAtividade  *string `json:"inicio_atividade"`
	Notas            *string `json:"notas"`
	Version          int     `json:"version"`
}

func TestSettings_CreateUpdateAndConflict(t *testing.T) {
	srv := server(t)
	id := companyID(t, srv, "208")
	path := "/api/v1/companies/" + id + "/factor-r/settings"

	var s settingsOut
	srv.Get(path).MustStatus(t, http.StatusOK).JSON(t, &s)
	if s.Version != 0 || s.QuantidadeSocios != 1 || s.HonorarioMensal != "0.00" || s.PacoteComercial != nil {
		t.Fatalf("padrões = %+v", s)
	}
	srv.Patch(path, map[string]any{
		"version": 0, "cnae": " 7500-1/00 ", "atividade": "Clínica veterinária", "quantidade_socios": 2,
		"pacote_comercial": "correcao", "honorario_mensal": "1500.5", "inicio_atividade": "2019-03", "notas": "",
	}).MustStatus(t, http.StatusOK).JSON(t, &s)
	if s.Version != 1 || str(s.Cnae) != "7500-1/00" || s.QuantidadeSocios != 2 || str(s.PacoteComercial) != "correcao" ||
		s.HonorarioMensal != "1500.50" || str(s.InicioAtividade) != "2019-03" || s.Notas != nil {
		t.Fatalf("criada = %+v", s)
	}
	// Só os campos enviados mudam; null limpa.
	srv.Patch(path, map[string]any{"version": 1, "pacote_comercial": nil, "contato": "Dra. Íris"}).MustStatus(t, http.StatusOK).JSON(t, &s)
	if s.Version != 2 || s.PacoteComercial != nil || str(s.Contato) != "Dra. Íris" || str(s.Atividade) != "Clínica veterinária" {
		t.Fatalf("atualizada = %+v", s)
	}
	// Versão desatualizada e criação repetida → 409 com a versão atual.
	for _, v := range []int{1, 0} {
		r := srv.Patch(path, map[string]any{"version": v, "notas": "x"}).MustStatus(t, http.StatusConflict)
		var e struct {
			Code    string         `json:"code"`
			Details map[string]any `json:"details"`
		}
		r.JSON(t, &e)
		if e.Code != "VERSION_CONFLICT" || e.Details["current_version"] != float64(2) {
			t.Fatalf("versão %d: %+v", v, e)
		}
	}
}

func fieldErrorsOf(t *testing.T, r testutil.Response) map[string]any {
	t.Helper()
	var e struct {
		Code    string `json:"code"`
		Details struct {
			Fields map[string]any `json:"fields"`
		} `json:"details"`
	}
	r.JSON(t, &e)
	if e.Code != "VALIDATION_ERROR" {
		t.Fatalf("code = %s", e.Code)
	}
	return e.Details.Fields
}

func TestSettings_Validation(t *testing.T) {
	srv := server(t)
	id := companyID(t, srv, "208")
	r := srv.Patch("/api/v1/companies/"+id+"/factor-r/settings", map[string]any{
		"version": 0, "cnae": strings.Repeat("9", 21), "pacote_comercial": "premium", "honorario_mensal": "12,50",
		"inicio_atividade": "03/2019", "quantidade_socios": -1,
	}).MustStatus(t, http.StatusBadRequest)
	fields := fieldErrorsOf(t, r)
	for _, f := range []string{"cnae", "pacote_comercial", "honorario_mensal", "inicio_atividade", "quantidade_socios"} {
		if _, ok := fields[f]; !ok {
			t.Errorf("esperado erro em %s: %v", f, fields)
		}
	}
	srv.Patch("/api/v1/companies/00000000-0000-0000-0000-00000000abcd/factor-r/settings", map[string]any{"version": 0}).
		MustStatus(t, http.StatusNotFound)
}

func TestMovements_PutValidatesMoneyAndNote(t *testing.T) {
	srv := server(t)
	id := companyID(t, srv, "202")
	path := "/api/v1/companies/" + id + "/factor-r/movements/2026-08"
	r := srv.Put(path, map[string]any{
		"receita_bruta": "-1", "pro_labore": "1.234", "salarios_13_ferias": "1e3", "cpp_recolhida": "1000000000000",
		"fgts_recolhido": "abc", "observacao": strings.Repeat("x", 1001),
	}).MustStatus(t, http.StatusBadRequest)
	fields := fieldErrorsOf(t, r)
	for _, f := range []string{"receita_bruta", "pro_labore", "salarios_13_ferias", "cpp_recolhida", "fgts_recolhido", "observacao"} {
		if _, ok := fields[f]; !ok {
			t.Errorf("esperado erro em %s: %v", f, fields)
		}
	}
	srv.Put("/api/v1/companies/"+id+"/factor-r/movements/2026-8", map[string]any{}).MustStatus(t, http.StatusBadRequest)
	// Número JSON não é aceito para dinheiro: o contrato é string decimal.
	srv.Put(path, map[string]any{"receita_bruta": 1000}).MustStatus(t, http.StatusBadRequest)
}

func TestMovements_UpsertWithVersionAndManualReplacesPGDAS(t *testing.T) {
	srv := server(t)
	id := companyID(t, srv, "202")
	m := put(t, srv, id, "2026-08", map[string]any{"receita_bruta": "50000", "pro_labore": "10000.5", "observacao": "  "})
	if m.Version != 1 || m.Origem != "manual" || m.ReceitaBruta != "50000.00" || m.ProLabore != "10000.50" ||
		m.Salarios13Ferias != "0.00" || m.FolhaMes != "10000.50" || m.Observacao != nil {
		t.Fatalf("criado = %+v", m)
	}
	// Criar de novo (version 0) → 409; versão certa substitui o mês inteiro.
	srv.Put("/api/v1/companies/"+id+"/factor-r/movements/2026-08", map[string]any{"receita_bruta": "1"}).MustStatus(t, http.StatusConflict)
	m = put(t, srv, id, "2026-08", map[string]any{"version": 1, "receita_bruta": "51000.00", "salarios_13_ferias": "3000.00"})
	if m.Version != 2 || m.ProLabore != "0.00" || m.Salarios13Ferias != "3000.00" || m.FolhaMes != "3000.00" {
		t.Fatalf("substituído = %+v", m)
	}
	// Mês vindo do PGDAS-D com folha total declarada (FR-03): "Detalhar folha" o torna manual.
	ctx := context.Background()
	if _, err := srv.Pool.Exec(ctx, `INSERT INTO factor_r_movements (company_id, competence, receita_bruta, folha_total_declarada_pgdas, origem, observacao)
		VALUES ($1, '2026-07-01', 48000.00, 14000.00, 'pgdas', 'total declarado no PGDAS-D')`, id); err != nil {
		t.Fatal(err)
	}
	var list struct {
		Items []movementOut `json:"items"`
	}
	srv.Get("/api/v1/companies/"+id+"/factor-r/movements?from=2026-07&to=2026-07").MustStatus(t, http.StatusOK).JSON(t, &list)
	if len(list.Items) != 1 || str(list.Items[0].FolhaTotalDeclaradaPgdas) != "14000.00" || list.Items[0].FolhaMes != "14000.00" || list.Items[0].Origem != "pgdas" {
		t.Fatalf("mês do PGDAS-D = %+v", list.Items)
	}
	m = put(t, srv, id, "2026-07", map[string]any{"version": 1, "receita_bruta": "48000.00", "pro_labore": "4000.00", "salarios_13_ferias": "10000.00"})
	if m.Origem != "manual" || m.FolhaTotalDeclaradaPgdas != nil || m.FolhaMes != "14000.00" {
		t.Fatalf("detalhado = %+v", m)
	}
}

func TestMovements_ListRange(t *testing.T) {
	srv := server(t)
	id := companyID(t, srv, "202")
	for _, c := range []string{"2026-06", "2026-07", "2026-08"} {
		put(t, srv, id, c, map[string]any{"receita_bruta": "1.00"})
	}
	var list struct {
		Items []movementOut `json:"items"`
	}
	srv.Get("/api/v1/companies/"+id+"/factor-r/movements?from=2026-07").MustStatus(t, http.StatusOK).JSON(t, &list)
	if len(list.Items) != 2 || list.Items[0].Competence != "2026-07" || list.Items[1].Competence != "2026-08" {
		t.Fatalf("de 07/2026 = %+v", list.Items)
	}
	srv.Get("/api/v1/companies/"+id+"/factor-r/movements").MustStatus(t, http.StatusOK).JSON(t, &list)
	if len(list.Items) != 3 {
		t.Fatalf("todos = %d", len(list.Items))
	}
	srv.Get("/api/v1/companies/"+id+"/factor-r/movements?from=2026-08&to=2026-07").MustStatus(t, http.StatusBadRequest)
	srv.Get("/api/v1/companies/"+id+"/factor-r/movements?from=agosto").MustStatus(t, http.StatusBadRequest)
}
