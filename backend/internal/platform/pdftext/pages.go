package pdftext

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"math"
	"strings"
	"time"
	"unicode"
	"unicode/utf16"

	"github.com/klippa-app/go-pdfium"
	"github.com/klippa-app/go-pdfium/references"
	"github.com/klippa-app/go-pdfium/requests"
)

// Ferramentas do Conciliador (conciliador-contabil, commit 2bc6e26, app/agentes/ferramentas.py):
// extrair_texto_pdf devolve o texto de cada página pelo page.get_text() do PyMuPDF, e
// renderizar_pagina devolve a página como PNG no DPI pedido (page.get_pixmap). Aqui as duas leem o
// PDF pelo mesmo PDFium em WebAssembly do extrator do PGDAS-D, com os mesmos limites.
//
// O texto segue as regras de linha do MuPDF, medidas contra o PyMuPDF 1.25.4 (testdata/
// mupdf_gen.py), sem copiar código do MuPDF: os caracteres ficam na ordem do conteúdo; um
// caractere continua a linha se está na mesma direção, a menos de 0,8 × o tamanho da fonte da
// linha de base e a menos de 0,8 × o tamanho do fim do caractere anterior; um intervalo de
// 0,15 × o tamanho em diante, para a frente, vira um espaço. O resto começa linha nova. Cada linha
// termina com "\n". O tamanho é o do caractere novo (fonte × escala da matriz).
//
// Diferença conhecida: o PDFium descarta o texto repetido no mesmo lugar ("negrito falso"); o
// MuPDF o repete numa linha nova.

// Erros das ferramentas do Conciliador.
var (
	// ErrUnreadable é o PDF que o PDFium não consegue abrir ou ler.
	ErrUnreadable = errors.New("pdftext: PDF ilegível")
	// ErrLimits é a leitura que passou do prazo (Timeout).
	ErrLimits = errors.New("pdftext: a leitura do PDF excedeu os limites")
	// ErrPageRange é a página pedida fora do documento.
	ErrPageRange = errors.New("pdftext: página fora do documento")
	// ErrDPI é o DPI fora da faixa permitida.
	ErrDPI = errors.New("pdftext: DPI fora da faixa permitida")
)

// Faixa do DPI do render (a origem usa 72, 150 e 300) e o limite de pixels por página: página
// enorme é renderizada num DPI menor, para caber no limite de memória.
const (
	MinDPI       = 36
	MaxDPI       = 600
	MaxPixels    = 25_000_000
	spaceRatio   = 0.15
	lineRatio    = 0.8
	sameDirLimit = 0.999
)

// run executa fn numa instância nova do PDFium, com o prazo (ErrLimits) e o cancelamento de ctx.
// Falha ou pânico de fn é PDF ilegível (ErrUnreadable), salvo ErrPageRange e ErrDPI.
func run[T any](ctx context.Context, e *Extractor, fn func(pdfium.Pdfium) (T, error)) (T, error) {
	var zero T
	if err := e.init(); err != nil {
		return zero, err
	}
	// O pool tem MaxConcurrent instâncias: pegar uma é o semáforo da origem.
	inst, err := e.pool.GetInstanceWithContext(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return zero, ctx.Err()
		}
		return zero, fmt.Errorf("pdftext: instância do PDFium: %w", err)
	}
	type outcome struct {
		v   T
		err error
	}
	done := make(chan outcome, 1)
	go func() {
		defer func() {
			if p := recover(); p != nil {
				done <- outcome{err: fmt.Errorf("pânico na leitura: %v", p)}
			}
		}()
		v, err := fn(inst)
		done <- outcome{v, err}
	}()
	timer := time.NewTimer(e.timeout)
	defer timer.Stop()
	select {
	case out := <-done:
		_ = inst.Close() // instância descartada; o erro de fechar não muda o resultado
		switch {
		case out.err == nil:
			return out.v, nil
		case errors.Is(out.err, ErrPageRange), errors.Is(out.err, ErrDPI):
			return zero, out.err
		default:
			return zero, fmt.Errorf("%w: %w", ErrUnreadable, out.err)
		}
	case <-timer.C:
		_ = inst.Kill() // interrompe o WebAssembly em execução
		return zero, ErrLimits
	case <-ctx.Done():
		_ = inst.Kill()
		return zero, ctx.Err()
	}
}

