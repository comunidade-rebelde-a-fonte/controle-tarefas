// Package metrics é o painel de métricas da Conciliação (11-observability §7), os playbooks por
// banco e o resumo da competência na ficha da empresa (TASK-021c, D-034). Porte das rotas
// /metricas e /playbooks do Conciliador (app/web/rotas.py), que mostravam o percentual
// automático, as conciliações por passe e a tabela dos playbooks; os demais números do §7 são
// novos, sempre consultas sobre as tabelas da Conciliação (não há tabela de métrica).
package metrics

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"controle/internal/platform/apperr"
	"controle/internal/platform/competence"
	"controle/internal/platform/decimal"
	"controle/internal/platform/httpx"
	"controle/internal/reconciliation"
	"controle/internal/reconciliation/ledger"
	"controle/internal/reconciliation/review"
	"controle/internal/reconciliation/statement"
	"controle/internal/store"
)

// Agentes cujo custo por item o painel mostra (M4.7 da origem).
const (
	agentResolver   = "resolvedor"
	agentClassifier = "classificador"
)

// Service lê as métricas; target é a meta da conciliação automática (META_CONCILIACAO_AUTO).
type Service struct {
	pool   *pgxpool.Pool
	target decimal.Decimal
}

// NewService cria o serviço.
func NewService(pool *pgxpool.Pool, target decimal.Decimal) *Service {
	return &Service{pool: pool, target: target}
}

// Filter são os filtros do painel, os dois opcionais: a empresa e a competência.
type Filter struct {
	CompanyID  *string
	Competence *competence.Competence
}

func (f Filter) day() *time.Time {
	if f.Competence == nil {
		return nil
	}
	d := f.Competence.FirstDay()
	return &d
}

// Matching é a conciliação das linhas do extrato: o percentual automático da origem
// (automáticas ÷ linhas, uma casa) contra a meta, e as conciliações ativas por método.
type Matching struct {
	Lines         int            `json:"lines"`
	Reconciled    int            `json:"reconciled"`
	Auto          int            `json:"auto"`
	Manual        int            `json:"manual"`
	Free          int            `json:"free"`
	AutoPercent   *string        `json:"auto_percent"`
	TargetPercent string         `json:"target_percent"`
	ByMethod      map[string]int `json:"by_method"`
	UndoneAuto    int            `json:"undone_auto"`
}

// Extraction é a leitura dos documentos de um tipo, como o make eval-extrator: o agente conta as
// tentativas; o leitor determinístico fica à parte; o custo é o de todas as chamadas de modelo.
type Extraction struct {
	Documents           int     `json:"documents"`
	LocalReader         int     `json:"local_reader"`
	Agent               int     `json:"agent"`
	FirstAttempt        int     `json:"first_attempt"`
	FirstAttemptPercent *string `json:"first_attempt_percent"`
	AverageAttempts     *string `json:"average_attempts"`
	NotApproved         int     `json:"not_approved"`
	CostUSD             string  `json:"cost_usd"`
	CostPerDocumentUSD  *string `json:"cost_per_document_usd"`
}

// ExtractionByKind separa extratos e comprovantes.
type ExtractionByKind struct {
	Statements Extraction `json:"statements"`
	Receipts   Extraction `json:"receipts"`
}

// Corrections são as correções humanas depois da leitura (campo corrigido e "que documento é
// esse?"): a taxa é dos documentos corrigidos sobre os lidos.
type Corrections struct {
	Actions     int     `json:"actions"`
	Documents   int     `json:"documents"`
	RatePercent *string `json:"rate_percent"`
}

// AfterClose é o "erro detectado depois do fechamento" (meta zero; decisão do usuário, D-034): as
// reaberturas do Contábil e, à parte, os documentos que chegaram com a competência fechada.
type AfterClose struct {
	Reopenings    int `json:"reopenings"`
	LateDocuments int `json:"late_documents"`
}

// AgentCost são as corridas de um agente e o custo por item.
type AgentCost struct {
	Runs          int     `json:"runs"`
	CostUSD       string  `json:"cost_usd"`
	CostPerRunUSD *string `json:"cost_per_run_usd"`
}

// Agents são os agentes que só sugerem: o resolvedor e o classificador.
type Agents struct {
	Resolver   AgentCost `json:"resolver"`
	Classifier AgentCost `json:"classifier"`
}

