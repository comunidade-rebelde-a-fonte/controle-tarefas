package user_test

import (
	"net/http"
	"testing"

	"controle/internal/testutil"
	"controle/internal/user"
)

type userDTO struct {
	ID     string  `json:"id"`
	Name   string  `json:"name"`
	Email  *string `json:"email"`
	Role   string  `json:"role"`
	Active bool    `json:"active"`
}

func TestUsers_CreateListUpdate(t *testing.T) {
	srv := testutil.NewServer(t, testutil.NewDB(t), testutil.FixedClock(t, "2026-10-01T12:00:00Z"))

	var created userDTO
	srv.Post("/api/v1/users", map[string]any{"name": "  Maria Fiscal ", "email": "Maria@Escritorio.Test"}).
		MustStatus(t, http.StatusCreated).JSON(t, &created)
	if created.Name != "Maria Fiscal" || created.Role != "OPERATOR" || created.Email == nil || *created.Email != "maria@escritorio.test" {
		t.Fatalf("criado = %+v", created)
	}

	var list struct{ Items []userDTO }
	srv.Get("/api/v1/users").MustStatus(t, http.StatusOK).JSON(t, &list)
	if len(list.Items) != 2 {
		t.Fatalf("usuários = %d, want 2 (Administrador + responsável)", len(list.Items))
	}

	var updated userDTO
	srv.Patch("/api/v1/users/"+created.ID, map[string]any{"email": nil, "active": false}).
		MustStatus(t, http.StatusOK).JSON(t, &updated)
	if updated.Email != nil || updated.Active || updated.Name != "Maria Fiscal" {
		t.Fatalf("atualizado = %+v", updated)
	}

	srv.Get("/api/v1/users?active=true").MustStatus(t, http.StatusOK).JSON(t, &list)
	if len(list.Items) != 1 || list.Items[0].ID != user.AdminID {
		t.Fatalf("ativos = %+v", list.Items)
	}
}

func TestUsers_Validation(t *testing.T) {
	srv := testutil.NewServer(t, testutil.NewDB(t), testutil.FixedClock(t, "2026-10-01T12:00:00Z"))

	if code := srv.Post("/api/v1/users", map[string]any{"name": " "}).MustStatus(t, http.StatusBadRequest).ErrorCode(t); code != "VALIDATION_ERROR" {
		t.Fatalf("code = %s", code)
	}
	srv.Post("/api/v1/users", map[string]any{"name": "X", "role": "ROOT"}).MustStatus(t, http.StatusBadRequest)
	srv.Post("/api/v1/users", map[string]any{"name": "X", "email": "sem-arroba"}).MustStatus(t, http.StatusBadRequest)

	srv.Post("/api/v1/users", map[string]any{"name": "A", "email": "a@x.test"}).MustStatus(t, http.StatusCreated)
	if code := srv.Post("/api/v1/users", map[string]any{"name": "B", "email": "A@x.test"}).MustStatus(t, http.StatusConflict).ErrorCode(t); code != "EMAIL_CONFLICT" {
		t.Fatalf("code = %s", code)
	}

	srv.Get("/api/v1/users/nao-e-uuid").MustStatus(t, http.StatusNotFound)
	srv.Get("/api/v1/users/7b0f8d7e-0000-4000-8000-000000000000").MustStatus(t, http.StatusNotFound)
	srv.Patch("/api/v1/users/"+user.AdminID, map[string]any{"active": false}).MustStatus(t, http.StatusUnprocessableEntity)
}

func TestUsers_NoDeleteEndpoint(t *testing.T) {
	srv := testutil.NewServer(t, testutil.NewDB(t), testutil.FixedClock(t, "2026-10-01T12:00:00Z"))
	srv.Do(http.MethodDelete, "/api/v1/users/"+user.AdminID, nil).MustStatus(t, http.StatusMethodNotAllowed)
}
