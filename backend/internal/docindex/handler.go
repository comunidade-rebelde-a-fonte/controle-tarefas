package docindex

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"controle/internal/platform/httpx"
)

// Handler expõe o índice de documentos.
type Handler struct{ svc *Service }

// NewHandler cria o handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Routes registra as rotas.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/documents", h.list)
}

func intParam(r *http.Request, name string) (int32, error) {
	v := strings.TrimSpace(r.URL.Query().Get(name))
	if v == "" {
		return 0, nil
	}
	n, err := strconv.ParseInt(v, 10, 32)
	if err != nil {
		return 0, badRequest(name, "Use um número inteiro.")
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
	out, err := h.svc.List(r.Context(), Query{
		Module: q.Get("module"), CompanyID: q.Get("company_id"), Competence: q.Get("competence"), Kind: q.Get("kind"),
		Status: q.Get("status"), Situation: q.Get("situation"), Bank: q.Get("bank"), Limit: limit, Offset: offset,
	})
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}
