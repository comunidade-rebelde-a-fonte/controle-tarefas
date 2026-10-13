package seed

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"controle/internal/platform/clock"
)

// Responsáveis fictícios usados pelas empresas de demonstração.
var demoUsers = []struct{ Key, Name, Email string }{
	{"ana", "Ana Souza", "ana.souza@escritorio.exemplo"},
	{"bruno", "Bruno Lima", "bruno.lima@escritorio.exemplo"},
	{"carla", "Carla Mendes", "carla.mendes@escritorio.exemplo"},
	{"diego", "Diego Rocha", "diego.rocha@escritorio.exemplo"},
}

// RegimeSpec é um período de regime de uma empresa fictícia.
type RegimeSpec struct {
	Regime, From string // competência inicial AAAA-MM-01
	To           string // "" = vigência aberta
}

// CompanySpec descreve uma empresa fictícia (demonstração ou massa sintética de carga).
type CompanySpec struct {
	Code, LegalName, TradeName, Cnpj, City, State string
	Regimes                                       []RegimeSpec
	Modules                                       []string // módulos habilitados; os demais ficam desabilitados
	PayrollType                                   string
	FactorR                                       *bool
	Fiscal, Accounting, Payroll, Declarations     string // chaves de demoUsers (ana, bruno, carla, diego)
	NotApplicable                                 []string
}

var (
	yes, no    = true, false
	allModules = []string{"FISCAL", "DECLARATIONS", "ACCOUNTING", "PAYROLL", "CREDENTIALS"}
	// AllModules são os módulos configuráveis por empresa.
	AllModules = allModules
)

// Cenário da Conciliação da demonstração (TASK-027, D-041): a Construtora Horizonte e a
// competência do cenário de aula da origem (extratos, comprovantes e contas de agosto de 2026).
const (
	HorizonteCode       = "111"
	HorizonteCompetence = "2026-08"
)

// AdminID é o administrador do sistema (migration 0002), autor dos dados de demonstração.
const AdminID = adminID

func open(regime string) []RegimeSpec { return []RegimeSpec{{Regime: regime, From: "2025-01-01"}} }

