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

	"controle/internal/ai/aliases"
	"controle/internal/ai/litellm"
	"controle/internal/ai/runs"
	"controle/internal/ai/tracer"
	"controle/internal/calendar"
	"controle/internal/catalog"
	"controle/internal/company"
	"controle/internal/competence"
	"controle/internal/credential"
	"controle/internal/dashboard"
	"controle/internal/demo"
	"controle/internal/exporter"
	"controle/internal/factorr"
	"controle/internal/factorr/agents"
	"controle/internal/factorr/inbox"
	"controle/internal/grid"
	"controle/internal/importer"
	"controle/internal/jobs"
	"controle/internal/obligation"
	"controle/internal/period"
	"controle/internal/platform/clock"
	"controle/internal/platform/config"
	"controle/internal/platform/httpx"
	"controle/internal/platform/objectstore"
	"controle/internal/platform/pdftext"
	"controle/internal/platform/settings"
	"controle/internal/reconciliation/bankaccount"
	"controle/internal/reconciliation/chart"
	"controle/internal/reconciliation/classify"
	"controle/internal/reconciliation/completion"
	"controle/internal/reconciliation/documents"
	"controle/internal/reconciliation/entries"
	"controle/internal/reconciliation/ledger"
	"controle/internal/reconciliation/metrics"
	"controle/internal/reconciliation/proposals"
	"controle/internal/reconciliation/reconcile"
	"controle/internal/reconciliation/review"
	"controle/internal/report"
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
	r.Use(httpx.SecurityHeaders)
	r.Use(httpx.RequestID(d.Logger))
	r.Use(httpx.Recoverer)
	r.Use(httpx.AccessLog)
	r.Use(httpx.CORS(httpx.NormalizeOrigins(d.Config.CORSOrigins)))
	r.Use(user.Middleware)
	// Grades (~300 linhas x ~13 células) são JSON grandes; gzip reduz a transferência [§45.1].
	r.Use(middleware.Compress(5, "application/json"))
	r.NotFound(httpx.NotFoundHandler)
	r.MethodNotAllowed(httpx.MethodNotAllowedHandler)
	// Imagem de produção: o mesmo binário serve a SPA (rotas fora de /api/v1) [plan M7].
	if d.Config.StaticDir != "" {
		r.NotFound(httpx.SPA(d.Config.StaticDir).ServeHTTP)
	}

	r.Route("/api/v1", func(api chi.Router) {
		api.Get("/health", httpx.Health(d.Pool))
		user.NewHandler(d.Pool).Routes(api)
		company.NewHandler(companies).Routes(api)
		competence.NewHandler(d.Pool, engine).Routes(api)
		obligation.NewHandler(obligation.NewService(d.Pool, d.Clock)).Routes(api)
		period.NewHandler(period.NewService(d.Pool, d.Clock)).Routes(api)
		grids := grid.NewBuilder(d.Pool, d.Clock)
		grid.NewHandler(grids).Routes(api)
		exporter.NewHandler(grids, d.Pool, d.Clock).Routes(api)
		catalog.NewHandler(catalog.NewService(d.Pool)).Routes(api)
		calendar.NewHandler(d.Pool).Routes(api)
		settings.NewHandler(settings.NewService(d.Pool)).Routes(api)
		// Configurações → IA: só configuração não secreta; a chave é a do gateway (TASK-008).
		gateway := litellm.New(d.Config.LiteLLMBaseURL, d.Config.LiteLLMAPIKey, d.Config.LiteLLMTimeout)
		aliases.NewHandler(aliases.NewService(d.Pool, d.Clock, d.Config.AIDefaults, gateway)).Routes(api)
		// Auditoria local de IA (TASK-009): tempo operacional, como a fila de jobs. O envio ao
		// Langfuse é do worker; a API só monta o link do trace.
		aiClock := clock.NewSystem(d.Clock.Location())
		tr := tracer.New(aiClock, jobs.NewQueue(aiClock, d.Config.JobMaxAttempts))
		runs.NewHandler(runs.NewService(d.Pool, aiClock, tr, d.Config.LangfuseTraceURL)).Routes(api)
		importer.NewHandler(importer.NewService(d.Pool, d.Clock, companies)).Routes(api)
		credentials := credential.NewService(d.Pool, d.Clock, d.Config.FileStorageDir, d.Logger)
		credential.NewHandler(credentials).Routes(api)
		dash := dashboard.NewService(d.Pool, d.Clock, credentials)
		dashboard.NewHandler(dash).Routes(api)
		factorr.NewHandler(factorr.NewService(d.Pool, d.Clock)).Routes(api)
		// Inbox PGDAS-D (TASK-012): a API guarda o original e enfileira; o worker processa.
		inbox.NewHandler(inbox.NewService(d.Pool, aiClock, tr, jobs.NewQueue(aiClock, d.Config.JobMaxAttempts),
			inbox.NewStorage(d.Config.FileStorageDir), companies, d.Logger)).Routes(api)
		// Simulador e agentes consultor/priorizador (TASK-013): números do motor; o LLM, pelo
		// gateway com os aliases da tela de IA, só classifica e redige.
		agents.NewHandler(agents.NewService(d.Pool, d.Clock, aiClock, tr, aliases.NewResolver(d.Config.AIDefaults), gateway, d.Logger)).Routes(api)
		// Conciliação (TASK-015): o original vai para o MinIO versionado e a triagem para a fila.
		originals := objectstore.New(objectstore.Config{
			Endpoint: d.Config.S3Endpoint, Region: d.Config.S3Region, AccessKey: d.Config.S3AccessKey,
			SecretKey: d.Config.S3SecretKey.Reveal(), Bucket: d.Config.S3Bucket,
		})
		docs := documents.NewService(d.Pool, aiClock, jobs.NewQueue(aiClock, d.Config.JobMaxAttempts), originals, d.Logger)
		documents.NewHandler(docs).Routes(api)
		// Visualizador da fila (TASK-021a): as páginas do PDF como imagem, no DPI do extrator; o
		// PDFium só carrega na primeira página pedida.
		documents.NewViewer(docs, pdftext.New(0)).Routes(api)
		// Lançamentos do cliente (TASK-017): os lidos dos comprovantes e os manuais (CC-12).
		entries.NewHandler(entries.NewService(d.Pool, aiClock)).Routes(api)
		// Rodada do matching da competência (TASK-018): passes 1 a 4 com a regra dos dois lados; com
		// RESOLVEDOR_AUTOMATICO, pede hipótese para as linhas que sobraram (TASK-019).
		rounds := reconcile.NewService(d.Pool, aiClock, reconcile.OptionsFrom(d.Config.FuzzyThreshold), d.Logger)
		if d.Config.ResolverAuto {
			rounds.AutoResolve(jobs.NewQueue(aiClock, d.Config.JobMaxAttempts))
		}
		if d.Config.ClassifierAuto {
			rounds.AutoClassify(jobs.NewQueue(aiClock, d.Config.JobMaxAttempts))
		}
		reconcile.NewHandler(rounds).Routes(api)
		// Resolvedor de exceções (TASK-019): pedir a hipótese (o agente roda no worker), aceitar e
		// rejeitar. Só o aceite humano concilia.
		proposals.NewHandler(proposals.NewService(d.Pool, aiClock, jobs.NewQueue(aiClock, d.Config.JobMaxAttempts))).Routes(api)
		// Classificação e razão (TASK-020): plano de contas e regras por empresa; a classificação
		// regra → histórico → agente (o agente roda no worker e só sugere); o razão como consulta.
		chart.NewHandler(chart.NewService(d.Pool, aiClock)).Routes(api)
		// Fila de revisão (TASK-021a): os baldes, a linha com os candidatos e as ações humanas
		// (conciliar à mão, sem comprovante, desfazer e corrigir o comprovante).
		review.NewHandler(review.NewService(d.Pool, aiClock, rounds)).Routes(api)
		classify.NewHandler(classify.NewService(d.Pool, aiClock, jobs.NewQueue(aiClock, d.Config.JobMaxAttempts))).Routes(api)
		ledger.NewHandler(ledger.NewService(d.Pool)).Routes(api)
		// Métricas, playbooks e o resumo da ficha (TASK-021c): consultas sobre as tabelas da
		// Conciliação, com a meta da automática de META_CONCILIACAO_AUTO; e as contas bancárias da
		// empresa (CC-11), que decidem a empresa dos extratos.
		metrics.NewHandler(metrics.NewService(d.Pool, d.Config.AutoTarget)).Routes(api)
		bankaccount.NewHandler(bankaccount.NewService(d.Pool, aiClock)).Routes(api)
		// Conclusão da conciliação (TASK-022): o contador conclui a competência pronta e a obrigação
		// Conciliação do Contábil vai para OK pelo serviço de obrigações; reaberta, volta a Pendente.
		// O relógio é o do Controle: o histórico da obrigação fica na mesma linha do tempo.
		completion.NewHandler(completion.NewService(d.Pool, d.Clock)).Routes(api)
		// Fila de jobs: tempo operacional (relógio real), mesmo com APP_FIXED_TIME.
		jobs.NewHandler(jobs.NewService(d.Pool, clock.NewSystem(d.Clock.Location()))).Routes(api)
		report.NewHandler(d.Pool, d.Clock, grids, dash, credentials).Routes(api)
		// Dados fictícios pela interface (estado vazio): nunca em produção.
		if !d.Config.IsProduction() {
			demo.NewHandler(d.Pool, d.Clock).Routes(api)
		}
	})
	return &App{Handler: r, Engine: engine, deps: d}
}
