package report

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
	"regexp"
	"strings"
	"testing"
	"time"

	"golang.org/x/text/encoding/charmap"

	"controle/internal/platform/competence"
)

var streamRe = regexp.MustCompile(`(?s)stream\r?\n(.*?)\r?\nendstream`)

// pdfText extrai o texto das strings literais dos fluxos de conteúdo do PDF. É suficiente
// para as fontes padrão (Helvetica, cp1252) usadas pelo layout.
func pdfText(t *testing.T, pdf []byte) string {
	t.Helper()
	var sb strings.Builder
	for _, m := range streamRe.FindAllSubmatch(pdf, -1) {
		data := m[1]
		if r, err := zlib.NewReader(bytes.NewReader(data)); err == nil {
			if dec, err := io.ReadAll(r); err == nil {
				data = dec
			}
		}
		for _, s := range literalStrings(data) {
			u, err := charmap.Windows1252.NewDecoder().Bytes(s)
			if err != nil {
				t.Fatalf("decodificar cp1252: %v", err)
			}
			sb.Write(u)
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

// literalStrings devolve as strings "(...)" de um fluxo de conteúdo, já sem escapes.
func literalStrings(b []byte) [][]byte {
	var out [][]byte
	for i := 0; i < len(b); i++ {
		if b[i] != '(' {
			continue
		}
		var s []byte
		depth := 1
		for i++; i < len(b) && depth > 0; i++ {
			c := b[i]
			switch {
			case c == '\\' && i+1 < len(b):
				i++
				switch e := b[i]; e {
				case 'n':
					s = append(s, '\n')
				case 'r':
					s = append(s, '\r')
				case 't':
					s = append(s, '\t')
				case '0', '1', '2', '3', '4', '5', '6', '7':
					v := 0
					for k := 0; k < 3 && i < len(b) && b[i] >= '0' && b[i] <= '7'; k++ {
						v = v*8 + int(b[i]-'0')
						i++
					}
					i--
					s = append(s, byte(v))
				default:
					s = append(s, e)
				}
			case c == '(':
				depth++
				s = append(s, c)
			case c == ')':
				depth--
				if depth > 0 {
					s = append(s, c)
				}
			default:
				s = append(s, c)
			}
		}
		i--
		out = append(out, s)
	}
	return out
}

func pageCount(pdf []byte) int { return bytes.Count(pdf, []byte("/Type /Page\n")) }

func sampleDoc() Document {
	c := competence.MustParse("2026-09")
	return Document{
		Title:       "Relatório — Situação geral da competência",
		Competence:  &c,
		GeneratedAt: time.Date(2026, 9, 29, 14, 5, 0, 0, time.FixedZone("BRT", -3*60*60)),
		Filters:     []Field{{"Regime", "Simples Nacional"}, {"Módulo", "Fiscal"}},
		Summary: []Field{
			{"Obrigações aplicáveis", "120"},
			{"Concluídas", "87"},
			{"Atrasadas", "5"},
			{"Progresso operacional", "72,5%"},
			{"Não aplicáveis", "9"},
		},
		Columns: []Column{{"Empresa", 3}, {"Obrigação", 2}, {"Situação", 1}},
		Rows: [][]string{
			{"Açúcar & Cia Comércio Ltda", "ICMS", "Não se aplica"},
			{"Serviços (ME) Ltda", "Contabilização da folha", "Pendente"},
		},
	}
}

func TestRender_TextAndAccents(t *testing.T) {
	pdf, err := Render(sampleDoc())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(pdf, []byte("%PDF-")) {
		t.Fatal("saída não é um PDF")
	}
	if n := pageCount(pdf); n != 1 {
		t.Fatalf("páginas = %d, want 1", n)
	}
	got := pdfText(t, pdf)
	for _, want := range []string{
		"Relatório — Situação geral da competência",
		"Competência: 09/2026",
		"Gerado em 29/09/2026 14:05",
		"Filtros: Regime: Simples Nacional   ·   Módulo: Fiscal",
		"Obrigações aplicáveis", "72,5%", "Não aplicáveis",
		"Empresa", "Obrigação", "Situação",
		"Açúcar & Cia Comércio Ltda", "Não se aplica",
		"Serviços (ME) Ltda", "Contabilização da folha",
		"Página 1/1",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("texto extraído não contém %q\n--- texto ---\n%s", want, got)
		}
	}
}

func TestRender_PaginatesWithRepeatedHeaders(t *testing.T) {
	doc := sampleDoc()
	doc.Columns = []Column{{"Razão social", 3}, {"Tipo de obrigação", 2}, {"Status atual", 1}}
	doc.Rows = nil
	for i := 1; i <= 150; i++ {
		doc.Rows = append(doc.Rows, []string{fmt.Sprintf("Empresa %03d", i), "DCTFWeb", "A iniciar"})
	}
	pdf, err := Render(doc)
	if err != nil {
		t.Fatal(err)
	}
	pages := pageCount(pdf)
	if pages < 3 {
		t.Fatalf("150 linhas deveriam ocupar ao menos 3 páginas, got %d", pages)
	}
	got := pdfText(t, pdf)
	// Cabeçalho do relatório e cabeçalho da tabela em todas as páginas.
	for _, h := range []string{"Relatório — Situação geral da competência", "Razão social", "Tipo de obrigação"} {
		if n := strings.Count(got, h+"\n"); n != pages {
			t.Errorf("%q aparece %d vezes, want %d (uma por página)", h, n, pages)
		}
	}
	// O resumo aparece só na primeira página.
	if n := strings.Count(got, "Obrigações aplicáveis\n"); n != 1 {
		t.Errorf("resumo aparece %d vezes, want 1", n)
	}
	for i := 1; i <= 150; i++ {
		if !strings.Contains(got, fmt.Sprintf("Empresa %03d\n", i)) {
			t.Fatalf("linha %d ausente do PDF", i)
		}
	}
	for p := 1; p <= pages; p++ {
		if want := fmt.Sprintf("Página %d/%d", p, pages); !strings.Contains(got, want) {
			t.Errorf("rodapé %q ausente", want)
		}
	}
}

func TestRender_Landscape(t *testing.T) {
	portrait, err := Render(sampleDoc())
	if err != nil {
		t.Fatal(err)
	}
	doc := sampleDoc()
	doc.Landscape = true
	landscape, err := Render(doc)
	if err != nil {
		t.Fatal(err)
	}
	// A4 em pontos: 595.28 × 841.89.
	if !bytes.Contains(landscape, []byte("841.89 595.28")) {
		t.Error("paisagem deveria ter a página A4 deitada")
	}
	if bytes.Contains(portrait, []byte("841.89 595.28")) {
		t.Error("retrato não deveria ter a página deitada")
	}
}

func TestRender_WithoutCompetenceAndRows(t *testing.T) {
	doc := sampleDoc()
	doc.Competence = nil
	doc.Filters = nil
	doc.Summary = nil
	doc.Rows = nil
	pdf, err := Render(doc)
	if err != nil {
		t.Fatal(err)
	}
	got := pdfText(t, pdf)
	if strings.Contains(got, "Competência:") {
		t.Error("relatório sem competência não deveria exibi-la")
	}
	for _, want := range []string{"Filtros: nenhum", "Nenhum registro para os filtros selecionados."} {
		if !strings.Contains(got, want) {
			t.Errorf("texto extraído não contém %q", want)
		}
	}
}

func TestRender_Invalid(t *testing.T) {
	noTitle := sampleDoc()
	noTitle.Title = "  "
	noCols := sampleDoc()
	noCols.Columns = nil
	badRow := sampleDoc()
	badRow.Rows = append(badRow.Rows, []string{"só uma célula"})
	for name, doc := range map[string]Document{"sem título": noTitle, "sem colunas": noCols, "linha incompleta": badRow} {
		if _, err := Render(doc); err == nil {
			t.Errorf("%s: esperava erro", name)
		}
	}
}

func TestColumnSizes(t *testing.T) {
	cases := []struct {
		cols []Column
		want []int
	}{
		{[]Column{{"a", 1}, {"b", 1}, {"c", 1}}, []int{20, 20, 20}},
		{[]Column{{"a", 3}, {"b", 2}, {"c", 1}}, []int{30, 20, 10}},
		{[]Column{{"a", 0}, {"b", 0}}, []int{30, 30}},
		{[]Column{{"a", 1}, {"b", 1}, {"c", 1}, {"d", 1}, {"e", 1}, {"f", 1}, {"g", 1}}, []int{8, 8, 8, 8, 8, 8, 12}},
	}
	for _, c := range cases {
		got := columnSizes(c.cols)
		sum := 0
		for _, s := range got {
			sum += s
		}
		if fmt.Sprint(got) != fmt.Sprint(c.want) || sum != gridSize {
			t.Errorf("columnSizes(%v) = %v (soma %d), want %v", c.cols, got, sum, c.want)
		}
	}
}