// PlaybookStats é o sucesso dos playbooks de um banco, somando as versões.
type PlaybookStats struct {
	Bank           string  `json:"bank"`
	Versions       int     `json:"versions"`
	ValidVersion   *int    `json:"valid_version"`
	Successes      int     `json:"successes"`
	Failures       int     `json:"failures"`
	SuccessPercent *string `json:"success_percent"`
}

// Metrics é o painel (11-observability §7).
type Metrics struct {
	CompanyID   *string          `json:"company_id"`
	Competence  *string          `json:"competence"`
	Matching    Matching         `json:"matching"`
	Buckets     map[string]int   `json:"buckets"`
	Extraction  ExtractionByKind `json:"extraction"`
	Corrections Corrections      `json:"corrections"`
	AfterClose  AfterClose       `json:"after_close"`
	Agents      Agents           `json:"agents"`
	Playbooks   []PlaybookStats  `json:"playbooks"`
}

// percent é part ÷ whole em percentual com as casas pedidas (half-even); nulo sem base.
func percent(part, whole int64, places int32) *string {
	if whole <= 0 {
		return nil
	}
	s := decimal.FromInt(part * 100).Quo(decimal.FromInt(whole)).Quantize(-places).Text()
	return &s
}

// ratio é amount ÷ n com as casas pedidas; nulo sem base.
func ratio(amount decimal.Decimal, n int64, places int32) *string {
	if n <= 0 {
		return nil
	}
	s := amount.Quo(decimal.FromInt(n)).Quantize(-places).Text()
	return &s
}

func (s *Service) matching(ctx context.Context, q *store.Queries, companyID *string, day *time.Time) (Matching, error) {
	m, err := q.GetMatchingMetrics(ctx, store.GetMatchingMetricsParams{CompanyID: companyID, Competence: day})
	if err != nil {
		return Matching{}, err
	}
	methods, err := q.ListMatchingByMethod(ctx, store.ListMatchingByMethodParams{CompanyID: companyID, Competence: day})
	if err != nil {
		return Matching{}, err
	}
	out := Matching{
		Lines: int(m.Lines), Auto: int(m.Auto), Manual: int(m.Manual), Reconciled: int(m.Auto + m.Manual),
		Free: int(m.Lines - m.Auto - m.Manual), AutoPercent: percent(m.Auto, m.Lines, 1), TargetPercent: s.target.Text(),
		ByMethod: map[string]int{}, UndoneAuto: int(m.UndoneAuto),
	}
	for _, r := range methods {
		out.ByMethod[r.Method] = int(r.Total)
	}
	return out, nil
}

// usd é o custo em dólares com as 8 casas do log (o zero lido do banco vem sem casas).
func usd(d decimal.Decimal) string { return d.Quantize(-8).Text() }

func extraction(r store.ListExtractionMetricsRow) Extraction {
	return Extraction{
		Documents: int(r.Documents), LocalReader: int(r.LocalReader), Agent: int(r.Agent), FirstAttempt: int(r.FirstAttempt),
		FirstAttemptPercent: percent(r.FirstAttempt, r.Agent, 1), AverageAttempts: ratio(decimal.FromInt(r.AgentAttempts), r.Agent, 2),
		NotApproved: int(r.NotApproved), CostUSD: usd(r.CostUsd), CostPerDocumentUSD: ratio(r.CostUsd, r.Documents, 8),
	}
}

func emptyExtraction() Extraction { return Extraction{CostUSD: usd(decimal.FromInt(0))} }

func agentCost(runs int64, cost decimal.Decimal) AgentCost {
	return AgentCost{Runs: int(runs), CostUSD: usd(cost), CostPerRunUSD: ratio(cost, runs, 8)}
}

