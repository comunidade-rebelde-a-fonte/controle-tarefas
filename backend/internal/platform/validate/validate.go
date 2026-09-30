// Package validate valida payloads com go-playground/validator e traduz as
// falhas para o envelope padrão (400 VALIDATION_ERROR, mensagens em português).
package validate

import (
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"sync"

	"github.com/go-playground/validator/v10"

	"controle/internal/platform/apperr"
)

var (
	once     sync.Once
	instance *validator.Validate

	competenceRe = regexp.MustCompile(`^[0-9]{4}-(0[1-9]|1[0-2])$`)
	ufRe         = regexp.MustCompile(`^[A-Z]{2}$`)
)

func get() *validator.Validate {
	once.Do(func() {
		v := validator.New(validator.WithRequiredStructEnabled())
		v.RegisterTagNameFunc(func(f reflect.StructField) string {
			name := strings.SplitN(f.Tag.Get("json"), ",", 2)[0]
			if name == "-" || name == "" {
				return f.Name
			}
			return name
		})
		_ = v.RegisterValidation("competence", func(fl validator.FieldLevel) bool {
			return competenceRe.MatchString(fl.Field().String())
		})
		_ = v.RegisterValidation("uf", func(fl validator.FieldLevel) bool {
			return ufRe.MatchString(fl.Field().String())
		})
		_ = v.RegisterValidation("notblank", func(fl validator.FieldLevel) bool {
			return strings.TrimSpace(fl.Field().String()) != ""
		})
		instance = v
	})
	return instance
}

// Struct valida s. Devolve nil ou um *apperr.Error 400 com details.fields.
func Struct(s any) error {
	err := get().Struct(s)
	if err == nil {
		return nil
	}
	var verrs validator.ValidationErrors
	if !errors.As(err, &verrs) {
		return apperr.BadRequest("VALIDATION_ERROR", "Dados inválidos.").Wrap(err)
	}
	fields := make(map[string]any, len(verrs))
	for _, fe := range verrs {
		fields[fieldPath(fe)] = message(fe)
	}
	return apperr.BadRequest("VALIDATION_ERROR", "Dados inválidos. Verifique os campos destacados.").
		WithDetails(map[string]any{"fields": fields})
}

// fieldPath remove o nome da struct raiz do namespace (ex.: "companyCreate.cnpj" → "cnpj").
func fieldPath(fe validator.FieldError) string {
	ns := fe.Namespace()
	if i := strings.Index(ns, "."); i >= 0 {
		return ns[i+1:]
	}
	return fe.Field()
}

func message(fe validator.FieldError) string {
	switch fe.Tag() {
	case "required", "required_if", "required_with", "notblank":
		return "Campo obrigatório."
	case "max":
		if fe.Kind() == reflect.String {
			return fmt.Sprintf("Máximo de %s caracteres.", fe.Param())
		}
		return fmt.Sprintf("Valor máximo: %s.", fe.Param())
	case "min":
		if fe.Kind() == reflect.String {
			return fmt.Sprintf("Mínimo de %s caracteres.", fe.Param())
		}
		return fmt.Sprintf("Valor mínimo: %s.", fe.Param())
	case "oneof":
		return "Valor deve ser um de: " + strings.ReplaceAll(fe.Param(), " ", ", ") + "."
	case "email":
		return "E-mail inválido."
	case "competence":
		return "Competência inválida (use AAAA-MM)."
	case "uf":
		return "UF inválida (use a sigla com 2 letras maiúsculas)."
	case "uuid", "uuid4":
		return "Identificador inválido."
	case "datetime":
		return "Data inválida (use AAAA-MM-DD)."
	case "gtfield", "gtefield":
		return "Valor deve ser posterior ao campo " + fe.Param() + "."
	default:
		return "Valor inválido."
	}
}

// Field devolve um erro de validação para um único campo.
func Field(field, msg string) *apperr.Error {
	return apperr.BadRequest("VALIDATION_ERROR", "Dados inválidos. Verifique os campos destacados.").
		WithDetails(map[string]any{"fields": map[string]any{field: msg}})
}
