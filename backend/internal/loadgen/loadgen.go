// Package loadgen gera massa sintética determinística para medição de desempenho e testes de
// consistência [T-M3-10, T-M5-09]: empresas fictícias com distribuição realista, competências
// geradas pelo motor e status definidos por hash da semente.
package loadgen

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"controle/internal/competence"
	"controle/internal/platform/clock"
	"controle/internal/platform/cnpj"
	comp "controle/internal/platform/competence"
	"controle/seed"
)

var (
	cities = []struct{ City, State string }{
		{"Curitiba", "PR"}, {"Londrina", "PR"}, {"Maringá", "PR"}, {"Joinville", "SC"}, {"Florianópolis", "SC"},
		{"São Paulo", "SP"}, {"Campinas", "SP"}, {"Porto Alegre", "RS"}, {"Belo Horizonte", "MG"}, {"Rio de Janeiro", "RJ"},
	}
	normalRegimes = []string{"LUCRO_PRESUMIDO", "LUCRO_PRESUMIDO", "LUCRO_PRESUMIDO", "LUCRO_REAL", "LUCRO_REAL", "LUCRO_ARBITRADO", "IMUNE_ISENTA"}
	payrollTypes  = []string{"PAYROLL", "PAYROLL", "PRO_LABORE", "NO_PAYROLL"}
	responsibles  = []string{"ana", "bruno", "carla", "diego"}
	overrides     = []string{"FISCAL_IPI", "FISCAL_ICMS", "FISCAL_ISSQN", "SINTEGRA", "FISCAL_RETIDOS"}
)

// prng é um gerador determinístico (splitmix64): a massa sintética precisa ser reproduzível
// pela semente, não imprevisível.
type prng struct{ state uint64 }

func (p *prng) next() uint64 {
	p.state += 0x9e3779b97f4a7c15
	z := p.state
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	return z ^ (z >> 31)
}

// IntN devolve um inteiro em [0, n) para n pequeno (n > 0).
func (p *prng) IntN(n int) int {
	return int(p.next()>>33) % n
}

// SyntheticCompanies monta N empresas fictícias determinísticas pela semente. Os CNPJs têm
// dígitos verificadores válidos, com raiz 99xxxxxx (faixa sintética).
func SyntheticCompanies(n int, s uint64) []seed.CompanySpec {
	r := &prng{state: s}
	out := make([]seed.CompanySpec, 0, n)
	for i := 1; i <= n; i++ {
		regime := "SIMPLES_NACIONAL"
		if r.IntN(100) >= 60 {
			regime = normalRegimes[r.IntN(len(normalRegimes))]
		}
		modules := []string{"FISCAL", "DECLARATIONS", "CREDENTIALS"}
		if r.IntN(100) < 80 {
			modules = append(modules, "ACCOUNTING")
		}
		payroll := payrollTypes[r.IntN(len(payrollTypes))]
		if payroll != "NO_PAYROLL" || r.IntN(2) == 0 {
			modules = append(modules, "PAYROLL")
		}
		var factorR *bool
		if regime == "SIMPLES_NACIONAL" {
			v := r.IntN(2) == 0
			factorR = &v
		}
		var na []string
		for _, o := range overrides {
			if r.IntN(100) < 15 {
				na = append(na, o)
			}
		}
		place := cities[r.IntN(len(cities))]
		who := responsibles[r.IntN(len(responsibles))]
		out = append(out, seed.CompanySpec{
			Code:          fmt.Sprintf("P%04d", i),
			LegalName:     fmt.Sprintf("Empresa Sintética %04d Ltda", i),
			TradeName:     fmt.Sprintf("Sintética %04d", i),
			Cnpj:          cnpj.WithCheckDigits(fmt.Sprintf("99%06d0001", i)),
			City:          place.City,
			State:         place.State,
			Regimes:       []seed.RegimeSpec{{Regime: regime, From: "2025-01-01"}},
			Modules:       modules,
			PayrollType:   payroll,
			FactorR:       factorR,
			Fiscal:        who,
			Accounting:    responsibles[(i+1)%len(responsibles)],
			Payroll:       responsibles[(i+2)%len(responsibles)],
			Declarations:  responsibles[(i+3)%len(responsibles)],
			NotApplicable: na,
		})
	}
	return out
}