// Metrics lê o painel com os filtros.
func (s *Service) Metrics(ctx context.Context, f Filter) (Metrics, error) {
	q := store.New(s.pool)
	day := f.day()
	out := Metrics{CompanyID: f.CompanyID, Extraction: ExtractionByKind{Statements: emptyExtraction(), Receipts: emptyExtraction()},
		Agents:    Agents{Resolver: agentCost(0, decimal.FromInt(0)), Classifier: agentCost(0, decimal.FromInt(0))},
		Playbooks: []PlaybookStats{}}
	if f.Competence != nil {
		c := f.Competence.String()
		out.Competence = &c
	}
	var err error
	if out.Matching, err = s.matching(ctx, q, f.CompanyID, day); err != nil {
		return Metrics{}, err
	}
	if out.Buckets, err = review.Counts(ctx, q, f.CompanyID, day); err != nil {
		return Metrics{}, err
	}
	kinds, err := q.ListExtractionMetrics(ctx, store.ListExtractionMetricsParams{CompanyID: f.CompanyID, Competence: day})
	if err != nil {
		return Metrics{}, err
	}
	read := int64(0)
	for _, k := range kinds {
		read += k.Documents
		switch k.Kind {
		case statement.KindStatement:
			out.Extraction.Statements = extraction(k)
		case statement.KindReceipt:
			out.Extraction.Receipts = extraction(k)
		}
	}
	c, err := q.GetCorrectionMetrics(ctx, store.GetCorrectionMetricsParams{CompanyID: f.CompanyID, Competence: day})
	if err != nil {
		return Metrics{}, err
	}
	out.Corrections = Corrections{Actions: int(c.Actions), Documents: int(c.Documents), RatePercent: percent(c.Documents, read, 1)}
	a, err := q.GetAfterCloseMetrics(ctx, store.GetAfterCloseMetricsParams{CompanyID: f.CompanyID, Competence: day})
	if err != nil {
		return Metrics{}, err
	}
	out.AfterClose = AfterClose{Reopenings: int(a.Reopenings), LateDocuments: int(a.LateDocuments)}
	agents, err := q.ListAgentRunMetrics(ctx, store.ListAgentRunMetricsParams{CompanyID: f.CompanyID, Competence: day})
	if err != nil {
		return Metrics{}, err
	}
	for _, r := range agents {
		switch r.Agent {
		case agentResolver:
			out.Agents.Resolver = agentCost(r.Runs, r.CostUsd)
		case agentClassifier:
			out.Agents.Classifier = agentCost(r.Runs, r.CostUsd)
		}
	}
	stats, err := q.ListPlaybookStats(ctx)
	if err != nil {
		return Metrics{}, err
	}
	for _, p := range stats {
		ps := PlaybookStats{Bank: p.Bank, Versions: int(p.Versions), Successes: int(p.Successes), Failures: int(p.Failures),
			SuccessPercent: percent(p.Successes, p.Successes+p.Failures, 1)}
		if p.ValidVersion != nil {
			v := int(*p.ValidVersion)
			ps.ValidVersion = &v
		}
		out.Playbooks = append(out.Playbooks, ps)
	}
	return out, nil
}

