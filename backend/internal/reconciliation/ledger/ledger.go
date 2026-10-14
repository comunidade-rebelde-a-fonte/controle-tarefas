// Package ledger é o razão da Conciliação [REQ-028, REQ-029, AC-021, AC-022; TASK-020, D-031]:
// porte de app/servicos/razao.py do Conciliador (commit 2bc6e26). O razão é uma consulta sobre as
// conciliações ativas, nunca uma tabela escrita: reclassificar ou desfazer muda a origem, e o
// razão acompanha.
//
// Cada linha leva o que o drill-down precisa (classificação → conciliação → transação →
// documento): a conta contábil e a origem dela, a conciliação, a linha do extrato, o original do
// extrato e os comprovantes. A exportação CSV mantém as colunas da origem e acrescenta as do
// AC-022; a planilha XLSX (TASK-028) tem as mesmas colunas e um resumo (xlsx.go).
package ledger

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/text/cases"

	"controle/internal/platform/apperr"
	"controle/internal/platform/competence"
	"controle/internal/platform/decimal"
	"controle/internal/platform/httpx"
	"controle/internal/platform/pymodel"
	"controle/internal/reconciliation"
	"controle/internal/store"
)

// Service lê o razão.
type Service struct{ pool *pgxpool.Pool }

// NewService cria o serviço.
func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

// Line é uma linha do razão: uma conciliação ativa.
type Line struct {
	Date                   string          `json:"date"`
	Competence             string          `json:"competence"`
	BankAccount            string          `json:"bank_account"`
	AccountCode            *string         `json:"account_code"`
	AccountName            *string         `json:"account_name"`
	ClassificationOrigin   *string         `json:"classification_origin"`
	ClassificationPending  bool            `json:"classification_pending"`
	History                string          `json:"history"`
	Counterparty           *string         `json:"counterparty"`
	Debit                  decimal.Decimal `json:"debit"`
	Credit                 decimal.Decimal `json:"credit"`
	ReconciliationID       string          `json:"reconciliation_id"`
	ReconciliationMethod   string          `json:"reconciliation_method"`
	ReconciliationOrigin   string          `json:"reconciliation_origin"`
	StatementTransactionID string          `json:"statement_transaction_id"`
	StatementDocumentID    string          `json:"statement_document_id"`
	EntryIDs               []string        `json:"entry_ids"`
	ReceiptDocumentIDs     []string        `json:"receipt_document_ids"`
}

// Ledger é o razão do período, com os totais.
type Ledger struct {
	CompanyID string          `json:"company_id"`
	From      string          `json:"from"`
	To        string          `json:"to"`
	Items     []Line          `json:"items"`
	Debit     decimal.Decimal `json:"total_debit"`
	Credit    decimal.Decimal `json:"total_credit"`
}

// Query é o filtro do razão: a empresa, o período de competências e a conta (bancária ou
// contábil), como o filtro da origem.
type Query struct {
	CompanyID string
	From, To  competence.Competence
	Account   string
}

var (
	zero   = decimal.MustParse("0.00")
	folder = cases.Fold()
)

// BankLabel é o rotulo_conta_bancaria da origem: "banco agência/conta"; sem agência e conta, só o
// banco.
func BankLabel(bank, branch, account string) string {
	bank, branch, account = pymodel.Strip(bank), pymodel.Strip(branch), pymodel.Strip(account)
	if branch != "" || account != "" {
		return pymodel.Strip(fmt.Sprintf("%s %s/%s", bank, branch, account))
	}
	return bank
}

// counterparty junta as contrapartes distintas dos comprovantes (normalizadas), na ordem.
func counterparty(list []string) *string {
	var out []string
	seen := map[string]bool{}
	for _, c := range list {
		k := reconciliation.NormalizeDescription(c)
		if k == "" || seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, strings.TrimSpace(c))
	}
	if len(out) == 0 {
		return nil
	}
	s := strings.Join(out, " / ")
	return &s
}

// matchesAccount é o filtro da origem: a conta bancária contém o texto (sem caixa) ou a conta
// contábil é igual a ele.
func matchesAccount(account, bankLabel string, code *string) bool {
	if account == "" {
		return true
	}
	return strings.Contains(folder.String(bankLabel), folder.String(account)) || (code != nil && *code == account)
}

func companyNotFound() error { return apperr.NotFound("COMPANY_NOT_FOUND", "Empresa não encontrada.") }

