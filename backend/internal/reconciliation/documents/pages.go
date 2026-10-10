package documents

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"controle/internal/platform/apperr"
	"controle/internal/platform/httpx"
	"controle/internal/platform/pdftext"
)

// ViewDPI é o DPI das páginas no visualizador da fila: o mesmo do render que o extrator do
// comprovante manda ao modelo (pipeline.receiptDPI), então o bbox de cada campo cai no lugar.
const ViewDPI = 150

// Viewer mostra o original da fila como imagem (decisão do usuário, D-032): a página do PDF
// renderizada pelo mesmo PDFium do extrator, sem relaxar os cabeçalhos de segurança da API.
type Viewer struct {
	svc *Service
	pdf *pdftext.Extractor
}

// NewViewer cria o visualizador.
func NewViewer(svc *Service, pdf *pdftext.Extractor) *Viewer { return &Viewer{svc: svc, pdf: pdf} }

// Routes registra as rotas.
func (v *Viewer) Routes(r chi.Router) {
	r.Get("/reconciliation/documents/{id}/pages", v.info)
	r.Get("/reconciliation/documents/{id}/pages/{page}", v.page)
}

// PagesInfo diz como mostrar o original: as páginas do PDF (renderizadas no DPI dado) ou, sem
// páginas, o próprio arquivo (imagem ou texto) pela rota do original.
type PagesInfo struct {
	ContentType string `json:"content_type"`
	Pages       int    `json:"pages"`
	DPI         int    `json:"dpi"`
}

func isPDF(contentType string) bool { return strings.HasPrefix(contentType, pdftext.MimePDF) }

// read abre e lê o original (até o limite do envio).
func (v *Viewer) read(ctx context.Context, id string) (Document, []byte, error) {
	doc, obj, err := v.svc.Open(ctx, id)
	if err != nil {
		return Document{}, nil, err
	}
	defer func() { _ = obj.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(obj.Body, MaxFileSize+1))
	if err != nil {
		return Document{}, nil, storageUnavailable(err)
	}
	return doc, data, nil
}

func (v *Viewer) info(w http.ResponseWriter, r *http.Request) {
	id, err := documentID(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	doc, data, err := v.read(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	out := PagesInfo{ContentType: doc.ContentType, DPI: ViewDPI}
	if isPDF(doc.ContentType) {
		if out.Pages, err = v.pdf.PageCount(r.Context(), data); err != nil {
			httpx.WriteError(w, r, unreadable(err))
			return
		}
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func unreadable(err error) error {
	return apperr.Unprocessable("DOCUMENT_UNREADABLE", "Não foi possível ler as páginas do PDF.").Wrap(err)
}

func (v *Viewer) page(w http.ResponseWriter, r *http.Request) {
	id, err := documentID(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	n, err := strconv.Atoi(chi.URLParam(r, "page"))
	if err != nil || n < 1 {
		httpx.WriteError(w, r, apperr.NotFound("PAGE_NOT_FOUND", "Página não encontrada."))
		return
	}
	doc, data, err := v.read(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if !isPDF(doc.ContentType) {
		httpx.WriteError(w, r, apperr.NotFound("PAGE_NOT_FOUND", "O original não é PDF: abra o arquivo."))
		return
	}
	pngs, err := v.pdf.Render(r.Context(), data, []int{n - 1}, ViewDPI)
	if errors.Is(err, pdftext.ErrPageRange) {
		httpx.WriteError(w, r, apperr.NotFound("PAGE_NOT_FOUND", "Página não encontrada."))
		return
	}
	if err != nil {
		httpx.WriteError(w, r, unreadable(err))
		return
	}
	// O original guardado não muda (versão do envio): a página pode ficar no cache do navegador.
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "private, max-age=3600")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, bytes.NewReader(pngs[0]))
}
