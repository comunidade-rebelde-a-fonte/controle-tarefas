// Comando api: servidor HTTP da aplicação e utilitários operacionais.
//
//	api                               inicia o servidor (padrão)
//	api serve                         inicia o servidor
//	api worker                        inicia o worker dos jobs assíncronos (processo separado)
//	api migrate up|down|status|reset  gerencia as migrations embutidas
//	api seed [--demo] [--fixtures]    carrega dados de referência (e de demonstração; com
//	                                  --fixtures, prepara os fixtures da demonstração, TASK-027)
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
	_ "time/tzdata" // fusos embutidos (imagem final mínima não tem tzdata)

	"github.com/jackc/pgx/v5/pgxpool"

	"controle/internal/ai/aliases"
	"controle/internal/ai/langfuse"
	"controle/internal/ai/litellm"
	"controle/internal/ai/tracer"
	"controle/internal/app"
	"controle/internal/factorr/agents"
	"controle/internal/factorr/inbox"
	"controle/internal/jobs"
	"controle/internal/platform/clock"
	"controle/internal/platform/config"
	"controle/internal/platform/db"
	"controle/internal/platform/migrate"
	"controle/internal/platform/objectstore"
	"controle/internal/platform/pdftext"
	"controle/internal/reconciliation/classify"
	"controle/internal/reconciliation/pipeline"
	"controle/internal/reconciliation/proposals"
	"controle/internal/reconciliation/reconcile"
	"controle/internal/worker"
	"controle/seed"
	"controle/seed/fixtures"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Getenv, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "erro:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, getenv func(string) string, stdout io.Writer) error {
	cmd := "serve"
	if len(args) > 0 {
		cmd, args = args[0], args[1:]
	}
	cfg, err := config.Load(getenv)
	if err != nil {
		return fmt.Errorf("configuração inválida:\n%w", err)
	}
	logger := newLogger(cfg, stdout)
	slog.SetDefault(logger)

	switch cmd {
	case "serve":
		return serve(ctx, cfg, logger)
	case "worker":
		return runWorker(ctx, cfg, logger)
	case "migrate":
		return runMigrate(ctx, cfg, args, stdout)
	case "seed":
		return runSeed(ctx, cfg, args, stdout)
	default:
		return fmt.Errorf("comando desconhecido %q (use: serve, worker, migrate, seed)", cmd)
	}
}

