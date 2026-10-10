// Package chart é o plano de contas e as regras de classificação de cada empresa [REQ-026,
// REQ-027; TASK-020, D-031]: as tabelas plano_contas e regra_classificacao do Conciliador
// (commit 2bc6e26), que a origem modelou sem tela nem rota (M5.1 e M5.2).
//
// Plano por empresa, com importação CSV e cópia de outra empresa (o "padrão" da D-020). Regras por
// CNPJ, contraparte ou regex do descritivo: a mais nova vale, e a regra confirmada com o mesmo tipo
// e padrão de outra desativa a antiga. Nada é apagado.
package chart

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"controle/internal/platform/apperr"
	"controle/internal/platform/clock"
	"controle/internal/platform/httpx"
	"controle/internal/platform/pytext"
	"controle/internal/reconciliation/classification"
	"controle/internal/reconciliation/entries"
	"controle/internal/store"
	"controle/internal/user"
)

// Limites do plano.
const (
	maxCode      = 32 // o String(32) da origem
	maxName      = 255
	MaxCSVBytes  = 2 << 20
	maxCSVLines  = 5000
	maxCSVErrors = 50
	maxPattern   = 512 // o String(512) da origem
)

// Service atende o plano de contas e as regras.
type Service struct {
	pool  *pgxpool.Pool
	clock clock.Clock
}

// NewService cria o serviço.
func NewService(pool *pgxpool.Pool, clk clock.Clock) *Service {
	return &Service{pool: pool, clock: clk}
}

