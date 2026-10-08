// Package pdftext extrai o texto de documentos enviados (PDF com camada de texto ou TXT), como
// fator_r/parsing/texto.py e isolado.py do Fator R (commit 9dd7a74).
//
// A origem lia o PDF com o pdfplumber (pdfminer). Aqui o PDF é lido pelo PDFium compilado para
// WebAssembly (go-pdfium + wazero, sem cgo) e o texto é montado pelo mesmo algoritmo do
// extract_text() do pdfplumber 0.11.10 (agrupamento de caracteres em palavras e linhas por
// tolerância de 3 pt). Nas fixtures da origem o texto sai byte a byte igual (testdata). Os
// limites da origem contra PDF malicioso continuam: 30 s por extração, 1 GiB de memória, duas
// extrações ao mesmo tempo e 2 milhões de caracteres; o PDFium roda sem acesso ao disco.
package pdftext

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/klippa-app/go-pdfium"
	"github.com/klippa-app/go-pdfium/references"
	"github.com/klippa-app/go-pdfium/requests"
	"github.com/klippa-app/go-pdfium/webassembly"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"

	"controle/internal/platform/pytext"
)

// Tipos aceitos, validados pelo conteúdo (nunca pela extensão).
const (
	MimePDF  = "application/pdf"
	MimeText = "text/plain"
)

// Motivos de documento sem texto utilizável (MotivoTexto da origem).
const (
	ReasonNoTextLayer = "sem_camada_texto"
	ReasonUnreadable  = "pdf_ilegivel"
	ReasonLimits      = "extracao_excedeu_limites"
)

// Limites da origem (texto.py e isolado.py).
const (
	MaxChars         = 2_000_000
	Timeout          = 30 * time.Second
	MemoryLimitBytes = 1 << 30
	MaxConcurrent    = 2
)

// Tolerâncias do extract_text() do pdfplumber (x_tolerance e y_tolerance padrão).
const (
	xTolerance = 3.0
	yTolerance = 3.0
)

// ErrUnsupported é o arquivo que não é PDF nem texto (detectar_mime da origem).
var ErrUnsupported = errors.New("envie um PDF ou TXT do extrato do PGDAS-D")

// DetectMIME identifica o tipo pelo conteúdo, como detectar_mime da origem: PDF pela assinatura;
// texto se não tem byte nulo e é UTF-8 válido ou, em latin-1, tem ao menos 95% de caracteres
// imprimíveis (ou quebra de linha e tabulação).
func DetectMIME(data []byte) (string, error) {
	if bytes.HasPrefix(data, []byte("%PDF-")) {
		return MimePDF, nil
	}
	if len(data) == 0 || bytes.IndexByte(data, 0) >= 0 {
		return "", ErrUnsupported
	}
	if utf8.Valid(data) {
		return MimeText, nil
	}
	printable := 0
	for _, b := range data {
		if latin1Printable(b) || b == '\r' || b == '\n' || b == '\t' {
			printable++
		}
	}
	// imprimiveis / len(texto) < 0.95, sem ponto flutuante.
	if printable*100 < len(data)*95 {
		return "", ErrUnsupported
	}
	return MimeText, nil
}

// latin1Printable segue str.isprintable() do Python para os caracteres do latin-1: fora os de
// controle (C0, DEL e C1), o espaço sem quebra (U+00A0) e o hífen discricionário (U+00AD).
func latin1Printable(b byte) bool {
	switch {
	case b < 0x20, b >= 0x7F && b <= 0xA0, b == 0xAD:
		return false
	}
	return true
}

// DecodeText decodifica o TXT como a origem: UTF-8 e, se não for válido, latin-1.
func DecodeText(data []byte) string {
	if utf8.Valid(data) {
		return string(data)
	}
	rs := make([]rune, len(data))
	for i, b := range data {
		rs[i] = rune(b)
	}
	return string(rs)
}

// Extracted é o texto extraído ou, sem texto utilizável, o motivo.
type Extracted struct {
	Text   string
	Reason string
}

// Extractor lê o texto de documentos com os limites da origem. O PDFium (cerca de 6 MB de
// WebAssembly) é compilado na primeira extração de PDF; cada documento roda numa instância
// nova, descartada no fim.
type Extractor struct {
	timeout time.Duration
	once    sync.Once
	pool    pdfium.Pool
	initErr error
}

// New cria o extrator. timeout zero = Timeout (30 s).
func New(timeout time.Duration) *Extractor {
	if timeout <= 0 {
		timeout = Timeout
	}
	return &Extractor{timeout: timeout}
}

