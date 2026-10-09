package inbox

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"controle/internal/platform/apperr"
	"controle/internal/platform/httpx"
	"controle/internal/platform/pdftext"
	"controle/internal/platform/validate"
)

// Handler expõe o Inbox PGDAS-D [06 §3].
type Handler struct{ svc *Service }

// NewHandler cria o handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Routes registra as rotas.
func (h *Handler) Routes(r chi.Router) {
	r.Post("/factor-r/pgdas", h.upload)
	r.Get("/factor-r/pgdas", h.list)
	r.Get("/factor-r/pgdas/{id}", h.get)
	r.Get("/factor-r/pgdas/{id}/file", h.file)
	r.Post("/factor-r/pgdas/{id}/link", h.link)
	r.Post("/factor-r/pgdas/{id}/review", h.review)
	r.Post("/factor-r/pgdas/{id}/create-company", h.createCompany)
}

func documentID(r *http.Request) (string, error) {
	return httpx.PathUUID(r, "id", codeNotFound, msgNotFound)
}

func readError(err error) error {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		return FileTooLarge()
	}
	return apperr.BadRequest("INVALID_MULTIPART", "Não foi possível ler o arquivo enviado.")
}

// readFile lê o campo "file" do multipart sem passar de 10 MB. O conteúdo nunca vai para o log.
func readFile(w http.ResponseWriter, r *http.Request) (name string, data []byte, err error) {
	r.Body = http.MaxBytesReader(w, r.Body, MaxFileSize+(1<<20))
	mr, err := r.MultipartReader()
	if err != nil {
		return "", nil, apperr.BadRequest("INVALID_MULTIPART", "Envie o extrato como multipart/form-data no campo \"file\".")
	}
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", nil, readError(err)
		}
		if part.FormName() != "file" {
			continue
		}
		data, err := io.ReadAll(io.LimitReader(part, MaxFileSize+1))
		if err != nil {
			return "", nil, readError(err)
		}
		if len(data) > MaxFileSize {
			return "", nil, FileTooLarge()
		}
		return part.FileName(), data, nil
	}
	return "", nil, apperr.BadRequest("FILE_REQUIRED", "Envie o extrato no campo \"file\".")
}

func (h *Handler) upload(w http.ResponseWriter, r *http.Request) {
	name, data, err := readFile(w, r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	out, err := h.svc.Upload(r.Context(), name, data)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	status := http.StatusAccepted
	if out.Duplicate {
		status = http.StatusOK
	}
	httpx.WriteJSON(w, status, out)
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
	out, err := h.svc.List(r.Context(), ListInput{Status: q.Get("status"), CompanyID: q.Get("company_id"), Limit: limit, Offset: offset})
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

// file entrega o original como anexo, sem deixar o navegador interpretar o conteúdo.
func (h *Handler) file(w http.ResponseWriter, r *http.Request) {
	id, err := documentID(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	data, mime, err := h.svc.File(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	ext := "txt"
	if mime == pdftext.MimePDF {
		ext = "pdf"
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="extrato-pgdas-%s.%s"`, id, ext))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(data))
}

func (h *Handler) link(w http.ResponseWriter, r *http.Request) {
	id, err := documentID(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	var in LinkInput
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	out, err := h.svc.Link(r.Context(), id, in)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) review(w http.ResponseWriter, r *http.Request) {
	id, err := documentID(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	var in ReviewInput
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	out, err := h.svc.Review(r.Context(), id, in)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) createCompany(w http.ResponseWriter, r *http.Request) {
	id, err := documentID(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	var in CreateCompanyInput
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	out, err := h.svc.CreateCompany(r.Context(), id, in)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, out)
}
