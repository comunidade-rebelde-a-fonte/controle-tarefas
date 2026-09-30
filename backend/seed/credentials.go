package seed

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"controle/internal/platform/clock"
)

// demoCredential é a procuração ou o certificado de uma empresa fictícia. Days é o
// vencimento relativo a hoje (nil = sem data: nenhum registro é gravado).
type demoCredential struct {
	Days    *int
	Renewal string
	Notes   string
}

func in(days int) *int { return &days }

// demoCredentials cobre todas as faixas de validade [PRD §19.3] e as etapas de renovação
// [§19.4], de modo que cada filtro rápido do §33 encontre empresas.
// Ordem: {procuração RFB, certificado digital}.
var demoCredentials = map[string][2]demoCredential{
	"101": {{in(-20), "AWAITING_CLIENT", "Aguardando o cliente assinar a nova procuração no e-CAC."}, {in(120), "CURRENT", ""}},
	"102": {{in(3), "RENEWAL_REQUESTED", ""}, {nil, "", ""}},
	"103": {{in(10), "CURRENT", ""}, {in(-5), "AWAITING_CLIENT", "Cliente vai agendar a validação do certificado."}},
	"104": {{in(22), "CURRENT", ""}, {in(2), "CURRENT", ""}},
	"105": {{in(45), "CURRENT", ""}, {in(14), "RENEWAL_REQUESTED", ""}},
	"106": {{in(200), "CURRENT", ""}, {in(28), "CURRENT", ""}},
	"107": {{nil, "", ""}, {in(55), "CURRENT", ""}},
	"108": {{in(330), "CURRENT", ""}, {in(-1), "RENEWAL_REQUESTED", ""}},
	"109": {{in(-3), "RENEWAL_REQUESTED", ""}, {in(300), "CURRENT", ""}},
	"110": {{in(6), "AWAITING_CLIENT", "Cliente informado por e-mail."}, {in(18), "CURRENT", ""}},
	"201": {{in(150), "CURRENT", ""}, {in(7), "CURRENT", ""}},
	"202": {{in(40), "CURRENT", ""}, {nil, "", ""}},
	"203": {{nil, "", ""}, {in(60), "CURRENT", ""}},
	"204": {{in(12), "RENEWAL_REQUESTED", ""}, {in(250), "CURRENT", ""}},
	"205": {{in(280), "CURRENT", ""}, {in(-10), "AWAITING_CLIENT", ""}},
	"206": {{in(25), "CURRENT", ""}, {in(33), "CURRENT", ""}},
	"207": {{in(-60), "CURRENT", "Empresa avisada; sem retorno até o momento."}, {in(100), "CURRENT", ""}},
	"208": {{in(90), "CURRENT", ""}, {in(11), "CURRENT", ""}},
	"209": {{in(5), "CURRENT", ""}, {in(29), "CURRENT", ""}},
	"210": {{in(365), "CURRENT", ""}, {in(61), "CURRENT", ""}},
}

var credentialTypes = [2]string{"RFB_POWER_OF_ATTORNEY", "DIGITAL_CERTIFICATE"}

// credentialsStep grava vencimentos relativos ao relógio (hoje no fuso de negócio).
// Idempotente: registros existentes não são alterados. Nenhum arquivo é gravado.
func credentialsStep() Step {
	return Step{Name: "demo: procurações e certificados", Run: func(ctx context.Context, tx pgx.Tx, clk clock.Clock) error {
		today := clock.Today(clk)
		for code, items := range demoCredentials {
			for i, it := range items {
				if it.Days == nil {
					continue
				}
				exp := today.AddDate(0, 0, *it.Days)
				var notes *string
				if it.Notes != "" {
					notes = &it.Notes
				}
				if _, err := tx.Exec(ctx, `
					INSERT INTO credential_controls (company_id, type, expires_at, renewal_status, notes, updated_by, updated_at)
					SELECT c.id, $2, $3, $4, $5, $6, $7 FROM companies c WHERE c.internal_code = $1
					ON CONFLICT (company_id, type) DO NOTHING`,
					code, credentialTypes[i], exp, it.Renewal, notes, adminID, clk.Now().In(time.UTC)); err != nil {
					return fmt.Errorf("credencial %s da empresa %s: %w", credentialTypes[i], code, err)
				}
			}
		}
		return nil
	}}
}
