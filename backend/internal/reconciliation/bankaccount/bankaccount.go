// Package bankaccount é o cadastro das contas bancárias da empresa (conta_bancaria da origem,
// CC-11; TASK-021c, D-034). A conta cadastrada decide a empresa dos próximos extratos dela, e o
// extrato dela enviado para outra empresa vai para a revisão (a regra da importação, D-027).
//
// O banco, a agência e a conta precisam ser exatamente os que a leitura do extrato grava (o leitor
// do cenário grava "a_fonte" ou "beta"). Por isso a lista traz também as contas que os extratos da
// empresa já leram e que ninguém cadastrou, para o cadastro com um clique.
package bankaccount

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"controle/internal/platform/apperr"
	"controle/internal/platform/clock"
	"controle/internal/platform/httpx"
	"controle/internal/reconciliation/ledger"
	"controle/internal/store"
	"controle/internal/user"
)

// Tamanhos da origem (conta_bancaria: banco 64, agência 16, conta 32).
const (
	maxBank    = 64
	maxBranch  = 16
	maxAccount = 32
)

// Service cadastra e lista as contas.
type Service struct {
	pool  *pgxpool.Pool
	clock clock.Clock
}

// NewService cria o serviço.
func NewService(pool *pgxpool.Pool, clk clock.Clock) *Service {
	return &Service{pool: pool, clock: clk}
}

// Account é uma conta cadastrada, com os extratos já importados dela pela empresa.
type Account struct {
	ID            string    `json:"id"`
	CompanyID     string    `json:"company_id"`
	Bank          string    `json:"bank"`
	Branch        string    `json:"branch"`
	Account       string    `json:"account"`
	Label         string    `json:"label"`
	Active        bool      `json:"active"`
	Statements    int       `json:"statements"`
	LastPeriodEnd *string   `json:"last_period_end"`
	CreatedBy     *string   `json:"created_by"`
	CreatedAt     time.Time `json:"created_at"`
}

// Read é uma conta lida nos extratos importados da empresa que ninguém cadastrou.
type Read struct {
	Bank          string `json:"bank"`
	Branch        string `json:"branch"`
	Account       string `json:"account"`
	Label         string `json:"label"`
	Statements    int    `json:"statements"`
	LastPeriodEnd string `json:"last_period_end"`
}

// List são as contas da empresa e as lidas sem cadastro.
type List struct {
	Items []Account `json:"items"`
	Read  []Read    `json:"read"`
}

// Input é o cadastro: banco, agência e conta.
type Input struct {
	Bank    string `json:"bank"`
	Branch  string `json:"branch"`
	Account string `json:"account"`
}

func companyNotFound() error { return apperr.NotFound("COMPANY_NOT_FOUND", "Empresa não encontrada.") }

func requireCompany(ctx context.Context, q *store.Queries, id string) error {
	if !httpx.IsUUID(id) {
		return companyNotFound()
	}
	if _, err := q.GetCompanyIdentity(ctx, id); errors.Is(err, pgx.ErrNoRows) {
		return companyNotFound()
	} else if err != nil {
		return err
	}
	return nil
}

// List lê as contas cadastradas da empresa e as lidas nos extratos sem cadastro.
func (s *Service) List(ctx context.Context, companyID string) (List, error) {
	q := store.New(s.pool)
	if err := requireCompany(ctx, q, companyID); err != nil {
		return List{}, err
	}
	rows, err := q.ListCompanyBankAccounts(ctx, companyID)
	if err != nil {
		return List{}, err
	}
	out := List{Items: make([]Account, 0, len(rows)), Read: []Read{}}
	for _, r := range rows {
		a := Account{ID: r.ID, CompanyID: r.CompanyID, Bank: r.Bank, Branch: r.Branch, Account: r.Account,
			Label: ledger.BankLabel(r.Bank, r.Branch, r.Account), Active: r.Active, Statements: int(r.Statements), CreatedBy: r.CreatedBy,
			CreatedAt: r.CreatedAt}
		if r.LastPeriodEnd != "" {
			last := r.LastPeriodEnd
			a.LastPeriodEnd = &last
		}
		out.Items = append(out.Items, a)
	}
	read, err := q.ListReadBankAccounts(ctx, companyID)
	if err != nil {
		return List{}, err
	}
	for _, r := range read {
		out.Read = append(out.Read, Read{Bank: r.Bank, Branch: r.Branch, Account: r.Account, Label: ledger.BankLabel(r.Bank, r.Branch, r.Account),
			Statements: int(r.Statements), LastPeriodEnd: r.LastPeriodEnd.Format(time.DateOnly)})
	}
	return out, nil
}

