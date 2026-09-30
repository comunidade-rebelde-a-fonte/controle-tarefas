// Package app monta a aplicação HTTP: dependências, middlewares e rotas de
// todos os domínios sob o prefixo /api/v1.
package app

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"controle/internal/platform/clock"
	"controle/internal/platform/config"
	"controle/internal/platform/httpx"
)

// Deps são as dependências compartilhadas pelos domínios.
type Deps struct {
	Config config.Config
	Pool   *pgxpool.Pool
	Clock  clock.Clock
	Logger *slog.Logger
}

// NewRouter cria o roteador completo da API.
func NewRouter(d Deps) http.Handler {
	r := chi.NewRouter()
	r.Use(httpx.RequestID(d.Logger))
	r.Use(httpx.Recoverer)
	r.Use(httpx.AccessLog)
	r.Use(httpx.CORS(httpx.NormalizeOrigins(d.Config.CORSOrigins)))
	r.NotFound(httpx.NotFoundHandler)
	r.MethodNotAllowed(httpx.MethodNotAllowedHandler)

	r.Route("/api/v1", func(api chi.Router) {
		api.Get("/health", httpx.Health(d.Pool))
	})
	return r
}
