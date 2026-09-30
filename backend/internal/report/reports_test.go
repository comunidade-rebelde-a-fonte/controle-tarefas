package report_test

import (
	"net/http"
	"strconv"
	"strings"
	"testing"

	"controle/internal/report"
	"controle/internal/testutil"
	"controle/internal/testutil/seeded"
)

// server sobe a API sobre o banco de demonstração (clonado do template, um por teste).
func server(t *testing.T) *testutil.Server {
	t.Helper()
	return testutil.NewServer(t, seeded.NewDemoDB(t), seeded.DemoClock())
}

// pdf baixa o relatório, confere o tipo e o nome do arquivo e devolve o texto extraído.
func pdf(t *testing.T, srv *testutil.Server, path, filename string) string {
	t.Helper()
	res := srv.Get(path).MustStatus(t, http.StatusOK)
	if ct := res.Header.Get("Content-Type"); ct != "application/pdf" {
		t.Fatalf("Content-Type = %q", ct)
	}
	if cd := res.Header.Get("Content-Disposition"); !strings.Contains(cd, `filename="`+filename+`"`) {
		t.Fatalf("Content-Disposition = %q, want %s", cd, filename)
	}
	if !strings.HasPrefix(string(res.Body), "%PDF-") {
		t.Fatal("resposta não é um PDF")
	}
	return report.PDFText(t, res.Body)
}

// sections confere as seções mínimas do §36.2: título, competência, data de geração, filtros,
// resumo de indicadores e a tabela (empresas, obrigações, status e atrasos) com linhas.
func sections(t *testing.T, text string, want ...string) {
	t.Helper()
	base := []string{"Gerado em 01/10/2026 09:00", "Filtros: ", "Página 1/"}
	for _, w := range append(base, want...) {
		if !strings.Contains(text, w) {
			t.Errorf("PDF sem %q\n--- texto ---\n%.3000s", w, text)
		}
	}
	if strings.Contains(text, "Nenhum registro para os filtros selecionados.") {
		t.Error("tabela vazia: o seed deveria gerar linhas para este relatório")
	}
}

type backlogPage struct {
	Items []struct {
		Company struct {
			LegalName string `json:"legal_name"`
		} `json:"company"`
		DaysOverdue int `json:"days_overdue"`
	} `json:"items"`
	Total int `json:"total"`
}

func backlogJSON(t *testing.T, srv *testutil.Server, query string) backlogPage {
	t.Helper()
	var b backlogPage
	srv.Get("/api/v1/backlog?page_size=500&"+query).MustStatus(t, http.StatusOK).JSON(t, &b)
	return b
}

// summaryValue confere que o indicador aparece seguido do valor esperado.
func summaryValue(t *testing.T, text, label string, value int) {
	t.Helper()
	if !strings.Contains(text, label+"\n"+strconv.Itoa(value)+"\n") {
		t.Errorf("indicador %q deveria valer %d", label, value)
	}
}

// prefix devolve o início do nome: as colunas estreitas cortam nomes longos com reticências.
func prefix(s string) string {
	if r := []rune(s); len(r) > 20 {
		return string(r[:20])
	}
	return s
}

func TestReport_General(t *testing.T) {
	srv := server(t)
	text := pdf(t, srv, "/api/v1/reports/general.pdf?competence=2026-09", "relatorio-general-2026-09.pdf")
	sections(t, text,
		"Situação geral da competência", "Competência: 09/2026", "Filtros: nenhum",
		"Obrigações aplicáveis", "Concluídas", "Pendentes", "Atrasadas", "Progresso operacional",
		"Código", "Empresa", "Regime", "Fiscal", "Declarações", "Folha", "Contábil",
		"Em aberto", "Empresa Fictícia Alfa Comércio Ltda",
	)
	var d struct {
		KPIs struct {
			Applicable struct{ Value int } `json:"applicable"`
			Overdue    struct{ Value int } `json:"overdue"`
		} `json:"kpis"`
	}
	srv.Get("/api/v1/dashboard?competence=2026-09").MustStatus(t, http.StatusOK).JSON(t, &d)
	summaryValue(t, text, "Obrigações aplicáveis", d.KPIs.Applicable.Value)
	summaryValue(t, text, "Atrasadas", d.KPIs.Overdue.Value)

	// Filtro de grupo: só empresas do Simples, descrito no cabeçalho.
	simples := pdf(t, srv, "/api/v1/reports/general.pdf?competence=2026-09&regime_group=SIMPLES", "relatorio-general-2026-09.pdf")
	if !strings.Contains(simples, "Filtros: Grupo: Simples Nacional") || strings.Contains(simples, "Lucro Real\n") {
		t.Error("filtro regime_group=SIMPLES não aplicado ou não descrito")
	}
}

