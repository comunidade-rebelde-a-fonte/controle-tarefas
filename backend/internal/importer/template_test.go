package importer

import (
	"bytes"
	"slices"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

// T-M6-01: o modelo reabre como XLSX válido, com os cabeçalhos, as listas e as instruções.
func TestTemplate_ReopensWithHeadersValidationsAndInstructions(t *testing.T) {
	data, err := Template()
	if err != nil {
		t.Fatal(err)
	}
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("o arquivo não abre como XLSX: %v", err)
	}
	defer func() { _ = f.Close() }()

	if got := f.GetSheetList(); !slices.Equal(got, []string{SheetCompanies, SheetInstructions}) {
		t.Fatalf("abas = %v", got)
	}
	rows, err := f.GetRows(SheetCompanies)
	if err != nil || len(rows) != 1 {
		t.Fatalf("aba Empresas: %d linhas (%v); want só o cabeçalho", len(rows), err)
	}
	want := make([]string, len(Columns))
	for i, c := range Columns {
		want[i] = c.Header
	}
	if !slices.Equal(rows[0], want) {
		t.Fatalf("cabeçalhos = %v\nwant %v", rows[0], want)
	}
	// Campos do §7.1 + regime inicial + tipo de folha + módulos.
	for _, h := range []string{"Código interno", "Razão social", "Nome fantasia", "CNPJ", "Regime tributário inicial", "Início do atendimento",
		"Encerramento do atendimento", "Situação", "Município", "UF", "Inscrição estadual", "Inscrição municipal", "Tipo de folha",
		"Módulo Fiscal", "Módulo Declarações", "Módulo Contábil", "Módulo Folha", "Módulo Procurações/Certificados",
		"Responsável Fiscal", "Responsável Contábil", "Responsável Folha", "Responsável Declarações", "Observações"} {
		if !slices.Contains(rows[0], h) {
			t.Errorf("cabeçalho %q ausente", h)
		}
	}

	dvs, err := f.GetDataValidations(SheetCompanies)
	if err != nil {
		t.Fatal(err)
	}
	lists := map[string]string{}
	for _, dv := range dvs {
		lists[strings.SplitN(dv.Sqref, ":", 2)[0]] = dv.Formula1
	}
	col := func(header string) string {
		i := slices.Index(want, header)
		name, _ := excelize.ColumnNumberToName(i + 1)
		return name + "2"
	}
	for header, value := range map[string]string{
		"Regime tributário inicial": "Simples Nacional",
		"UF":                        "PR",
		"Tipo de folha":             "Pró-labore",
		"Módulo Fiscal":             "Sim",
		"Fator R":                   "Não",
		"Situação":                  "Inativa",
	} {
		if !strings.Contains(lists[col(header)], value) {
			t.Errorf("lista de %q = %q, want conter %q", header, lists[col(header)], value)
		}
	}

	instructions, err := f.GetRows(SheetInstructions)
	if err != nil {
		t.Fatal(err)
	}
	text := ""
	for _, r := range instructions {
		text += strings.Join(r, " | ") + "\n"
	}
	for _, want := range []string{"instruções", "CNPJ é a chave de conciliação", "dd/mm/aaaa", "Razão social | Sim", "Nome fantasia | Não"} {
		if !strings.Contains(text, want) {
			t.Errorf("instruções sem %q", want)
		}
	}
}