func runWorker(ctx context.Context, cfg config.Config, logger *slog.Logger) error {
	pool, err := db.NewPool(ctx, cfg.DatabaseURL, cfg.DBMaxConns)
	if err != nil {
		return err
	}
	defer pool.Close()
	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", ":"+strconv.Itoa(cfg.WorkerHealthPort))
	if err != nil {
		return fmt.Errorf("worker: abrir a porta do healthcheck: %w", err)
	}
	// A fila usa o relógio real (tempo operacional: backoff, posse e batimento), mesmo com
	// APP_FIXED_TIME, que congela só as datas de negócio da API.
	sysClock := clock.NewSystem(cfg.Location)
	// Langfuse (TASK-009): só o worker conhece a chave secreta e envia traces e notas.
	syncer := tracer.NewSyncer(langfuse.New(langfuse.Config{
		Host: cfg.LangfuseHost, PublicKey: cfg.LangfusePublicKey, SecretKey: cfg.LangfuseSecretKey,
		Environment: cfg.LangfuseEnvironment, Timeout: cfg.LangfuseTimeout, MaxPayloadBytes: cfg.TracePayloadMaxKB * 1024,
	}), sysClock, jobs.NewQueue(sysClock, cfg.JobMaxAttempts), logger)
	// Inbox PGDAS-D (TASK-012): o parser_pgdas roda aqui; o PDFium só carrega no primeiro PDF.
	extractor := pdftext.New(0)
	defer func() { _ = extractor.Close() }()
	pgdasAgent := inbox.NewAgent(sysClock, tracer.New(sysClock, jobs.NewQueue(sysClock, cfg.JobMaxAttempts)),
		inbox.NewStorage(cfg.FileStorageDir), extractor)
	// Conciliação (TASK-016/017): triagem e extração do extrato e do comprovante. O original vem do
	// MinIO; os agentes, pelo LiteLLM com os aliases da tela de IA.
	originals := objectstore.New(objectstore.Config{
		Endpoint: cfg.S3Endpoint, Region: cfg.S3Region, AccessKey: cfg.S3AccessKey, SecretKey: cfg.S3SecretKey.Reveal(), Bucket: cfg.S3Bucket,
	})
	processor := pipeline.New(sysClock, tracer.New(sysClock, jobs.NewQueue(sysClock, cfg.JobMaxAttempts)),
		jobs.NewQueue(sysClock, cfg.JobMaxAttempts), originals, extractor,
		litellm.New(cfg.LiteLLMBaseURL, cfg.LiteLLMAPIKey, cfg.LiteLLMTimeout), aliases.NewResolver(cfg.AIDefaults),
		pipeline.Config{TriageThreshold: cfg.TriageThreshold, FieldThreshold: cfg.FieldThreshold}, logger)
	// Rodada do matching (TASK-018), enfileirada quando chega extrato importado ou comprovante; com
	// RESOLVEDOR_AUTOMATICO, pede hipótese para as linhas que sobraram (TASK-019); com
	// CLASSIFICADOR_AUTOMATICO, sugestão de conta para as conciliadas sem regra nem histórico (TASK-020).
	rounds := reconcile.NewService(pool, sysClock, reconcile.OptionsFrom(cfg.FuzzyThreshold), logger)
	if cfg.ResolverAuto {
		rounds.AutoResolve(jobs.NewQueue(sysClock, cfg.JobMaxAttempts))
	}
	if cfg.ClassifierAuto {
		rounds.AutoClassify(jobs.NewQueue(sysClock, cfg.JobMaxAttempts))
	}
	// Resolvedor de exceções (TASK-019): grava só a proposta.
	resolverAgent := proposals.NewAgent(sysClock, tracer.New(sysClock, jobs.NewQueue(sysClock, cfg.JobMaxAttempts)),
		litellm.New(cfg.LiteLLMBaseURL, cfg.LiteLLMAPIKey, cfg.LiteLLMTimeout), aliases.NewResolver(cfg.AIDefaults), logger)
	// Classificador (TASK-020): só para o que a regra e o histórico não classificaram; grava só a
	// sugestão.
	classifierAgent := classify.NewAgent(sysClock, tracer.New(sysClock, jobs.NewQueue(sysClock, cfg.JobMaxAttempts)),
		litellm.New(cfg.LiteLLMBaseURL, cfg.LiteLLMAPIKey, cfg.LiteLLMTimeout), aliases.NewResolver(cfg.AIDefaults), logger)
	runner := jobs.NewRunner(pool, sysClock, logger, jobs.RunnerConfig{
		WorkerID: cfg.WorkerID, Concurrency: cfg.WorkerConcurrency, PollInterval: cfg.JobPollInterval, Lease: cfg.JobLease,
	}, worker.Handlers(worker.HandlerDeps{
		LangfuseSync: syncer.Handle, PgdasParse: pgdasAgent.HandleParse,
		ReconciliationTriage: processor.HandleTriage, StatementExtract: processor.HandleExtract,
		ReceiptExtract: processor.HandleReceipt, ReconciliationRun: rounds.HandleRun, AgentResolver: resolverAgent.HandleResolve,
		AgentClassifier: classifierAgent.HandleSuggest,
	}))
	// Fila semanal do priorizador (TASK-013): aos domingos, a partir de ROTINA_PRIORIZADOR_HORA,
	// uma vez por semana e sem LLM, como a rotina_semanal da origem.
	routine := agents.NewService(pool, sysClock, sysClock, tracer.New(sysClock, jobs.NewQueue(sysClock, cfg.JobMaxAttempts)), nil, nil, logger)
	tasks := []worker.Task{{Name: "priorizador_rotina", Every: 15 * time.Minute, Run: func(ctx context.Context) error {
		now := sysClock.Now()
		if !agents.RoutineDue(now, cfg.RoutinePriorizadorHour) {
			return nil
		}
		n, err := routine.WeeklyRoutine(ctx, now)
		if n > 0 {
			logger.Info("priorizador: fila semanal registrada")
		}
		return err
	}}}
	if syncer.Configured() {
		// Reenvio periódico das corridas e notas com falha (origem: de 5 em 5 minutos, 50 por vez).
		tasks = append(tasks, worker.Task{Name: "langfuse_resync", Every: cfg.LangfuseRetry, Run: func(ctx context.Context) error {
			n, err := syncer.Requeue(ctx, pool, cfg.LangfuseRetry, 50)
			if n > 0 {
				logger.Info("Langfuse: corridas reenfileiradas para reenvio", "jobs", n)
			}
			return err
		}})
	} else {
		logger.Info("Langfuse não configurado (LANGFUSE_HOST): as corridas ficam só na trilha local")
	}
	return worker.Run(ctx, worker.Deps{ID: cfg.WorkerID, DB: pool, Logger: logger, Jobs: runner, Tasks: tasks}, ln)
}

func runMigrate(ctx context.Context, cfg config.Config, args []string, stdout io.Writer) error {
	sub := "up"
	if len(args) > 0 {
		sub = args[0]
	}
	switch sub {
	case "up":
		return migrate.Up(ctx, cfg.DatabaseURL, stdout)
	case "down":
		return migrate.Down(ctx, cfg.DatabaseURL, stdout)
	case "status":
		return migrate.Status(ctx, cfg.DatabaseURL, stdout)
	case "reset":
		if cfg.IsProduction() {
			return errors.New("migrate reset não é permitido em produção")
		}
		return migrate.Reset(ctx, cfg.DatabaseURL, stdout)
	default:
		return fmt.Errorf("subcomando de migrate desconhecido %q (use: up, down, status, reset)", sub)
	}
}