// demoCompanies são as 20 empresas fictícias do PRD §37 (dados apenas para demonstração),
// com módulos, overrides, tipos de folha, fator R e responsáveis variados, e a Construtora
// Horizonte (111), a empresa do cenário de aula da Conciliação (TASK-027, D-041).
//
// Bypass documentado: os CNPJs fictícios do PRD não têm dígitos verificadores válidos, por
// isso o seed grava diretamente no banco (somente 14 dígitos). A API continua validando os
// dígitos verificadores de todo cadastro feito pela interface ou importação.
var demoCompanies = []CompanySpec{
	{Code: "101", LegalName: "Empresa Fictícia Alfa Comércio Ltda", TradeName: "Alfa Comércio", Cnpj: "00000001000101", City: "Curitiba", State: "PR",
		Regimes: open("LUCRO_PRESUMIDO"), Modules: allModules, PayrollType: "PAYROLL", Fiscal: "ana", Accounting: "bruno", Payroll: "carla", Declarations: "diego"},
	{Code: "102", LegalName: "Empresa Fictícia Beta Serviços Ltda", TradeName: "Beta Serviços", Cnpj: "00000002000102", City: "Curitiba", State: "PR",
		Regimes: open("LUCRO_PRESUMIDO"), Modules: allModules, PayrollType: "PRO_LABORE", Fiscal: "ana", Accounting: "bruno", Payroll: "carla", Declarations: "diego",
		NotApplicable: []string{"FISCAL_IPI"}},
	{Code: "103", LegalName: "Empresa Fictícia Gama Indústria Ltda", TradeName: "Gama Indústria", Cnpj: "00000003000103", City: "Joinville", State: "SC",
		Regimes: open("LUCRO_REAL"), Modules: allModules, PayrollType: "PAYROLL", Fiscal: "ana", Accounting: "bruno", Payroll: "carla", Declarations: "diego"},
	{Code: "104", LegalName: "Empresa Fictícia Delta Tecnologia Ltda", TradeName: "Delta Tecnologia", Cnpj: "00000004000104", City: "São Paulo", State: "SP",
		Regimes: open("LUCRO_PRESUMIDO"), Modules: []string{"FISCAL", "DECLARATIONS", "ACCOUNTING", "CREDENTIALS"}, PayrollType: "NO_PAYROLL",
		Fiscal: "ana", Accounting: "bruno", Declarations: "diego", NotApplicable: []string{"FISCAL_IPI", "FISCAL_ICMS"}},
	{Code: "105", LegalName: "Empresa Fictícia Épsilon Logística Ltda", TradeName: "Épsilon Logística", Cnpj: "00000005000105", City: "Campinas", State: "SP",
		Regimes: open("LUCRO_REAL"), Modules: allModules, PayrollType: "PAYROLL", Fiscal: "ana", Accounting: "bruno", Payroll: "carla", Declarations: "diego"},
	{Code: "106", LegalName: "Empresa Fictícia Zeta Consultoria Ltda", TradeName: "Zeta Consultoria", Cnpj: "00000006000106", City: "Florianópolis", State: "SC",
		Regimes: open("LUCRO_PRESUMIDO"), Modules: []string{"FISCAL", "DECLARATIONS", "PAYROLL", "CREDENTIALS"}, PayrollType: "PRO_LABORE",
		Fiscal: "ana", Payroll: "carla", Declarations: "diego", NotApplicable: []string{"FISCAL_ICMS", "FISCAL_IPI"}},
	{Code: "107", LegalName: "Empresa Fictícia Eta Varejo Ltda", TradeName: "Eta Varejo", Cnpj: "00000007000107", City: "Porto Alegre", State: "RS",
		Regimes: open("LUCRO_REAL"), Modules: allModules, PayrollType: "PAYROLL", Fiscal: "ana", Accounting: "bruno", Payroll: "carla", Declarations: "diego"},
	{Code: "108", LegalName: "Empresa Fictícia Teta Engenharia Ltda", TradeName: "Teta Engenharia", Cnpj: "00000008000108", City: "Curitiba", State: "PR",
		Regimes: open("LUCRO_PRESUMIDO"), Modules: allModules, PayrollType: "PAYROLL", Fiscal: "ana", Accounting: "bruno", Payroll: "carla", Declarations: "diego",
		NotApplicable: []string{"FISCAL_IPI"}},
	{Code: "109", LegalName: "Empresa Fictícia Iota Marketing Ltda", TradeName: "Iota Marketing", Cnpj: "00000009000109", City: "São Paulo", State: "SP",
		Regimes: open("IMUNE_ISENTA"), Modules: []string{"FISCAL", "DECLARATIONS", "ACCOUNTING", "CREDENTIALS"}, PayrollType: "NO_PAYROLL",
		Fiscal: "ana", Accounting: "bruno", Declarations: "diego", NotApplicable: []string{"FISCAL_IPI", "FISCAL_ICMS", "FISCAL_IRPJ", "FISCAL_CSLL"}},
	{Code: "110", LegalName: "Empresa Fictícia Kappa Distribuidora Ltda", TradeName: "Kappa Distribuidora", Cnpj: "00000010000110", City: "Londrina", State: "PR",
		Regimes: open("LUCRO_REAL"), Modules: allModules, PayrollType: "PAYROLL", Fiscal: "ana", Accounting: "bruno", Payroll: "carla", Declarations: "diego"},
	// A empresa do cenário da Conciliação (agosto de 2026): o nome e o CNPJ fictício são os que os
	// extratos e comprovantes da origem imprimem (o leitor determinístico acha a empresa pelo nome).
	// A preparação dos fixtures carrega o cenário (make demo-fixtures).
	{Code: HorizonteCode, LegalName: "Construtora Horizonte Ltda", TradeName: "Construtora Horizonte", Cnpj: "12345678000190", City: "Rio Claro", State: "SP",
		Regimes: open("LUCRO_PRESUMIDO"), Modules: allModules, PayrollType: "PAYROLL", Fiscal: "ana", Accounting: "bruno", Payroll: "carla", Declarations: "diego"},

	{Code: "201", LegalName: "Alvorada Comércio Simples Ltda", TradeName: "Alvorada Comércio", Cnpj: "00100001000101", City: "Curitiba", State: "PR",
		Regimes: open("SIMPLES_NACIONAL"), Modules: allModules, PayrollType: "PRO_LABORE", FactorR: &no, Fiscal: "diego", Accounting: "bruno", Payroll: "carla", Declarations: "diego"},
	{Code: "202", LegalName: "Brisa Serviços Digitais Ltda", TradeName: "Brisa Digital", Cnpj: "00100002000102", City: "Curitiba", State: "PR",
		Regimes: open("SIMPLES_NACIONAL"), Modules: allModules, PayrollType: "PAYROLL", FactorR: &yes, Fiscal: "diego", Accounting: "bruno", Payroll: "carla", Declarations: "diego",
		NotApplicable: []string{"FISCAL_ICMS"}},
	{Code: "203", LegalName: "Canto Sul Restaurante Ltda", TradeName: "Canto Sul", Cnpj: "00100003000103", City: "Porto Alegre", State: "RS",
		Regimes: open("SIMPLES_NACIONAL"), Modules: allModules, PayrollType: "PAYROLL", FactorR: &no, Fiscal: "diego", Accounting: "bruno", Payroll: "carla", Declarations: "diego"},
	{Code: "204", LegalName: "Delta Engenharia ME", TradeName: "Delta Engenharia", Cnpj: "00100004000104", City: "Maringá", State: "PR",
		Regimes: open("SIMPLES_NACIONAL"), Modules: []string{"FISCAL", "DECLARATIONS", "PAYROLL", "CREDENTIALS"}, PayrollType: "NO_PAYROLL", FactorR: &no,
		Fiscal: "diego", Payroll: "carla", Declarations: "diego"},
	{Code: "205", LegalName: "Estação Criativa Publicidade Ltda", TradeName: "Estação Criativa", Cnpj: "00100005000105", City: "São Paulo", State: "SP",
		// Troca de regime histórica: Lucro Presumido em 2025, Simples Nacional a partir de 01/2026.
		Regimes: []RegimeSpec{{Regime: "LUCRO_PRESUMIDO", From: "2025-01-01", To: "2025-12-01"}, {Regime: "SIMPLES_NACIONAL", From: "2026-01-01"}},
		Modules: allModules, PayrollType: "PAYROLL", FactorR: &yes, Fiscal: "diego", Accounting: "bruno", Payroll: "carla", Declarations: "diego"},
	{Code: "206", LegalName: "Fênix Manutenção Predial Ltda", TradeName: "Fênix Manutenção", Cnpj: "00100006000106", City: "São Paulo", State: "SP",
		Regimes: open("SIMPLES_NACIONAL"), Modules: allModules, PayrollType: "PAYROLL", FactorR: &yes, Fiscal: "diego", Accounting: "bruno", Payroll: "carla", Declarations: "diego"},
	{Code: "207", LegalName: "Horizonte Pet Shop Ltda", TradeName: "Horizonte Pet", Cnpj: "00100007000107", City: "Campinas", State: "SP",
		Regimes: open("SIMPLES_NACIONAL"), Modules: []string{"FISCAL", "DECLARATIONS", "ACCOUNTING", "CREDENTIALS"}, PayrollType: "NO_PAYROLL", FactorR: &no,
		Fiscal: "diego", Accounting: "bruno", Declarations: "diego"},
	{Code: "208", LegalName: "Íris Clínica Veterinária Ltda", TradeName: "Íris Veterinária", Cnpj: "00100008000108", City: "Florianópolis", State: "SC",
		Regimes: open("SIMPLES_NACIONAL"), Modules: allModules, PayrollType: "PRO_LABORE", FactorR: &yes, Fiscal: "diego", Accounting: "bruno", Payroll: "carla", Declarations: "diego"},
	{Code: "209", LegalName: "Jatobá Móveis Planejados Ltda", TradeName: "Jatobá Móveis", Cnpj: "00100009000109", City: "Joinville", State: "SC",
		Regimes: open("SIMPLES_NACIONAL"), Modules: allModules, PayrollType: "PAYROLL", FactorR: &no, Fiscal: "diego", Accounting: "bruno", Payroll: "carla", Declarations: "diego"},
	{Code: "210", LegalName: "Lume Consultoria Empresarial Ltda", TradeName: "Lume Consultoria", Cnpj: "00100010000110", City: "Curitiba", State: "PR",
		Regimes: open("SIMPLES_NACIONAL"), Modules: []string{"FISCAL", "DECLARATIONS", "CREDENTIALS"}, PayrollType: "NO_PAYROLL", FactorR: &yes,
		Fiscal: "diego", Declarations: "diego", NotApplicable: []string{"FISCAL_ICMS"}},
}

