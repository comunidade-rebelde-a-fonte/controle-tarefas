package httpx

import (
	"net/http"
	"regexp"

	"github.com/go-chi/chi/v5"

	"controle/internal/platform/apperr"
)

var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// IsUUID indica se s tem formato de UUID.
func IsUUID(s string) bool { return uuidRe.MatchString(s) }

// PathUUID lê o parâmetro de rota name exigindo formato UUID; caso contrário
// devolve 404 com o código informado (recurso inexistente).
func PathUUID(r *http.Request, name, notFoundCode, notFoundMsg string) (string, error) {
	v := chi.URLParam(r, name)
	if !IsUUID(v) {
		return "", apperr.NotFound(notFoundCode, notFoundMsg)
	}
	return v, nil
}
