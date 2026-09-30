// Package app monta a aplicação HTTP: dependências, middlewares e rotas de
// todos os domínios sob o prefixo /api/v1.
package app

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"

	"controle/internal/calendar"
	"controle/internal/catalog"
	"controle/internal/company"
	"controle/internal/competence"
	"controle/internal/credential"
	"controle/internal/demo"
	"controle/internal/grid"
	"controle/internal/importer"
	"controle/internal/obligation"
	"controle/internal/period"
	"controle/internal/platform/clock"
	"controle/internal/platform/config"
	"controle/internal/platform/httpx"
	"controle/internal/platform/settings"
	"controle/internal/user"
)

// Deps são as dependências compartilhadas pelos domínios.
type Deps struct {
	Config config.Config
	Pool   *pgxpool.Pool
	Clock  clock.Clock
	Logger *slog.Logger
}

// App é a aplicação montada: roteador HTTP e motor de competências.
type App struct {
	Handler http.Handler
	Engine  *competence.Engine
	deps    Deps
}

// NewRouter cria o roteador completo da API (sem os gatilhos de boot).
func NewRouter(d Deps) http.Handler { return New(d).Handler }

// Boot executa os gatilhos de inicialização [PRD §10.2]: garante a competência corrente e
// inicia o agendador diário (00:05 no fuso de negócio), encerrado quando ctx é cancelado.
// Falha de banco no boot não derruba a API: é registrada e o agendador tenta de novo.
func (a *App) Boot(ctx context.Context) {
	if s, err := a.Engine.EnsureCurrent(ctx); err != nil {
		a.deps.Logger.Error("boot: falha ao garantir a competência corrente", "error", err)
	} else {
		a.deps.Logger.Info("boot: competência garantida", "competence", s.Competence.String(),
			"companies", s.CompaniesProcessed, "instances", s.InstancesCreated)
	}
	go competence.RunDaily(ctx, a.Engine, a.deps.Clock, a.deps.Logger)
}

// New monta a aplicação.
func New(d Deps) *App {
	engine := competence.NewEngine(d.Pool, d.Clock)
	companies := company.NewService(d.Pool, d.Clock)
	companies.SetLatestGenerated(engine.LatestGenerated)

	r := chi.NewRouter()
	r.Use(httpx.RequestID(d.Logger))
	r.Use(httpx.Recoverer)
	r.Use(httpx.AccessLog)
	r.Use(httpx.CORS(httpx.NormalizeOrigins(d.Config.CORSOrigins)))
	r.Use(user.Middleware)
	// Grades (~300 linhas x ~13 células) são JSON grandes; gzip reduz a transferência [§45.1].
	r.Use(middleware.Compress(5, "application/json"))
	r.NotFound(httpx.NotFoundHandler)
	r.MethodNotAllowed(httpx.MethodNotAllowedHandler)

	r.Route("/api/v1", func(api chi.Router) {
		api.Get("/health", httpx.Health(d.Pool))
		user.NewHandler(d.Pool).Routes(api)
		company.NewHandler(companies).Routes(api)
		competence.NewHandler(d.Pool, engine).Routes(api)
		obligation.NewHandler(obligation.NewService(d.Pool, d.Clock)).Routes(api)
		period.NewHandler(period.NewService(d.Pool, d.Clock)).Routes(api)
		grid.NewHandler(grid.NewBuilder(d.Pool, d.Clock)).Routes(api)
		catalog.NewHandler(catalog.NewService(d.Pool)).Routes(api)
		calendar.NewHandler(d.Pool).Routes(api)
		settings.NewHandler(settings.NewService(d.Pool)).Routes(api)
		importer.NewHandler().Routes(api)
		credential.NewHandler(credential.NewService(d.Pool, d.Clock, d.Config.FileStorageDir, d.Logger)).Routes(api)
		// Dados fictícios pela interface (estado vazio): nunca em produção.
		if !d.Config.IsProduction() {
			demo.NewHandler(d.Pool, d.Clock).Routes(api)
		}
	})
	return &App{Handler: r, Engine: engine, deps: d}
}