// DemoCompanyCount é a quantidade de empresas de demonstração.
var DemoCompanyCount = len(demoCompanies)

const adminID = "00000000-0000-0000-0000-000000000001"

// companiesStep grava responsáveis e empresas fictícias. Empresas já existentes (mesmo CNPJ)
// são mantidas como estão: rodar de novo não duplica nem sobrescreve edições.
func companiesStep() Step {
	return Step{Name: fmt.Sprintf("demo: %d empresas fictícias", len(demoCompanies)), Run: func(ctx context.Context, tx pgx.Tx, _ clock.Clock) error {
		return InsertCompanies(ctx, tx, demoCompanies)
	}}
}

// InsertCompanies grava os responsáveis fictícios e as empresas, com regimes, módulos e
// overrides. Empresas já existentes (mesmo CNPJ) são mantidas: rodar de novo não duplica.
func InsertCompanies(ctx context.Context, tx pgx.Tx, companies []CompanySpec) error {
	users := map[string]string{}
	for _, u := range demoUsers {
		var id string
		err := tx.QueryRow(ctx, `SELECT id FROM users WHERE lower(email) = lower($1)`, u.Email).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			err = tx.QueryRow(ctx, `INSERT INTO users (name, email, role) VALUES ($1, $2, 'OPERATOR') RETURNING id`, u.Name, u.Email).Scan(&id)
		}
		if err != nil {
			return fmt.Errorf("responsável %s: %w", u.Name, err)
		}
		users[u.Key] = id
	}
	ref := func(key string) *string {
		if key == "" {
			return nil
		}
		id := users[key]
		return &id
	}
	for _, c := range companies {
		var id string
		err := tx.QueryRow(ctx, `
			INSERT INTO companies (internal_code, legal_name, trade_name, cnpj, city, state, service_start_date,
			    payroll_type, factor_r, fiscal_responsible_user_id, accounting_responsible_user_id,
			    payroll_responsible_user_id, declarations_responsible_user_id, notes, created_by, updated_by)
			VALUES ($1, $2, $3, $4, $5, $6, '2025-01-01', $7, $8, $9, $10, $11, $12, 'Empresa fictícia de demonstração.', $13, $13)
			ON CONFLICT (cnpj) DO NOTHING
			RETURNING id`,
			c.Code, c.LegalName, c.TradeName, c.Cnpj, c.City, c.State, c.PayrollType, c.FactorR,
			ref(c.Fiscal), ref(c.Accounting), ref(c.Payroll), ref(c.Declarations), adminID).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			continue // já cadastrada
		}
		if err != nil {
			return fmt.Errorf("empresa %s: %w", c.Code, err)
		}
		for _, r := range c.Regimes {
			var to *string
			if r.To != "" {
				to = &r.To
			}
			if _, err := tx.Exec(ctx, `INSERT INTO company_tax_regime_history (company_id, tax_regime, effective_from_competence, effective_to_competence, created_by)
				VALUES ($1, $2, $3, $4, $5)`, id, r.Regime, r.From, to, adminID); err != nil {
				return fmt.Errorf("regime da empresa %s: %w", c.Code, err)
			}
		}
		for _, m := range allModules {
			enabled := false
			for _, e := range c.Modules {
				enabled = enabled || e == m
			}
			if _, err := tx.Exec(ctx, `INSERT INTO company_module_settings (company_id, module_code, enabled, effective_from_competence, created_by)
				VALUES ($1, $2, $3, '2025-01-01', $4)`, id, m, enabled, adminID); err != nil {
				return fmt.Errorf("módulo %s da empresa %s: %w", m, c.Code, err)
			}
		}
		for _, code := range c.NotApplicable {
			if _, err := tx.Exec(ctx, `INSERT INTO company_obligation_overrides (company_id, obligation_type_id, applicability, effective_from_competence, created_by)
				SELECT $1, t.id, 'NOT_APPLICABLE', '2025-01-01', $3 FROM obligation_types t WHERE t.code = $2`, id, code, adminID); err != nil {
				return fmt.Errorf("override %s da empresa %s: %w", code, c.Code, err)
			}
		}
	}
	return nil
}
