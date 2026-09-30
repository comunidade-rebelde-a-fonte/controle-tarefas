// Package grid monta as grades dos módulos [PRD §26, §28-§33, §41.6]: colunas vindas do
// catálogo, linhas por empresa com snapshot, período, observação e agregados, e células com
// status e indicadores. Toda regra (atraso, progresso, pronto para fechar, dependências) é
// calculada aqui; a UI só exibe.
package grid

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"controle/internal/catalog"
	"controle/internal/obligation"
	"controle/internal/platform/apperr"
	"controle/internal/platform/civil"
	"controle/internal/platform/clock"
	comp "controle/internal/platform/competence"
	"controle/internal/store"
)

// Spec descreve uma grade: contexto de exibição do catálogo, módulo do fechamento e os
// regimes que definem as colunas (e, no Fiscal, o agrupamento das linhas).
type Spec struct {
	Module       string   // obligation_display_contexts.module_code
	PeriodModule string   // company_module_periods.module_code: linhas, observação e agregado da linha
	Regimes      []string // regimes cujas regras definem as colunas
	RegimeGroup  string   // NORMAL | SIMPLES (Fiscal); "" = todos
	Group        string   // valor de "group" devolvido (normal | simples); "" = sem agrupamento
	// Closable: a grade tem fechamento próprio. A visão DCTFWeb não tem (plan R1).
	Closable bool
	// DependencyColumn: código do tipo cujo indicador SOFT vira a coluna "dependências" (DCTFWeb).
	DependencyColumn string
}

// Filters são os filtros server-side do §26.4. A busca livre é aplicada no cliente.
type Filters struct {
	TaxRegime        *string
	ResponsibleID    *string
	Status           *string // linha com ao menos uma célula aplicável nesse status
	ObligationTypeID *string // restringe Status a uma coluna
	ClosingStatus    *string // status do estágio de fechamento (Fiscal: coluna Fechamento)
	PeriodState      *string // OPEN | CLOSED
	HasNote          *bool
	Overdue          bool
	Pending          bool
	NotStarted       bool
	ReadyToClose     bool
}

// Column é uma coluna de obrigação da grade.
type Column struct {
	ObligationType         obligation.ObligationTypeRef `json:"obligation_type"`
	SortOrder              int32                        `json:"sort_order"`
	IsClosingStage         bool                         `json:"is_closing_stage"`
	AllowsTransmissionDate bool                         `json:"allows_transmission_date"`
	AllowsProtocol         bool                         `json:"allows_protocol"`
	StatusSetCode          string                       `json:"status_set_code"`
	Statuses               []catalog.StatusDefinition   `json:"statuses"`
}

// Company identifica a empresa da linha.
type Company struct {
	ID           string  `json:"id"`
	InternalCode string  `json:"internal_code"`
	LegalName    string  `json:"legal_name"`
	TradeName    *string `json:"trade_name"`
	Cnpj         string  `json:"cnpj"`
}

// Snapshot é a configuração usada na geração da competência [D4].
type Snapshot struct {
	TaxRegime   string              `json:"tax_regime"`
	RegimeGroup string              `json:"regime_group"`
	PayrollType string              `json:"payroll_type"`
	FactorR     *bool               `json:"factor_r"`
	Responsible *obligation.UserRef `json:"responsible"`
}

// Period é o estado do módulo da empresa na competência, com a observação geral [RN-016].
type Period struct {
	ID          string              `json:"id"`
	State       string              `json:"state"`
	ClosedAt    *time.Time          `json:"closed_at"`
	ClosedBy    *obligation.UserRef `json:"closed_by"`
	GeneralNote *string             `json:"general_note"`
	Version     int32               `json:"version"`
}

// Cell é a instância da coluna com os indicadores de dependência.
type Cell struct {
	obligation.Instance
	// Módulo principal da instância fechado na competência: edição bloqueada (plan R9).
	Locked bool `json:"locked"`
	// Etapas anteriores (HARD) que impedem concluir; vazio se nada bloqueia.
	BlockedBy []obligation.DepInstance `json:"blocked_by"`
	// Mensagem do bloqueio [PRD §18.3], para a UI exibir; null se nada bloqueia.
	BlockedMessage *string `json:"blocked_message"`
	// Indicador SOFT (DCTFWeb): apto/pendente com a lista; null se o tipo não tem SOFT.
	DependencyStatus *obligation.DependencyStatus `json:"dependency_status"`
}

