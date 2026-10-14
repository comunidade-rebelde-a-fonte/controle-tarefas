package factorr

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"

	co "controle/internal/company"
	"controle/internal/platform/competence"
	"controle/internal/platform/httpx"
	"controle/internal/platform/validate"
)

// Handler expõe as rotas do Fator R [docs/ai-spec/06 §3].
type Handler struct{ svc *Service }

// NewHandler cria o handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Routes registra as rotas.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/factor-r/portfolio", h.portfolio)
	r.Get("/companies/{id}/factor-r/settings", h.settings)
	r.Patch("/companies/{id}/factor-r/settings", h.updateSettings)
	r.Get("/companies/{id}/factor-r/movements", h.movements)
	r.Put("/companies/{id}/factor-r/movements/{competence}", h.putMovement)
	r.Get("/companies/{id}/factor-r/calc", h.calc)
}

func companyID(r *http.Request) (string, error) {
	return httpx.PathUUID(r, "id", companyNotFound, companyMissing)
}

// optionalCompetence lê uma competência opcional da query; ausente → nil.
func optionalCompetence(q url.Values, name string) (*competence.Competence, error) {
	v := strings.TrimSpace(q.Get(name))
	if v == "" {
		return nil, nil
	}
	c, err := competence.Parse(v)
	if err != nil {
		return nil, validate.Field(name, "Informe a competência no formato AAAA-MM.")
	}
	return &c, nil
}

// pa lê o período de apuração; ausente, a competência corrente (origem).
func (h *Handler) pa(r *http.Request) (competence.Competence, error) {
	c, err := optionalCompetence(r.URL.Query(), "pa")
	if err != nil || c == nil {
		return h.svc.CurrentPA(), err
	}
	return *c, nil
}

func (h *Handler) portfolio(w http.ResponseWriter, r *http.Request) {
	pa, err := h.pa(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	f, err := co.ParseFilter(r.URL.Query())
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	out, err := h.svc.Portfolio(r.Context(), pa, f)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) settings(w http.ResponseWriter, r *http.Request) {
	id, err := companyID(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	out, err := h.svc.Settings(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) updateSettings(w http.ResponseWriter, r *http.Request) {
	id, err := companyID(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	var in SettingsInput
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	out, err := h.svc.UpdateSettings(r.Context(), id, in)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) movements(w http.ResponseWriter, r *http.Request) {
	id, err := companyID(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	q := r.URL.Query()
	from, err := optionalCompetence(q, "from")
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	to, err := optionalCompetence(q, "to")
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if from != nil && to != nil && to.Before(*from) {
		httpx.WriteError(w, r, validate.Field("to", "A competência final deve ser igual ou posterior à inicial."))
		return
	}
	out, err := h.svc.Movements(r.Context(), id, MovementsRange{From: from, To: to})
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) putMovement(w http.ResponseWriter, r *http.Request) {
	id, err := companyID(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	c, err := competence.Parse(chi.URLParam(r, "competence"))
	if err != nil {
		httpx.WriteError(w, r, validate.Field("competence", "Informe a competência no formato AAAA-MM."))
		return
	}
	var in MovementInput
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	out, err := h.svc.PutMovement(r.Context(), id, c, in)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) calc(w http.ResponseWriter, r *http.Request) {
	id, err := companyID(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	pa, err := h.pa(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	out, err := h.svc.Calc(r.Context(), id, pa)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}
