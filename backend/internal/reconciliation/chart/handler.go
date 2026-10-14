package chart

import (
	"errors"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"

	"controle/internal/platform/apperr"
	"controle/internal/platform/httpx"
)

// Handler expõe o plano de contas e as regras da empresa.
type Handler struct{ svc *Service }

// NewHandler cria o handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Routes registra as rotas.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/companies/{id}/chart-accounts", h.accounts)
	r.Post("/companies/{id}/chart-accounts", h.createAccount)
	r.Patch("/companies/{id}/chart-accounts/{accountId}", h.updateAccount)
	r.Post("/companies/{id}/chart-accounts/import", h.importAccounts)
	r.Post("/companies/{id}/chart-accounts/copy", h.copyAccounts)
	r.Get("/companies/{id}/classification-rules", h.rules)
	r.Post("/companies/{id}/classification-rules", h.createRule)
	r.Post("/companies/{id}/classification-rules/{ruleId}/deactivate", h.deactivateRule)
}

// onlyActive lê ?active=true (só as ativas); ausente, todas.
func onlyActive(r *http.Request) (bool, error) {
	switch r.URL.Query().Get("active") {
	case "":
		return false, nil
	case "true":
		return true, nil
	}
	return false, validation(map[string]any{"active": "Use active=true para listar só as ativas."})
}

func (h *Handler) accounts(w http.ResponseWriter, r *http.Request) {
	active, err := onlyActive(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	out, err := h.svc.Accounts(r.Context(), chi.URLParam(r, "id"), active)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": out})
}

func (h *Handler) createAccount(w http.ResponseWriter, r *http.Request) {
	var in AccountInput
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	out, err := h.svc.CreateAccount(r.Context(), chi.URLParam(r, "id"), in)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, out)
}

func (h *Handler) updateAccount(w http.ResponseWriter, r *http.Request) {
	var in AccountPatch
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	out, err := h.svc.UpdateAccount(r.Context(), chi.URLParam(r, "id"), chi.URLParam(r, "accountId"), in)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// importAccounts lê o multipart: o CSV em "file" (até 2 MB).
func (h *Handler) importAccounts(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, MaxCSVBytes+(1<<16))
	mr, err := r.MultipartReader()
	if err != nil {
		httpx.WriteError(w, r, apperr.BadRequest("INVALID_MULTIPART", "Envie o CSV como multipart/form-data no campo \"file\"."))
		return
	}
	var fileName string
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
		if part.FormName() != "file" {
			continue
		}
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
	}
	if !found {
		httpx.WriteError(w, r, apperr.BadRequest("FILE_REQUIRED", "Envie o CSV no campo \"file\"."))
		return
	}
	out, err := h.svc.ImportAccounts(r.Context(), chi.URLParam(r, "id"), fileName, data)
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

type copyInput struct {
	SourceCompanyID string `json:"source_company_id"`
}

func (h *Handler) copyAccounts(w http.ResponseWriter, r *http.Request) {
	var in copyInput
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	out, err := h.svc.CopyAccounts(r.Context(), chi.URLParam(r, "id"), in.SourceCompanyID)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, out)
}

func (h *Handler) rules(w http.ResponseWriter, r *http.Request) {
	active, err := onlyActive(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	out, err := h.svc.Rules(r.Context(), chi.URLParam(r, "id"), active)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": out})
}

func (h *Handler) createRule(w http.ResponseWriter, r *http.Request) {
	var in RuleInput
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	out, err := h.svc.CreateRule(r.Context(), chi.URLParam(r, "id"), in)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, out)
}

func (h *Handler) deactivateRule(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.DeactivateRule(r.Context(), chi.URLParam(r, "id"), chi.URLParam(r, "ruleId"))
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}