// Playbook é uma versão do playbook de um banco (playbook_banco da origem).
type Playbook struct {
	ID                 string     `json:"id"`
	Bank               string     `json:"bank"`
	Version            int        `json:"version"`
	Valid              bool       `json:"valid"`
	Successes          int        `json:"successes"`
	Failures           int        `json:"failures"`
	SuccessPercent     *string    `json:"success_percent"`
	TableStartPage     int        `json:"table_start_page"`
	IgnoredPages       []int      `json:"ignored_pages"`
	DateFormat         string     `json:"date_format"`
	DecimalSeparator   string     `json:"decimal_separator"`
	SignRepresentation string     `json:"sign_representation"`
	BalanceLocation    string     `json:"balance_location"`
	WinningStrategy    string     `json:"winning_strategy"`
	ValidatedAt        *time.Time `json:"validated_at"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

// Playbooks lista os playbooks, por banco e da versão mais nova para a mais antiga.
func (s *Service) Playbooks(ctx context.Context) ([]Playbook, error) {
	rows, err := store.New(s.pool).ListBankPlaybooks(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Playbook, 0, len(rows))
	for _, p := range rows {
		pages := []int{}
		if err := json.Unmarshal(p.IgnoredPages, &pages); err != nil {
			return nil, err
		}
		out = append(out, Playbook{
			ID: p.ID, Bank: p.Bank, Version: int(p.Version), Valid: p.Valid, Successes: int(p.Successes), Failures: int(p.Failures),
			SuccessPercent: percent(int64(p.Successes), int64(p.Successes)+int64(p.Failures), 1), TableStartPage: int(p.TableStartPage),
			IgnoredPages: pages, DateFormat: p.DateFormat, DecimalSeparator: p.DecimalSeparator, SignRepresentation: p.SignRepresentation,
			BalanceLocation: p.BalanceLocation, WinningStrategy: p.WinningStrategy, ValidatedAt: p.ValidatedAt, CreatedAt: p.CreatedAt,
			UpdatedAt: p.UpdatedAt,
		})
	}
	return out, nil
}

// Statement é um extrato importado que cobre a competência.
type Statement struct {
	ImportID       string `json:"import_id"`
	DocumentID     string `json:"document_id"`
	FileName       string `json:"file_name"`
	BankAccount    string `json:"bank_account"`
	PeriodStart    string `json:"period_start"`
	PeriodEnd      string `json:"period_end"`
	OpeningBalance string `json:"opening_balance"`
	ClosingBalance string `json:"closing_balance"`
	Attempts       int    `json:"attempts"`
	Transactions   int    `json:"transactions"`
}

// EntryCounts são os lançamentos do cliente da competência.
type EntryCounts struct {
	Total   int `json:"total"`
	Partial int `json:"partial"`
}

// ClassificationCounts são as linhas conciliadas da competência, classificadas ou à espera.
type ClassificationCounts struct {
	Classified int `json:"classified"`
	Pending    int `json:"pending"`
}

// Summary é o estado da Conciliação da empresa na competência (a aba da ficha).
type Summary struct {
	CompanyID        string               `json:"company_id"`
	Competence       string               `json:"competence"`
	AccountingClosed bool                 `json:"accounting_closed"`
	Statements       []Statement          `json:"statements"`
	Matching         Matching             `json:"matching"`
	Buckets          map[string]int       `json:"buckets"`
	Entries          EntryCounts          `json:"entries"`
	Classification   ClassificationCounts `json:"classification"`
}

func companyNotFound() error { return apperr.NotFound("COMPANY_NOT_FOUND", "Empresa não encontrada.") }

// Summary lê o estado da Conciliação da empresa na competência.
func (s *Service) Summary(ctx context.Context, companyID string, c competence.Competence) (Summary, error) {
	q := store.New(s.pool)
	if !httpx.IsUUID(companyID) {
		return Summary{}, companyNotFound()
	}
	if _, err := q.GetCompanyIdentity(ctx, companyID); errors.Is(err, pgx.ErrNoRows) {
		return Summary{}, companyNotFound()
	} else if err != nil {
		return Summary{}, err
	}
	day := c.FirstDay()
	out := Summary{CompanyID: companyID, Competence: c.String(), Statements: []Statement{}}
	var err error
	if out.AccountingClosed, err = reconciliation.AccountingClosed(ctx, s.pool, companyID, c); err != nil {
		return Summary{}, err
	}
	rows, err := q.ListCompetenceStatements(ctx, store.ListCompetenceStatementsParams{CompanyID: companyID, Competence: day})
	if err != nil {
		return Summary{}, err
	}
	for _, r := range rows {
		out.Statements = append(out.Statements, Statement{
			ImportID: r.ID, DocumentID: r.DocumentID, FileName: r.FileName, BankAccount: ledger.BankLabel(r.Bank, r.Branch, r.Account),
			PeriodStart: r.PeriodStart.Format(time.DateOnly), PeriodEnd: r.PeriodEnd.Format(time.DateOnly),
			OpeningBalance: r.OpeningBalance.Text(), ClosingBalance: r.ClosingBalance.Text(), Attempts: int(r.Attempts),
			Transactions: int(r.Transactions),
		})
	}
	if out.Matching, err = s.matching(ctx, q, &companyID, &day); err != nil {
		return Summary{}, err
	}
	if out.Buckets, err = review.Counts(ctx, q, &companyID, &day); err != nil {
		return Summary{}, err
	}
	e, err := q.GetCompetenceEntries(ctx, store.GetCompetenceEntriesParams{CompanyID: companyID, Competence: day})
	if err != nil {
		return Summary{}, err
	}
	out.Entries = EntryCounts{Total: int(e.Total), Partial: int(e.Partial)}
	cl, err := q.GetCompetenceClassification(ctx, store.GetCompetenceClassificationParams{CompanyID: companyID, Competence: day})
	if err != nil {
		return Summary{}, err
	}
	out.Classification = ClassificationCounts{Classified: int(cl.Classified), Pending: int(cl.Pending)}
	return out, nil
}