// withDocument abre o PDF na instância e o fecha no fim.
func withDocument[T any](inst pdfium.Pdfium, data []byte, fn func(doc references.FPDF_DOCUMENT, pages int) (T, error)) (T, error) {
	var zero T
	doc, err := inst.OpenDocument(&requests.OpenDocument{File: &data})
	if err != nil {
		return zero, err
	}
	defer func() {
		_, _ = inst.FPDF_CloseDocument(&requests.FPDF_CloseDocument{Document: doc.Document})
	}()
	count, err := inst.FPDF_GetPageCount(&requests.FPDF_GetPageCount{Document: doc.Document})
	if err != nil {
		return zero, err
	}
	return fn(doc.Document, count.PageCount)
}

// Document é o texto do PDF página a página (extrair_texto_pdf).
type Document struct {
	// PageCount é o total de páginas do documento.
	PageCount int
	// Pages é o texto das primeiras páginas pedidas, no formato do get_text() do PyMuPDF.
	Pages []string
}

// Pages lê o texto das primeiras maxPages páginas (0 = todas), como extrair_texto_pdf da origem.
func (e *Extractor) Pages(ctx context.Context, data []byte, maxPages int) (Document, error) {
	return run(ctx, e, func(inst pdfium.Pdfium) (Document, error) {
		return withDocument(inst, data, func(doc references.FPDF_DOCUMENT, count int) (Document, error) {
			n := count
			if maxPages > 0 && maxPages < n {
				n = maxPages
			}
			out := Document{PageCount: count, Pages: make([]string, 0, n)}
			for p := 0; p < n; p++ {
				text, err := mupdfPageText(inst, doc, p)
				if err != nil {
					return Document{}, err
				}
				out.Pages = append(out.Pages, text)
			}
			return out, nil
		})
	})
}

// Render devolve as páginas pedidas (base 0) como PNG em dpi, sobre fundo branco e sem canal
// alfa, como page.get_pixmap(matrix=zoom) da origem. Página cujo render passaria de MaxPixels
// sai num DPI menor, na mesma proporção.
func (e *Extractor) Render(ctx context.Context, data []byte, pages []int, dpi int) ([][]byte, error) {
	if dpi < MinDPI || dpi > MaxDPI {
		return nil, fmt.Errorf("%w: %d (use de %d a %d)", ErrDPI, dpi, MinDPI, MaxDPI)
	}
	return run(ctx, e, func(inst pdfium.Pdfium) ([][]byte, error) {
		return withDocument(inst, data, func(doc references.FPDF_DOCUMENT, count int) ([][]byte, error) {
			out := make([][]byte, 0, len(pages))
			for _, p := range pages {
				if p < 0 || p >= count {
					return nil, fmt.Errorf("%w: %d de %d", ErrPageRange, p+1, count)
				}
				img, err := renderPNG(inst, doc, p, dpi)
				if err != nil {
					return nil, err
				}
				out = append(out, img)
			}
			return out, nil
		})
	})
}