func TestReport_Pending(t *testing.T) {
	srv := server(t)
	text := pdf(t, srv, "/api/v1/reports/pending.pdf?competence=2026-09", "relatorio-pending-2026-09.pdf")
	sections(t, text,
		"Pendências por módulo", "Competência: 09/2026",
		"Pendências", "Empresas com pendências", "Atrasadas",
		"Módulo", "Código", "Empresa", "Obrigação", "Status", "Vencimento", "Atraso (dias)", "Responsável",
	)
	b := backlogJSON(t, srv, "competence=2026-09")
	summaryValue(t, text, "Pendências", b.Total)
	if b.Total == 0 || !strings.Contains(text, prefix(b.Items[0].Company.LegalName)) {
		t.Errorf("pendências da API (%d) ausentes do PDF", b.Total)
	}

	fiscal := pdf(t, srv, "/api/v1/reports/pending.pdf?competence=2026-09&module=fiscal&state=pending", "relatorio-pending-2026-09.pdf")
	if !strings.Contains(fiscal, "Módulo: Fiscal") || !strings.Contains(fiscal, "Classe: Pendentes") {
		t.Error("filtros de módulo e classe não descritos")
	}
	summaryValue(t, fiscal, "Pendências", backlogJSON(t, srv, "competence=2026-09&module=fiscal&state=pending").Total)
}

func TestReport_Overdue(t *testing.T) {
	srv := server(t)
	text := pdf(t, srv, "/api/v1/reports/overdue.pdf?competence=2026-08", "relatorio-overdue-2026-08.pdf")
	sections(t, text,
		"Obrigações atrasadas", "Competência: 08/2026",
		"Empresas afetadas", "Maior atraso (dias)",
		"Código", "Empresa", "Módulo", "Obrigação", "Status", "Vencimento", "Atraso (dias)",
	)
	b := backlogJSON(t, srv, "competence=2026-08&state=overdue")
	if b.Total == 0 {
		t.Fatal("o seed deveria ter obrigações atrasadas em 08/2026")
	}
	summaryValue(t, text, "Obrigações atrasadas", b.Total)
	summaryValue(t, text, "Maior atraso (dias)", b.Items[0].DaysOverdue)
	if !strings.Contains(text, prefix(b.Items[0].Company.LegalName)) {
		t.Errorf("a obrigação mais atrasada (%s) deveria estar no PDF", b.Items[0].Company.LegalName)
	}
}

func TestReport_Backlog(t *testing.T) {
	srv := server(t)
	text := pdf(t, srv, "/api/v1/reports/backlog.pdf?competence=2026-10", "relatorio-backlog-2026-10.pdf")
	sections(t, text,
		"Backlog", "Competência: 10/2026", "Competências: anteriores a 10/2026",
		"Itens no backlog", "Obrigações em aberto", "Módulos a fechar", "Competência mais antiga", "Maior atraso (dias)",
		"Competência", "Código", "Empresa", "Módulo", "Obrigação", "Status", "Vencimento", "Atraso (dias)",
	)
	b := backlogJSON(t, srv, "before=2026-10")
	summaryValue(t, text, "Itens no backlog", b.Total)
	if strings.Contains(text, "\n10/2026\n") {
		t.Error("o backlog não pode listar a competência informada, só as anteriores")
	}
}

