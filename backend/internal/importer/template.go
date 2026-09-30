package importer

import (
	"bytes"
	"fmt"

	"github.com/xuri/excelize/v2"
)

// Nomes das abas e limite de linhas com validação de lista.
const (
	SheetCompanies    = "Empresas"
	SheetInstructions = "Instruções"
	validationRows    = 1000
)

// TemplateFileName é o nome sugerido para download.
const TemplateFileName = "modelo-importacao-empresas.xlsx"

// Template gera o modelo de importação [PRD §35.3]: aba "Empresas" com os cabeçalhos do §7.1,
// regime inicial, tipo de folha e módulos, com listas de validação; e aba "Instruções".
func Template() ([]byte, error) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()

	if err := f.SetSheetName("Sheet1", SheetCompanies); err != nil {
		return nil, err
	}
	required, err := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"1E3A5F"}},
		Alignment: &excelize.Alignment{Vertical: "center", WrapText: true},
	})
	if err != nil {
		return nil, err
	}
	optional, err := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true},
		Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"E2E8F0"}},
		Alignment: &excelize.Alignment{Vertical: "center", WrapText: true},
	})
	if err != nil {
		return nil, err
	}

	for i, c := range Columns {
		col, err := excelize.ColumnNumberToName(i + 1)
		if err != nil {
			return nil, err
		}
		cell := col + "1"
		if err := f.SetCellValue(SheetCompanies, cell, c.Header); err != nil {
			return nil, err
		}
		style := optional
		if c.Required {
			style = required
		}
		if err := f.SetCellStyle(SheetCompanies, cell, cell, style); err != nil {
			return nil, err
		}
		if c.Width > 0 {
			if err := f.SetColWidth(SheetCompanies, col, col, c.Width); err != nil {
				return nil, err
			}
		}
		if len(c.Options) > 0 {
			dv := excelize.NewDataValidation(true)
			dv.Sqref = fmt.Sprintf("%s2:%s%d", col, col, validationRows+1)
			if err := dv.SetDropList(c.Options); err != nil {
				return nil, err
			}
			dv.SetError(excelize.DataValidationErrorStyleStop, c.Header, "Escolha um valor da lista.")
			if err := f.AddDataValidation(SheetCompanies, dv); err != nil {
				return nil, err
			}
		}
	}
	if err := f.SetRowHeight(SheetCompanies, 1, 32); err != nil {
		return nil, err
	}
	if err := f.SetPanes(SheetCompanies, &excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"}); err != nil {
		return nil, err
	}
	if err := writeInstructions(f); err != nil {
		return nil, err
	}
	f.SetActiveSheet(0)

	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return bytes.Clone(buf.Bytes()), nil
}

func writeInstructions(f *excelize.File) error {
	if _, err := f.NewSheet(SheetInstructions); err != nil {
		return err
	}
	bold, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})
	if err != nil {
		return err
	}
	title, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true, Size: 14}})
	if err != nil {
		return err
	}
	intro := []string{
		"Preencha uma linha por empresa na aba \"Empresas\", a partir da linha 2. Não altere os cabeçalhos.",
		"Colunas com cabeçalho escuro são obrigatórias.",
		"O CNPJ é a chave de conciliação: empresas já cadastradas aparecem como \"Empresa existente\" e só são atualizadas se você confirmar na prévia.",
		"O código interno também precisa ser único.",
		"Datas no formato dd/mm/aaaa. Listas (regime, UF, tipo de folha, Sim/Não) aceitam só os valores sugeridos.",
		"A importação mostra uma prévia com os erros por linha antes de gravar; nada é importado sem confirmação.",
		"Use apenas dados reais do escritório; não inclua senhas nem arquivos de certificado.",
	}
	rows := [][]any{{"Importação de empresas — instruções"}, {}}
	for _, l := range intro {
		rows = append(rows, []any{l})
	}
	rows = append(rows, []any{}, []any{"Coluna", "Obrigatória", "Como preencher"})
	for _, c := range Columns {
		req := "Não"
		if c.Required {
			req = "Sim"
		}
		rows = append(rows, []any{c.Header, req, c.Help})
	}
	for i, r := range rows {
		cell, err := excelize.CoordinatesToCellName(1, i+1)
		if err != nil {
			return err
		}
		if len(r) == 0 {
			continue
		}
		if err := f.SetSheetRow(SheetInstructions, cell, &r); err != nil {
			return err
		}
	}
	if err := f.SetCellStyle(SheetInstructions, "A1", "A1", title); err != nil {
		return err
	}
	header := len(intro) + 4
	if err := f.SetCellStyle(SheetInstructions, fmt.Sprintf("A%d", header), fmt.Sprintf("C%d", header), bold); err != nil {
		return err
	}
	if err := f.SetColWidth(SheetInstructions, "A", "A", 34); err != nil {
		return err
	}
	if err := f.SetColWidth(SheetInstructions, "B", "B", 12); err != nil {
		return err
	}
	return f.SetColWidth(SheetInstructions, "C", "C", 90)
}