// Row é uma empresa na grade.
type Row struct {
	Company         Company             `json:"company"`
	Snapshot        Snapshot            `json:"snapshot"`
	Period          Period              `json:"period"`
	Progress        obligation.Progress `json:"progress"`
	Overdue         int                 `json:"overdue"`
	NearestDeadline *civil.Date         `json:"nearest_deadline"`
	// Todos os itens relevantes finais e módulo ainda aberto: sugerir o fechamento [§10.5, R2].
	// Sempre false em grades sem fechamento próprio.
	ReadyToClose bool `json:"ready_to_close"`
	// Coluna "dependências" (DCTFWeb): apta ou pendente, com a lista; null nas demais grades.
	Dependencies *obligation.DependencyStatus `json:"dependencies"`
	Cells        map[string]Cell              `json:"cells"` // por código do tipo
}

// Grid é a resposta de GET /modules/{module}/grid.
type Grid struct {
	Module string `json:"module"`
	// Módulo do período das linhas (observação e agregado); é o módulo do RowSummary que
	// acompanha as alterações de status desse módulo.
	PeriodModule string          `json:"period_module"`
	Closable     bool            `json:"closable"`
	Competence   comp.Competence `json:"competence"`
	Group        *string         `json:"group"`
	Today        civil.Date      `json:"today"`
	Columns      []Column        `json:"columns"`
	Rows         []Row           `json:"rows"`
	Total        int             `json:"total"`
}

// Builder monta as grades.
type Builder struct {
	pool  *pgxpool.Pool
	clock clock.Clock
}

// NewBuilder cria o construtor.
func NewBuilder(pool *pgxpool.Pool, clk clock.Clock) *Builder {
	return &Builder{pool: pool, clock: clk}
}

type depJSON struct {
	TypeID   string `json:"type_id"`
	PrereqID string `json:"prereq_id"`
	Mode     string `json:"mode"`
}

// BuildGrid monta a grade de um módulo na competência com os filtros [T-M3-01]: três
// consultas (colunas + competência, linhas, células), sem N+1, numa transação somente
// leitura (visão consistente entre as consultas).
func (b *Builder) BuildGrid(ctx context.Context, spec Spec, c comp.Competence, f Filters) (Grid, error) {
	today := clock.Today(b.clock)
	g := Grid{Module: spec.Module, PeriodModule: spec.PeriodModule, Closable: spec.Closable, Competence: c,
		Today: civil.Date{Time: today}, Columns: []Column{}, Rows: []Row{}}
	if spec.Group != "" {
		g.Group = &spec.Group
	}
	tx, err := b.pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly, IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return g, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := store.New(tx)

	cols, deps, competenceID, err := columns(ctx, q, spec, c)
	if err != nil {
		return g, err
	}
	g.Columns = cols

	var group *string
	if spec.RegimeGroup != "" {
		group = &spec.RegimeGroup
	}
	rows, err := q.GridRows(ctx, store.GridRowsParams{
		PeriodModule: spec.PeriodModule, CompetenceID: competenceID, RegimeGroup: group, TaxRegime: f.TaxRegime,
		ResponsibleID: f.ResponsibleID, PeriodState: f.PeriodState, HasNote: f.HasNote,
	})
	if err != nil {
		return g, err
	}
	if len(rows) == 0 || len(cols) == 0 {
		g.Rows = buildRows(spec, rows, cols, nil, deps, c, today, f)
		g.Total = len(g.Rows)
		return g, nil
	}

	companyIDs := make([]string, len(rows))
	for i, r := range rows {
		companyIDs[i] = r.CompanyID
	}
	cells, err := q.GridCells(ctx, store.GridCellsParams{
		CompetenceID: competenceID, CompanyIds: companyIDs, TypeIds: cellTypes(cols, deps), PeriodModule: spec.PeriodModule,
	})
	if err != nil {
		return g, err
	}
	g.Rows = buildRows(spec, rows, cols, cells, deps, c, today, f)
	g.Total = len(g.Rows)
	return g, nil
}

