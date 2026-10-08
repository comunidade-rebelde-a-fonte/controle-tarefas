package worker

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

type fakeDB struct{ err error }

func (f fakeDB) Ping(context.Context) error { return f.err }

func discard() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestHandler_HealthReflectsDatabase(t *testing.T) {
	cases := []struct {
		name string
		db   fakeDB
		want int
	}{
		{"banco acessível", fakeDB{}, http.StatusOK},
		{"banco fora", fakeDB{err: errors.New("connection refused")}, http.StatusServiceUnavailable},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			Handler(Deps{ID: "w", DB: c.db, Logger: discard()}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
			if rec.Code != c.want {
				t.Fatalf("status = %d, want %d", rec.Code, c.want)
			}
		})
	}
}

func TestHandler_OnlyHealthIsServed(t *testing.T) {
	rec := httptest.NewRecorder()
	Handler(Deps{ID: "w", DB: fakeDB{}, Logger: discard()}).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/health", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST /health = %d, want 405", rec.Code)
	}
}

func TestRun_ServesUntilContextIsCanceled(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Run(ctx, Deps{ID: "worker-teste", DB: fakeDB{}, Logger: discard()}, ln) }()

	resp, err := http.Get("http://" + ln.Addr().String() + "/health")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("health = %d", resp.StatusCode)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run devolveu %v, want nil no encerramento", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("o worker não encerrou após o cancelamento")
	}
}

// Rotinas periódicas rodam ao iniciar e a cada intervalo; erro não para a rotina; todas
// encerram com o worker.
func TestRun_PeriodicTasks(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	ran := make(chan struct{}, 10)
	task := Task{Name: "teste", Every: 10 * time.Millisecond, Run: func(context.Context) error {
		calls.Add(1)
		ran <- struct{}{}
		return errors.New("falha transitória")
	}}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, Deps{ID: "worker-teste", DB: fakeDB{}, Logger: discard(), Tasks: []Task{task}}, ln)
	}()
	for range 3 {
		select {
		case <-ran:
		case <-time.After(5 * time.Second):
			t.Fatal("a rotina não rodou de novo depois do erro")
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("o worker não encerrou com a rotina em andamento")
	}
	after := calls.Load()
	time.Sleep(30 * time.Millisecond)
	if calls.Load() != after {
		t.Fatal("a rotina continuou depois do encerramento")
	}
}
