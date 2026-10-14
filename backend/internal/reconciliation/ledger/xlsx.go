package ledger

import (
	"fmt"
	"time"

	"github.com/xuri/excelize/v2"

	"controle/internal/platform/cnpj"
	"controle/internal/platform/competence"
)

// Planilha do razão (TASK-028, D-042), com as duas abas da decisão do usuário:
//   - "Razão": os títulos do CSV na 1ª linha e uma linha por conciliação, na ordem do razão, pronta
//     para filtrar ou importar;
//   - "Resumo": a empresa, o período, o filtro de conta, a quantidade de lançamentos e os totais.
//
// Débito e crédito são números de duas casas, gravados a partir do texto exato do decimal (nenhum
// float no caminho); a data é data (dd/mm/aaaa); o resto é texto, nunca fórmula. O arquivo não
// leva data de geração: o mesmo razão dá sempre o mesmo arquivo (AC-022).

const (
	sheetLedger  = "Razão"
	sheetSummary = "Resumo"
	// moneyFmt é o formato embutido "#,##0.00"; o Excel mostra no separador da região.
	moneyFmt = 4
	dateFmt  = "dd/mm/yyyy"
)

// MimeXLSX é o tipo da planilha.
const MimeXLSX = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"

// ledgerWidths são as larguras das colunas da aba Razão, na ordem do csvHeader.
var ledgerWidths = []float64{12, 24, 40, 14, 14, 12, 16, 30, 32, 20, 18}

// Info é o que a aba Resumo mostra além do razão: a empresa e o filtro de conta.
type Info struct {
	CompanyCode, CompanyName, Cnpj string
	// Account é o filtro de conta (bancária ou contábil); vazio, nenhum.
	Account string
}

func cellName(col, row int) string {
	name, _ := excelize.CoordinatesToCellName(col, row)
	return name
}

// XLSX escreve o razão na planilha.
func XLSX(l Ledger, info Info) ([]byte, error) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	if err := f.SetSheetName("Sheet1", sheetLedger); err != nil {
		return nil, err
	}
	head, err := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true},
		Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"E2E8F0"}},
	})
	if err != nil {
		return nil, err
	}
	money, err := f.NewStyle(&excelize.Style{NumFmt: moneyFmt})
	if err != nil {
		return nil, err
	}
	date, err := f.NewStyle(&excelize.Style{CustomNumFmt: ptr(dateFmt)})
	if err != nil {
		return nil, err
	}
	bold, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})
	if err != nil {
		return nil, err
	}

	if err := writeLedger(f, l, head, money, date); err != nil {
		return nil, err
	}
	if err := writeSummary(f, l, info, bold, money); err != nil {
		return nil, err
	}
	f.SetActiveSheet(0)
	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func ptr[T any](v T) *T { return &v }

// writeLedger é a aba Razão: os títulos do CSV e uma linha por conciliação, com o cabeçalho
// congelado e o autofiltro.
func writeLedger(f *excelize.File, l Ledger, head, money, date int) error {
	s := sheetLedger
	for i, h := range csvHeader {
		if err := f.SetCellStr(s, cellName(i+1, 1), h); err != nil {
			return err
		}
	}
	last := cellName(len(csvHeader), 1)
	if err := f.SetCellStyle(s, "A1", last, head); err != nil {
		return err
	}
	// Os estilos das colunas vêm antes dos valores: com a célula já formatada, a data não ganha o
	// formato padrão da biblioteca.
	if n := len(l.Items); n > 0 {
		if err := f.SetCellStyle(s, "A2", cellName(1, n+1), date); err != nil {
			return err
		}
		if err := f.SetCellStyle(s, "D2", cellName(5, n+1), money); err != nil {
			return err
		}
	}
	for i, r := range l.Items {
		row := i + 2
		day, err := time.Parse(time.DateOnly, r.Date)
		if err != nil {
			return fmt.Errorf("razão: data %q: %w", r.Date, err)
		}
		if err := f.SetCellValue(s, cellName(1, row), day); err != nil {
			return err
		}
		origin := ""
		if r.ClassificationOrigin != nil && !r.ClassificationPending {
			origin = *r.ClassificationOrigin
		}
		texts := map[int]string{2: r.BankAccount, 3: r.History, 6: r.Competence, 7: text(r.AccountCode), 8: text(r.AccountName),
			9: text(r.Counterparty), 10: origin, 11: r.ReconciliationOrigin}
		for col := 2; col <= len(csvHeader); col++ {
			v, ok := texts[col]
			if !ok || v == "" {
				continue
			}
			if err := f.SetCellStr(s, cellName(col, row), v); err != nil {
				return err
			}
		}
		// Número com o texto exato do decimal (SetCellDefault grava o valor como está).
		if err := f.SetCellDefault(s, cellName(4, row), r.Debit.Text()); err != nil {
			return err
		}
		if err := f.SetCellDefault(s, cellName(5, row), r.Credit.Text()); err != nil {
			return err
		}
	}
	for i, w := range ledgerWidths {
		col, _ := excelize.ColumnNumberToName(i + 1)
		if err := f.SetColWidth(s, col, col, w); err != nil {
			return err
		}
	}
	if err := f.SetPanes(s, &excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"}); err != nil {
		return err
	}
	return f.AutoFilter(s, "A1:"+cellName(len(csvHeader), max(len(l.Items)+1, 2)), nil)
}

// writeSummary é a aba Resumo: rótulo e valor por linha.
func writeSummary(f *excelize.File, l Ledger, info Info, bold, money int) error {
	s := sheetSummary
	if _, err := f.NewSheet(s); err != nil {
		return err
	}
	period := l.From
	if from, err := competence.Parse(l.From); err == nil {
		period = from.Display()
	}
	if to, err := competence.Parse(l.To); err == nil {
		period += " a " + to.Display()
	}
	account := info.Account
	if account == "" {
		account = "nenhum"
	}
	rows := []struct {
		label, text, number string
	}{
		{label: "Empresa", text: info.CompanyCode + " · " + info.CompanyName},
		{label: "CNPJ", text: cnpj.Format(info.Cnpj)},
		{label: "Período", text: period},
		{label: "Filtro de conta", text: account},
		{label: "Lançamentos", number: fmt.Sprint(len(l.Items))},
		{label: "Total de débito", number: l.Debit.Text()},
		{label: "Total de crédito", number: l.Credit.Text()},
	}
	for i, r := range rows {
		row := i + 1
		if err := f.SetCellStr(s, cellName(1, row), r.label); err != nil {
			return err
		}
		if r.number != "" {
			if err := f.SetCellDefault(s, cellName(2, row), r.number); err != nil {
				return err
			}
			continue
		}
		if err := f.SetCellStr(s, cellName(2, row), r.text); err != nil {
			return err
		}
	}
	if err := f.SetCellStyle(s, "A1", cellName(1, len(rows)), bold); err != nil {
		return err
	}
	// Os totais em reais; a quantidade fica sem casas.
	if err := f.SetCellStyle(s, cellName(2, len(rows)-1), cellName(2, len(rows)), money); err != nil {
		return err
	}
	if err := f.SetColWidth(s, "A", "A", 20); err != nil {
		return err
	}
	return f.SetColWidth(s, "B", "B", 48)
}