func TestReport_Credentials(t *testing.T) {
	srv := server(t)
	text := pdf(t, srv, "/api/v1/reports/credentials.pdf", "relatorio-credentials-2026-10-01.pdf")
	sections(t, text,
		"Procurações e certificados a vencer", "Filtros: Situação: vencidos ou a vencer em até",
		"Documentos listados", "Procurações vencidas", "Certificados a vencer",
		"Código", "Empresa", "CNPJ", "Documento", "Vencimento", "Situação", "Prazo (dias)", "Renovação",
	)
	if !strings.Contains(text, "Vencido") && !strings.Contains(text, "Vence em") {
		t.Error("deveria listar documentos vencidos ou a vencer do seed")
	}
	if strings.Contains(text, "\nSem data\n") {
		t.Error("sem filtro, só vencidos e a vencer")
	}

	expired := pdf(t, srv, "/api/v1/reports/credentials.pdf?competence=2026-09&filter=expired&type=DIGITAL_CERTIFICATE",
		"relatorio-credentials-2026-09.pdf")
	if !strings.Contains(expired, "Competência: 09/2026") || !strings.Contains(expired, "Filtro: Vencidos") ||
		!strings.Contains(expired, "Documento: Certificado digital") || strings.Contains(expired, "\nProcuração RFB\n") {
		t.Error("filtro expired + type=DIGITAL_CERTIFICATE não aplicado ou não descrito")
	}
	srv.Get("/api/v1/reports/credentials.pdf?filter=due_0").MustStatus(t, http.StatusBadRequest)
}

func TestReport_Module(t *testing.T) {
	srv := server(t)
	text := pdf(t, srv, "/api/v1/reports/module.pdf?competence=2026-09&module=accounting", "relatorio-module-accounting-2026-09.pdf")
	sections(t, text,
		"Controle completo — Contábil", "Competência: 09/2026", "Filtros: nenhum",
		"Empresas", "Obrigações aplicáveis", "Concluídas", "Atrasadas", "Módulos fechados",
		"Código", "Empresa", "Obrigação", "Status", "Vencimento", "Atraso (dias)", "Situação do módulo",
		"Movimento recebido", "Contábil digitado/processado", "Conciliação",
	)

	// Fiscal sem group: Normais e Simples no mesmo relatório; com filtros da grade e busca.
	fiscal := pdf(t, srv, "/api/v1/reports/module.pdf?competence=2026-09&module=fiscal", "relatorio-module-fiscal-2026-09.pdf")
	if !strings.Contains(fiscal, "Controle completo — Fiscal\n") || !strings.Contains(fiscal, "Não se aplica") {
		t.Error("Fiscal completo deveria juntar os grupos e mostrar os não aplicáveis")
	}
	one := pdf(t, srv, "/api/v1/reports/module.pdf?competence=2026-09&module=fiscal&group=normal&tax_regime=LUCRO_REAL&q=Alfa",
		"relatorio-module-fiscal-normal-2026-09.pdf")
	if !strings.Contains(one, "Controle completo — Fiscal — Normais") || !strings.Contains(one, "Busca: Alfa") ||
		!strings.Contains(one, "Regime: Lucro Real") {
		t.Error("filtros da grade não descritos")
	}

	srv.Get("/api/v1/reports/module.pdf?competence=2026-09").MustStatus(t, http.StatusBadRequest)
	srv.Get("/api/v1/reports/module.pdf?competence=2026-09&module=credentials").MustStatus(t, http.StatusNotFound)
}

func TestReport_Errors(t *testing.T) {
	srv := server(t)
	srv.Get("/api/v1/reports/unknown.pdf?competence=2026-09").MustStatus(t, http.StatusNotFound)
	srv.Get("/api/v1/reports/general.xlsx?competence=2026-09").MustStatus(t, http.StatusNotFound)
	for _, r := range []string{"general", "pending", "overdue", "backlog", "module"} {
		srv.Get("/api/v1/reports/"+r+".pdf").MustStatus(t, http.StatusBadRequest)
	}
	srv.Get("/api/v1/reports/overdue.pdf?competence=2026-09&regime_group=OUTRO").MustStatus(t, http.StatusBadRequest)
}
