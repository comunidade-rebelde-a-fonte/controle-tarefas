// Package exporter exporta as grades dos módulos para XLSX, com os mesmos filtros da tela
// [PRD §36.1, §41.13, CA-017].
package exporter

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/xuri/excelize/v2"
	"golang.org/x/text/unicode/norm"

	"controle/internal/company"
	"controle/internal/grid"
	"controle/internal/platform/civil"
)

// Rótulos exibidos no cabeçalho da planilha (a regra está toda no BuildGrid).
var (
	regimeLabels  = company.RegimeLabels
	payrollLabels = map[string]string{"NO_PAYROLL": "Sem folha", "PRO_LABORE": "Pró-labore", "PAYROLL": "Folha"}
	periodLabels  = map[string]string{"OPEN": "Aberto", "CLOSED": "Fechado"}
)

// Meta descreve o que foi exportado (cabeçalho da planilha).
type Meta struct {
	ModuleLabel string
	Competence  string // MM/AAAA
	Filters     []string
	GeneratedAt time.Time // já no fuso de negócio
}

const headerRows = 5 // título, competência, filtros, geração e linha em branco

func normalize(s string) string {
	var b strings.Builder
	for _, r := range norm.NFD.String(strings.ToLower(s)) {
		if !unicode.Is(unicode.Mn, r) {
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

var cnpjTermRe = regexp.MustCompile(`^[\d./\-\s]+$`)

func digits(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, s)
}

// MatchSearch é a busca livre da tela [PRD §26.5]: nome, razão social, nome fantasia, código
// e CNPJ com ou sem máscara (mesma regra do cliente, para o XLSX refletir a grade).
func MatchSearch(c grid.Company, q string) bool {
	term := normalize(q)
	if term == "" {
		return true
	}
	if strings.Contains(normalize(c.InternalCode), term) {
		return true
	}
	if d := digits(q); cnpjTermRe.MatchString(strings.TrimSpace(q)) && len(d) >= 3 && strings.Contains(c.Cnpj, d) {
		return true
	}
	trade := ""
	if c.TradeName != nil {
		trade = *c.TradeName
	}
	return strings.Contains(normalize(c.LegalName), term) || strings.Contains(normalize(trade), term)
}

func formatCnpj(d string) string {
	if len(d) != 14 {
		return d
	}
	return d[0:2] + "." + d[2:5] + "." + d[5:8] + "/" + d[8:12] + "-" + d[12:14]
}

// XLSX monta a planilha da grade: cabeçalho com módulo, competência, filtros e data de
// geração; uma linha por empresa; por obrigação, status em texto, vencimento e atraso;
// cabeçalho da tabela congelado e autofiltro.
func XLSX(g grid.Grid, rows []grid.Row, meta Meta) ([]byte, error) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	sheet := sheetName(meta.ModuleLabel)
	if err := f.SetSheetName("Sheet1", sheet); err != nil {
		return nil, err
	}

	bold, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true, Size: 13}})
	if err != nil {
		return nil, err
	}
	head, err := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true},
		Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"E2E8F0"}},
		Alignment: &excelize.Alignment{WrapText: true, Vertical: "center"},
	})
	if err != nil {
		return nil, err
	}
	pct, err := f.NewStyle(&excelize.Style{NumFmt: 9})
	if err != nil {
		return nil, err
	}
	date, err := f.NewStyle(&excelize.Style{CustomNumFmt: ptr("dd/mm/yyyy")})
	if err != nil {
		return nil, err
	}

	filters := "Filtros: nenhum"
	if len(meta.Filters) > 0 {
		filters = "Filtros: " + strings.Join(meta.Filters, " · ")
	}
	top := []string{
		"Controle de Obrigações — " + meta.ModuleLabel,
		"Competência: " + meta.Competence,
		filters,
		"Gerado em " + meta.GeneratedAt.Format("02/01/2006 15:04"),
	}
	for i, v := range top {
		if err := f.SetCellValue(sheet, fmt.Sprintf("A%d", i+1), v); err != nil {
			return nil, err
		}
	}
	if err := f.SetCellStyle(sheet, "A1", "A1", bold); err != nil {
		return nil, err
	}

	fixed := []string{"Código", "Empresa", "CNPJ", "Regime", "Tipo de folha", "Responsável", "Módulo", "Progresso operacional",
		"Conclusão", "Obrigações atrasadas", "Próximo vencimento", "Observação"}
	headers := append([]string{}, fixed...)
	for _, c := range g.Columns {
		name := c.ObligationType.Name
		headers = append(headers, name, name+" — vencimento", name+" — atrasada")
	}
	headerRow := headerRows + 1
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, headerRow)
		if err := f.SetCellValue(sheet, cell, h); err != nil {
			return nil, err
		}
	}
	lastCol, _ := excelize.ColumnNumberToName(len(headers))
	if err := f.SetCellStyle(sheet, fmt.Sprintf("A%d", headerRow), fmt.Sprintf("%s%d", lastCol, headerRow), head); err != nil {
		return nil, err
	}

	for i, r := range rows {
		n := headerRow + 1 + i
		values := []any{
			r.Company.InternalCode, r.Company.LegalName, formatCnpj(r.Company.Cnpj),
			regimeLabels[r.Snapshot.TaxRegime], payrollLabels[r.Snapshot.PayrollType], responsible(r),
			periodLabels[r.Period.State], r.Progress.OperationalPct, r.Progress.FinalPct, r.Overdue, civilTime(r.NearestDeadline),
			deref(r.Period.GeneralNote),
		}
		for _, c := range g.Columns {
			cell, ok := r.Cells[c.ObligationType.Code]
			switch {
			case !ok:
				values = append(values, "—", nil, "")
			case !cell.Applicable:
				values = append(values, "Não se aplica", civilTime(cell.DueDateEffective), "")
			default:
				late := ""
				if cell.IsOverdue {
					late = "Sim"
				}
				values = append(values, cell.StatusLabel, civilTime(cell.DueDateEffective), late)
			}
		}
		start, _ := excelize.CoordinatesToCellName(1, n)
		if err := f.SetSheetRow(sheet, start, &values); err != nil {
			return nil, err
		}
	}

	last := headerRow + len(rows)
	if len(rows) > 0 {
		if err := f.SetCellStyle(sheet, fmt.Sprintf("H%d", headerRow+1), fmt.Sprintf("I%d", last), pct); err != nil {
			return nil, err
		}
		if err := f.SetCellStyle(sheet, fmt.Sprintf("K%d", headerRow+1), fmt.Sprintf("K%d", last), date); err != nil {
			return nil, err
		}
		for i := range g.Columns {
			col, _ := excelize.ColumnNumberToName(len(fixed) + 3*i + 2)
			if err := f.SetCellStyle(sheet, fmt.Sprintf("%s%d", col, headerRow+1), fmt.Sprintf("%s%d", col, last), date); err != nil {
				return nil, err
			}
		}
	}
	if err := f.AutoFilter(sheet, fmt.Sprintf("A%d:%s%d", headerRow, lastCol, max(last, headerRow)), nil); err != nil {
		return nil, err
	}
	if err := f.SetPanes(sheet, &excelize.Panes{
		Freeze: true, XSplit: 2, YSplit: headerRow, TopLeftCell: fmt.Sprintf("C%d", headerRow+1), ActivePane: "bottomRight",
	}); err != nil {
		return nil, err
	}
	if err := f.SetColWidth(sheet, "A", "A", 10); err != nil {
		return nil, err
	}
	if err := f.SetColWidth(sheet, "B", "B", 42); err != nil {
		return nil, err
	}
	if err := f.SetColWidth(sheet, "C", lastCol, 16); err != nil {
		return nil, err
	}

	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return bytes.Clone(buf.Bytes()), nil
}

func ptr[T any](v T) *T { return &v }

// civilTime devolve a data para a célula (excelize grava time.Time como data do Excel) ou nil.
func civilTime(d *civil.Date) any {
	if d == nil {
		return nil
	}
	return d.Time
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func responsible(r grid.Row) string {
	if r.Snapshot.Responsible == nil {
		return ""
	}
	return r.Snapshot.Responsible.Name
}

// sheetName respeita o limite de 31 caracteres e os caracteres proibidos do Excel.
func sheetName(label string) string {
	s := strings.NewReplacer("/", "-", "\\", "-", "?", "", "*", "", "[", "(", "]", ")", ":", "-").Replace(label)
	if r := []rune(s); len(r) > 31 {
		s = string(r[:31])
	}
	return s
}