// Account é uma conta do plano.
type Account struct {
	ID        string    `json:"id"`
	CompanyID string    `json:"company_id"`
	Code      string    `json:"code"`
	Name      string    `json:"name"`
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func toAccount(a store.ChartAccount) Account {
	return Account{ID: a.ID, CompanyID: a.CompanyID, Code: a.Code, Name: a.Name, Active: a.Active, CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt}
}

// Rule é uma regra de classificação.
type Rule struct {
	ID            string     `json:"id"`
	CompanyID     string     `json:"company_id"`
	RuleType      string     `json:"rule_type"`
	Pattern       string     `json:"pattern"`
	AccountCode   string     `json:"account_code"`
	Active        bool       `json:"active"`
	SourceLineID  *string    `json:"source_statement_transaction_id"`
	CreatedBy     *string    `json:"created_by"`
	CreatedAt     time.Time  `json:"created_at"`
	DeactivatedBy *string    `json:"deactivated_by"`
	DeactivatedAt *time.Time `json:"deactivated_at"`
}

func toRule(r store.ClassificationRule) Rule {
	return Rule{ID: r.ID, CompanyID: r.CompanyID, RuleType: r.RuleType, Pattern: r.Pattern, AccountCode: r.AccountCode, Active: r.Active,
		SourceLineID: r.SourceStatementTransactionID, CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt, DeactivatedBy: r.DeactivatedBy,
		DeactivatedAt: r.DeactivatedAt}
}

func companyNotFound() error { return apperr.NotFound("COMPANY_NOT_FOUND", "Empresa não encontrada.") }

func validation(fields map[string]any) error {
	return apperr.BadRequest("VALIDATION_ERROR", "Dados inválidos. Verifique os campos destacados.").WithDetails(map[string]any{"fields": fields})
}

// requireCompany confere a empresa do caminho (404).
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

// audit registra a ação humana no log de extração da empresa.
func (s *Service) audit(ctx context.Context, q *store.Queries, companyID string, change map[string]any) error {
	uid := user.FromContext(ctx)
	b, err := json.Marshal(change)
	if err != nil {
		return err
	}
	text := string(b)
	_, err = q.InsertExtractionLog(ctx, store.InsertExtractionLogParams{
		CompanyID: &companyID, Alias: "humano", Attempt: 0, ActorID: &uid, Change: &text, Now: s.clock.Now(),
	})
	return err
}

// cleanCode valida o código da conta: sem espaços nas pontas, até 32 caracteres, sem espaço nem
// ponto e vírgula no meio (o separador do CSV).
func cleanCode(code string) (string, string) {
	c := strings.TrimSpace(code)
	switch {
	case c == "":
		return "", "Informe o código da conta."
	case len([]rune(c)) > maxCode:
		return "", fmt.Sprintf("O código passa de %d caracteres.", maxCode)
	case strings.ContainsAny(c, ";") || strings.IndexFunc(c, pytext.IsSpace) >= 0:
		return "", "O código não pode ter espaço nem ponto e vírgula."
	}
	return c, ""
}

func cleanName(name string) (string, string) {
	n := strings.TrimSpace(name)
	switch {
	case n == "":
		return "", "Informe o nome da conta."
	case len([]rune(n)) > maxName:
		return "", fmt.Sprintf("O nome passa de %d caracteres.", maxName)
	}
	return n, ""
}

// Accounts lista o plano da empresa por código (onlyActive: só as ativas).
func (s *Service) Accounts(ctx context.Context, companyID string, onlyActive bool) ([]Account, error) {
	q := store.New(s.pool)
	if err := requireCompany(ctx, q, companyID); err != nil {
		return nil, err
	}
	rows, err := q.ListChartAccounts(ctx, store.ListChartAccountsParams{CompanyID: companyID, OnlyActive: onlyActive})
	if err != nil {
		return nil, err
	}
	out := make([]Account, 0, len(rows))
	for _, r := range rows {
		out = append(out, toAccount(r))
	}
	return out, nil
}

// AccountInput é a conta nova.
type AccountInput struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

func isUnique(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "23505"
}

// CreateAccount inclui uma conta no plano (409 com o código repetido).
func (s *Service) CreateAccount(ctx context.Context, companyID string, in AccountInput) (Account, error) {
	code, codeErr := cleanCode(in.Code)
	name, nameErr := cleanName(in.Name)
	fields := map[string]any{}
	if codeErr != "" {
		fields["code"] = codeErr
	}
	if nameErr != "" {
		fields["name"] = nameErr
	}
	var out Account
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		if err := requireCompany(ctx, q, companyID); err != nil {
			return err
		}
		if len(fields) > 0 {
			return validation(fields)
		}
		a, err := q.InsertChartAccount(ctx, store.InsertChartAccountParams{CompanyID: companyID, Code: code, Name: name, Now: s.clock.Now()})
		if isUnique(err) {
			return apperr.Conflict("ACCOUNT_CODE_EXISTS", "Já existe uma conta com esse código no plano da empresa.")
		}
		if err != nil {
			return err
		}
		out = toAccount(a)
		return s.audit(ctx, q, companyID, map[string]any{"acao": "incluir_conta", "conta": code})
	})
	return out, err
}

// AccountPatch altera o nome ou desativa (ou reativa) a conta; o código não muda.
type AccountPatch struct {
	Name   *string `json:"name"`
	Active *bool   `json:"active"`
}

// UpdateAccount altera a conta. Desativada, ela sai das novas classificações e das regras que
// apontam para ela; as classificações feitas ficam.
func (s *Service) UpdateAccount(ctx context.Context, companyID, accountID string, in AccountPatch) (Account, error) {
	var out Account
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		if err := requireCompany(ctx, q, companyID); err != nil {
			return err
		}
		if !httpx.IsUUID(accountID) {
			return accountNotFound()
		}
		cur, err := q.GetChartAccount(ctx, store.GetChartAccountParams{ID: accountID, CompanyID: companyID})
		if errors.Is(err, pgx.ErrNoRows) {
			return accountNotFound()
		}
		if err != nil {
			return err
		}
		name, active := cur.Name, cur.Active
		if in.Name != nil {
			n, msg := cleanName(*in.Name)
			if msg != "" {
				return validation(map[string]any{"name": msg})
			}
			name = n
		}
		if in.Active != nil {
			active = *in.Active
		}
		a, err := q.UpdateChartAccount(ctx, store.UpdateChartAccountParams{Name: name, Active: active, Now: s.clock.Now(), ID: accountID, CompanyID: companyID})
		if err != nil {
			return err
		}
		out = toAccount(a)
		return s.audit(ctx, q, companyID, map[string]any{"acao": "alterar_conta", "conta": a.Code, "ativa": active})
	})
	return out, err
}

