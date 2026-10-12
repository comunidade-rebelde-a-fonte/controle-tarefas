package review

import (
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	co "controle/internal/company"
	"controle/internal/platform/competence"
	"controle/internal/platform/httpx"
	"controle/internal/reconciliation/entries"
)

// Handler expõe a fila de revisão. Na revisão, o {id} é a linha do extrato.
type Handler struct{ svc *Service }

// NewHandler cria o handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Routes registra as rotas.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/reconciliation/review-queue", h.queue)
	r.Get("/reconciliation/review/{id}", h.line)
	r.Post("/reconciliation/review/{id}/manual-match", h.manualMatch)
	r.Post("/reconciliation/review/{id}/confirm-no-receipt", h.noReceipt)
	r.Get("/reconciliation/entries/{id}", h.entry)
	r.Patch("/reconciliation/entries/{id}", h.correctFields)
	r.Get("/reconciliation/reconciliations", h.recent)
	r.Post("/reconciliation/reconciliations/{id}/undo", h.undo)
}

// intParam lê um inteiro opcional no intervalo dado.
func intParam(r *http.Request, name string, def, lo, hi int) (int32, error) {
	v := r.URL.Query().Get(name)
	if v == "" {
		return int32(def), nil //nolint:gosec // def está no intervalo
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < lo || n > hi {
		return 0, validation(map[string]any{name: "Valor fora do intervalo."})
	}
	return int32(n), nil //nolint:gosec // conferido no intervalo
}

func (h *Handler) queue(w http.ResponseWriter, r *http.Request) {
	in := QueueQuery{}
	fields := map[string]any{}
	if v := r.URL.Query().Get("company_id"); v != "" {
		if !httpx.IsUUID(v) {
			fields["company_id"] = "Empresa inválida."
		}
		in.CompanyID = &v
	}
	if v := r.URL.Query().Get("competence"); v != "" {
		c, err := competence.Parse(v)
		if err != nil {
			fields["competence"] = "Use AAAA-MM."
		}
		in.Competence = &c
	}
	if v := r.URL.Query().Get("bucket"); v != "" {
		if !slices.Contains(Buckets, v) {
			fields["bucket"] = "Balde inválido: " + strings.Join(Buckets, ", ") + "."
		}
		in.Bucket = &v
	}
	f, err := co.ParseFilter(r.URL.Query())
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if f.Active() && in.Competence == nil {
		fields["competence"] = "Os filtros de agrupamento, regime e responsável valem com a competência."
	}
	in.Filter = f
	if in.Limit, err = intParam(r, "limit", 100, 1, 500); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if in.Offset, err = intParam(r, "offset", 0, 0, 1_000_000); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if len(fields) > 0 {
		httpx.WriteError(w, r, validation(fields))
		return
	}
	out, err := h.svc.List(r.Context(), in)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) line(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.Line(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

type matchInput struct {
	EntryIDs []string `json:"entry_ids"`
}

func (h *Handler) manualMatch(w http.ResponseWriter, r *http.Request) {
	var in matchInput
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	out, err := h.svc.ManualMatch(r.Context(), chi.URLParam(r, "id"), in.EntryIDs)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, out)
}

func (h *Handler) noReceipt(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.ConfirmNoReceipt(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, out)
}

func (h *Handler) entry(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.Entry(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) correctFields(w http.ResponseWriter, r *http.Request) {
	var in entries.FieldsPatch
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	out, err := h.svc.CorrectFields(r.Context(), chi.URLParam(r, "id"), in)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) recent(w http.ResponseWriter, r *http.Request) {
	var company, origin *string
	fields := map[string]any{}
	if v := r.URL.Query().Get("company_id"); v != "" {
		if !httpx.IsUUID(v) {
			fields["company_id"] = "Empresa inválida."
		}
		company = &v
	}
	if v := r.URL.Query().Get("origin"); v != "" {
		if v != "auto" && v != "manual" {
			fields["origin"] = "Use auto ou manual."
		}
		origin = &v
	}
	limit, err := intParam(r, "limit", 8, 1, 50)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if len(fields) > 0 {
		httpx.WriteError(w, r, validation(fields))
		return
	}
	out, err := h.svc.Recent(r.Context(), company, origin, limit)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": out})
}

type undoInput struct {
	Reason string `json:"reason"`
}

func (h *Handler) undo(w http.ResponseWriter, r *http.Request) {
	var in undoInput
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	out, err := h.svc.Undo(r.Context(), chi.URLParam(r, "id"), in.Reason)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}
