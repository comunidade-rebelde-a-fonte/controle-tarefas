// Package report gera os relatórios PDF [PRD §36.2-§36.3] sobre um layout comum:
// cabeçalho (título, competência, data de geração, filtros), resumo de indicadores,
// tabela paginada com o cabeçalho de colunas repetido e rodapé "Página X/Y".
package report

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/johnfercher/maroto/v2"
	"github.com/johnfercher/maroto/v2/pkg/components/col"
	"github.com/johnfercher/maroto/v2/pkg/components/line"
	"github.com/johnfercher/maroto/v2/pkg/components/page"
	"github.com/johnfercher/maroto/v2/pkg/components/row"
	"github.com/johnfercher/maroto/v2/pkg/components/text"
	"github.com/johnfercher/maroto/v2/pkg/config"
	"github.com/johnfercher/maroto/v2/pkg/consts/align"
	"github.com/johnfercher/maroto/v2/pkg/consts/fontfamily"
	"github.com/johnfercher/maroto/v2/pkg/consts/fontstyle"
	"github.com/johnfercher/maroto/v2/pkg/consts/orientation"
	"github.com/johnfercher/maroto/v2/pkg/core"
	"github.com/johnfercher/maroto/v2/pkg/props"

	"controle/internal/platform/competence"
)

// Field é um par rótulo/valor (filtro aplicado ou indicador do resumo).
type Field struct {
	Label string
	Value string
}

// Column é uma coluna da tabela. Weight é a largura relativa (0 vale 1).
type Column struct {
	Title  string
	Weight int
}

// Document é o conteúdo de um relatório.
type Document struct {
	Title       string
	Competence  *competence.Competence // nil em relatórios sem competência
	GeneratedAt time.Time              // já no fuso de negócio (APP_TIMEZONE)
	Filters     []Field
	Summary     []Field
	Columns     []Column
	Rows        [][]string
	Landscape   bool
}

// Dimensões do layout, em mm. As linhas têm altura fixa para a paginação ser previsível.
const (
	gridSize         = 60 // divisível por 1..6: resumo e colunas proporcionais
	summaryPerRow    = 4
	titleHeight      = 9.0
	metaHeight       = 5.0
	ruleHeight       = 3.0
	summaryHeight    = 11.0
	tableHeadHeight  = 7.0
	tableRowHeight   = 6.0
	sectionGapHeight = 3.0
	pageEpsilon      = 1e-6

	// headerHeight é a soma das linhas de headerRows.
	headerHeight = titleHeight + 2*metaHeight + ruleHeight
)

var (
	headFill   = &props.Color{Red: 226, Green: 232, Blue: 240}
	zebraFill  = &props.Color{Red: 246, Green: 248, Blue: 250}
	mutedColor = &props.Color{Red: 90, Green: 90, Blue: 90}
)

// Fontes padrão do PDF (Helvetica) com tradução para cp1252, que cobre a acentuação pt-BR.
func font(size float64, style fontstyle.Type) props.Text {
	return props.Text{Family: fontfamily.Helvetica, Style: style, Size: size, Top: 1.2, Left: 1, Right: 1}
}

// Render gera o PDF do documento.
func Render(doc Document) ([]byte, error) {
	if strings.TrimSpace(doc.Title) == "" {
		return nil, errors.New("relatório sem título")
	}
	if len(doc.Columns) == 0 {
		return nil, errors.New("relatório sem colunas")
	}
	for i, r := range doc.Rows {
		if len(r) != len(doc.Columns) {
			return nil, fmt.Errorf("linha %d tem %d células; esperado %d", i+1, len(r), len(doc.Columns))
		}
	}

	b := config.NewBuilder().
		WithMaxGridSize(gridSize).
		WithCompression(true).
		WithTitle(doc.Title, true).
		WithCreationDate(doc.GeneratedAt).
		WithDefaultFont(&props.Font{Family: fontfamily.Helvetica, Size: 8}).
		WithPageNumber(props.PageNumber{
			Pattern: "Página {current}/{total}",
			Place:   props.RightBottom,
			Family:  fontfamily.Helvetica,
			Size:    8,
			Color:   mutedColor,
		})
	if doc.Landscape {
		b = b.WithOrientation(orientation.Horizontal)
	}
	cfg := b.Build()
	m := maroto.New(cfg)

	if err := m.RegisterHeader(headerRows(doc)...); err != nil {
		return nil, fmt.Errorf("cabeçalho do relatório: %w", err)
	}

	usable := cfg.Dimensions.Height - cfg.Margins.Top - cfg.Margins.Bottom
	used := headerHeight
	add := func(r core.Row, h float64) {
		m.AddRows(r)
		used += h
	}

	for _, r := range summaryRows(doc.Summary) {
		add(r, summaryHeight)
	}
	if len(doc.Summary) > 0 {
		add(row.New(sectionGapHeight), sectionGapHeight)
	}

	sizes := columnSizes(doc.Columns)
	add(tableHead(doc.Columns, sizes), tableHeadHeight)
	if len(doc.Rows) == 0 {
		add(text.NewRow(tableRowHeight, "Nenhum registro para os filtros selecionados.", font(8, fontstyle.Italic)), tableRowHeight)
	}
	for i, cells := range doc.Rows {
		// Quebra a página antes do maroto para repetir o cabeçalho da tabela no topo.
		if used+tableRowHeight > usable-pageEpsilon {
			m.AddPages(page.New().Add(tableHead(doc.Columns, sizes)))
			used = headerHeight + tableHeadHeight
		}
		add(tableRow(cells, sizes, i%2 == 1), tableRowHeight)
	}

	out, err := m.Generate()
	if err != nil {
		return nil, fmt.Errorf("gerar PDF: %w", err)
	}
	return out.GetBytes(), nil
}

