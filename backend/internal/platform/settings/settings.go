// Package settings guarda configurações chave/valor (JSON) editáveis pela interface,
// como as faixas de validade de procurações e certificados [PRD §19.3].
package settings

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"controle/internal/platform/httpx"
	"controle/internal/platform/validate"
	"controle/internal/store"
	"controle/internal/user"
)

// ExpiryThresholdsKey é a chave das faixas de validade de procurações e certificados.
const ExpiryThresholdsKey = "credential_thresholds"

// DefaultThresholds são as faixas padrão: vence em até 7, 15, 30 e 60 dias.
var DefaultThresholds = []int{7, 15, 30, 60}

// Thresholds são as faixas de "vence em até N dias", em ordem crescente.
type Thresholds struct {
	Days []int `json:"days"`
}

// Validate exige de 1 a 6 faixas positivas, crescentes e sem repetição.
func (t Thresholds) Validate() error {
	if len(t.Days) == 0 || len(t.Days) > 6 {
		return validate.Field("days", "Informe de 1 a 6 faixas.")
	}
	for i, d := range t.Days {
		if d < 1 || d > 365 {
			return validate.Field("days", "Cada faixa deve estar entre 1 e 365 dias.")
		}
		if i > 0 && d <= t.Days[i-1] {
			return validate.Field("days", "As faixas devem ser crescentes e sem repetição.")
		}
	}
	return nil
}

// Service lê e grava as configurações.
type Service struct{ q *store.Queries }

// NewService cria o serviço.
func NewService(pool *pgxpool.Pool) *Service { return &Service{q: store.New(pool)} }

// CredentialThresholds devolve as faixas configuradas (ou as padrão).
func (s *Service) CredentialThresholds(ctx context.Context) (Thresholds, error) {
	row, err := s.q.GetSetting(ctx, ExpiryThresholdsKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return Thresholds{Days: slices.Clone(DefaultThresholds)}, nil
	}
	if err != nil {
		return Thresholds{}, err
	}
	if t, ok := parseThresholds(row.Value); ok {
		return t, nil
	}
	// Valor corrompido: volta às faixas padrão.
	return Thresholds{Days: slices.Clone(DefaultThresholds)}, nil
}

func parseThresholds(b []byte) (Thresholds, bool) {
	var t Thresholds
	if json.Unmarshal(b, &t) != nil || t.Validate() != nil {
		return Thresholds{}, false
	}
	return t, true
}

// SetCredentialThresholds grava novas faixas.
func (s *Service) SetCredentialThresholds(ctx context.Context, t Thresholds) (Thresholds, error) {
	if err := t.Validate(); err != nil {
		return Thresholds{}, err
	}
	b, err := json.Marshal(t)
	if err != nil {
		return Thresholds{}, err
	}
	uid := user.FromContext(ctx)
	if _, err := s.q.UpsertSetting(ctx, store.UpsertSettingParams{Key: ExpiryThresholdsKey, Value: b, UpdatedBy: &uid}); err != nil {
		return Thresholds{}, err
	}
	return t, nil
}

// Handler expõe /settings/credential-thresholds.
type Handler struct{ svc *Service }

// NewHandler cria o handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Routes registra as rotas.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/settings/credential-thresholds", h.get)
	r.Put("/settings/credential-thresholds", h.put)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	t, err := h.svc.CredentialThresholds(r.Context())
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, t)
}

func (h *Handler) put(w http.ResponseWriter, r *http.Request) {
	var t Thresholds
	if err := httpx.DecodeJSON(w, r, &t); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	out, err := h.svc.SetCredentialThresholds(r.Context(), t)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}
