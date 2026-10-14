package classify

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"controle/internal/platform/apperr"
	"controle/internal/platform/competence"
	"controle/internal/platform/httpx"
)

// Handler expõe a classificação: a rodada da competência e, na revisão (o {id} é a linha do
// extrato), a correção e a sugestão do agente.
type Handler struct{ svc *Service }

// NewHandler cria o handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Routes registra as rotas.
func (h *Handler) Routes(r chi.Router) {
	r.Post("/companies/{id}/classification/run", h.run)
	r.Put("/reconciliation/review/{id}/classification", h.correct)
	r.Post("/reconciliation/review/{id}/classification-suggestion", h.request)
	r.Get("/reconciliation/review/{id}/classification-suggestions", h.list)
	r.Post("/reconciliation/review/{id}/accept-classification-suggestion", h.accept)
	r.Post("/reconciliation/review/{id}/reject-classification-suggestion", h.reject)
}

func (h *Handler) run(w http.ResponseWriter, r *http.Request) {
	comp, err := competence.Parse(r.URL.Query().Get("competence"))
	if err != nil {
		httpx.WriteError(w, r, validation(map[string]any{"competence": "Informe a competência no formato AAAA-MM."}))
		return
	}
	out, err := h.svc.Run(r.Context(), chi.URLParam(r, "id"), comp)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func lineID(r *http.Request) (string, error) {
	return httpx.PathUUID(r, "id", "RECONCILIATION_LINE_NOT_FOUND", "Linha do extrato não encontrada.")
}

type correction struct {
	AccountCode string `json:"account_code"`
}

func (h *Handler) correct(w http.ResponseWriter, r *http.Request) {
	id, err := lineID(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	var in correction
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	code := strings.TrimSpace(in.AccountCode)
	if code == "" {
		httpx.WriteError(w, r, validation(map[string]any{"account_code": "Informe a conta."}))
		return
	}
	out, err := h.svc.Correct(r.Context(), id, code)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) request(w http.ResponseWriter, r *http.Request) {
	id, err := lineID(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	out, err := h.svc.RequestSuggestion(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusAccepted, out)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	id, err := lineID(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	out, err := h.svc.Suggestions(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": out})
}

type decision struct {
	SuggestionID string `json:"suggestion_id"`
}

// readDecision lê a linha e a sugestão (400 sem a sugestão).
func readDecision(w http.ResponseWriter, r *http.Request) (string, string, error) {
	id, err := lineID(r)
	if err != nil {
		return "", "", err
	}
	var in decision
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		return "", "", err
	}
	if strings.TrimSpace(in.SuggestionID) == "" {
		return "", "", apperr.BadRequest("VALIDATION_ERROR", "Dados inválidos. Verifique os campos destacados.").
			WithDetails(map[string]any{"fields": map[string]any{"suggestion_id": "Informe a sugestão."}})
	}
	return id, in.SuggestionID, nil
}

func (h *Handler) accept(w http.ResponseWriter, r *http.Request) {
	id, sid, err := readDecision(w, r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	out, err := h.svc.AcceptSuggestion(r.Context(), id, sid)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) reject(w http.ResponseWriter, r *http.Request) {
	id, sid, err := readDecision(w, r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	out, err := h.svc.RejectSuggestion(r.Context(), id, sid)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}
