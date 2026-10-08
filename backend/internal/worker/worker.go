// Package worker é o processo dos jobs assíncronos [spec 04 §7]: roda separado da API (subcomando
// `api worker`, com container próprio no docker-compose.yml) e atende um healthcheck HTTP próprio.
//
// Desde a TASK-005 o worker consome a fila de jobs em PostgreSQL (pacote jobs: SELECT ... FOR
// UPDATE SKIP LOCKED, tentativas, backoff, posse com prazo) com os handlers de Handlers().
package worker

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"time"

	"controle/internal/jobs"
	"controle/internal/platform/httpx"
)

// Deps são as dependências do processo worker.
type Deps struct {
	ID     string
	DB     httpx.Pinger
	Logger *slog.Logger
	// Jobs consome a fila; nil = só o healthcheck.
	Jobs *jobs.Runner
}

// Handler é o roteador do healthcheck: GET /health responde 200 com o banco acessível e 503 sem ele,
// no mesmo formato do /api/v1/health da API.
func Handler(d Deps) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /health", httpx.Health(d.DB))
	return mux
}

// Run atende o healthcheck em ln e consome a fila até ctx ser cancelado; então encerra de forma
// ordenada, esperando os jobs em andamento pelo prazo de encerramento da fila.
func Run(ctx context.Context, d Deps, ln net.Listener) error {
	jobsDone := make(chan struct{})
	if d.Jobs != nil {
		go func() {
			defer close(jobsDone)
			_ = d.Jobs.Run(ctx)
		}()
	} else {
		close(jobsDone)
	}

	srv := &http.Server{
		Handler:           Handler(d),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		d.Logger.Info("worker iniciado", "worker_id", d.ID, "health", ln.Addr().String())
		errCh <- srv.Serve(ln)
	}()

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
	}

	d.Logger.Info("encerrando o worker", "worker_id", d.ID)
	<-jobsDone
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}
