// Comando api: servidor HTTP da aplicação e utilitários operacionais.
//
//	api                               inicia o servidor (padrão)
//	api serve                         inicia o servidor
//	api worker                        inicia o worker dos jobs assíncronos (processo separado)
//	api migrate up|down|status|reset  gerencia as migrations embutidas
//	api seed [--demo]                 carrega dados de referência (e de demonstração)
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
	"strconv"
	"syscall"
	"time"
	_ "time/tzdata" // fusos embutidos (imagem final mínima não tem tzdata)

	"controle/internal/ai/langfuse"
	"controle/internal/ai/tracer"
	"controle/internal/app"
	"controle/internal/factorr/agents"
	"controle/internal/factorr/inbox"
	"controle/internal/jobs"
	"controle/internal/platform/clock"
	"controle/internal/platform/config"
	"controle/internal/platform/db"
	"controle/internal/platform/migrate"
	"controle/internal/platform/pdftext"
	"controle/internal/worker"
	"controle/seed"
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
	runner := jobs.NewRunner(pool, sysClock, logger, jobs.RunnerConfig{
		WorkerID: cfg.WorkerID, Concurrency: cfg.WorkerConcurrency, PollInterval: cfg.JobPollInterval, Lease: cfg.JobLease,
	}, worker.Handlers(worker.HandlerDeps{LangfuseSync: syncer.Handle, PgdasParse: pgdasAgent.HandleParse}))
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
	demo := false
	for _, a := range args {
		switch a {
		case "--demo":
			demo = true
		default:
			return fmt.Errorf("opção de seed desconhecida %q (use: --demo)", a)
		}
	}
	if demo && cfg.IsProduction() {
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
		return seed.Demo(ctx, pool, clk, stdout)
	}
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