// headerRows é o cabeçalho repetido em todas as páginas.
func headerRows(doc Document) []core.Row {
	meta := "Gerado em " + doc.GeneratedAt.Format("02/01/2006 15:04")
	if doc.Competence != nil {
		meta = "Competência: " + doc.Competence.Display() + "   ·   " + meta
	}
	filters := "Filtros: nenhum"
	if len(doc.Filters) > 0 {
		parts := make([]string, len(doc.Filters))
		for i, f := range doc.Filters {
			parts[i] = f.Label
			if f.Value != "" { // filtros de marcar (ex.: "Somente atrasadas") não têm valor
				parts[i] += ": " + f.Value
			}
		}
		filters = "Filtros: " + strings.Join(parts, "   ·   ")
	}
	muted := font(8, fontstyle.Normal)
	muted.Color = mutedColor
	return []core.Row{
		text.NewRow(titleHeight, doc.Title, font(14, fontstyle.Bold)),
		text.NewRow(metaHeight, meta, muted),
		text.NewRow(metaHeight, filters, muted),
		line.NewRow(ruleHeight, props.Line{Color: mutedColor, Thickness: 0.2, SizePercent: 100}),
	}
}

// summaryRows distribui os indicadores em linhas de até summaryPerRow blocos.
func summaryRows(fields []Field) []core.Row {
	var out []core.Row
	size := gridSize / summaryPerRow
	for start := 0; start < len(fields); start += summaryPerRow {
		end := min(start+summaryPerRow, len(fields))
		cols := make([]core.Col, 0, summaryPerRow)
		for _, f := range fields[start:end] {
			label := font(7, fontstyle.Normal)
			label.Color = mutedColor
			value := font(11, fontstyle.Bold)
			value.Top = 4.5
			cols = append(cols, col.New(size).Add(text.New(f.Label, label), text.New(f.Value, value)))
		}
		out = append(out, row.New(summaryHeight).Add(cols...))
	}
	return out
}

// columnSizes converte os pesos em tamanhos da grade; a última coluna fica com o resto.
func columnSizes(cols []Column) []int {
	total := 0
	for _, c := range cols {
		total += max(c.Weight, 1)
	}
	sizes := make([]int, len(cols))
	used := 0
	for i, c := range cols {
		if i == len(cols)-1 {
			sizes[i] = gridSize - used
			break
		}
		sizes[i] = max(gridSize*max(c.Weight, 1)/total, 1)
		used += sizes[i]
	}
	return sizes
}

func tableHead(cols []Column, sizes []int) core.Row {
	cs := make([]core.Col, len(cols))
	for i, c := range cols {
		cs[i] = text.NewCol(sizes[i], c.Title, font(8, fontstyle.Bold))
	}
	return row.New(tableHeadHeight).Add(cs...).WithStyle(&props.Cell{BackgroundColor: headFill})
}

func tableRow(cells []string, sizes []int, zebra bool) core.Row {
	cs := make([]core.Col, len(cells))
	for i, v := range cells {
		p := font(8, fontstyle.Normal)
		p.Align = align.Left
		cs[i] = text.NewCol(sizes[i], v, p)
	}
	r := row.New(tableRowHeight).Add(cs...)
	if zebra {
		r = r.WithStyle(&props.Cell{BackgroundColor: zebraFill})
	}
	return r
}
