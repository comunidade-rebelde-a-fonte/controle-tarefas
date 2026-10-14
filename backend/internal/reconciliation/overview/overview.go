// Package overview é a Conciliação por empresa na competência (TASK-024, D-038): o estado da
// conclusão de cada empresa (em andamento, pronta, concluída), o que falta, o Contábil e a
// obrigação Conciliação, e se a Conciliação bloqueia o fechamento do Contábil. É a lista que
// explica os números "prontas para concluir" e "Contábil bloqueado" da seção Conciliação do
// Dashboard (decisão do usuário), numa consulta só para a competência inteira.
package overview

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	co "controle/internal/company"
	"controle/internal/platform/competence"
	"controle/internal/reconciliation/completion"
	"controle/internal/store"
)

// CompanyRef identifica a empresa.
type CompanyRef struct {
	ID           string `json:"id"`
	InternalCode string `json:"internal_code"`
	LegalName    string `json:"legal_name"`
}

// Company é a Conciliação de uma empresa na competência.
type Company struct {
	Company CompanyRef `json:"company"`
	// State segue a regra da conclusão (completion.Load): concluída com a conclusão em vigor;
	// pronta com extrato e sem pendência; senão, em andamento.
	State              string                 `json:"state"`
	Statements         int                    `json:"statements"`
	Blocking           completion.Blocking    `json:"blocking"`
	WithoutStatement   int                    `json:"without_statement"`
	UsesReconciliation bool                   `json:"uses_reconciliation"`
	AccountingState    *string                `json:"accounting_state"`
	Obligation         *completion.Obligation `json:"obligation"`
	// BlocksAccounting: o Contábil está aberto e não fecha por causa da Conciliação (a guarda do
	// fechamento, D-037): a empresa usa a Conciliação e a obrigação aplicável não está final.
	BlocksAccounting bool `json:"blocks_accounting"`
}

// Counts são os totais da lista antes dos filtros de estado.
type Counts struct {
	Total            int `json:"total"`
	InProgress       int `json:"em_andamento"`
	Ready            int `json:"pronta"`
	Completed        int `json:"concluida"`
	BlocksAccounting int `json:"blocks_accounting"`
}

// Overview é a lista da competência.
type Overview struct {
	Competence string    `json:"competence"`
	Counts     Counts    `json:"counts"`
	Items      []Company `json:"items"`
}

// Query são os filtros da lista: o estado, só as que bloqueiam o Contábil e os filtros de
// empresa do Dashboard (pelo snapshot da competência, com o responsável do Contábil).
type Query struct {
	State   *string
	Blocked bool
	Filter  co.Filter
}

// States são os estados da conclusão.
var States = []string{completion.StateInProgress, completion.StateReady, completion.StateCompleted}

// Service lê a lista.
type Service struct{ pool *pgxpool.Pool }

// NewService cria o serviço.
func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

// List lê a Conciliação por empresa na competência com os filtros.
func (s *Service) List(ctx context.Context, c competence.Competence, in Query) (Overview, error) {
	q := store.New(s.pool)
	ids, err := in.Filter.CompanyIDs(ctx, q, c, co.ModuleAccounting)
	if err != nil {
		return Overview{}, err
	}
	all, err := Load(ctx, q, c, ids)
	if err != nil {
		return Overview{}, err
	}
	out := Overview{Competence: c.String(), Counts: Count(all), Items: []Company{}}
	for _, it := range all {
		if (in.State == nil || it.State == *in.State) && (!in.Blocked || it.BlocksAccounting) {
			out.Items = append(out.Items, it)
		}
	}
	return out, nil
}

// Load lê as empresas da competência (só as dadas; nil: todas), na ordem do nome.
func Load(ctx context.Context, q *store.Queries, c competence.Competence, companyIDs []string) ([]Company, error) {
	rows, err := q.ListReconciliationOverview(ctx, store.ListReconciliationOverviewParams{Competence: c.FirstDay(), CompanyIds: companyIDs})
	if err != nil {
		return nil, err
	}
	out := make([]Company, 0, len(rows))
	for _, r := range rows {
		it := Company{
			Company:    CompanyRef{ID: r.ID, InternalCode: r.InternalCode, LegalName: r.LegalName},
			Statements: int(r.Statements),
			Blocking: completion.Blocking{MissingStatement: r.Statements == 0, Divergent: int(r.Divergent),
				WithoutReceipt: int(r.WithoutReceipt), Review: int(r.Review), Classification: int(r.Classification)},
			WithoutStatement: int(r.WithoutStatement), UsesReconciliation: r.UsesReconciliation,
		}
		if r.AccountingState != "" {
			st := r.AccountingState
			it.AccountingState = &st
		}
		if r.HasObligation {
			it.Obligation = &completion.Obligation{InstanceID: r.ObligationID, StatusCode: r.ObligationStatusCode,
				StatusLabel: r.ObligationStatusLabel, IsFinal: r.ObligationIsFinal, Applicable: r.ObligationApplicable}
		}
		b := it.Blocking
		switch {
		case r.Completed:
			it.State = completion.StateCompleted
		case !b.MissingStatement && b.Divergent == 0 && b.WithoutReceipt == 0 && b.Review == 0 && b.Classification == 0:
			it.State = completion.StateReady
		default:
			it.State = completion.StateInProgress
		}
		it.BlocksAccounting = it.AccountingState != nil && *it.AccountingState == "OPEN" && it.UsesReconciliation &&
			it.Obligation != nil && it.Obligation.Applicable && !it.Obligation.IsFinal
		out = append(out, it)
	}
	return out, nil
}

// Count totaliza a lista por estado e as que bloqueiam o Contábil.
func Count(items []Company) Counts {
	k := Counts{Total: len(items)}
	for _, it := range items {
		switch it.State {
		case completion.StateCompleted:
			k.Completed++
		case completion.StateReady:
			k.Ready++
		default:
			k.InProgress++
		}
		if it.BlocksAccounting {
			k.BlocksAccounting++
		}
	}
	return k
}
