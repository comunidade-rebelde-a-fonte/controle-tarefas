package factorr

import (
	"context"
	"errors"
	"strings"

	"controle/internal/factorr/engine"
	"controle/internal/platform/apperr"
	"controle/internal/platform/competence"
	"controle/internal/store"
)

// Disclaimer acompanha toda resposta de agente do Fator R (CLAUDE §3.14 da origem).
const Disclaimer = "Esta plataforma não é a apuração oficial. O PGDAS-D da Receita Federal prevalece."

// WithDisclaimer termina o texto com o disclaimer, uma vez só (com_disclaimer da origem).
func WithDisclaimer(text string) string {
	text = strings.TrimSpace(text)
	if strings.HasSuffix(text, Disclaimer) {
		return text
	}
	return text + "\n\n" + Disclaimer
}

// GoldReference é o motor sobre a série gravada da empresa no PA: a referência da nota ouro do
// parser do PGDAS-D (_referencia de services/ouro_pgdas.py da origem). Nil quando algum mês da
// janela veio do PGDAS-D (o parser nunca se avalia contra o que ele mesmo criou), quando não há
// tabela do Simples vigente no PA ou quando o cálculo não é ok ou tem meses faltantes.
func GoldReference(ctx context.Context, q *store.Queries, companyID string, pa competence.Competence) (*engine.Result, error) {
	w := engine.NewWindow(pa)
	rows, err := q.ListFactorRMovements(ctx, store.ListFactorRMovementsParams{
		CompanyID: companyID, FromCompetence: w.Start.FirstDay(), ToCompetence: w.End.FirstDay(),
	})
	if err != nil {
		return nil, err
	}
	ms := make([]engine.Movement, len(rows))
	for i, r := range rows {
		if r.Origem == "pgdas" {
			return nil, nil
		}
		ms[i] = toEngineMovement(r)
	}
	tb, err := tables(ctx, q, pa)
	var ae *apperr.Error
	if errors.As(err, &ae) && ae.Code == "SIMPLES_TABLE_NOT_IN_FORCE" {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	pol, err := policy(ctx, q)
	if err != nil {
		return nil, err
	}
	set, _, err := settingsOf(ctx, q, companyID)
	if err != nil {
		return nil, err
	}
	res := engine.Calculate(pa, set.InicioAtividade, ms, tb, pol)
	if res.Status != engine.StatusOK || len(res.MissingMonths) > 0 {
		return nil, nil
	}
	return &res, nil
}
