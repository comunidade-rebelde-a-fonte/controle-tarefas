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

	"controle/internal/app"
	"controle/internal/jobs"
	"controle/internal/platform/clock"
	"controle/internal/platform/config"
	"controle/internal/platform/db"
	"controle/internal/platform/migrate"
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
	runner := jobs.NewRunner(pool, clock.NewSystem(cfg.Location), logger, jobs.RunnerConfig{
		WorkerID: cfg.WorkerID, Concurrency: cfg.WorkerConcurrency, PollInterval: cfg.JobPollInterval, Lease: cfg.JobLease,
	}, worker.Handlers())
	return worker.Run(ctx, worker.Deps{ID: cfg.WorkerID, DB: pool, Logger: logger, Jobs: runner}, ln)
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