// List lê o razão: uma linha por conciliação ativa do período, por data e conta bancária. O
// débito é a saída do extrato e o crédito a entrada, como na origem.
func (s *Service) List(ctx context.Context, in Query) (Ledger, error) {
	if !httpx.IsUUID(in.CompanyID) {
		return Ledger{}, companyNotFound()
	}
	if in.To.Before(in.From) {
		return Ledger{}, apperr.BadRequest("VALIDATION_ERROR", "Dados inválidos. Verifique os campos destacados.").
			WithDetails(map[string]any{"fields": map[string]any{"to": "O fim do período vem antes do início."}})
	}
	q := store.New(s.pool)
	if _, err := q.GetCompanyIdentity(ctx, in.CompanyID); errors.Is(err, pgx.ErrNoRows) {
		return Ledger{}, companyNotFound()
	} else if err != nil {
		return Ledger{}, err
	}
	from, to := in.From.FirstDay(), in.To.FirstDay()
	rows, err := q.ListLedgerLines(ctx, store.ListLedgerLinesParams{CompanyID: in.CompanyID, CompetenceFrom: &from, CompetenceTo: &to})
	if err != nil {
		return Ledger{}, err
	}
	out := Ledger{CompanyID: in.CompanyID, From: in.From.String(), To: in.To.String(), Items: []Line{}, Debit: zero, Credit: zero}
	for _, r := range rows {
		label := BankLabel(r.Bank, r.Branch, r.BankAccount)
		if !matchesAccount(in.Account, label, r.AccountCode) {
			continue
		}
		l := Line{
			Date: r.TxnDate.Format(time.DateOnly), Competence: competence.FromDate(r.TxnDate).String(), BankAccount: label,
			AccountCode: r.AccountCode, AccountName: r.AccountName, ClassificationOrigin: r.ClassificationOrigin,
			ClassificationPending: r.ClassificationPending, History: r.Description, Counterparty: counterparty(r.Counterparties),
			Debit: zero, Credit: zero, ReconciliationID: r.ReconciliationID, ReconciliationMethod: r.Method,
			ReconciliationOrigin: r.ReconciliationOrigin, StatementTransactionID: r.StatementTransactionID,
			StatementDocumentID: r.StatementDocumentID, EntryIDs: r.EntryIds, ReceiptDocumentIDs: r.ReceiptDocumentIds,
		}
		if r.Direction == "D" {
			l.Debit = r.Amount
		} else {
			l.Credit = r.Amount
		}
		out.Debit, out.Credit = out.Debit.Add(l.Debit), out.Credit.Add(l.Credit)
		out.Items = append(out.Items, l)
	}
	return out, nil
}

// csvHeader são as colunas da origem (data;conta;historico;debito;credito) e as do AC-022.
var csvHeader = []string{"data", "conta", "historico", "debito", "credito", "competencia", "conta_contabil", "nome_conta",
	"contraparte", "origem_classificacao", "origem_conciliacao"}

func text(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// CSV escreve o razão separado por ponto e vírgula, uma linha por conciliação, na ordem do razão:
// determinístico para o mesmo banco. Campo com ponto e vírgula, aspas ou quebra vai entre aspas
// (a origem não escapava).
func CSV(l Ledger) []byte {
	var b bytes.Buffer
	w := csv.NewWriter(&b)
	w.Comma = ';'
	_ = w.Write(csvHeader)
	for _, r := range l.Items {
		origin := ""
		if r.ClassificationOrigin != nil && !r.ClassificationPending {
			origin = *r.ClassificationOrigin
		}
		_ = w.Write([]string{r.Date, r.BankAccount, r.History, r.Debit.Text(), r.Credit.Text(), r.Competence, text(r.AccountCode),
			text(r.AccountName), text(r.Counterparty), origin, r.ReconciliationOrigin})
	}
	w.Flush()
	return b.Bytes()
}

// Formatos da exportação: o CSV da origem e a planilha (TASK-028).
const (
	FormatCSV  = "csv"
	FormatXLSX = "xlsx"
)

// Export monta o arquivo do razão no formato pedido: o nome razao-<cnpj>-<de>-<até>.<formato>, o
// tipo e o conteúdo. account é o filtro de conta, que a planilha mostra no resumo.
func (s *Service) Export(ctx context.Context, l Ledger, account, format string) (name, contentType string, body []byte, err error) {
	q := store.New(s.pool)
	c, err := q.GetCompanyIdentity(ctx, l.CompanyID)
	if err != nil {
		return "", "", nil, err
	}
	name = fmt.Sprintf("razao-%s-%s-%s.%s", c.Cnpj, l.From, l.To, format)
	if format != FormatXLSX {
		return name, "text/csv; charset=utf-8", CSV(l), nil
	}
	ref, err := q.GetCompanyRef(ctx, l.CompanyID)
	if err != nil {
		return "", "", nil, err
	}
	body, err = XLSX(l, Info{CompanyCode: ref.InternalCode, CompanyName: ref.LegalName, Cnpj: c.Cnpj, Account: account})
	return name, MimeXLSX, body, err
}
