// Package opt oferece Optional[T] para PATCH: distingue campo ausente de null.
package opt

import (
	"bytes"
	"encoding/json"
)

// Optional registra se o campo veio no JSON (Set) e seu valor (nil = null).
type Optional[T any] struct {
	Set   bool
	Value *T
}

// UnmarshalJSON marca o campo como presente; "null" limpa o valor.
func (o *Optional[T]) UnmarshalJSON(b []byte) error {
	o.Set = true
	if bytes.Equal(bytes.TrimSpace(b), []byte("null")) {
		o.Value = nil
		return nil
	}
	var v T
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	o.Value = &v
	return nil
}

// Of cria um Optional presente com valor v (útil em testes).
func Of[T any](v T) Optional[T] { return Optional[T]{Set: true, Value: &v} }

// Null cria um Optional presente com valor nulo.
func Null[T any]() Optional[T] { return Optional[T]{Set: true} }

// MarshalJSON serializa o valor (null quando ausente ou nulo).
func (o Optional[T]) MarshalJSON() ([]byte, error) {
	if o.Value == nil {
		return []byte("null"), nil
	}
	return json.Marshal(*o.Value)
}
