// Package demo expõe a carga dos dados fictícios de demonstração pela interface
// (estado vazio, PRD §49.1). Disponível somente fora de produção.
package demo

import (
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"controle/internal/platform/clock"
	"controle/internal/platform/httpx"
	"controle/seed"
)

// Handler expõe POST /demo/seed.
type Handler struct {
	pool  *pgxpool.Pool
	clock clock.Clock
}

// NewHandler cria o handler.
func NewHandler(pool *pgxpool.Pool, clk clock.Clock) *Handler {
	return &Handler{pool: pool, clock: clk}
}

// Routes registra a rota. Não deve ser chamado em produção.
func (h *Handler) Routes(r chi.Router) {
	r.Post("/demo/seed", h.seed)
}

// SeedResult resume a carga de demonstração.
type SeedResult struct {
	Companies int `json:"companies"`
}

func (h *Handler) seed(w http.ResponseWriter, r *http.Request) {
	if err := seed.Demo(r.Context(), h.pool, h.clock, io.Discard); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	var n int
	if err := h.pool.QueryRow(r.Context(), `SELECT count(*) FROM companies`).Scan(&n); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, SeedResult{Companies: n})
}
