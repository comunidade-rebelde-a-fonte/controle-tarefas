package httpx

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"
	"time"
)

type ctxKey int

const (
	requestIDKey ctxKey = iota
	loggerKey
)

// RequestIDHeader é o cabeçalho de correlação das requisições.
const RequestIDHeader = "X-Request-ID"

// RequestIDFromContext devolve o request-id da requisição corrente.
func RequestIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}

func logger(r *http.Request) *slog.Logger {
	if l, ok := r.Context().Value(loggerKey).(*slog.Logger); ok {
		return l
	}
	return slog.Default()
}

// Logger devolve o logger da requisição (já com o request-id).
func Logger(r *http.Request) *slog.Logger { return logger(r) }

// RequestID reaproveita um X-Request-ID válido recebido ou gera um novo.
func RequestID(base *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := r.Header.Get(RequestIDHeader)
			if !validRequestID(id) {
				id = newRequestID()
			}
			w.Header().Set(RequestIDHeader, id)
			ctx := context.WithValue(r.Context(), requestIDKey, id)
			ctx = context.WithValue(ctx, loggerKey, base.With("request_id", id))
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func validRequestID(id string) bool {
	if id == "" || len(id) > 64 {
		return false
	}
	for _, c := range id {
		valid := c == '-' || c == '_' || (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
		if !valid {
			return false
		}
	}
	return true
}

func newRequestID() string {
	var b [12]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// statusRecorder captura o status da resposta para o log de acesso.
type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	n, err := s.ResponseWriter.Write(b)
	s.bytes += n
	return n, err
}

// Unwrap permite que http.ResponseController alcance o writer original.
func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

// AccessLog registra método, rota, status e duração. Nunca registra corpo
// de requisição/resposta nem query string (podem conter dados sensíveis).
func AccessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)
		status := rec.status
		if status == 0 {
			status = http.StatusOK
		}
		level := slog.LevelInfo
		if status >= 500 {
			level = slog.LevelError
		}
		logger(r).Log(r.Context(), level, "http",
			"method", r.Method,
			"path", r.URL.Path,
			"status", status,
			"bytes", rec.bytes,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}

// Recoverer converte panics em 500 com o envelope padrão.
func Recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				if err, ok := rec.(error); ok && errors.Is(err, http.ErrAbortHandler) {
					panic(rec)
				}
				logger(r).Error("panic", "panic", rec, "stack", string(debug.Stack()))
				WriteJSON(w, http.StatusInternalServerError, ErrorBody{
					Code:    "INTERNAL_ERROR",
					Message: "Erro interno. Tente novamente em instantes.",
				})
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// CORS permite somente as origens listadas explicitamente [PRD §45.3].
// Requisições de origens não listadas não recebem cabeçalhos CORS e
// preflights delas são recusados com 403.
func CORS(allowed []string) func(http.Handler) http.Handler {
	origins := slices.Clone(allowed)
	methods := "GET, POST, PUT, PATCH, DELETE, OPTIONS"
	headers := "Content-Type, Accept, X-Request-ID"
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin == "" {
				next.ServeHTTP(w, r)
				return
			}
			w.Header().Add("Vary", "Origin")
			ok := slices.Contains(origins, origin)
			preflight := r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != ""
			if !ok {
				if preflight {
					WriteJSON(w, http.StatusForbidden, ErrorBody{Code: "CORS_FORBIDDEN", Message: "Origem não permitida."})
					return
				}
				next.ServeHTTP(w, r)
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Expose-Headers", "X-Request-ID, Content-Disposition")
			if preflight {
				w.Header().Set("Access-Control-Allow-Methods", methods)
				w.Header().Set("Access-Control-Allow-Headers", headers)
				w.Header().Set("Access-Control-Max-Age", strconv.Itoa(600))
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// NormalizeOrigins remove barras finais das origens configuradas.
func NormalizeOrigins(in []string) []string {
	out := make([]string, 0, len(in))
	for _, o := range in {
		out = append(out, strings.TrimRight(o, "/"))
	}
	return out
}
