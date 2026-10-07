// Package worker é o processo dos jobs assíncronos [spec 04 §7]: roda separado da API (subcomando
// `api worker`, com container próprio no docker-compose.yml) e atende um healthcheck HTTP próprio.
//
// Nesta etapa (TASK-003) o worker sobe, confere o banco pelo healthcheck e encerra de forma
// ordenada. A fila de jobs em PostgreSQL (SELECT ... FOR UPDATE SKIP LOCKED) entra na TASK-005.
package worker

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"time"

	"controle/internal/platform/httpx"
)

// Deps são as dependências do processo worker.
type Deps struct {
	ID     string
	DB     httpx.Pinger
	Logger *slog.Logger
}

// Handler é o roteador do healthcheck: GET /health responde 200 com o banco acessível e 503 sem ele,
// no mesmo formato do /api/v1/health da API.
func Handler(d Deps) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /health", httpx.Health(d.DB))
	return mux
}

// Run atende o healthcheck em ln até ctx ser cancelado e então encerra de forma ordenada.
func Run(ctx context.Context, d Deps, ln net.Listener) error {
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
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}