func renderPNG(inst pdfium.Pdfium, doc references.FPDF_DOCUMENT, index, dpi int) ([]byte, error) {
	page := requests.Page{ByIndex: &requests.PageByIndex{Document: doc, Index: index}}
	size, err := inst.GetPageSize(&requests.GetPageSize{Page: page})
	if err != nil {
		return nil, err
	}
	scale := float64(dpi) / 72
	if px := size.Width * scale * size.Height * scale; px > MaxPixels {
		dpi = max(1, int(float64(dpi)*math.Sqrt(MaxPixels/px)))
	}
	res, err := inst.RenderPageInDPI(&requests.RenderPageInDPI{Page: page, DPI: dpi})
	if err != nil {
		return nil, err
	}
	defer res.Cleanup()
	src := res.Result.RenderedImage
	if src == nil {
		return nil, errors.New("render sem imagem")
	}
	// Sobre fundo branco (a página com transparência vem sobre preto transparente).
	b := src.Bounds()
	dst := image.NewRGBA(b)
	draw.Draw(dst, b, image.White, image.Point{}, draw.Src)
	draw.Draw(dst, b, src, b.Min, draw.Over)
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if err := enc.Encode(&buf, dst); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// mchar é um caractere da página com o que a regra de linha usa (espaço do PDF, y para cima).
type mchar struct {
	r            rune
	x, y         float64 // origem
	dirX, dirY   float64 // direção da escrita (normalizada)
	size         float64 // tamanho efetivo: fonte × escala da matriz
	adv          float64 // avanço ao longo da direção
	matrix       [6]float32
	l, b, rt, tp float64 // caixa larga (para o recorte pela página)
}

// mupdfPageText lê os caracteres da página e monta o texto pelas regras de linha do MuPDF.
func mupdfPageText(inst pdfium.Pdfium, doc references.FPDF_DOCUMENT, index int) (string, error) {
	page, err := inst.FPDF_LoadPage(&requests.FPDF_LoadPage{Document: doc, Index: index})
	if err != nil {
		return "", err
	}
	defer func() { _, _ = inst.FPDF_ClosePage(&requests.FPDF_ClosePage{Page: page.Page}) }()
	ref := requests.Page{ByReference: &page.Page}
	bbox, err := inst.FPDF_GetPageBoundingBox(&requests.FPDF_GetPageBoundingBox{Page: ref})
	if err != nil {
		return "", err
	}
	tp, err := inst.FPDFText_LoadPage(&requests.FPDFText_LoadPage{Page: ref})
	if err != nil {
		return "", err
	}
	defer func() { _, _ = inst.FPDFText_ClosePage(&requests.FPDFText_ClosePage{TextPage: tp.TextPage}) }()
	n, err := inst.FPDFText_CountChars(&requests.FPDFText_CountChars{TextPage: tp.TextPage})
	if err != nil {
		return "", err
	}
	unicodeAt := func(i int) rune {
		u, err := inst.FPDFText_GetUnicode(&requests.FPDFText_GetUnicode{TextPage: tp.TextPage, Index: i})
		if err != nil || u == nil || u.Unicode > unicode.MaxRune {
			return 0
		}
		return rune(u.Unicode)
	}
	chars := make([]mchar, 0, n.Count)
	for c := 0; c < n.Count; c++ {
		gen, _ := inst.FPDFText_IsGenerated(&requests.FPDFText_IsGenerated{TextPage: tp.TextPage, Index: c})
		if gen != nil && gen.IsGenerated {
			continue // espaços e quebras sintéticos do PDFium; o MuPDF põe os seus pela regra
		}
		idx := c
		r := unicodeAt(c)
		if r == 0 {
			continue
		}
		// Fora do plano básico o PDFium devolve o par substituto do UTF-16 em dois caracteres.
		if utf16.IsSurrogate(r) && c+1 < n.Count {
			if pair := utf16.DecodeRune(r, unicodeAt(c+1)); pair != unicode.ReplacementChar {
				r = pair
				c++
			}
		}
		if utf16.IsSurrogate(r) {
			r = unicode.ReplacementChar
		}
		ch, err := charInfo(inst, tp.TextPage, idx, r)
		if err != nil {
			return "", err
		}
		chars = append(chars, ch)
	}
	fillDiagonalAdvances(chars)
	page4 := [4]float64{float64(bbox.Rect.Left), float64(bbox.Rect.Bottom), float64(bbox.Rect.Right), float64(bbox.Rect.Top)}
	return layoutMuPDF(chars, page4), nil
}

func charInfo(inst pdfium.Pdfium, tp references.FPDF_TEXTPAGE, i int, r rune) (mchar, error) {
	o, err := inst.FPDFText_GetCharOrigin(&requests.FPDFText_GetCharOrigin{TextPage: tp, Index: i})
	if err != nil {
		return mchar{}, err
	}
	m, err := inst.FPDFText_GetMatrix(&requests.FPDFText_GetMatrix{TextPage: tp, Index: i})
	if err != nil {
		return mchar{}, err
	}
	fs, err := inst.FPDFText_GetFontSize(&requests.FPDFText_GetFontSize{TextPage: tp, Index: i})
	if err != nil {
		return mchar{}, err
	}
	box, err := inst.FPDFText_GetLooseCharBox(&requests.FPDFText_GetLooseCharBox{TextPage: tp, Index: i})
	if err != nil {
		return mchar{}, err
	}
	a, b, c, d := float64(m.Matrix.A), float64(m.Matrix.B), float64(m.Matrix.C), float64(m.Matrix.D)
	ch := mchar{
		r: r, x: o.X, y: o.Y,
		matrix: [6]float32{m.Matrix.A, m.Matrix.B, m.Matrix.C, m.Matrix.D, m.Matrix.E, m.Matrix.F},
		l:      float64(box.Rect.Left), b: float64(box.Rect.Bottom), rt: float64(box.Rect.Right), tp: float64(box.Rect.Top),
	}
	// Direção: o eixo x do texto transformado pela matriz do caractere.
	if n := math.Hypot(a, b); n > 1e-9 {
		ch.dirX, ch.dirY = a/n, b/n
	} else {
		ch.dirX, ch.dirY = 1, 0
	}
	// Tamanho efetivo: a fonte (Tf) vezes a escala da matriz, que já traz o Tz.
	ch.size = fs.FontSize * math.Sqrt(math.Abs(a*d-b*c))
	if ch.size <= 0 || math.IsNaN(ch.size) {
		ch.size = fs.FontSize
	}
	// Avanço: a caixa larga do PDFium vai da origem à origem mais o avanço do glifo, nos textos
	// alinhados aos eixos. Na diagonal, ele vem da origem do próximo caractere (fillDiagonalAdvances).
	switch {
	case math.Abs(ch.dirX) >= sameDirLimit:
		ch.adv = ch.rt - ch.l
	case math.Abs(ch.dirY) >= sameDirLimit:
		ch.adv = ch.tp - ch.b
	default:
		ch.adv = math.NaN()
	}
	return ch, nil
}

// fillDiagonalAdvances completa o avanço do texto inclinado: a distância até a origem do próximo
// caractere do mesmo objeto de texto (mesma matriz), ou meio tamanho no último.
func fillDiagonalAdvances(chars []mchar) {
	for i := range chars {
		if !math.IsNaN(chars[i].adv) {
			continue
		}
		c := &chars[i]
		c.adv = c.size / 2
		if i+1 < len(chars) && chars[i+1].matrix == c.matrix {
			n := chars[i+1]
			if d := (n.x-c.x)*c.dirX + (n.y-c.y)*c.dirY; d > 0 {
				c.adv = d
			}
		}
	}
}

// layoutMuPDF aplica as regras de linha e espaço do MuPDF e devolve o texto da página.
func layoutMuPDF(chars []mchar, page [4]float64) string {
	var sb strings.Builder
	var line []rune
	flush := func() {
		if len(line) == 0 {
			return
		}
		sb.WriteString(string(line))
		if line[len(line)-1] != '\n' {
			sb.WriteByte('\n')
		}
		line = line[:0]
	}
	var penX, penY, dirX, dirY float64
	var prev *mchar
	for i := range chars {
		c := &chars[i]
		// Recorte pela página (TEXT_MEDIABOX_CLIP): caractere sem interseção com a página sai.
		if c.l >= page[2] || c.b >= page[3] || c.rt <= page[0] || c.tp <= page[1] {
			continue
		}
		newLine, spaces := true, 0
		if prev != nil && c.dirX*dirX+c.dirY*dirY >= sameDirLimit {
			dx, dy := c.x-penX, c.y-penY
			spacing := dx*c.dirX + dy*c.dirY
			base := -dx*c.dirY + dy*c.dirX
			if math.Abs(base) < lineRatio*c.size {
				switch {
				case math.Abs(spacing) < spaceRatio*c.size:
					newLine = false
				case droppedSpaces(prev, c, spacing) > 0:
					newLine, spaces = false, droppedSpaces(prev, c, spacing)
				case spacing > 0 && spacing < lineRatio*c.size:
					newLine = false
					if prev.r != ' ' {
						spaces = 1
					}
				case spacing < 0 && spacing > -lineRatio*c.size:
					newLine = false
				}
			}
		}
		if newLine {
			flush()
			dirX, dirY = c.dirX, c.dirY
		}
		for range spaces {
			line = append(line, ' ')
		}
		line = append(line, c.r)
		penX, penY = c.x+c.adv*c.dirX, c.y+c.adv*c.dirY
		prev = c
	}
	flush()
	return sb.String()
}

// droppedSpaces devolve os espaços repetidos que o PDFium descartou: ele guarda só o primeiro
// de uma sequência de espaços da mesma string, e o MuPDF guarda todos. Depois de um espaço, um
// intervalo que é múltiplo exato do avanço desse espaço, na mesma string (mesma matriz), é
// essa sequência.
func droppedSpaces(prev, c *mchar, spacing float64) int {
	if prev.r != ' ' || prev.matrix != c.matrix || prev.adv <= 0 || spacing < prev.adv/2 {
		return 0
	}
	n := math.Round(spacing / prev.adv)
	if math.Abs(spacing-n*prev.adv) > 0.02*prev.adv+0.01 {
		return 0
	}
	return int(n)
}

// PageCount é o total de páginas do PDF, sem ler o texto (o visualizador da fila, TASK-021a).
func (e *Extractor) PageCount(ctx context.Context, data []byte) (int, error) {
	return run(ctx, e, func(inst pdfium.Pdfium) (int, error) {
		return withDocument(inst, data, func(_ references.FPDF_DOCUMENT, count int) (int, error) { return count, nil })
	})
}
