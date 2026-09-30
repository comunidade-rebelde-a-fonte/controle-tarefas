package importer

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"controle/internal/company"
	"controle/internal/platform/apperr"
	"controle/internal/platform/civil"
	"controle/internal/platform/cnpj"
	"controle/internal/platform/validate"
)

// Classificação de cada linha na prévia [PRD §35.5]. INVALID_VALUE cobre valores que não
// cabem nas outras classes (data, lista, tamanho, CNPJ repetido no arquivo).
const (
	StatusNew             = "NEW"
	StatusExisting        = "EXISTING"
	StatusInvalidCnpj     = "INVALID_CNPJ"
	StatusDuplicateCode   = "DUPLICATE_CODE"
	StatusMissingRequired = "MISSING_REQUIRED"
	StatusInvalidValue    = "INVALID_VALUE"
)

// StatusLabels são os rótulos da prévia.
var StatusLabels = map[string]string{
	StatusNew:             "Nova empresa",
	StatusExisting:        "Empresa existente",
	StatusInvalidCnpj:     "CNPJ inválido",
	StatusDuplicateCode:   "Código duplicado",
	StatusMissingRequired: "Campo obrigatório ausente",
	StatusInvalidValue:    "Valor inválido",
}

// Importable indica se a linha pode ser gravada (existentes só com confirmação).
func Importable(status string) bool { return status == StatusNew || status == StatusExisting }

// ExistingCompany é uma empresa já cadastrada, para conciliação.
type ExistingCompany struct {
	ID           string
	InternalCode string
	Cnpj         string
	LegalName    string
}

// Lookup consulta o cadastro atual (empresas por CNPJ e código, responsáveis por nome ou e-mail).
type Lookup interface {
	CompanyByCnpj(cnpj string) (ExistingCompany, bool)
	CompanyByCode(code string) (ExistingCompany, bool)
	UserID(nameOrEmail string) (string, bool)
}

// FieldError é um erro de uma coluna da linha.
type FieldError struct {
	Field   string `json:"field"`
	Column  string `json:"column"`
	Message string `json:"message"`
}

// Row é o resultado da validação de uma linha.
type Row struct {
	Line         int                 `json:"line"`
	Status       string              `json:"status"`
	StatusLabel  string              `json:"status_label"`
	Errors       []FieldError        `json:"errors"`
	InternalCode string              `json:"internal_code"`
	LegalName    string              `json:"legal_name"`
	Cnpj         string              `json:"cnpj"`
	ExistingID   *string             `json:"existing_company_id"`
	Input        company.CreateInput `json:"-"`
	Active       bool                `json:"-"`
}

var columnIndex = func() map[string]int {
	m := make(map[string]int, len(Columns))
	for i, c := range Columns {
		m[c.Key] = i
	}
	return m
}()

var headerOf = func() map[string]string {
	m := make(map[string]string, len(Columns))
	for _, c := range Columns {
		m[c.Key] = c.Header
	}
	return m
}()

type rowCheck struct {
	missing, invalidCnpj, dupCode, invalid bool
	errs                                   []FieldError
}

// structFieldColumn liga os campos do cadastro (erros do validate) às colunas da planilha.
var structFieldColumn = map[string]string{
	"fiscal_responsible_user_id":       "responsible_fiscal",
	"accounting_responsible_user_id":   "responsible_accounting",
	"payroll_responsible_user_id":      "responsible_payroll",
	"declarations_responsible_user_id": "responsible_declarations",
}

func (c *rowCheck) add(field, msg string) {
	if col, ok := structFieldColumn[field]; ok {
		field = col
	}
	c.errs = append(c.errs, FieldError{Field: field, Column: headerOf[field], Message: msg})
}

// option aceita o rótulo (sem acento/caixa) ou o código.
func option(opts []Option, v string) (string, bool) {
	n := normalizeHeader(v)
	for _, o := range opts {
		if n == normalizeHeader(o.Label) || strings.EqualFold(v, o.Code) {
			return o.Code, true
		}
	}
	return "", false
}

