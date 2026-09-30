// Package apperr define erros de domínio tipados, convertidos pelo pacote
// httpx no envelope padrão {code, message, details} [AGENTS §8.4].
//
// A mensagem (Message) é exibida ao usuário e fica em português.
package apperr

import (
	"errors"
	"fmt"
	"net/http"
)

// Error é um erro de domínio com status HTTP associado.
type Error struct {
	Status  int
	Code    string
	Message string
	Details map[string]any
	cause   error
}

func (e *Error) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.cause)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *Error) Unwrap() error { return e.cause }

// WithDetails devolve uma cópia do erro com detalhes adicionais.
func (e *Error) WithDetails(details map[string]any) *Error {
	c := *e
	c.Details = details
	return &c
}

// Wrap devolve uma cópia do erro que referencia a causa original (para logs).
func (e *Error) Wrap(cause error) *Error {
	c := *e
	c.cause = cause
	return &c
}

// New cria um erro de domínio.
func New(status int, code, message string) *Error {
	return &Error{Status: status, Code: code, Message: message}
}

// BadRequest (400): payload inválido.
func BadRequest(code, message string) *Error { return New(http.StatusBadRequest, code, message) }

// NotFound (404): recurso não encontrado.
func NotFound(code, message string) *Error { return New(http.StatusNotFound, code, message) }

// Conflict (409): conflito de versão, unicidade ou alerta que exige confirmação.
func Conflict(code, message string) *Error { return New(http.StatusConflict, code, message) }

// Gone (410): recurso expirado (ex.: token de importação).
func Gone(code, message string) *Error { return New(http.StatusGone, code, message) }

// Unprocessable (422): regra de negócio violada.
func Unprocessable(code, message string) *Error {
	return New(http.StatusUnprocessableEntity, code, message)
}

// Locked (423): período fechado.
func Locked(code, message string) *Error { return New(http.StatusLocked, code, message) }

// As extrai um *Error de err, se houver.
func As(err error) (*Error, bool) {
	var e *Error
	if errors.As(err, &e) {
		return e, true
	}
	return nil, false
}

// VersionConflict é o erro padrão de concorrência otimista [plan D8].
func VersionConflict() *Error {
	return Conflict("VERSION_CONFLICT", "O registro foi alterado por outra operação. Recarregue e tente novamente.")
}