func accountNotFound() error {
	return apperr.NotFound("ACCOUNT_NOT_FOUND", "Conta não encontrada no plano da empresa.")
}

// LineError é o erro de uma linha do CSV.
type LineError struct {
	Line    int    `json:"line"`
	Message string `json:"message"`
}

// ImportResult é a importação do plano: contas novas e contas atualizadas (nome, reativada).
type ImportResult struct {
	Inserted int `json:"inserted"`
	Updated  int `json:"updated"`
}

type csvAccount struct {
	line       int
	code, name string
}

// parseAccountsCSV lê o plano em CSV: codigo;nome por linha (colunas a mais são ignoradas), com o
// cabeçalho opcional (a 1ª linha que contém "codigo" ou "código") e linhas em branco puladas.
func parseAccountsCSV(data []byte) ([]csvAccount, []LineError) {
	var out []csvAccount
	var errs []LineError
	seen := map[string]int{}
	for i, line := range pytext.SplitLines(entries.DecodeCSV(data)) {
		n := i + 1
		if i == 0 {
			if l := strings.ToLower(line); strings.Contains(l, "codigo") || strings.Contains(l, "código") {
				continue
			}
		}
		if strings.TrimSpace(line) == "" {
			continue
		}
		if len(out)+len(errs) >= maxCSVLines {
			errs = append(errs, LineError{Line: n, Message: fmt.Sprintf("O arquivo passa de %d contas: divida em partes.", maxCSVLines)})
			break
		}
		parts := strings.Split(line, ";")
		if len(parts) < 2 {
			errs = append(errs, LineError{Line: n, Message: "Use codigo;nome, separados por ponto e vírgula."})
			continue
		}
		code, codeErr := cleanCode(parts[0])
		name, nameErr := cleanName(parts[1])
		msgs := []string{}
		for _, m := range []string{codeErr, nameErr} {
			if m != "" {
				msgs = append(msgs, m)
			}
		}
		if prev, dup := seen[code]; dup && code != "" {
			msgs = append(msgs, fmt.Sprintf("Código repetido (linha %d).", prev))
		}
		if len(msgs) > 0 {
			errs = append(errs, LineError{Line: n, Message: strings.Join(msgs, " ")})
			continue
		}
		seen[code] = n
		out = append(out, csvAccount{line: n, code: code, name: name})
	}
	return out, errs
}

// ImportAccounts importa o plano do CSV, tudo ou nada: com uma linha inválida, nada é gravado e a
// resposta traz o erro de cada linha (400 INVALID_ACCOUNTS_CSV). Conta que já existe ganha o nome
// do arquivo e volta a ativa.
func (s *Service) ImportAccounts(ctx context.Context, companyID, fileName string, data []byte) (ImportResult, error) {
	var out ImportResult
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		if err := requireCompany(ctx, q, companyID); err != nil {
			return err
		}
		accounts, errs := parseAccountsCSV(data)
		if len(errs) > 0 {
			if len(errs) > maxCSVErrors {
				errs = errs[:maxCSVErrors]
			}
			return apperr.BadRequest("INVALID_ACCOUNTS_CSV", "O CSV tem linhas inválidas; nada foi importado.").
				WithDetails(map[string]any{"lines": errs})
		}
		if len(accounts) == 0 {
			return apperr.BadRequest("INVALID_ACCOUNTS_CSV", "O CSV não tem contas.").WithDetails(map[string]any{"lines": []LineError{}})
		}
		now := s.clock.Now()
		for _, a := range accounts {
			inserted, err := q.UpsertChartAccount(ctx, store.UpsertChartAccountParams{CompanyID: companyID, Code: a.code, Name: a.name, Now: now})
			if err != nil {
				return fmt.Errorf("gravar a conta da linha %d: %w", a.line, err)
			}
			if inserted {
				out.Inserted++
			} else {
				out.Updated++
			}
		}
		return s.audit(ctx, q, companyID, map[string]any{"acao": "importar_plano", "arquivo": fileName, "novas": out.Inserted,
			"atualizadas": out.Updated})
	})
	return out, err
}