func yesNo(v string) (bool, bool) {
	switch normalizeHeader(v) {
	case "sim", "s", "true", "1", "x":
		return true, true
	case "nao", "n", "false", "0":
		return false, true
	}
	return false, false
}

// excelEpoch é o dia 0 das datas seriais do Excel (sistema 1900, com o ajuste de 1900 bissexto).
var excelEpoch = time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)

// parseDate aceita dd/mm/aaaa, aaaa-mm-dd e o número serial de datas do Excel.
func parseDate(v string) (civil.Date, bool) {
	for _, layout := range []string{"02/01/2006", "2/1/2006", time.DateOnly} {
		if t, err := time.Parse(layout, v); err == nil {
			return civil.Of(t), true
		}
	}
	if n, err := strconv.ParseFloat(v, 64); err == nil && n >= 1 && n < 2958466 && n == float64(int(n)) {
		return civil.Of(excelEpoch.AddDate(0, 0, int(n))), true
	}
	return civil.Date{}, false
}

var moduleKeys = []struct{ key, module string }{
	{"module_fiscal", company.ModuleFiscal},
	{"module_declarations", company.ModuleDeclarations},
	{"module_accounting", company.ModuleAccounting},
	{"module_payroll", company.ModulePayroll},
	{"module_credentials", company.ModuleCredentials},
}

