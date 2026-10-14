package overview

import (
	"net/http"
	"slices"
	"strings"

	"github.com/go-chi/chi/v5"

	co "controle/internal/company"
	"controle/internal/platform/competence"
	"controle/internal/platform/httpx"
	"controle/internal/platform/validate"
)

// Handler expõe a Conciliação por empresa.
type Handler struct{ svc *Service }

// NewHandler cria o handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Routes registra as rotas.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/reconciliation/companies", h.list)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	c, err := competence.Parse(q.Get("competence"))
	if err != nil {
		httpx.WriteError(w, r, validate.Field("competence", "Informe a competência (AAAA-MM)."))
		return
	}
	var in Query
	if v := strings.TrimSpace(q.Get("state")); v != "" {
		if !slices.Contains(States, v) {
			httpx.WriteError(w, r, validate.Field("state", "Use em_andamento, pronta ou concluida."))
			return
		}
		in.State = &v
	}
	switch strings.TrimSpace(q.Get("blocked")) {
	case "", "false":
	case "true":
		in.Blocked = true
	default:
		httpx.WriteError(w, r, validate.Field("blocked", "Use true ou false."))
		return
	}
	if in.Filter, err = co.ParseFilter(q); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	out, err := h.svc.List(r.Context(), c, in)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}
