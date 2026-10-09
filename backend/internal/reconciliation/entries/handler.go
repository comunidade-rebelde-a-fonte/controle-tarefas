package entries

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"controle/internal/platform/apperr"
	"controle/internal/platform/httpx"
	"controle/internal/platform/validate"
)

// Handler expõe os lançamentos do cliente [CC-12].
type Handler struct{ svc *Service }

// NewHandler cria o handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Routes registra as rotas.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/reconciliation/entries", h.list)
	r.Post("/reconciliation/entries", h.create)
	r.Post("/reconciliation/entries/import", h.importCSV)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var in CreateInput
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	out, err := h.svc.Create(r.Context(), in)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, out)
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
	out, err := h.svc.List(r.Context(), ListInput{
		CompanyID: q.Get("company_id"), Status: q.Get("status"), DateFrom: q.Get("date_from"), DateTo: q.Get("date_to"),
		Limit: limit, Offset: offset,
	})
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// importCSV lê o multipart: o CSV em "file" (até 2 MB) e a empresa em "company_id".
func (h *Handler) importCSV(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, MaxCSVBytes+(1<<16))
	mr, err := r.MultipartReader()
	if err != nil {
		httpx.WriteError(w, r, apperr.BadRequest("INVALID_MULTIPART", "Envie o CSV como multipart/form-data no campo \"file\"."))
		return
	}
	var companyID, fileName string
	var data []byte
	found := false
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			httpx.WriteError(w, r, readError(err))
			return
		}
		switch part.FormName() {
		case "file":
			data, err = io.ReadAll(io.LimitReader(part, MaxCSVBytes+1))
			if err != nil {
				httpx.WriteError(w, r, readError(err))
				return
			}
			if len(data) > MaxCSVBytes {
				httpx.WriteError(w, r, tooLarge())
				return
			}
			fileName, found = part.FileName(), true
		case "company_id":
			v, err := io.ReadAll(io.LimitReader(part, 64))
			if err != nil {
				httpx.WriteError(w, r, readError(err))
				return
			}
			companyID = strings.TrimSpace(string(v))
		}
	}
	if !found {
		httpx.WriteError(w, r, apperr.BadRequest("FILE_REQUIRED", "Envie o CSV no campo \"file\"."))
		return
	}
	out, err := h.svc.ImportCSV(r.Context(), companyID, fileName, data)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, out)
}

func tooLarge() error {
	return apperr.New(http.StatusRequestEntityTooLarge, "FILE_TOO_LARGE", "O CSV excede o limite de 2 MB.")
}

func readError(err error) error {
	var mbe *http.MaxBytesError
	if errors.As(err, &mbe) {
		return tooLarge()
	}
	return apperr.BadRequest("INVALID_MULTIPART", "Não foi possível ler o arquivo enviado.")
}