func (e *Extractor) init() error {
	e.once.Do(func() {
		rc := wazero.NewRuntimeConfig().
			// O PDFium empacotado usa exceções do WebAssembly (setjmp/longjmp).
			WithCoreFeatures(api.CoreFeaturesV2 | experimental.CoreFeaturesExceptionHandling).
			// Permite interromper a extração em andamento no fim do prazo.
			WithCloseOnContextDone(true).
			WithMemoryLimitPages(MemoryLimitBytes / 65536)
		e.pool, e.initErr = webassembly.Init(webassembly.Config{
			MinIdle: 0, MaxIdle: MaxConcurrent, MaxTotal: MaxConcurrent,
			RuntimeConfig: rc,
			// Sem diretório montado: o documento entra só como bytes (o padrão monta o disco).
			FSConfig: wazero.NewFSConfig(),
			Stdout:   io.Discard, Stderr: io.Discard,
		})
		if e.initErr != nil {
			e.initErr = fmt.Errorf("pdftext: iniciar o PDFium: %w", e.initErr)
		}
	})
	return e.initErr
}

// Close libera o PDFium.
func (e *Extractor) Close() error {
	if e.pool == nil {
		return nil
	}
	return e.pool.Close()
}

// Extract devolve o texto do documento (extrair_texto + os limites de extrair_e_parsear_isolado).
// Erro só para falha do próprio extrator ou contexto cancelado (o job tenta de novo); documento
// ruim ou pesado demais vira motivo, como na origem.
func (e *Extractor) Extract(ctx context.Context, data []byte, mime string) (Extracted, error) {
	switch mime {
	case MimeText:
		return Extracted{Text: truncate(DecodeText(data), MaxChars)}, nil
	case MimePDF:
	default:
		return Extracted{}, fmt.Errorf("pdftext: tipo %q não suportado", mime)
	}
	if err := e.init(); err != nil {
		return Extracted{}, err
	}
	// O pool tem MaxConcurrent instâncias: pegar uma é o semáforo da origem.
	inst, err := e.pool.GetInstanceWithContext(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return Extracted{}, ctx.Err()
		}
		return Extracted{}, fmt.Errorf("pdftext: instância do PDFium: %w", err)
	}
	type outcome struct {
		text string
		err  error
	}
	done := make(chan outcome, 1)
	go func() {
		defer func() {
			if p := recover(); p != nil {
				done <- outcome{err: fmt.Errorf("pdftext: pânico na extração: %v", p)}
			}
		}()
		text, err := pdfText(inst, data)
		done <- outcome{text, err}
	}()
	timer := time.NewTimer(e.timeout)
	defer timer.Stop()
	select {
	case out := <-done:
		_ = inst.Close() // instância descartada; o erro de fechar não muda o resultado
		if out.err != nil {
			// O pdfplumber e o pypdf falharam: PDF ilegível.
			return Extracted{Reason: ReasonUnreadable}, nil
		}
		if strings.TrimFunc(out.text, pytext.IsSpace) == "" {
			return Extracted{Reason: ReasonNoTextLayer}, nil
		}
		return Extracted{Text: truncate(out.text, MaxChars)}, nil
	case <-timer.C:
		_ = inst.Kill() // interrompe o WebAssembly em execução
		return Extracted{Reason: ReasonLimits}, nil
	case <-ctx.Done():
		_ = inst.Kill()
		return Extracted{}, ctx.Err()
	}
}

func truncate(s string, n int) string {
	if len(s) <= n || utf8.RuneCountInString(s) <= n {
		return s
	}
	i := 0
	for k := range s {
		if i == n {
			return s[:k]
		}
		i++
	}
	return s
}

// char é um caractere da página com a caixa (coordenadas do pdfplumber: top medido do alto).
type char struct {
	text        string
	x0, x1, top float64
}

// pdfText lê todas as páginas e junta o texto de cada uma com "\n", como a origem.
func pdfText(inst pdfium.Pdfium, data []byte) (string, error) {
	doc, err := inst.OpenDocument(&requests.OpenDocument{File: &data})
	if err != nil {
		return "", err
	}
	defer func() {
		_, _ = inst.FPDF_CloseDocument(&requests.FPDF_CloseDocument{Document: doc.Document})
	}()
	count, err := inst.FPDF_GetPageCount(&requests.FPDF_GetPageCount{Document: doc.Document})
	if err != nil {
		return "", err
	}
	pages := make([]string, 0, count.PageCount)
	for p := 0; p < count.PageCount; p++ {
		chars, err := pageChars(inst, doc.Document, p)
		if err != nil {
			return "", err
		}
		pages = append(pages, extractText(chars))
	}
	return strings.Join(pages, "\n"), nil
}