func runSeed(ctx context.Context, cfg config.Config, args []string, stdout io.Writer) error {
	demo, prepare := false, false
	for _, a := range args {
		switch a {
		case "--demo":
			demo = true
		case "--fixtures":
			prepare = true
		default:
			return fmt.Errorf("opção de seed desconhecida %q (use: --demo, --fixtures)", a)
		}
	}
	if (demo || prepare) && cfg.IsProduction() {
		return errors.New("dados de demonstração não são carregados em produção")
	}
	pool, err := db.NewPool(ctx, cfg.DatabaseURL, cfg.DBMaxConns)
	if err != nil {
		return err
	}
	defer pool.Close()
	clk := newClock(cfg)
	if err := seed.Reference(ctx, pool, clk, stdout); err != nil {
		return err
	}
	if demo {
		if err := seed.Demo(ctx, pool, clk, stdout); err != nil {
			return err
		}
	}
	if prepare {
		return runFixtures(ctx, cfg, pool, clk, stdout)
	}
	return nil
}

// Pastas da preparação dos fixtures (TASK-027), relativas a backend/, onde o make roda o comando:
// o cenário da Conciliação e as fixtures do parser do PGDAS-D são as dos testes; os extratos do
// PGDAS-D gerados vão para demo/pgdas, na raiz do repositório (fora do git).
const (
	fixturesScenarioDir = "internal/reconciliation/testdata/conciliacao"
	fixturesPgdasDir    = "internal/factorr/pgdas/testdata/fixtures"
	fixturesOutDir      = "../demo/pgdas"
)

// runFixtures prepara os fixtures da demonstração (make demo-fixtures): o cenário da Conciliação na
// Construtora Horizonte, com os documentos lidos pelo worker no ar, e os extratos do PGDAS-D.
func runFixtures(ctx context.Context, cfg config.Config, pool *pgxpool.Pool, clk clock.Clock, stdout io.Writer) error {
	sysClock := clock.NewSystem(cfg.Location)
	originals := objectstore.New(objectstore.Config{
		Endpoint: cfg.S3Endpoint, Region: cfg.S3Region, AccessKey: cfg.S3AccessKey, SecretKey: cfg.S3SecretKey.Reveal(), Bucket: cfg.S3Bucket,
	})
	if !originals.Configured() {
		return errors.New("fixtures: os originais da Conciliação vão para o MinIO: defina S3_ENDPOINT, S3_BUCKET, S3_ACCESS_KEY e S3_SECRET_KEY " +
			"(o make demo-fixtures usa o MinIO de dev em localhost)")
	}
	online, err := jobs.NewService(pool, sysClock).WorkerOnline(ctx)
	if err != nil {
		return err
	}
	if !online {
		return errors.New("fixtures: o worker não está no ar (ele lê os extratos e comprovantes do cenário): suba os serviços com make dev e rode de novo")
	}
	// O resumo mostra a pasta dos extratos pelo caminho completo, para quem vai enviá-los pela tela.
	outDir := fixturesOutDir
	if abs, err := filepath.Abs(fixturesOutDir); err == nil {
		outDir = abs
	}
	rep, err := fixtures.Load(ctx, fixtures.Deps{
		Pool: pool, Clock: clk, Ops: sysClock, Originals: originals, Queue: jobs.NewQueue(sysClock, cfg.JobMaxAttempts),
		Matching: reconcile.OptionsFrom(cfg.FuzzyThreshold), Logger: slog.Default(),
	}, fixtures.Options{ScenarioDir: fixturesScenarioDir, PgdasTemplates: fixturesPgdasDir, OutDir: outDir}, stdout)
	if err != nil {
		return fmt.Errorf("fixtures: %w", err)
	}
	rep.Write(stdout, outDir)
	return nil
}

func newLogger(cfg config.Config, w io.Writer) *slog.Logger {
	opts := &slog.HandlerOptions{Level: cfg.LogLevel}
	if cfg.IsProduction() {
		return slog.New(slog.NewJSONHandler(w, opts))
	}
	return slog.New(slog.NewTextHandler(w, opts))
}

func newClock(cfg config.Config) clock.Clock {
	if cfg.FixedTime != nil {
		return clock.NewFixed(*cfg.FixedTime, cfg.Location)
	}
	return clock.NewSystem(cfg.Location)
}

func serve(ctx context.Context, cfg config.Config, logger *slog.Logger) error {
	if cfg.MigrateOnBoot {
		if err := migrate.Up(ctx, cfg.DatabaseURL, io.Discard); err != nil {
			return err
		}
		logger.Info("migrations aplicadas no boot")
	}
	pool, err := db.NewPool(ctx, cfg.DatabaseURL, cfg.DBMaxConns)
	if err != nil {
		return err
	}
	defer pool.Close()

	clk := newClock(cfg)
	application := app.New(app.Deps{Config: cfg, Pool: pool, Clock: clk, Logger: logger})
	application.Boot(ctx)
	handler := application.Handler

	srv := &http.Server{
		Addr:              ":" + strconv.Itoa(cfg.HTTPPort),
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("api iniciada", "port", cfg.HTTPPort, "env", cfg.Env, "timezone", cfg.TimezoneName)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
	}

	logger.Info("encerrando a api")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}
