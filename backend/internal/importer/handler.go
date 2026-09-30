package importer

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"controle/internal/platform/httpx"
)

// Handler expõe /imports/companies [PRD §41.12].
type Handler struct{}

// NewHandler cria o handler.
func NewHandler() *Handler { return &Handler{} }

// Routes registra as rotas.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/imports/companies/template", h.template)
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