// Generate carrega os dados de referência, as N empresas sintéticas e gera K competências (a
// corrente e as anteriores) com status determinísticos pela semente. Idempotente.
func Generate(ctx context.Context, pool *pgxpool.Pool, clk clock.Clock, n, k int, s uint64, out io.Writer) ([]comp.Competence, error) {
	if n < 1 || k < 1 {
		return nil, errors.New("companies e competences devem ser positivos")
	}
	if err := seed.Reference(ctx, pool, clk, io.Discard); err != nil {
		return nil, err
	}
	if err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		return seed.InsertCompanies(ctx, tx, SyntheticCompanies(n, s))
	}); err != nil {
		return nil, err
	}
	engine := competence.NewEngine(pool, clk)
	cur := comp.Current(clk)
	var out2 []comp.Competence
	for age := k - 1; age >= 0; age-- {
		c := cur.AddMonths(-age)
		sum, err := engine.Ensure(ctx, c)
		if err != nil {
			return nil, fmt.Errorf("competência %s: %w", c, err)
		}
		out2 = append(out2, c)
		_, _ = fmt.Fprintf(out, "competência %s: %d empresas, %d instâncias novas\n", c.Display(), sum.CompaniesProcessed, sum.InstancesCreated)
	}
	if err := ApplyStatuses(ctx, pool, cur.String(), s); err != nil {
		return nil, err
	}
	return out2, nil
}

// ApplyStatuses aplica status determinísticos (hash da empresa, tipo, competência e semente)
// às instâncias ainda na versão 1, e fecha parte dos períodos das competências anteriores.
// Competências anteriores ficam majoritariamente concluídas; a corrente, em andamento.
func ApplyStatuses(ctx context.Context, pool *pgxpool.Pool, current string, s uint64) error {
	return pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			WITH defs AS (
			    SELECT status_set_id,
			           array_agg(code ORDER BY sort_order) FILTER (WHERE code <> 'NOT_APPLICABLE') AS codes,
			           array_agg(code ORDER BY sort_order) FILTER (WHERE is_final AND code NOT IN ('NOT_APPLICABLE', 'NO_MOVEMENT')) AS finals
			    FROM status_definitions GROUP BY status_set_id
			), cand AS (
			    SELECT i.id, t.status_set_id, k.competence_key,
			           abs(hashtextextended(c.internal_code || '|' || t.code || '|' || k.competence_key, $1::bigint)) AS h
			    FROM obligation_instances i
			    JOIN companies c ON c.id = i.company_id
			    JOIN obligation_types t ON t.id = i.obligation_type_id
			    JOIN competencies k ON k.id = i.competence_id
			    WHERE i.applicable AND i.version = 1 AND c.internal_code LIKE 'P%'
			)
			UPDATE obligation_instances i SET
			    status_code = CASE
			        WHEN cand.competence_key < $2 AND cand.h % 10 < 8 AND cardinality(defs.finals) > 0
			            THEN defs.finals[1 + (cand.h / 10) % cardinality(defs.finals)]
			        ELSE defs.codes[1 + (cand.h / 10) % cardinality(defs.codes)]
			    END,
			    version = i.version + 1
			FROM cand JOIN defs USING (status_set_id)
			WHERE i.id = cand.id`, int64(s&0x7fffffffffffffff), current); err != nil {
			return fmt.Errorf("status sintéticos: %w", err)
		}
		const admin = "00000000-0000-0000-0000-000000000001"
		if _, err := tx.Exec(ctx, `
			WITH closed AS (
			    UPDATE company_module_periods p SET state = 'CLOSED', closed_at = now(), closed_by = $3, version = p.version + 1, updated_at = now()
			    FROM competencies k, companies c
			    WHERE k.id = p.competence_id AND c.id = p.company_id AND p.state = 'OPEN' AND k.competence_key < $2
			      AND c.internal_code LIKE 'P%'
			      AND abs(hashtextextended(c.internal_code || '|' || p.module_code || '|' || k.competence_key, $1::bigint)) % 10 < 7
			    RETURNING p.id
			)
			INSERT INTO company_module_period_events (company_module_period_id, event_type, user_id, created_at)
			SELECT id, 'CLOSED', $3, now() FROM closed`, int64(s&0x7fffffffffffffff), current, admin); err != nil {
			return fmt.Errorf("fechamentos sintéticos: %w", err)
		}
		return nil
	})
}
