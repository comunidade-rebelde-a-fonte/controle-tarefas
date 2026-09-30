package importer

import (
	"errors"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"

	"controle/internal/platform/apperr"
	"controle/internal/platform/httpx"
)

// MaxUploadSize é o limite da planilha enviada para a prévia.
const MaxUploadSize = 10 << 20

// Handler expõe /imports/companies [PRD §41.12].
type Handler struct{ svc *Service }

// NewHandler cria o handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Routes registra as rotas.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/imports/companies/template", h.template)
	r.Post("/imports/companies/preview", h.preview)
	r.Post("/imports/companies/commit", h.commit)
}

func (h *Handler) template(w http.ResponseWriter, r *http.Request) {
	data, err := Template()
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", `attachment; filename="`+TemplateFileName+`"`)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func tooLarge() error {
	return apperr.BadRequest("FILE_TOO_LARGE", "A planilha excede o limite de 10 MB.")
}

// readFile lê o campo "file" do multipart sem registrar o conteúdo em log [AGENTS §6.4].
func readFile(w http.ResponseWriter, r *http.Request) (string, []byte, error) {
	r.Body = http.MaxBytesReader(w, r.Body, MaxUploadSize+(1<<20))
	mr, err := r.MultipartReader()
	if err != nil {
		return "", nil, apperr.BadRequest("INVALID_MULTIPART", "Envie a planilha como multipart/form-data no campo \"file\".")
	}
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			var mbe *http.MaxBytesError
			if errors.As(err, &mbe) {
				return "", nil, tooLarge()
			}
			return "", nil, apperr.BadRequest("INVALID_MULTIPART", "Não foi possível ler o arquivo enviado.")
		}
		if part.FormName() != "file" {
			continue
		}
		data, err := io.ReadAll(io.LimitReader(part, MaxUploadSize+1))
		if err != nil {
			var mbe *http.MaxBytesError
			if errors.As(err, &mbe) {
				return "", nil, tooLarge()
			}
			return "", nil, apperr.BadRequest("INVALID_MULTIPART", "Não foi possível ler o arquivo enviado.")
		}
		if len(data) > MaxUploadSize {
			return "", nil, tooLarge()
		}
		return part.FileName(), data, nil
	}
	return "", nil, apperr.BadRequest("FILE_REQUIRED", "Envie a planilha no campo \"file\".")
}

func (h *Handler) preview(w http.ResponseWriter, r *http.Request) {
	name, data, err := readFile(w, r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	out, err := h.svc.Preview(r.Context(), name, data)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) commit(w http.ResponseWriter, r *http.Request) {
	var in CommitInput
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	out, err := h.svc.Commit(r.Context(), in)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}
