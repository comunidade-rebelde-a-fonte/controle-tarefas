package pgdas

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"controle/internal/platform/decimal"
)

// Porte de tests/parsing/test_pgdas_unit.py da origem (o golden cobre o resto).

func mustParse(t *testing.T, text string) Result {
	t.Helper()
	r, err := Parse(text)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return r
}

func val(p *string) string {
	if p == nil {
		return "<None>"
	}
	return *p
}

func TestNumeroBR(t *testing.T) {
	for in, want := range map[string]string{"1.234,56": "1234.56", "600.000,00": "600000.00", "0,30": "0.30", "1234.56": "1234.56"} {
		got := NumeroBR(in)
		if got == nil || got.Cmp(decimal.MustParse(want)) != 0 {
			t.Errorf("NumeroBR(%q) = %v; quer %s", in, got, want)
		}
	}
}

func TestParse_OriginUnitCases(t *testing.T) {
	if r := mustParse(t, "Período de Apuração (PA): 13/2026"); r.Field("pa") != nil {
		t.Error("mês inválido no PA foi aceito")
	}
	if r := mustParse(t, "Atividade do Anexo VI"); r.Field("anexo") != nil {
		t.Error("Anexo VI virou Anexo V")
	}
	r := mustParse(t, "PA: 09/2026\nCNPJ: 11222333000181\nRBT12: 600.000,00\nFS12: 180.000,00\nFator r: 0,10")
	if !slices.Contains(r.Reasons, "fator_r_inconsistente_com_fs12_rbt12") {
		t.Errorf("motivos = %v", r.Reasons)
	}
}

func TestParse_PAInterval(t *testing.T) {
	cases := map[string]string{
		"Periodo de Apuracao: 01/08/2026 a 31/08/2026":              "2026-08",
		"Período de Apuração (PA): 01/12/2025 a 31/12/2025":         "2025-12",
		"Periodo de Apuracao: 01/08/2026 a 30/09/2026":              "<None>",
		"Periodo de Apuracao: 01/12/2025 a 31/12/2026":              "<None>",
		"Periodo de Apuracao: 01/13/2026 a 31/13/2026":              "<None>",
		"PA: 09/2026\nPeriodo de Apuracao: 01/08/2026 a 31/08/2026": "2026-09", // mês/ano tem precedência
	}
	for text, want := range cases {
		if got := val(mustParse(t, text).Field("pa")); got != want {
			t.Errorf("PA de %q = %s; quer %s", text, got, want)
		}
	}
}

func TestParse_Annex(t *testing.T) {
	cases := map[string]string{
		"Fator r = Nao se aplica - Anexo I\nCriterio: atividades sujeitas ao fator r vao para o Anexo III.": "<None>",
		"Fator r = 0,20 - Anexo V":                     "V",
		"Enquadramento Anexo III":                      "III",
		"Atividade: Prestação de serviços - Anexo III": "III",
		"Enquadramento: Anexo II":                      "<None>",
		"Texto livre citando o Anexo V":                "V",
	}
	for text, want := range cases {
		if got := val(mustParse(t, text).Field("anexo")); got != want {
			t.Errorf("anexo de %q = %s; quer %s", text, got, want)
		}
	}
}