// CopyResult é a cópia do plano: as contas incluídas e as que já existiam.
type CopyResult struct {
	Inserted int `json:"inserted"`
	Skipped  int `json:"skipped"`
}

// CopyAccounts copia para a empresa as contas ativas de outra empresa (o padrão), sem mexer nas
// que ela já tem.
func (s *Service) CopyAccounts(ctx context.Context, companyID, sourceID string) (CopyResult, error) {
	var out CopyResult
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		if err := requireCompany(ctx, q, companyID); err != nil {
			return err
		}
		if sourceID == companyID {
			return validation(map[string]any{"source_company_id": "Escolha outra empresa como padrão."})
		}
		if !httpx.IsUUID(sourceID) {
			return validation(map[string]any{"source_company_id": "Empresa padrão inválida."})
		}
		if _, err := q.GetCompanyIdentity(ctx, sourceID); errors.Is(err, pgx.ErrNoRows) {
			return validation(map[string]any{"source_company_id": "Empresa padrão não encontrada."})
		} else if err != nil {
			return err
		}
		total, err := q.CountActiveChartAccounts(ctx, sourceID)
		if err != nil {
			return err
		}
		if total == 0 {
			return validation(map[string]any{"source_company_id": "A empresa padrão não tem contas ativas."})
		}
		ids, err := q.CopyChartAccounts(ctx, store.CopyChartAccountsParams{CompanyID: companyID, SourceCompanyID: sourceID, Now: s.clock.Now()})
		if err != nil {
			return err
		}
		out = CopyResult{Inserted: len(ids), Skipped: int(total) - len(ids)}
		return s.audit(ctx, q, companyID, map[string]any{"acao": "copiar_plano", "origem": sourceID, "novas": out.Inserted})
	})
	return out, err
}

// Rules lista as regras da empresa, da mais nova para a mais antiga (a ordem da classificação).
func (s *Service) Rules(ctx context.Context, companyID string, onlyActive bool) ([]Rule, error) {
	q := store.New(s.pool)
	if err := requireCompany(ctx, q, companyID); err != nil {
		return nil, err
	}
	rows, err := q.ListClassificationRules(ctx, store.ListClassificationRulesParams{CompanyID: companyID, OnlyActive: onlyActive})
	if err != nil {
		return nil, err
	}
	out := make([]Rule, 0, len(rows))
	for _, r := range rows {
		out = append(out, toRule(r))
	}
	return out, nil
}

// RuleInput é a regra confirmada pelo contador (a regra derivada da correção ou a sugerida pelo
// agente, M5.5), com a linha que a originou, se houver.
type RuleInput struct {
	RuleType     string  `json:"rule_type"`
	Pattern      string  `json:"pattern"`
	AccountCode  string  `json:"account_code"`
	SourceLineID *string `json:"source_statement_transaction_id"`
}

// RuleResult é a regra criada e as que ela substituiu (mesmo tipo e padrão).
type RuleResult struct {
	Rule     Rule     `json:"rule"`
	Replaced []string `json:"replaced_rule_ids"`
}

