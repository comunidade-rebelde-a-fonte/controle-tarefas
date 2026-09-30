package credential

import (
	"errors"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"controle/internal/platform/apperr"
	"controle/internal/platform/httpx"
)

// Handler expõe /credential-controls e /companies/{id}/credential-controls [PRD §41.11].
type Handler struct{ svc *Service }

// NewHandler cria o handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Routes registra as rotas.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/credential-controls", h.list)
	r.Get("/companies/{id}/credential-controls", h.forCompany)
	r.Put("/companies/{id}/credential-controls/{type}", h.update)
	r.Put("/companies/{id}/credential-controls/{type}/file", h.uploadFile)
	r.Delete("/companies/{id}/credential-controls/{type}/file", h.deleteFile)
}

// pathParams valida empresa e tipo da rota.
func pathParams(r *http.Request) (string, string, error) {
	id, err := companyID(r)
	if err != nil {
		return "", "", err
	}
	typ := chi.URLParam(r, "type")
	if !slices.Contains(Types, typ) {
		return "", "", apperr.NotFound("CREDENTIAL_TYPE_NOT_FOUND", "Tipo inválido: use RFB_POWER_OF_ATTORNEY ou DIGITAL_CERTIFICATE.")
	}
	return id, typ, nil
}

// readUpload lê o multipart (campo "file" e "version" opcional) sem passar de 10 MB.
// O conteúdo nunca é registrado em log [AGENTS §6.4].
func readUpload(w http.ResponseWriter, r *http.Request) (FileInput, error) {
	r.Body = http.MaxBytesReader(w, r.Body, MaxFileSize+(1<<20))
	mr, err := r.MultipartReader()
	if err != nil {
		return FileInput{}, apperr.BadRequest("INVALID_MULTIPART", "Envie o documento como multipart/form-data no campo \"file\".")
	}
	var in FileInput
	found := false
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return FileInput{}, uploadReadError(err)
		}
		switch part.FormName() {
		case "file":
			data, err := io.ReadAll(io.LimitReader(part, MaxFileSize+1))
			if err != nil {
				return FileInput{}, uploadReadError(err)
			}
			if len(data) > MaxFileSize {
				return FileInput{}, fileTooLarge()
			}
			in.Name, in.Data, found = part.FileName(), data, true
		case "version":
			raw, err := io.ReadAll(io.LimitReader(part, 16))
			if err != nil {
				return FileInput{}, uploadReadError(err)
			}
			v, err := strconv.ParseInt(strings.TrimSpace(string(raw)), 10, 32)
			if err != nil || v < 0 {
				return FileInput{}, apperr.BadRequest("INVALID_VERSION", "Versão inválida.")
			}
			in.Version = int32(v)
		}
	}
	if !found {
		return FileInput{}, apperr.BadRequest("FILE_REQUIRED", "Envie o documento no campo \"file\".")
	}
	return in, nil
}

func uploadReadError(err error) error {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		return fileTooLarge()
	}
	return apperr.BadRequest("INVALID_MULTIPART", "Não foi possível ler o arquivo enviado.")
}

func (h *Handler) uploadFile(w http.ResponseWriter, r *http.Request) {
	id, typ, err := pathParams(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	in, err := readUpload(w, r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	out, err := h.svc.UploadFile(r.Context(), id, typ, in)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) deleteFile(w http.ResponseWriter, r *http.Request) {
	id, typ, err := pathParams(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	out, err := h.svc.DeleteFile(r.Context(), id, typ)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	out, err := h.svc.List(r.Context(), ListParams{Filter: q.Get("filter"), Type: q.Get("type")})
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func companyID(r *http.Request) (string, error) {
	return httpx.PathUUID(r, "id", companyNotFound, companyMissing)
}

func (h *Handler) forCompany(w http.ResponseWriter, r *http.Request) {
	id, err := companyID(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	out, err := h.svc.ForCompany(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	id, typ, err := pathParams(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	var in UpdateInput
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	out, err := h.svc.Update(r.Context(), id, typ, in)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}
