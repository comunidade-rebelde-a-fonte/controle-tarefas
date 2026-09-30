package httpx

import (
	"context"
	"net/http"
	"time"
)

// Pinger é satisfeito por *pgxpool.Pool.
type Pinger interface {
	Ping(ctx context.Context) error
}

// HealthResponse é a resposta de GET /api/v1/health.
type HealthResponse struct {
	Status   string `json:"status"`
	Database string `json:"database"`
}

// Health responde 200 com o banco acessível e 503 caso contrário.
func Health(db Pinger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := db.Ping(ctx); err != nil {
			logger(r).Warn("health: banco indisponível", "error", err)
			WriteJSON(w, http.StatusServiceUnavailable, HealthResponse{Status: "unavailable", Database: "unavailable"})
			return
		}
		WriteJSON(w, http.StatusOK, HealthResponse{Status: "ok", Database: "ok"})
	}
}

// NotFoundHandler e MethodNotAllowedHandler usam o envelope padrão.
func NotFoundHandler(w http.ResponseWriter, _ *http.Request) {
	WriteJSON(w, http.StatusNotFound, ErrorBody{Code: "NOT_FOUND", Message: "Recurso não encontrado."})
}

func MethodNotAllowedHandler(w http.ResponseWriter, _ *http.Request) {
	WriteJSON(w, http.StatusMethodNotAllowed, ErrorBody{Code: "METHOD_NOT_ALLOWED", Message: "Método não permitido."})
}