func TestParse_Identification(t *testing.T) {
	names := map[string]string{
		"Nome empresarial: CLINICA EXEMPLO LTDA":        "CLINICA EXEMPLO LTDA",
		"NOME EMPRESARIAL CLINICA EXEMPLO LTDA":         "CLINICA EXEMPLO LTDA",
		"Nome empresarial:":                             "<None>",
		"Razão social: CLINICA EXEMPLO LTDA":            "<None>",
		"Nome empresarial: " + strings.Repeat("A", 300): strings.Repeat("A", 200),
	}
	for text, want := range names {
		if got := val(mustParse(t, text).Identification.NomeEmpresarial); got != want {
			t.Errorf("nome de %.40q = %.40s; quer %.40s", text, got, want)
		}
	}
	subject := map[string]string{
		"Fator r = 0,20 - Anexo V":                          "true",
		"Fator r: 30,00%":                                   "true",
		"Fator r = Nao se aplica - Anexo I":                 "false",
		"Fator r = Não se aplica - Anexo III":               "false",
		"Folha para fator r Nao se aplica a esta atividade": "false",
		"Sem nenhuma menção":                                "<None>",
	}
	for text, want := range subject {
		got := "<None>"
		if p := mustParse(t, text).Identification.SujeitaFatorR; p != nil {
			got = fmt.Sprint(*p)
		}
		if got != want {
			t.Errorf("sujeita de %q = %s; quer %s", text, got, want)
		}
	}
	base := "PA: 09/2026\nCNPJ: 11222333000181\nRPA: 50.000,00"
	withName := base + "\nNome empresarial: CLINICA EXEMPLO LTDA\nFator r = Nao se aplica"
	if mustParse(t, base).Confidence.Cmp(mustParse(t, withName).Confidence) != 0 {
		t.Error("a identificação alterou a confiança")
	}
	opening := map[string]string{
		"Data de abertura no CNPJ: 12/03/2019": "2019-03",
		"Data de abertura: 01/12/2020":         "2020-12",
		"Data de abertura no CNPJ: XX/XX/XXXX": "<None>",
		"Data de abertura no CNPJ: 01/13/2020": "<None>",
	}
	for text, want := range opening {
		if got := val(mustParse(t, text).Identification.InicioAtividade); got != want {
			t.Errorf("abertura de %q = %s; quer %s", text, got, want)
		}
	}
}

// declaratorio monta o layout da declaração com as tabelas 2.2 e 2.3 (como o teste da origem).
func declaratorio(receitas, folhas []string, rbt12, fs12 string) string {
	meses := []string{"08/2025", "09/2025", "10/2025", "11/2025", "12/2025", "01/2026",
		"02/2026", "03/2026", "04/2026", "05/2026", "06/2026", "07/2026"}
	lines := []string{
		"Periodo de Apuracao: 01/08/2026 a 31/08/2026",
		"Data de abertura no CNPJ: 12/03/2019",
		fmt.Sprintf("Receita bruta acumulada nos 12 meses anteriores (RBT12) %s 0,00 %s", rbt12, rbt12),
		"2.2 Receitas Brutas Anteriores (R$) - Mercado Interno",
		"Mes Valor Mes Valor Mes Valor",
	}
	row := func(vals []string, i int) string {
		var parts []string
		for j := i; j < i+3; j++ {
			parts = append(parts, meses[j]+" "+vals[j])
		}
		return strings.Join(parts, " ")
	}
	for i := 0; i < 12; i += 3 {
		lines = append(lines, row(receitas, i))
	}
	lines = append(lines, "2.3 Folha de Salarios Anteriores (R$)")
	if folhas != nil {
		for i := 0; i < 12; i += 3 {
			lines = append(lines, row(folhas, i))
		}
		lines = append(lines, "Total FS12 "+fs12)
	}
	lines = append(lines, "2.4 Fator r", "Fator r = 0,20 - Anexo V")
	return strings.Join(lines, "\n")
}

func repeat(v string, n int) []string { return slices.Repeat([]string{v}, n) }

