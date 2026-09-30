// Package company implementa o cadastro único de empresas, regimes tributários
// com vigência, módulos e overrides de aplicabilidade [PRD §7-§9, §27].
package company

import (
	"slices"
	"time"

	"controle/internal/platform/civil"
)

// Regimes tributários [PRD §8.1]. MEI não existe no MVP.
const (
	RegimeSimples   = "SIMPLES_NACIONAL"
	RegimePresumido = "LUCRO_PRESUMIDO"
	RegimeReal      = "LUCRO_REAL"
	RegimeArbitrado = "LUCRO_ARBITRADO"
	RegimeImune     = "IMUNE_ISENTA"
)

// Regimes lista os regimes válidos.
var Regimes = []string{RegimeSimples, RegimePresumido, RegimeReal, RegimeArbitrado, RegimeImune}

// Agrupamentos do Fiscal [PRD §5.1-§5.2]: "Normal" é agrupamento de interface, não regime.
const (
	GroupNormal  = "NORMAL"
	GroupSimples = "SIMPLES"
)

// RegimeGroup devolve o agrupamento operacional do regime ("" se desconhecido).
func RegimeGroup(regime string) string {
	switch regime {
	case RegimeSimples:
		return GroupSimples
	case RegimePresumido, RegimeReal, RegimeArbitrado, RegimeImune:
		return GroupNormal
	}
	return ""
}

// Módulos contratuais [PRD §7.4]. DCTFWeb não é módulo.
const (
	ModuleFiscal       = "FISCAL"
	ModuleDeclarations = "DECLARATIONS"
	ModuleAccounting   = "ACCOUNTING"
	ModulePayroll      = "PAYROLL"
	ModuleCredentials  = "CREDENTIALS"
)

// Modules lista os módulos contratuais.
var Modules = []string{ModuleFiscal, ModuleDeclarations, ModuleAccounting, ModulePayroll, ModuleCredentials}

// IsModule indica se m é um módulo válido.
func IsModule(m string) bool { return slices.Contains(Modules, m) }

// Tipos de folha [PRD §17.1].
const (
	PayrollNone      = "NO_PAYROLL"
	PayrollProLabore = "PRO_LABORE"
	PayrollFull      = "PAYROLL"
)

// UserRef identifica um responsável.
type UserRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Responsibles são os responsáveis por setor [PRD §34.4].
type Responsibles struct {
	Fiscal       *UserRef `json:"fiscal"`
	Accounting   *UserRef `json:"accounting"`
	Payroll      *UserRef `json:"payroll"`
	Declarations *UserRef `json:"declarations"`
}

// Company é a representação completa da empresa na API.
type Company struct {
	ID                    string       `json:"id"`
	InternalCode          string       `json:"internal_code"`
	LegalName             string       `json:"legal_name"`
	TradeName             *string      `json:"trade_name"`
	Cnpj                  string       `json:"cnpj"`
	City                  *string      `json:"city"`
	State                 *string      `json:"state"`
	StateRegistration     *string      `json:"state_registration"`
	MunicipalRegistration *string      `json:"municipal_registration"`
	ServiceStartDate      civil.Date   `json:"service_start_date"`
	ServiceEndDate        *civil.Date  `json:"service_end_date"`
	Active                bool         `json:"active"`
	PayrollType           string       `json:"payroll_type"`
	FactorR               *bool        `json:"factor_r"`
	Responsibles          Responsibles `json:"responsibles"`
	Notes                 *string      `json:"notes"`
	CurrentTaxRegime      *string      `json:"current_tax_regime"`
	CurrentRegimeGroup    *string      `json:"current_regime_group"`
	ActiveModules         []string     `json:"active_modules"`
	Version               int32        `json:"version"`
	CreatedAt             *time.Time   `json:"created_at,omitempty"`
	UpdatedAt             *time.Time   `json:"updated_at,omitempty"`
}

// ConflictingCompany identifica a empresa que já usa o CNPJ/código [CA-002].
type ConflictingCompany struct {
	ID           string `json:"id"`
	InternalCode string `json:"internal_code"`
	LegalName    string `json:"legal_name"`
}
