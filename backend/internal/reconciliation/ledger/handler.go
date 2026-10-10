package ledger

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"controle/internal/platform/apperr"
	"controle/internal/platform/competence"
	"controle/internal/platform/httpx"
)

// Handler expõe o razão, a exportação e o detalhe da conciliação (drill-down).
type Handler struct{ svc *Service }

// NewHandler cria o handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Routes registra as rotas.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/companies/{id}/ledger", h.list)
	r.Get("/companies/{id}/ledger/export", h.export)
	r.Get("/reconciliation/reconciliations/{id}", h.detail)
}

// query lê o período (from e to, AAAA-MM) e o filtro de conta.
func query(r *http.Request) (Query, error) {
	fields := map[string]any{}
	from, err := competence.Parse(r.URL.Query().Get("from"))
	if err != nil {
		fields["from"] = "Informe o início no formato AAAA-MM."
	}
	to, err := competence.Parse(r.URL.Query().Get("to"))
	if err != nil {
		fields["to"] = "Informe o fim no formato AAAA-MM."
	}
	if len(fields) > 0 {
		return Query{}, apperr.BadRequest("VALIDATION_ERROR", "Dados inválidos. Verifique os campos destacados.").
			WithDetails(map[string]any{"fields": fields})
	}
	return Query{CompanyID: chi.URLParam(r, "id"), From: from, To: to, Account: strings.TrimSpace(r.URL.Query().Get("account"))}, nil
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	in, err := query(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	out, err := h.svc.List(r.Context(), in)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// export devolve o razão filtrado em CSV (o XLSX chega com a TASK-028).
func (h *Handler) export(w http.ResponseWriter, r *http.Request) {
	if f := r.URL.Query().Get("format"); f != "" && f != "csv" {
		httpx.WriteError(w, r, apperr.BadRequest("VALIDATION_ERROR", "Dados inválidos. Verifique os campos destacados.").
			WithDetails(map[string]any{"fields": map[string]any{"format": "Formato não suportado: use csv."}}))
		return
	}
	in, err := query(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	out, err := h.svc.List(r.Context(), in)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	name, err := h.svc.FileName(r.Context(), out)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(CSV(out))
}

func (h *Handler) detail(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.Reconciliation(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}
