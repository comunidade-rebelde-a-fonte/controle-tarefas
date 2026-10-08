package runs

import (
	"fmt"
	"net/http"
	"regexp"
	"strconv"

	"github.com/go-chi/chi/v5"

	"controle/internal/platform/httpx"
	"controle/internal/platform/validate"
)

// Handler expõe a auditoria local de IA em /ai [06 §9].
type Handler struct{ svc *Service }

// NewHandler cria o handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Routes registra as rotas.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/ai/summary", h.summary)
	r.Get("/ai/runs", h.list)
	r.Get("/ai/runs/{trace_id}", h.get)
	r.Post("/ai/runs/{trace_id}/evaluation", h.evaluate)
}

var agentRe = regexp.MustCompile(`^[a-z][a-z0-9_]{1,39}$`)

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := Filter{Limit: 50}
	if v := q.Get("agent"); v != "" {
		if !agentRe.MatchString(v) {
			httpx.WriteError(w, r, validate.Field("agent", "Agente inválido."))
			return
		}
		f.Agent = &v
	}
	if v := q.Get("status"); v != "" {
		if v != "ok" && v != "error" && v != "needs_review" {
			httpx.WriteError(w, r, validate.Field("status", "Use ok, error ou needs_review."))
			return
		}
		f.Status = &v
	}
	if v := q.Get("company_id"); v != "" {
		if !httpx.IsUUID(v) {
			httpx.WriteError(w, r, validate.Field("company_id", "Empresa inválida."))
			return
		}
		f.CompanyID = &v
	}
	if v := q.Get("without_human_eval"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			httpx.WriteError(w, r, validate.Field("without_human_eval", "Use true ou false."))
			return
		}
		f.WithoutHumanEval = b
	}
	for _, p := range []struct {
		name     string
		dst      *int32
		min, max int32
	}{{"limit", &f.Limit, 1, 200}, {"offset", &f.Offset, 0, 1_000_000}} {
		if v := q.Get(p.name); v != "" {
			n, err := strconv.ParseInt(v, 10, 32)
			if err != nil || n < int64(p.min) || n > int64(p.max) {
				httpx.WriteError(w, r, validate.Field(p.name, fmt.Sprintf("Informe um número de %d a %d.", p.min, p.max)))
				return
			}
			*p.dst = int32(n)
		}
	}
	out, err := h.svc.List(r.Context(), f)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.Get(r.Context(), chi.URLParam(r, "trace_id"))
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) evaluate(w http.ResponseWriter, r *http.Request) {
	var in EvaluationInput
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	out, err := h.svc.Evaluate(r.Context(), chi.URLParam(r, "trace_id"), in)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, out)
}

func (h *Handler) summary(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.Summary(r.Context())
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}
