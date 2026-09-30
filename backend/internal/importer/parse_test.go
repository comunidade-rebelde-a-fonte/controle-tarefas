package importer

import (
	"bytes"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
	"golang.org/x/text/encoding/charmap"
)

func headers() []string {
	out := make([]string, len(Columns))
	for i, c := range Columns {
		out[i] = c.Header
	}
	return out
}

// xlsxFrom preenche o modelo oficial com as linhas e deixa linhas "vazias" no fim.
func xlsxFrom(t *testing.T, rows [][]any, trailingEmpty int) []byte {
	t.Helper()
	tpl, err := Template()
	if err != nil {
		t.Fatal(err)
	}
	f, err := excelize.OpenReader(bytes.NewReader(tpl))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	for i, r := range rows {
		cell, _ := excelize.CoordinatesToCellName(1, i+2)
		if err := f.SetSheetRow(SheetCompanies, cell, &r); err != nil {
			t.Fatal(err)
		}
	}
	for i := range trailingEmpty {
		cell, _ := excelize.CoordinatesToCellName(1, len(rows)+2+i)
		if err := f.SetCellValue(SheetCompanies, cell, "   "); err != nil {
			t.Fatal(err)
		}
	}
	buf, err := f.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestParse_XLSXIgnoresTrailingEmptyRowsAndMapsHeaders(t *testing.T) {
	row := make([]any, len(Columns))
	row[0], row[1], row[3] = "301", "Nova Empresa Ltda", "00.000.011/0001-91"
	data := xlsxFrom(t, [][]any{row}, 5)
	sheet, err := Parse("empresas.xlsx", data)
	if err != nil {
		t.Fatal(err)
	}
	if len(sheet.Records) != 1 {
		t.Fatalf("linhas = %d, want 1 (linhas vazias no fim ignoradas)", len(sheet.Records))
	}
	r := sheet.Records[0]
	if r.Line != 2 || r.Values["internal_code"] != "301" || r.Values["legal_name"] != "Nova Empresa Ltda" || r.Values["cnpj"] != "00.000.011/0001-91" {
		t.Fatalf("registro = %+v", r)
	}
}

func TestParse_CSVSemicolonLatin1(t *testing.T) {
	csv := strings.Join(headers(), ";") + "\r\n" +
		"302;Padaria São João Ltda;Pão Quente;00000012000137;Simples Nacional;01/02/2026;;;Curitiba;PR;;;Sem folha;Não;Sim;Sim;Não;Não;Não;;;;;Observação com acentuação\r\n"
	latin1, err := charmap.Windows1252.NewEncoder().String(csv)
	if err != nil {
		t.Fatal(err)
	}
	sheet, err := Parse("empresas.csv", []byte(latin1))
	if err != nil {
		t.Fatal(err)
	}
	if len(sheet.Records) != 1 {
		t.Fatalf("linhas = %d", len(sheet.Records))
	}
	v := sheet.Records[0].Values
	if v["legal_name"] != "Padaria São João Ltda" || v["trade_name"] != "Pão Quente" || v["notes"] != "Observação com acentuação" || v["payroll_type"] != "Sem folha" {
		t.Fatalf("valores = %+v", v)
	}
}

func TestParse_CSVCommaUTF8WithBOMAndLooseHeaders(t *testing.T) {
	// Cabeçalhos sem acento, em maiúsculas, com asterisco e coluna extra desconhecida.
	csv := "\ufeffCODIGO INTERNO*,razao social,CNPJ,Regime tributario inicial,Inicio do atendimento,Tipo de folha,Coluna extra\n" +
		"303,\"Comércio, Varejo e Cia\",00000013000181,Lucro Presumido,2026-01-15,Folha,x\n"
	sheet, err := Parse("empresas.csv", []byte(csv))
	if err != nil {
		t.Fatal(err)
	}
	if len(sheet.UnknownHeaders) != 1 || sheet.UnknownHeaders[0] != "Coluna extra" {
		t.Fatalf("desconhecidas = %v", sheet.UnknownHeaders)
	}
	v := sheet.Records[0].Values
	if v["internal_code"] != "303" || v["legal_name"] != "Comércio, Varejo e Cia" || v["service_start_date"] != "2026-01-15" {
		t.Fatalf("valores = %+v", v)
	}
}

func TestParse_InvalidFiles(t *testing.T) {
	cases := map[string]struct {
		name string
		data []byte
		msg  string
	}{
		"vazio":              {"a.csv", nil, "vazio"},
		"formato":            {"a.pdf", []byte("%PDF-1.4"), "Formato não aceito"},
		"sem obrigatórias":   {"a.csv", []byte("Razão social;CNPJ\nX;1\n"), "Código interno"},
		"só cabeçalho":       {"a.csv", []byte(strings.Join(headers(), ";") + "\n;;;\n"), "não tem linhas"},
		"cabeçalho repetido": {"a.csv", []byte(strings.Join(append(headers(), "CNPJ"), ";") + "\n"), "repetida"},
		"xlsx corrompido":    {"a.xlsx", []byte("PK\x03\x04lixo"), "Não foi possível ler"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Parse(c.name, c.data)
			if err == nil || !strings.Contains(errMessage(err), c.msg) {
				t.Fatalf("err = %v, want mensagem com %q", err, c.msg)
			}
		})
	}
}
