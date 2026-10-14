package seed

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"controle/internal/platform/clock"
	"controle/internal/platform/competence"
)

// demoFactorR são os dados fictícios do Fator R das empresas sujeitas do seed (factor_r = true).
// No PA corrente cobrem os quatro casos da carteira: vermelho (208 e 205, esta com dois meses
// sem lançamento), amarelo (206), verde (202, com os meses mais antigos da janela declarados
// no PGDAS-D) e dados insuficientes (210, sem lançamentos). Valores em reais por mês.
type demoFactorR struct {
	Code                           string
	Package                        string // vazio = sem pacote
	Fee                            string
	Cnae, Activity, Contact        string
	Partners                       int
	ActivityStart                  string // AAAA-MM; vazio = não informado
	Revenue                        int64
	ProLabore, Salaries, CPP, FGTS int64
	MissingAges, PGDASAges         []int // idade do mês: 1 = competência anterior à corrente
	WithSettings, WithoutMovements bool
}

var demoFactorRCompanies = []demoFactorR{
	{Code: "208", Package: "correcao", Fee: "1200.00", Cnae: "7500-1/00", Activity: "Clínica veterinária",
		Contact: "Dra. Helena (contato fictício)", Partners: 2, ActivityStart: "2019-03", Revenue: 52000, ProLabore: 7000, WithSettings: true},
	{Code: "205", Revenue: 38000, ProLabore: 6000, Salaries: 4000, FGTS: 320, MissingAges: []int{2, 5}},
	{Code: "206", Package: "monitoramento", Fee: "450.00", Cnae: "8111-7/00", Activity: "Manutenção predial",
		Partners: 1, Revenue: 70000, ProLabore: 8000, Salaries: 11000, CPP: 500, FGTS: 880, WithSettings: true},
	{Code: "202", Package: "retainer", Fee: "900.00", Cnae: "6201-5/01", Activity: "Serviços digitais",
		Partners: 3, ActivityStart: "2021-06", Revenue: 90000, ProLabore: 12000, Salaries: 16000, CPP: 900, FGTS: 1280,
		PGDASAges: []int{12, 11, 10, 9}, WithSettings: true},
	{Code: "210", Package: "monitoramento", Fee: "250.00", Cnae: "7020-4/00", Activity: "Consultoria empresarial",
		Partners: 1, WithSettings: true, WithoutMovements: true},
}

// demoFactorRMonths é quantos meses anteriores à competência corrente recebem lançamentos:
// a janela do PA corrente e a do PA de um ano antes.
const demoFactorRMonths = 24

// demoVariation varia a receita mês a mês (%), sem aleatoriedade.
var demoVariation = [12]int64{0, 3, -2, 5, 1, -4, 2, 6, -1, 4, -3, 0}

func reais(v int64) string { return fmt.Sprintf("%d.00", v) }

// factorRDemoStep grava configuração e movimentos relativos à competência corrente.
// Idempotente: configuração e meses existentes não são alterados.
func factorRDemoStep() Step {
	return Step{Name: "demo: Fator R", Run: func(ctx context.Context, tx pgx.Tx, clk clock.Clock) error {
		current := competence.Current(clk)
		now := clk.Now().In(time.UTC)
		for _, d := range demoFactorRCompanies {
			if d.WithSettings {
				var pkg, start any
				if d.Package != "" {
					pkg = d.Package
				}
				if d.ActivityStart != "" {
					start = competence.MustParse(d.ActivityStart).FirstDay()
				}
				if _, err := tx.Exec(ctx, `
					INSERT INTO company_factor_r_settings (company_id, cnae, atividade, quantidade_socios, contato,
					    pacote_comercial, honorario_mensal, inicio_atividade, updated_by, updated_at)
					SELECT c.id, $2, $3, $4, NULLIF($5, ''), $6, $7, $8, $9, $10 FROM companies c WHERE c.internal_code = $1
					ON CONFLICT (company_id) DO NOTHING`,
					d.Code, d.Cnae, d.Activity, d.Partners, d.Contact, pkg, d.Fee, start, adminID, now); err != nil {
					return fmt.Errorf("configuração do Fator R da empresa %s: %w", d.Code, err)
				}
			}
			if d.WithoutMovements {
				continue
			}
			for age := demoFactorRMonths; age >= 1; age-- {
				if slices.Contains(d.MissingAges, age) {
					continue
				}
				month := current.AddMonths(-age)
				revenue := d.Revenue + d.Revenue*demoVariation[int(month.Month)-1]/100
				payroll := d.ProLabore + d.Salaries + d.CPP + d.FGTS
				if slices.Contains(d.PGDASAges, age) {
					if _, err := tx.Exec(ctx, `
						INSERT INTO factor_r_movements (company_id, competence, receita_bruta, folha_total_declarada_pgdas,
						    origem, observacao, updated_by, updated_at)
						SELECT c.id, $2, $3, $4, 'pgdas', 'Total declarado no PGDAS-D (dados fictícios de demonstração)', $5, $6
						FROM companies c WHERE c.internal_code = $1
						ON CONFLICT (company_id, competence) DO NOTHING`,
						d.Code, month.FirstDay(), reais(revenue), reais(payroll), adminID, now); err != nil {
						return fmt.Errorf("movimento %s da empresa %s: %w", month, d.Code, err)
					}
					continue
				}
				if _, err := tx.Exec(ctx, `
					INSERT INTO factor_r_movements (company_id, competence, receita_bruta, pro_labore, salarios_13_ferias,
					    cpp_recolhida, fgts_recolhido, origem, updated_by, updated_at)
					SELECT c.id, $2, $3, $4, $5, $6, $7, 'manual', $8, $9 FROM companies c WHERE c.internal_code = $1
					ON CONFLICT (company_id, competence) DO NOTHING`,
					d.Code, month.FirstDay(), reais(revenue), reais(d.ProLabore), reais(d.Salaries), reais(d.CPP), reais(d.FGTS),
					adminID, now); err != nil {
					return fmt.Errorf("movimento %s da empresa %s: %w", month, d.Code, err)
				}
			}
		}
		return nil
	}}
}
