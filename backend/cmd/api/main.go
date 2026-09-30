// Comando api: servidor HTTP da aplicação e utilitários operacionais.
//
//	api            inicia o servidor (padrão)
//	api serve      inicia o servidor
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
	_ "time/tzdata" // fusos embutidos (imagem final mínima não tem tzdata)

	"controle/internal/app"
	"controle/internal/platform/clock"
	"controle/internal/platform/config"
	"controle/internal/platform/db"
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
	default:
		_ = args
		return fmt.Errorf("comando desconhecido %q (use: serve)", cmd)
	}
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
	pool, err := db.NewPool(ctx, cfg.DatabaseURL, cfg.DBMaxConns)
	if err != nil {
		return err
	}
	defer pool.Close()

	clk := newClock(cfg)
	handler := app.NewRouter(app.Deps{Config: cfg, Pool: pool, Clock: clk, Logger: logger})

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
