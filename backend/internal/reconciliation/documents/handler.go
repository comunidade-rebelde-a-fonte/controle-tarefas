package documents

import (
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	co "controle/internal/company"
	"controle/internal/platform/apperr"
	"controle/internal/platform/httpx"
	"controle/internal/platform/pdftext"
	"controle/internal/platform/validate"
)

// Handler expõe a ingestão da Conciliação [06 §4].
type Handler struct{ svc *Service }

// NewHandler cria o handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Routes registra as rotas.
func (h *Handler) Routes(r chi.Router) {
	r.Post("/reconciliation/documents", h.upload)
	r.Get("/reconciliation/documents", h.list)
	r.Get("/reconciliation/documents/{id}", h.get)
	r.Get("/reconciliation/documents/{id}/file", h.file)
	r.Post("/reconciliation/documents/{id}/retry", h.retry)
	r.Post("/reconciliation/documents/{id}/identify", h.identify)
}

// identify é "que documento é esse?" (TASK-021a): 202, com o job da extração.
func (h *Handler) identify(w http.ResponseWriter, r *http.Request) {
	id, err := documentID(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	var in IdentifyInput
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	out, err := h.svc.Identify(r.Context(), id, in)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusAccepted, out)
}

func documentID(r *http.Request) (string, error) {
	return httpx.PathUUID(r, "id", codeNotFound, "Documento da Conciliação não encontrado.")
}

// maxField limita os campos de texto do formulário (empresa e competência).
const maxField = 256

// readUpload lê o multipart: o arquivo em "file" (até 20 MB) e, opcionais, "company_id" e
// "competence". É o mesmo formato que o workflow do n8n envia (n8n/workflows/ingestao.json).
func readUpload(w http.ResponseWriter, r *http.Request) (UploadInput, error) {
	r.Body = http.MaxBytesReader(w, r.Body, MaxFileSize+(1<<20))
	mr, err := r.MultipartReader()
	if err != nil {
		return UploadInput{}, apperr.BadRequest("INVALID_MULTIPART", "Envie o documento como multipart/form-data no campo \"file\".")
	}
	var in UploadInput
	found := false
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return UploadInput{}, readError(err)
		}
		switch part.FormName() {
		case "file":
			data, err := io.ReadAll(io.LimitReader(part, MaxFileSize+1))
			if err != nil {
				return UploadInput{}, readError(err)
			}
			if len(data) > MaxFileSize {
				return UploadInput{}, FileTooLarge()
			}
			in.FileName, in.Data, found = part.FileName(), data, true
		case "company_id", "competence":
			v, err := io.ReadAll(io.LimitReader(part, maxField+1))
			if err != nil {
				return UploadInput{}, readError(err)
			}
			if len(v) > maxField {
				return UploadInput{}, validate.Field(part.FormName(), "Valor longo demais.")
			}
			if part.FormName() == "company_id" {
				in.CompanyID = string(v)
			} else {
				in.Competence = string(v)
			}
		}
	}
	if !found {
		return UploadInput{}, apperr.BadRequest("FILE_REQUIRED", "Envie o documento no campo \"file\".")
	}
	return in, nil
}

func readError(err error) error {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		return FileTooLarge()
	}
	return apperr.BadRequest("INVALID_MULTIPART", "Não foi possível ler o arquivo enviado.")
}

func (h *Handler) upload(w http.ResponseWriter, r *http.Request) {
	in, err := readUpload(w, r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	out, err := h.svc.Upload(r.Context(), in)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusAccepted, out)
}

func intParam(r *http.Request, name string) (int32, error) {
	v := strings.TrimSpace(r.URL.Query().Get(name))
	if v == "" {
		return 0, nil
	}
	n, err := strconv.ParseInt(v, 10, 32)
	if err != nil {
		return 0, validate.Field(name, "Use um número inteiro.")
	}
	return int32(n), nil
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	limit, err := intParam(r, "limit")
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	offset, err := intParam(r, "offset")
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	q := r.URL.Query()
	f, err := co.ParseFilter(q)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	out, err := h.svc.List(r.Context(), ListInput{
		Status: q.Get("status"), CompanyID: q.Get("company_id"), Kind: q.Get("kind"), Bank: q.Get("bank"),
		Competence: q.Get("competence"), Filter: f, FilterCompetence: q.Get("filter_competence"), Limit: limit, Offset: offset,
	})
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id, err := documentID(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	out, err := h.svc.Get(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// retry reprocessa o documento (202 com o novo job) [06 §4].
func (h *Handler) retry(w http.ResponseWriter, r *http.Request) {
	id, err := documentID(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	out, err := h.svc.Retry(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusAccepted, out)
}

// extensions dá a extensão do nome de download pelo tipo detectado no envio.
var extensions = map[string]string{pdftext.MimePDF: "pdf", MimePNG: "png", MimeJPEG: "jpg", pdftext.MimeText: "txt"}

// file entrega a versão gravada do original, para ver no navegador (inline, como a rota
// /documentos/{id}/pdf da origem) ou baixar (?download=1). O tipo é o detectado no envio, e o
// navegador não deve adivinhar outro (nosniff); o nome enviado só vai no Content-Disposition.
func (h *Handler) file(w http.ResponseWriter, r *http.Request) {
	id, err := documentID(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	doc, obj, err := h.svc.Open(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	defer func() { _ = obj.Body.Close() }()
	disposition := "inline"
	if r.URL.Query().Get("download") == "1" {
		disposition = "attachment"
	}
	name := doc.FileName
	base, _, _ := mime.ParseMediaType(doc.ContentType)
	if ext := extensions[base]; ext != "" && !strings.HasSuffix(strings.ToLower(name), "."+ext) {
		name += "." + ext
	}
	hdr := w.Header()
	hdr.Set("Content-Type", doc.ContentType)
	hdr.Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": name}))
	hdr.Set("X-Content-Type-Options", "nosniff")
	hdr.Set("Cache-Control", "private, no-store")
	if obj.Size > 0 {
		hdr.Set("Content-Length", strconv.FormatInt(obj.Size, 10))
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, obj.Body)
}
