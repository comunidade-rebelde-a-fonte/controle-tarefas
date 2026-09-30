package user

import (
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"controle/internal/platform/apperr"
	"controle/internal/platform/db"
	"controle/internal/platform/httpx"
	"controle/internal/platform/opt"
	"controle/internal/platform/validate"
	"controle/internal/store"
)

// DTO de resposta.
type User struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Email     *string   `json:"email"`
	Role      string    `json:"role"`
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type createRequest struct {
	Name  string  `json:"name" validate:"notblank,max=120"`
	Email *string `json:"email" validate:"omitempty,email,max=200"`
	Role  string  `json:"role" validate:"omitempty,oneof=ADMIN MANAGER OPERATOR READ_ONLY"`
}

type updateRequest struct {
	Name   *string              `json:"name" validate:"omitempty,notblank,max=120"`
	Email  opt.Optional[string] `json:"email"`
	Role   *string              `json:"role" validate:"omitempty,oneof=ADMIN MANAGER OPERATOR READ_ONLY"`
	Active *bool                `json:"active"`
}

// Handler expõe /users (responsáveis). Não há endpoint de exclusão [RN-008].
type Handler struct {
	q *store.Queries
}

func NewHandler(pool *pgxpool.Pool) *Handler {
	return &Handler{q: store.New(pool)}
}

func (h *Handler) Routes(r chi.Router) {
	r.Get("/users", h.list)
	r.Post("/users", h.create)
	r.Get("/users/{id}", h.get)
	r.Patch("/users/{id}", h.update)
}

func toDTO(u store.User) User {
	return User{ID: u.ID, Name: u.Name, Email: u.Email, Role: u.Role, Active: u.Active, CreatedAt: u.CreatedAt, UpdatedAt: u.UpdatedAt}
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	var active *bool
	if raw := r.URL.Query().Get("active"); raw != "" {
		b, err := strconv.ParseBool(raw)
		if err != nil {
			httpx.WriteError(w, r, validate.Field("active", "Use true ou false."))
			return
		}
		active = &b
	}
	rows, err := h.q.ListUsers(r.Context(), active)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	out := make([]User, len(rows))
	for i, u := range rows {
		out[i] = toDTO(u)
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": out})
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "id", "USER_NOT_FOUND", "Usuário não encontrado.")
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	u, err := h.q.GetUser(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, r, mapErr(err))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toDTO(u))
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if err := validate.Struct(req); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	role := req.Role
	if role == "" {
		role = "OPERATOR"
	}
	u, err := h.q.CreateUser(r.Context(), store.CreateUserParams{
		Name:  strings.TrimSpace(req.Name),
		Email: normalizeEmail(req.Email),
		Role:  role,
	})
	if err != nil {
		httpx.WriteError(w, r, mapErr(err))
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, toDTO(u))
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	var req updateRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if err := validate.Struct(req); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	id, err := httpx.PathUUID(r, "id", "USER_NOT_FOUND", "Usuário não encontrado.")
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if req.Email.Set && req.Email.Value != nil && strings.TrimSpace(*req.Email.Value) != "" && !emailRe.MatchString(strings.TrimSpace(*req.Email.Value)) {
		httpx.WriteError(w, r, validate.Field("email", "E-mail inválido."))
		return
	}
	params := store.UpdateUserParams{ID: id, Role: req.Role, Active: req.Active}
	if req.Name != nil {
		n := strings.TrimSpace(*req.Name)
		params.Name = &n
	}
	if req.Email.Set {
		params.SetEmail = true
		params.Email = normalizeEmail(req.Email.Value)
	}
	if params.ID == AdminID && req.Active != nil && !*req.Active {
		httpx.WriteError(w, r, apperr.Unprocessable("ADMIN_REQUIRED", "O usuário Administrador não pode ser inativado."))
		return
	}
	u, err := h.q.UpdateUser(r.Context(), params)
	if err != nil {
		httpx.WriteError(w, r, mapErr(err))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toDTO(u))
}

var emailRe = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

func normalizeEmail(e *string) *string {
	if e == nil {
		return nil
	}
	v := strings.ToLower(strings.TrimSpace(*e))
	if v == "" {
		return nil
	}
	return &v
}

func mapErr(err error) error {
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return apperr.NotFound("USER_NOT_FOUND", "Usuário não encontrado.")
	case db.IsConstraint(err, db.UniqueViolation, "users_email_uq"):
		return apperr.Conflict("EMAIL_CONFLICT", "Já existe um usuário com este e-mail.")
	case db.IsInvalidText(err):
		return apperr.NotFound("USER_NOT_FOUND", "Usuário não encontrado.")
	}
	return err
}
