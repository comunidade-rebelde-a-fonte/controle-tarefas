package pdftext

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image/png"
	"math"
	"os"
	"testing"
	"time"
)

// goldenCase é um PDF com o texto do page.get_text() do PyMuPDF (testdata/mupdf_gen.py).
type goldenCase struct {
	PDF     string       `json:"pdf"`
	Paginas []string     `json:"paginas"`
	Sizes   [][2]float64 `json:"tamanhos"`
}

func loadMuPDFGolden(t *testing.T) []goldenCase {
	t.Helper()
	raw, err := os.ReadFile("testdata/mupdf_golden.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []goldenCase
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) < 60 {
		t.Fatalf("golden com %d PDFs; gere de novo com testdata/mupdf_gen.py", len(cases))
	}
	return cases
}

// knownDiff são as diferenças conhecidas do PDFium em relação ao MuPDF (ver pages.go).
var knownDiff = map[string][]string{
	// O PDFium descarta o texto repetido no mesmo lugar (negrito falso); o MuPDF o repete.
	"mupdf/negrito_falso.pdf": {"Negrito\n"},
	// O PDFium ordena os objetos de texto de uma mesma linha pela posição; o MuPDF segue a ordem
	// do conteúdo (aqui, "CD" escrito depois de "AB", mas à esquerda dele).
	"mupdf/intervalo_-2.0.pdf": {"CDAB\n"},
}

// O texto de cada página sai igual ao do PyMuPDF (extrair_texto_pdf da origem) nos PDFs do
// PGDAS-D, nos do cenário da Conciliação e nos sintéticos das regras de linha e espaço.
func TestPages_MatchesPyMuPDF(t *testing.T) {
	for _, c := range loadMuPDFGolden(t) {
		t.Run(c.PDF, func(t *testing.T) {
			data, err := os.ReadFile("testdata/" + c.PDF)
			if err != nil {
				t.Fatal(err)
			}
			got, err := shared.Pages(context.Background(), data, 0)
			if err != nil {
				t.Fatal(err)
			}
			want := c.Paginas
			if d, ok := knownDiff[c.PDF]; ok {
				want = d
			}
			if got.PageCount != len(c.Paginas) || len(got.Pages) != len(want) {
				t.Fatalf("páginas = %d (%d lidas), PyMuPDF = %d", got.PageCount, len(got.Pages), len(c.Paginas))
			}
			for i := range want {
				if got.Pages[i] != want[i] {
					t.Fatalf("página %d diverge do PyMuPDF:\n--- porte\n%q\n--- PyMuPDF\n%q", i, got.Pages[i], want[i])
				}
			}
		})
	}
}

func TestPages_MaxPagesKeepsTheCount(t *testing.T) {
	data, err := os.ReadFile("../../reconciliation/testdata/conciliacao/extratos/extrato_construtora_horizonte_2026-08.pdf")
	if err != nil {
		t.Fatal(err)
	}
	got, err := shared.Pages(context.Background(), data, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got.PageCount != 2 || len(got.Pages) != 1 {
		t.Fatalf("Pages(1) = %d páginas de %d", len(got.Pages), got.PageCount)
	}
}

// O render sai em PNG, no tamanho da página no DPI pedido, sobre fundo branco e sem alfa.
func TestRender_PNGAtDPI(t *testing.T) {
	data, err := os.ReadFile("../../reconciliation/testdata/conciliacao/extratos/extrato_banco_beta_2026-08.pdf")
	if err != nil {
		t.Fatal(err)
	}
	for _, dpi := range []int{72, 150, 300} {
		imgs, err := shared.Render(context.Background(), data, []int{0}, dpi)
		if err != nil || len(imgs) != 1 {
			t.Fatalf("Render(%d) = %d imagens, %v", dpi, len(imgs), err)
		}
		if !bytes.HasPrefix(imgs[0], []byte("\x89PNG\r\n\x1a\n")) {
			t.Fatalf("Render(%d) não é PNG", dpi)
		}
		img, err := png.Decode(bytes.NewReader(imgs[0]))
		if err != nil {
			t.Fatal(err)
		}
		// A4 (595,28 × 841,89 pt), arredondado para cima como o go-pdfium.
		w, h := int(math.Ceil(595.2756*float64(dpi)/72)), int(math.Ceil(841.8898*float64(dpi)/72))
		if b := img.Bounds(); b.Dx() != w || b.Dy() != h {
			t.Fatalf("Render(%d) = %dx%d, esperado %dx%d", dpi, b.Dx(), b.Dy(), w, h)
		}
		r, g, b, a := img.At(2, 2).RGBA()
		if r>>8 != 255 || g>>8 != 255 || b>>8 != 255 || a>>8 != 255 {
			t.Fatalf("Render(%d): canto não é branco opaco: %d %d %d %d", dpi, r>>8, g>>8, b>>8, a>>8)
		}
	}
}

// rawPDF monta um PDF mínimo com a página e o conteúdo dados.
func rawPDF(mediaBox, content string) []byte {
	objs := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [" + mediaBox + "] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	}
	var b bytes.Buffer
	b.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objs))
	for i, o := range objs {
		offsets[i] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(objs)+1)
	for _, off := range offsets {
		fmt.Fprintf(&b, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objs)+1, xref)
	return b.Bytes()
}

// Página gigante não estoura a memória: o DPI cai até caber em MaxPixels.
func TestRender_HugePageIsScaledDown(t *testing.T) {
	data := rawPDF("0 0 14400 14400", "BT /F1 400 Tf 100 7000 Td (Grande) Tj ET")
	imgs, err := shared.Render(context.Background(), data, []int{0}, 300)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(imgs[0]))
	if err != nil {
		t.Fatal(err)
	}
	if px := cfg.Width * cfg.Height; px > MaxPixels || px < MaxPixels/2 {
		t.Fatalf("render da página gigante = %dx%d (%d px)", cfg.Width, cfg.Height, px)
	}
}

func TestPagesAndRender_Errors(t *testing.T) {
	ctx := context.Background()
	good, err := os.ReadFile("../../reconciliation/testdata/conciliacao/extratos/extrato_banco_beta_2026-08.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := shared.Pages(ctx, []byte("%PDF-1.4 lixo sem estrutura"), 0); !errors.Is(err, ErrUnreadable) {
		t.Fatalf("PDF corrompido = %v", err)
	}
	if _, err := shared.Render(ctx, []byte("%PDF-1.4 lixo"), []int{0}, 150); !errors.Is(err, ErrUnreadable) {
		t.Fatalf("render de PDF corrompido = %v", err)
	}
	if _, err := shared.Render(ctx, good, []int{1}, 150); !errors.Is(err, ErrPageRange) {
		t.Fatalf("página inexistente = %v", err)
	}
	for _, dpi := range []int{0, MinDPI - 1, MaxDPI + 1} {
		if _, err := shared.Render(ctx, good, []int{0}, dpi); !errors.Is(err, ErrDPI) {
			t.Fatalf("DPI %d = %v", dpi, err)
		}
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := shared.Pages(canceled, good, 0); !errors.Is(err, context.Canceled) {
		t.Fatalf("contexto cancelado = %v", err)
	}
	slow := New(time.Nanosecond)
	t.Cleanup(func() { _ = slow.Close() })
	if _, err := slow.Pages(ctx, good, 0); !errors.Is(err, ErrLimits) {
		t.Fatalf("prazo de 1 ns = %v", err)
	}
	if _, err := slow.Render(ctx, good, []int{0}, 72); !errors.Is(err, ErrLimits) {
		t.Fatalf("render no prazo de 1 ns = %v", err)
	}
}