// pageChars lê os caracteres da página: os gerados pelo PDFium (espaços e quebras sintéticos),
// CR, LF e os sem Unicode ficam de fora, como no pdfminer.
func pageChars(inst pdfium.Pdfium, doc references.FPDF_DOCUMENT, index int) ([]char, error) {
	page, err := inst.FPDF_LoadPage(&requests.FPDF_LoadPage{Document: doc, Index: index})
	if err != nil {
		return nil, err
	}
	defer func() { _, _ = inst.FPDF_ClosePage(&requests.FPDF_ClosePage{Page: page.Page}) }()
	ref := requests.Page{ByReference: &page.Page}
	h, err := inst.FPDF_GetPageHeightF(&requests.FPDF_GetPageHeightF{Page: ref})
	if err != nil {
		return nil, err
	}
	tp, err := inst.FPDFText_LoadPage(&requests.FPDFText_LoadPage{Page: ref})
	if err != nil {
		return nil, err
	}
	defer func() { _, _ = inst.FPDFText_ClosePage(&requests.FPDFText_ClosePage{TextPage: tp.TextPage}) }()
	n, err := inst.FPDFText_CountChars(&requests.FPDFText_CountChars{TextPage: tp.TextPage})
	if err != nil {
		return nil, err
	}
	height := float64(h.PageHeight)
	unicodeAt := func(i int) rune {
		u, err := inst.FPDFText_GetUnicode(&requests.FPDFText_GetUnicode{TextPage: tp.TextPage, Index: i})
		if err != nil || u == nil || u.Unicode > unicode.MaxRune {
			return 0
		}
		return rune(u.Unicode)
	}
	var out []char
	for c := 0; c < n.Count; c++ {
		gen, _ := inst.FPDFText_IsGenerated(&requests.FPDFText_IsGenerated{TextPage: tp.TextPage, Index: c})
		if gen != nil && gen.IsGenerated {
			continue
		}
		r := unicodeAt(c)
		box, err := inst.FPDFText_GetLooseCharBox(&requests.FPDFText_GetLooseCharBox{TextPage: tp.TextPage, Index: c})
		if err != nil || r == 0 || r == '\r' || r == '\n' {
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
		out = append(out, char{text: string(r), x0: float64(box.Rect.Left), x1: float64(box.Rect.Right), top: height - float64(box.Rect.Top)})
	}
	return out, nil
}

// extractText é o extract_text() do pdfplumber sem layout: caracteres agrupados em linhas pelo
// topo (cluster com tolerância de 3 pt), ordenados por x em cada linha, quebrados em palavras
// por espaço ou distância maior que 3 pt, e as palavras de novo agrupadas em linhas pelo topo.
func extractText(chars []char) string {
	if len(chars) == 0 {
		return ""
	}
	tops := make([]float64, len(chars))
	for i, c := range chars {
		tops[i] = c.top
	}
	idx := clusterIndex(tops, yTolerance)
	sort.SliceStable(chars, func(a, b int) bool { return idx[chars[a].top] < idx[chars[b].top] })
	type word struct {
		text string
		top  float64
	}
	var words []word
	for i := 0; i < len(chars); {
		j := i
		for j < len(chars) && idx[chars[j].top] == idx[chars[i].top] {
			j++
		}
		line := append([]char(nil), chars[i:j]...)
		sort.SliceStable(line, func(a, b int) bool {
			if line[a].x0 != line[b].x0 {
				return line[a].x0 < line[b].x0
			}
			return line[a].x1 < line[b].x1
		})
		var cur []char
		flush := func() {
			if len(cur) == 0 {
				return
			}
			var sb strings.Builder
			top := cur[0].top
			for _, c := range cur {
				sb.WriteString(c.text)
				top = min(top, c.top)
			}
			words = append(words, word{sb.String(), top})
			cur = nil
		}
		for _, c := range line {
			switch {
			case strings.TrimFunc(c.text, unicode.IsSpace) == "":
				flush()
			case len(cur) > 0 && (c.x0 < cur[len(cur)-1].x0 || c.x0 > cur[len(cur)-1].x1+xTolerance || c.top > cur[len(cur)-1].top+yTolerance):
				flush()
				cur = []char{c}
			default:
				cur = append(cur, c)
			}
		}
		flush()
		i = j
	}
	wtops := make([]float64, len(words))
	for k, w := range words {
		wtops[k] = w.top
	}
	widx := clusterIndex(wtops, yTolerance)
	sort.SliceStable(words, func(a, b int) bool { return widx[words[a].top] < widx[words[b].top] })
	var lines []string
	for k := 0; k < len(words); {
		m := k
		var parts []string
		for m < len(words) && widx[words[m].top] == widx[words[k].top] {
			parts = append(parts, words[m].text)
			m++
		}
		lines = append(lines, strings.Join(parts, " "))
		k = m
	}
	return strings.Join(lines, "\n")
}

// clusterIndex agrupa valores encadeados (cluster_list do pdfplumber): valores ordenados ficam
// no mesmo grupo enquanto cada um não passa do anterior mais a tolerância.
func clusterIndex(values []float64, tol float64) map[float64]int {
	uniq := make(map[float64]struct{}, len(values))
	for _, v := range values {
		uniq[v] = struct{}{}
	}
	xs := make([]float64, 0, len(uniq))
	for v := range uniq {
		xs = append(xs, v)
	}
	sort.Float64s(xs)
	out := make(map[float64]int, len(xs))
	g := 0
	for i, x := range xs {
		if i > 0 && x > xs[i-1]+tol {
			g++
		}
		out[x] = g
	}
	return out
}