func ptr(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

// Classify valida e classifica as linhas: obrigatórios, CNPJ (dígitos verificadores),
// código único (no cadastro e no arquivo), CNPJ repetido no arquivo e valores de lista,
// data e tamanho [PRD §35.3-§35.5]. Empresa com o mesmo CNPJ é "existente".
func Classify(records []Record, lk Lookup) []Row {
	out := make([]Row, 0, len(records))
	firstCnpj := map[string]int{}
	firstCode := map[string]int{}
	for _, rec := range records {
		v := rec.Values
		var c rowCheck
		in := company.CreateInput{
			InternalCode: v["internal_code"], LegalName: v["legal_name"], TradeName: ptr(v["trade_name"]),
			City: ptr(v["city"]), StateRegistration: ptr(v["state_registration"]), MunicipalRegistration: ptr(v["municipal_registration"]),
			Notes: ptr(v["notes"]),
		}
		row := Row{Line: rec.Line, InternalCode: v["internal_code"], LegalName: v["legal_name"], Active: true}

		for _, col := range Columns {
			if col.Required && v[col.Key] == "" {
				c.missing = true
				c.add(col.Key, "Preencha "+col.Header+".")
			}
		}

		if raw := v["cnpj"]; raw != "" {
			digits := cnpj.Sanitize(raw)
			row.Cnpj = digits
			in.Cnpj = digits
			if !cnpj.Valid(digits) {
				c.invalidCnpj = true
				c.add("cnpj", "CNPJ inválido: confira os 14 dígitos e os dígitos verificadores.")
			} else if line, dup := firstCnpj[digits]; dup {
				c.invalid = true
				c.add("cnpj", fmt.Sprintf("CNPJ repetido no arquivo (linha %d).", line))
			} else {
				firstCnpj[digits] = rec.Line
			}
		}

		var existing *ExistingCompany
		if row.Cnpj != "" && !c.invalidCnpj {
			if e, ok := lk.CompanyByCnpj(row.Cnpj); ok {
				existing = &e
				row.ExistingID = &e.ID
			}
		}

		if code := v["internal_code"]; code != "" {
			key := strings.ToLower(code)
			if line, dup := firstCode[key]; dup {
				c.dupCode = true
				c.add("internal_code", fmt.Sprintf("Código repetido no arquivo (linha %d).", line))
			} else {
				firstCode[key] = rec.Line
			}
			if other, ok := lk.CompanyByCode(code); ok && (existing == nil || other.ID != existing.ID) {
				c.dupCode = true
				c.add("internal_code", fmt.Sprintf("Código já usado por %s.", other.LegalName))
			}
		}

		if s := v["tax_regime"]; s != "" {
			if code, ok := option(RegimeOptions, s); ok {
				in.TaxRegime = code
			} else {
				c.invalid = true
				c.add("tax_regime", "Regime inválido: use Simples Nacional, Lucro Presumido, Lucro Real, Lucro Arbitrado ou Imune/Isenta.")
			}
		}
		if s := v["payroll_type"]; s != "" {
			if code, ok := option(PayrollOptions, s); ok {
				in.PayrollType = code
			} else {
				c.invalid = true
				c.add("payroll_type", "Tipo de folha inválido: use Sem folha, Pró-labore ou Folha.")
			}
		}
		if s := v["service_start_date"]; s != "" {
			if d, ok := parseDate(s); ok {
				in.ServiceStartDate = d
			} else {
				c.invalid = true
				c.add("service_start_date", "Data inválida: use dd/mm/aaaa.")
			}
		}
		if s := v["service_end_date"]; s != "" {
			if d, ok := parseDate(s); !ok {
				c.invalid = true
				c.add("service_end_date", "Data inválida: use dd/mm/aaaa.")
			} else if !in.ServiceStartDate.IsZero() && d.Before(in.ServiceStartDate.Time) {
				c.invalid = true
				c.add("service_end_date", "O encerramento não pode ser anterior ao início do atendimento.")
			} else {
				in.ServiceEndDate = &d
			}
		}
		if s := v["state"]; s != "" {
			uf := strings.ToUpper(s)
			if slices.Contains(UFs, uf) {
				in.State = &uf
			} else {
				c.invalid = true
				c.add("state", "UF inválida: use a sigla de um estado (ex.: PR).")
			}
		}
		if s := v["active"]; s != "" {
			if a, ok := option(ActiveOptions, s); ok {
				row.Active = a == "true"
			} else {
				c.invalid = true
				c.add("active", "Situação inválida: use Ativa ou Inativa.")
			}
		}
		if s := v["factor_r"]; s != "" {
			if b, ok := yesNo(s); ok {
				in.FactorR = &b
			} else {
				c.invalid = true
				c.add("factor_r", "Fator R inválido: use Sim ou Não.")
			}
		}
		modules := []string{}
		for _, m := range moduleKeys {
			if s := v[m.key]; s != "" {
				b, ok := yesNo(s)
				if !ok {
					c.invalid = true
					c.add(m.key, "Use Sim ou Não.")
				} else if b {
					modules = append(modules, m.module)
				}
			}
		}
		in.Modules = &modules
		for key, target := range map[string]**string{
			"responsible_fiscal":       &in.FiscalResponsibleUserID,
			"responsible_accounting":   &in.AccountingResponsibleUserID,
			"responsible_payroll":      &in.PayrollResponsibleUserID,
			"responsible_declarations": &in.DeclarationsResponsibleUserID,
		} {
			if s := v[key]; s != "" {
				if id, ok := lk.UserID(s); ok {
					*target = &id
				} else {
					c.invalid = true
					c.add(key, "Responsável não encontrado: cadastre-o em Configurações ou deixe em branco.")
				}
			}
		}

		// Mesmas regras do cadastro manual (tamanhos, UF etc.), sem repetir erros já apontados.
		if len(c.errs) == 0 {
			if err := validate.Struct(in); err != nil {
				var ae *apperr.Error
				if errors.As(err, &ae) {
					if fields, ok := ae.Details["fields"].(map[string]any); ok {
						for f, msg := range fields {
							c.invalid = true
							c.add(f, fmt.Sprint(msg))
						}
					}
				}
			}
		}

		row.Input = in
		slices.SortStableFunc(c.errs, func(a, b FieldError) int { return columnIndex[a.Field] - columnIndex[b.Field] })
		row.Errors = c.errs
		if row.Errors == nil {
			row.Errors = []FieldError{}
		}
		switch {
		case c.missing:
			row.Status = StatusMissingRequired
		case c.invalidCnpj:
			row.Status = StatusInvalidCnpj
		case c.dupCode:
			row.Status = StatusDuplicateCode
		case c.invalid:
			row.Status = StatusInvalidValue
		case existing != nil:
			row.Status = StatusExisting
		default:
			row.Status = StatusNew
		}
		row.StatusLabel = StatusLabels[row.Status]
		out = append(out, row)
	}
	return out
}