func TestParse_PriorSeries(t *testing.T) {
	r := mustParse(t, declaratorio(repeat("10.000,00", 12), repeat("2.000,00", 12), "120.000,00", "24.000,00"))
	s := r.Series
	if val(r.Field("rbt12")) != "120000.00" || val(r.Field("fs12")) != "24000.00" || s.Receitas["2025-08"] != "10000.00" ||
		s.Folhas["2026-07"] != "2000.00" || len(s.Receitas) != 12 || len(s.Folhas) != 12 || !s.ReceitasConferem || !s.FolhasConferem ||
		val(r.Identification.InicioAtividade) != "2019-03" {
		t.Fatalf("séries = %+v, campos = %v", s, r.Fields)
	}
	receitas := append(repeat("10.000,00", 11), "11.000,00")
	if s := mustParse(t, declaratorio(receitas, repeat("2.000,00", 12), "120.000,00", "24.000,00")).Series; s.ReceitasConferem || !s.FolhasConferem {
		t.Errorf("série que não soma o total conferiu: %+v", s)
	}
	outside := strings.Replace(declaratorio(repeat("10.000,00", 12), repeat("2.000,00", 12), "120.000,00", "24.000,00"),
		"01/08/2026 a 31/08/2026", "01/09/2026 a 30/09/2026", 1)
	if r := mustParse(t, outside); val(r.Field("pa")) != "2026-09" || r.Series.ReceitasConferem || r.Series.FolhasConferem {
		t.Errorf("série fora da janela = %+v", r.Series)
	}
	if s := mustParse(t, declaratorio(repeat("10.000,00", 12), nil, "120.000,00", "0,00")).Series; !s.ReceitasConferem || len(s.Folhas) != 0 || s.FolhasConferem {
		t.Errorf("sem tabela de folha = %+v", s)
	}
	if s := mustParse(t, "PA: 09/2026\nCNPJ: 11222333000181\nRBT12: 600.000,00").Series; len(s.Receitas) != 0 || len(s.Folhas) != 0 || s.ReceitasConferem {
		t.Errorf("layout sem tabelas = %+v", s)
	}
}

// Meses anteriores que o vínculo grava (_meses_para_importar).
func TestStored_PriorMonths(t *testing.T) {
	conferida := mustParse(t, declaratorio(repeat("10.000,00", 12), repeat("2.000,00", 12), "120.000,00", "24.000,00")).Stored()
	months, comFolha, motivo, err := conferida.PriorMonths()
	if err != nil || len(months) != 12 || !comFolha || motivo != "" || months["2026-07"].Folha.Text() != "2000.00" {
		t.Fatalf("série conferida = %v %v %q %v", months, comFolha, motivo, err)
	}
	semFolha := mustParse(t, declaratorio(repeat("10.000,00", 12), nil, "120.000,00", "0,00")).Stored()
	if _, _, motivo, _ := semFolha.PriorMonths(); motivo != FolhasNaoConferem {
		t.Errorf("sujeita sem folha conferida: motivo = %q", motivo)
	}
	f := false
	semFolha.Identificacao.SujeitaFatorR = &f
	months, comFolha, motivo, _ = semFolha.PriorMonths()
	if len(months) != 12 || comFolha || motivo != "" || !months["2025-08"].Folha.IsZero() {
		t.Errorf("não sujeita = %v %v %q", months, comFolha, motivo)
	}
	naoConfere := mustParse(t, declaratorio(append(repeat("10.000,00", 11), "11.000,00"), repeat("2.000,00", 12), "120.000,00", "24.000,00")).Stored()
	if months, _, motivo, _ := naoConfere.PriorMonths(); months != nil || motivo != ReceitasNaoConferem {
		t.Errorf("receitas que não conferem = %v %q", months, motivo)
	}
	if months, _, motivo, _ := mustParse(t, "PA: 09/2026").Stored().PriorMonths(); months != nil || motivo != "" {
		t.Errorf("sem séries = %v %q", months, motivo)
	}
}

func TestCNPJValido_LikeOrigin(t *testing.T) {
	for s, want := range map[string]bool{
		"11222333000181":     true,
		"11.222.333/0001-81": true,
		"11222333000182":     false,
		"11111111111111":     false,
		"1122233300018":      false,
		"11222333000١٨١":     false, // verificadores em dígitos árabes não batem com o texto
		"١١222333000181":     true,  // int() lê o dígito árabe; os verificadores são ASCII
	} {
		if got := CNPJValido(s); got != want {
			t.Errorf("CNPJValido(%q) = %v; quer %v", s, got, want)
		}
	}
}