// columns carrega as colunas com seus status, as dependências ativas e o id da competência
// (404 se ainda não gerada).
func columns(ctx context.Context, q *store.Queries, spec Spec, c comp.Competence) ([]Column, []obligation.Dependency, string, error) {
	rows, err := q.GridColumns(ctx, store.GridColumnsParams{CompetenceKey: c.String(), Module: spec.Module, Regimes: spec.Regimes})
	if err != nil {
		return nil, nil, "", err
	}
	if len(rows) == 0 || rows[0].CompetenceID == nil {
		return nil, nil, "", apperr.NotFound("COMPETENCE_NOT_FOUND", fmt.Sprintf("A competência %s ainda não foi gerada.", c.Display()))
	}
	cols := make([]Column, len(rows))
	var deps []obligation.Dependency
	for i, r := range rows {
		cols[i] = Column{
			ObligationType: obligation.ObligationTypeRef{ID: r.ID, Code: r.Code, Name: r.Name, PrimaryModule: r.PrimaryModule},
			SortOrder:      r.SortOrder, IsClosingStage: r.IsClosingStage,
			AllowsTransmissionDate: r.AllowsTransmissionDate, AllowsProtocol: r.AllowsProtocol, StatusSetCode: r.StatusSetCode,
		}
		if err := json.Unmarshal(r.Statuses, &cols[i].Statuses); err != nil {
			return nil, nil, "", fmt.Errorf("grid: status de %s: %w", r.Code, err)
		}
		if i == 0 {
			var ds []depJSON
			if err := json.Unmarshal(r.Dependencies, &ds); err != nil {
				return nil, nil, "", fmt.Errorf("grid: dependências: %w", err)
			}
			for _, d := range ds {
				deps = append(deps, obligation.Dependency{TypeID: d.TypeID, PrereqID: d.PrereqID, Mode: d.Mode})
			}
		}
	}
	return cols, deps, *rows[0].CompetenceID, nil
}