// CreateRule confirma uma regra. Ela vale para as próximas classificações (nunca troca o que já
// foi confirmado); a ativa com o mesmo tipo e padrão é desativada, porque a mais nova vale.
func (s *Service) CreateRule(ctx context.Context, companyID string, in RuleInput) (RuleResult, error) {
	var out RuleResult
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		if err := requireCompany(ctx, q, companyID); err != nil {
			return err
		}
		fields := map[string]any{}
		pattern, err := classification.NormalizePattern(in.RuleType, in.Pattern)
		switch {
		case !contains(classification.RuleTypes, in.RuleType):
			fields["rule_type"] = "Use cnpj, contraparte ou regex_descritivo."
		case err != nil:
			fields["pattern"] = strings.TrimPrefix(err.Error(), classification.ErrInvalidPattern.Error()+": ")
		case len([]rune(pattern)) > maxPattern:
			fields["pattern"] = fmt.Sprintf("O padrão passa de %d caracteres.", maxPattern)
		}
		code := strings.TrimSpace(in.AccountCode)
		active, err := q.AccountIsActive(ctx, store.AccountIsActiveParams{CompanyID: companyID, Code: code})
		if err != nil {
			return err
		}
		if !active {
			fields["account_code"] = "Conta fora do plano ativo da empresa."
		}
		if in.SourceLineID != nil {
			if !httpx.IsUUID(*in.SourceLineID) {
				fields["source_statement_transaction_id"] = "Linha do extrato inválida."
			} else if l, err := q.GetLineClassification(ctx, *in.SourceLineID); errors.Is(err, pgx.ErrNoRows) || (err == nil && l.CompanyID != companyID) {
				fields["source_statement_transaction_id"] = "Linha do extrato não encontrada nesta empresa."
			} else if err != nil {
				return err
			}
		}
		if len(fields) > 0 {
			return validation(fields)
		}
		// Uma regra de classificação por vez na empresa (a confirmação que corre junto não duplica).
		if err := q.LockReconciliationCompany(ctx, companyID); err != nil {
			return err
		}
		uid := user.FromContext(ctx)
		now := s.clock.Now()
		same, err := q.ListActiveRulesOfType(ctx, store.ListActiveRulesOfTypeParams{CompanyID: companyID, RuleType: in.RuleType})
		if err != nil {
			return err
		}
		out.Replaced = []string{}
		for _, r := range same {
			if !classification.SamePattern(in.RuleType, r.Pattern, pattern) {
				continue
			}
			if _, err := q.DeactivateClassificationRule(ctx, store.DeactivateClassificationRuleParams{
				DeactivatedBy: &uid, Now: &now, ID: r.ID, CompanyID: companyID,
			}); err != nil {
				return err
			}
			out.Replaced = append(out.Replaced, r.ID)
		}
		r, err := q.InsertClassificationRule(ctx, store.InsertClassificationRuleParams{
			CompanyID: companyID, RuleType: in.RuleType, Pattern: pattern, AccountCode: code, SourceStatementTransactionID: in.SourceLineID,
			CreatedBy: &uid, Now: now,
		})
		if err != nil {
			return err
		}
		out.Rule = toRule(r)
		return s.audit(ctx, q, companyID, map[string]any{"acao": "confirmar_regra", "regra": r.ID, "tipo": r.RuleType, "conta": code,
			"substituidas": out.Replaced})
	})
	return out, err
}

// DeactivateRule desativa a regra (nunca apaga).
func (s *Service) DeactivateRule(ctx context.Context, companyID, ruleID string) (Rule, error) {
	var out Rule
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		if err := requireCompany(ctx, q, companyID); err != nil {
			return err
		}
		if !httpx.IsUUID(ruleID) {
			return ruleNotFound()
		}
		if _, err := q.GetClassificationRule(ctx, store.GetClassificationRuleParams{ID: ruleID, CompanyID: companyID}); errors.Is(err, pgx.ErrNoRows) {
			return ruleNotFound()
		} else if err != nil {
			return err
		}
		uid := user.FromContext(ctx)
		now := s.clock.Now()
		r, err := q.DeactivateClassificationRule(ctx, store.DeactivateClassificationRuleParams{DeactivatedBy: &uid, Now: &now, ID: ruleID, CompanyID: companyID})
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.Conflict("RULE_INACTIVE", "A regra já está desativada.")
		}
		if err != nil {
			return err
		}
		out = toRule(r)
		return s.audit(ctx, q, companyID, map[string]any{"acao": "desativar_regra", "regra": ruleID})
	})
	return out, err
}

func ruleNotFound() error {
	return apperr.NotFound("RULE_NOT_FOUND", "Regra de classificação não encontrada nesta empresa.")
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
