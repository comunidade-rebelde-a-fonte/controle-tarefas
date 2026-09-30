// Package user gerencia usuários/responsáveis e o usuário implícito do MVP.
//
// Não há autenticação no MVP [PRD §34.1]: o middleware injeta o usuário
// "Administrador" (UUID fixo) em toda requisição. Todas as escritas registram
// o usuário do contexto, o que prepara a entrada futura de autenticação sem
// refatoração [plan D9].
package user

import (
	"context"
	"net/http"
)

// AdminID é o UUID fixo do usuário implícito "Administrador" (migration 0002).
const AdminID = "00000000-0000-0000-0000-000000000001"

type ctxKey struct{}

// WithUser devolve um contexto com o usuário informado.
func WithUser(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, ctxKey{}, userID)
}

// FromContext devolve o usuário da requisição corrente (o Administrador no MVP).
func FromContext(ctx context.Context) string {
	if id, ok := ctx.Value(ctxKey{}).(string); ok && id != "" {
		return id
	}
	return AdminID
}

// Middleware injeta o usuário implícito no contexto de toda requisição.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(WithUser(r.Context(), AdminID)))
	})
}