// cellTypes devolve os tipos das colunas e todos os seus pré-requisitos (cadeia HARD e SOFT),
// necessários para calcular blocked_by e dependency_status.
func cellTypes(cols []Column, deps []obligation.Dependency) []string {
	seen := map[string]bool{}
	var out []string
	add := func(id string) {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	for _, c := range cols {
		add(c.ObligationType.ID)
		for _, mode := range []string{obligation.DependencyHard, obligation.DependencySoft} {
			for _, p := range obligation.Prerequisites(c.ObligationType.ID, mode, deps, true) {
				add(p)
			}
		}
	}
	return out
}

func buildRows(spec Spec, rows []store.GridRowsRow, cols []Column, cells []store.GridCellsRow, deps []obligation.Dependency,
	c comp.Competence, today time.Time, f Filters) []Row {
	byCompany := map[string][]store.GridCellsRow{}
	for _, cell := range cells {
		byCompany[cell.CompanyID] = append(byCompany[cell.CompanyID], cell)
	}
	colByType := make(map[string]Column, len(cols))
	for _, col := range cols {
		colByType[col.ObligationType.ID] = col
	}

	out := []Row{}
	for _, r := range rows {
		factorR := r.FactorR
		if r.RegimeGroup != "SIMPLES" {
			factorR = nil // Fator R só se aplica ao Simples [PRD §17.2]
		}
		row := Row{
			Company: Company{ID: r.CompanyID, InternalCode: r.InternalCode, LegalName: r.LegalName, TradeName: r.TradeName, Cnpj: r.Cnpj},
			Snapshot: Snapshot{TaxRegime: r.TaxRegime, RegimeGroup: r.RegimeGroup, PayrollType: r.PayrollType, FactorR: factorR,
				Responsible: userRef(r.ResponsibleID, r.ResponsibleName)},
			Period: Period{ID: r.PeriodID, State: r.PeriodState, ClosedAt: r.ClosedAt, ClosedBy: userRef(r.ClosedByID, r.ClosedByName),
				GeneralNote: r.GeneralNote, Version: r.PeriodVersion},
			Cells: map[string]Cell{},
		}
		state := make(map[string]obligation.DepInstance, len(byCompany[r.CompanyID]))
		for _, cell := range byCompany[r.CompanyID] {
			state[cell.ObligationTypeID] = obligation.DepInstance{
				TypeID: cell.ObligationTypeID, Code: cell.TypeCode, Name: cell.TypeName, StatusCode: cell.StatusCode,
				Applicable: cell.Applicable, IsFinal: cell.IsFinal, Order: int(cell.TypeSortOrder),
			}
		}

		var items []obligation.Item
		var dues []*time.Time
		var applicable, final []bool
		m := match{}
		for _, cell := range byCompany[r.CompanyID] {
			// Agregado da linha: obrigações do módulo do período, como no fechamento (R1) e no
			// RowSummary devolvido pelas alterações de status.
			if cell.PrimaryModule == spec.PeriodModule {
				items = append(items, obligation.Item{
					Applicable: cell.Applicable, CountsInProgress: cell.CountsInProgress, CountsAsProgress: cell.CountsAsOperationalProgress,
					IsFinal: cell.IsFinal, IsBlocking: cell.IsBlocking, ExcludedFromDenominator: cell.ExcludedFromDenominator,
				})
				dues = append(dues, cell.DueDateEffective)
				applicable = append(applicable, cell.Applicable)
				final = append(final, cell.IsFinal)
				if obligation.IsOverdue(cell.Applicable, cell.IsFinal, cell.DueDateEffective, today) {
					row.Overdue++
				}
			}
			col, shown := colByType[cell.ObligationTypeID]
			if !shown {
				continue // pré-requisito ou obrigação do módulo fora das colunas desta grade
			}
			gc := toCell(cell, r.CompanyID, c, today)
			gc.Locked = cell.PeriodState == "CLOSED"
			if cell.Applicable && !cell.IsFinal {
				gc.BlockedBy = obligation.CheckHard(cell.ObligationTypeID, deps, state)
			}
			if gc.BlockedBy == nil {
				gc.BlockedBy = []obligation.DepInstance{}
			} else {
				msg := obligation.BlockedMessage(cell.TypeName, gc.BlockedBy)
				gc.BlockedMessage = &msg
			}
			gc.DependencyStatus = obligation.SoftDependencyStatus(cell.ObligationTypeID, deps, state)
			if spec.DependencyColumn != "" && cell.TypeCode == spec.DependencyColumn {
				row.Dependencies = gc.DependencyStatus
			}
			row.Cells[cell.TypeCode] = gc
			m.observe(cell, col, f)
		}
		row.Progress = obligation.Aggregate(items)
		row.NearestDeadline = civil.Ptr(obligation.NearestDeadline(dues, applicable, final))
		row.ReadyToClose = spec.Closable && row.Progress.ReadyToClose && r.PeriodState == "OPEN"
		if keep(row, m, f) {
			out = append(out, row)
		}
	}
	return out
}

// match acumula, por linha, o que os filtros derivados precisam.
type match struct {
	pending, notStarted, status, closingStatus bool
}

func (m *match) observe(cell store.GridCellsRow, col Column, f Filters) {
	if cell.Applicable && cell.IsBlocking {
		m.pending = true
	}
	if cell.Applicable && cell.CountsInProgress && !cell.ExcludedFromDenominator && !cell.CountsAsStarted && !cell.IsFinal {
		m.notStarted = true
	}
	if f.Status != nil && cell.Applicable && cell.StatusCode == *f.Status &&
		(f.ObligationTypeID == nil || *f.ObligationTypeID == cell.ObligationTypeID) {
		m.status = true
	}
	if f.ClosingStatus != nil && col.IsClosingStage && cell.StatusCode == *f.ClosingStatus {
		m.closingStatus = true
	}
}

func keep(row Row, m match, f Filters) bool {
	switch {
	case f.Overdue && row.Overdue == 0,
		f.Pending && !m.pending,
		f.NotStarted && !m.notStarted,
		f.ReadyToClose && !row.ReadyToClose,
		f.Status != nil && !m.status,
		f.ClosingStatus != nil && !m.closingStatus:
		return false
	}
	return true
}

func userRef(id, name *string) *obligation.UserRef {
	if id == nil || name == nil {
		return nil
	}
	return &obligation.UserRef{ID: *id, Name: *name}
}

func toCell(v store.GridCellsRow, companyID string, c comp.Competence, today time.Time) Cell {
	return Cell{Instance: obligation.Instance{
		ID: v.ID, CompanyID: companyID, Competence: c,
		ObligationType: obligation.ObligationTypeRef{ID: v.ObligationTypeID, Code: v.TypeCode, Name: v.TypeName, PrimaryModule: v.PrimaryModule},
		StatusCode:     v.StatusCode, StatusLabel: v.StatusLabel, Applicable: v.Applicable, CountsInProgress: v.CountsInProgress,
		IsFinal: v.IsFinal, IsStarted: v.CountsAsStarted, IsBlocking: v.IsBlocking,
		IsOverdue:       obligation.IsOverdue(v.Applicable, v.IsFinal, v.DueDateEffective, today),
		DueDateOriginal: civil.Ptr(v.DueDateOriginal), DueDateEffective: civil.Ptr(v.DueDateEffective),
		DueOverridden: v.DueDateOverrideReason != nil, DueDateOverrideReason: v.DueDateOverrideReason,
		TransmissionDate: civil.Ptr(v.TransmissionDate), Protocol: v.Protocol,
		HasHistory: v.HasHistory, Version: v.Version, LastChangedAt: v.LastChangedAt, LastChangedBy: v.LastChangedByName,
	}}
}