// clean confere um campo: sem espaços nas pontas, obrigatório e no tamanho da origem.
func clean(fields map[string]any, name, value, label string, limit int) string {
	v := strings.TrimSpace(value)
	switch {
	case v == "":
		fields[name] = "Informe " + label + "."
	case len([]rune(v)) > limit:
		fields[name] = fmt.Sprintf("Passa de %d caracteres.", limit)
	case strings.ContainsAny(v, "\r\n\t"):
		fields[name] = "Não use quebra de linha nem tabulação."
	}
	return v
}

// Create cadastra a conta na empresa. A identidade (banco, agência e conta) é única no sistema:
// já cadastrada, aqui ou em outra empresa, é 409 BANK_ACCOUNT_EXISTS.
func (s *Service) Create(ctx context.Context, companyID string, in Input) (Account, error) {
	fields := map[string]any{}
	bank := clean(fields, "bank", in.Bank, "o banco", maxBank)
	branch := clean(fields, "branch", in.Branch, "a agência", maxBranch)
	account := clean(fields, "account", in.Account, "a conta", maxAccount)
	var created store.BankAccount
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		if err := requireCompany(ctx, q, companyID); err != nil {
			return err
		}
		if len(fields) > 0 {
			return apperr.BadRequest("VALIDATION_ERROR", "Dados inválidos. Verifique os campos destacados.").
				WithDetails(map[string]any{"fields": fields})
		}
		uid := user.FromContext(ctx)
		now := s.clock.Now()
		var err error
		created, err = q.InsertBankAccount(ctx, store.InsertBankAccountParams{CompanyID: companyID, Bank: bank, Branch: branch,
			Account: account, CreatedBy: &uid, Now: now})
		if errors.Is(err, pgx.ErrNoRows) {
			return exists(ctx, q, companyID, bank, branch, account)
		}
		if err != nil {
			return err
		}
		change, err := json.Marshal(map[string]any{"acao": "cadastrar_conta_bancaria", "conta_bancaria": created.ID, "banco": bank,
			"agencia": branch, "conta": account})
		if err != nil {
			return err
		}
		text := string(change)
		_, err = q.InsertExtractionLog(ctx, store.InsertExtractionLogParams{
			CompanyID: &companyID, Alias: "humano", Attempt: 0, ActorID: &uid, Change: &text, Now: now,
		})
		return err
	})
	if err != nil {
		return Account{}, err
	}
	list, err := s.List(ctx, companyID)
	if err != nil {
		return Account{}, err
	}
	for _, a := range list.Items {
		if a.ID == created.ID {
			return a, nil
		}
	}
	return Account{}, fmt.Errorf("conta bancária %s não encontrada depois do cadastro", created.ID)
}

// exists explica o 409: a conta já é desta empresa ou de outra.
func exists(ctx context.Context, q *store.Queries, companyID, bank, branch, account string) error {
	owner, err := q.GetBankAccountOwner(ctx, store.GetBankAccountOwnerParams{Bank: bank, Branch: branch, Account: account})
	if err != nil {
		return err
	}
	if owner.CompanyID == companyID {
		return apperr.Conflict("BANK_ACCOUNT_EXISTS", "Essa conta já está cadastrada nesta empresa.")
	}
	return apperr.Conflict("BANK_ACCOUNT_EXISTS", fmt.Sprintf("Essa conta já está cadastrada na empresa %s · %s.", owner.InternalCode,
		owner.LegalName)).WithDetails(map[string]any{"company_id": owner.CompanyID})
}
