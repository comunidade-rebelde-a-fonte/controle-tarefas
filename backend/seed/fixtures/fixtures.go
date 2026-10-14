// Package fixtures é a preparação dos fixtures da demonstração (TASK-027, D-041; F-012, AC-032).
//
// Depois do make seed-demo, com os serviços no ar, carrega na Construtora Horizonte (111) o cenário
// de aula da Conciliação de agosto de 2026, como o scripts/carregar_horizonte.py da origem: as
// contas bancárias, os lançamentos de contas a pagar e a receber, os 2 extratos e os 13
// comprovantes (guardados no MinIO e lidos pelo worker com os leitores determinísticos, sem o
// modelo) e a rodada da conciliação. O porte acrescenta um plano de contas fictício, tirado do
// gabarito do cenário, para a classificação. Também grava extratos do PGDAS-D numa pasta, para o
// envio pelo inbox do Fator R (pgdas.go).
//
// Tudo passa pelos serviços dos módulos (os mesmos da API), com o administrador como autor, e
// rodar de novo não duplica nada.
package fixtures

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/csv"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"controle/internal/competence"
	"controle/internal/jobs"
	"controle/internal/platform/apperr"
	"controle/internal/platform/clock"
	comp "controle/internal/platform/competence"
	"controle/internal/reconciliation"
	"controle/internal/reconciliation/bankaccount"
	"controle/internal/reconciliation/chart"
	"controle/internal/reconciliation/documents"
	"controle/internal/reconciliation/entries"
	"controle/internal/reconciliation/matching"
	"controle/internal/reconciliation/reconcile"
	"controle/internal/reconciliation/statement"
	"controle/internal/user"
	"controle/seed"
)

// Os extratos do cenário (extratos/), na ordem do carregar_horizonte.py: o Banco A Fonte e o Banco
// Beta. Os comprovantes são todos os PDFs de comprovantes/, em ordem de nome.
var statementFiles = []string{"extrato_construtora_horizonte_2026-08.pdf", "extrato_banco_beta_2026-08.pdf"}

// bankAccounts são as contas do cenário (CONTAS da origem), com o banco, a agência e a conta que a
// leitura dos extratos grava: a conta cadastrada decide a empresa do extrato (D-027).
var bankAccounts = []bankaccount.Input{
	{Bank: "a_fonte", Branch: "1234", Account: "56789-0"},
	{Bank: "beta", Branch: "0021", Account: "33412-7"},
}

// chartCSV é o plano de contas fictício da 111, com as contas do gabarito do cenário (bancos,
// clientes, fornecedores, adiantamentos, ISS retido, tarifas, juros e multas) e as dos títulos a
// pagar (folha, impostos, despesas, financiamento e lucros). A origem não tem plano no cenário.
//
//go:embed plano_de_contas_horizonte.csv
var chartCSV []byte

// chartFileName é o nome do arquivo no registro da importação do plano.
const chartFileName = "plano_de_contas_horizonte.csv"

// Deps são as dependências da preparação.
type Deps struct {
	Pool *pgxpool.Pool
	// Clock é o relógio de negócio (o do seed: APP_FIXED_TIME, quando houver), que decide a
	// competência corrente dos extratos do PGDAS-D e a geração da competência do cenário.
	Clock clock.Clock
	// Ops é o tempo operacional (o da fila e dos registros da Conciliação, como na API).
	Ops clock.Clock
	// Originals é o MinIO dos originais da Conciliação.
	Originals documents.Storage
	Queue     *jobs.Queue
	// Matching são as opções da rodada (reconcile.OptionsFrom do FUZZY_THRESHOLD).
	Matching matching.Options
	Logger   *slog.Logger
}

// Options são as pastas e a espera do processamento.
type Options struct {
	// ScenarioDir é a pasta do cenário (extratos/, comprovantes/ e contabil/).
	ScenarioDir string
	// PgdasTemplates é a pasta das fixtures do parser do PGDAS-D (a txt_padrao é o modelo).
	PgdasTemplates string
	// OutDir recebe os extratos do PGDAS-D para o envio pela tela.
	OutDir string
	// Process roda enquanto a preparação espera os documentos. Na linha de comando é nil: quem
	// processa é o worker do ambiente (a preparação exige o worker no ar).
	Process func(ctx context.Context) error
	// Timeout é o limite da espera (padrão: 5 minutos); Poll, o intervalo entre as conferências
	// (padrão: meio segundo).
	Timeout, Poll time.Duration
}

// Report é o que a preparação fez.
type Report struct {
	CompanyID string
	// Locked é o motivo da trava da Conciliação na competência do cenário (Contábil fechado ou
	// conciliação concluída): com ele, o cenário fica como está.
	Locked string
	// Contas bancárias, contas do plano e lançamentos criados nesta execução.
	BankAccounts, ChartAccounts, Entries int
	// Uploaded são os arquivos enviados nesta execução; AlreadyUploaded, os que já estavam lá.
	Uploaded, AlreadyUploaded int
	// Documents conta os documentos do cenário da 111 por estado.
	Documents map[string]int
	// Summary é a rodada da conciliação da competência.
	Summary *reconcile.Summary
	// Pgdas são os extratos do PGDAS-D gravados na pasta.
	Pgdas []PgdasFile
}

// Load prepara os fixtures da demonstração.
func Load(ctx context.Context, d Deps, o Options, out io.Writer) (Report, error) {
	ctx = user.WithUser(ctx, seed.AdminID)
	rep := Report{Documents: map[string]int{}}
	c := comp.MustParse(seed.HorizonteCompetence)
	var companyID string
	if err := d.Pool.QueryRow(ctx, `SELECT id FROM companies WHERE internal_code = $1`, seed.HorizonteCode).Scan(&companyID); err != nil {
		return rep, fmt.Errorf("a empresa %s (Construtora Horizonte) não está no banco: rode antes o make seed-demo (%w)", seed.HorizonteCode, err)
	}
	rep.CompanyID = companyID
	if err := ensureCompetence(ctx, d, c, out); err != nil {
		return rep, err
	}
	reason, err := reconciliation.LockReason(ctx, d.Pool, companyID, c)
	if err != nil {
		return rep, err
	}
	if reason != "" {
		rep.Locked = reason
		_, _ = fmt.Fprintf(out, "fixtures: a Conciliação de %s da %s está travada (%s): o cenário fica como está\n", c.Display(),
			seed.HorizonteCode, reason)
	} else if err := loadReconciliation(ctx, d, o, companyID, c, &rep, out); err != nil {
		return rep, err
	}
	files, err := writePgdas(ctx, d, o, out)
	if err != nil {
		return rep, fmt.Errorf("extratos do PGDAS-D: %w", err)
	}
	rep.Pgdas = files
	return rep, nil
}

// ensureCompetence gera a competência do cenário se o histórico do seed não chegou a ela (seed
// rodado com o relógio depois de 11/2026): a Conciliação e o fechamento do Contábil precisam dela.
func ensureCompetence(ctx context.Context, d Deps, c comp.Competence, out io.Writer) error {
	var exists bool
	if err := d.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM competencies WHERE competence_key = $1)`, c.String()).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return nil
	}
	if _, err := competence.NewEngine(d.Pool, d.Clock).Ensure(ctx, c); err != nil {
		return fmt.Errorf("gerar a competência %s do cenário: %w", c.Display(), err)
	}
	_, _ = fmt.Fprintf(out, "fixtures: competência %s gerada (fora da janela do histórico de demonstração)\n", c.Display())
	return nil
}

// loadReconciliation é o carregar_horizonte.py: contas, plano, lançamentos, envio dos documentos,
// a espera do worker e a rodada.
func loadReconciliation(ctx context.Context, d Deps, o Options, companyID string, c comp.Competence, rep *Report, out io.Writer) error {
	accounts := bankaccount.NewService(d.Pool, d.Ops)
	for _, in := range bankAccounts {
		_, err := accounts.Create(ctx, companyID, in)
		var ae *apperr.Error
		switch {
		case err == nil:
			rep.BankAccounts++
		case errors.As(err, &ae) && ae.Code == "BANK_ACCOUNT_EXISTS":
			if owner, _ := ae.Details["company_id"].(string); owner != "" && owner != companyID {
				_, _ = fmt.Fprintf(out, "fixtures: aviso: conta %s %s/%s: %s\n", in.Bank, in.Branch, in.Account, ae.Message)
			}
		default:
			return fmt.Errorf("conta bancária %s %s/%s: %w", in.Bank, in.Branch, in.Account, err)
		}
	}

	plan := chart.NewService(d.Pool, d.Ops)
	existing, err := plan.Accounts(ctx, companyID, false)
	if err != nil {
		return err
	}
	if len(existing) == 0 {
		res, err := plan.ImportAccounts(ctx, companyID, chartFileName, chartCSV)
		if err != nil {
			return fmt.Errorf("plano de contas: %w", err)
		}
		rep.ChartAccounts = res.Inserted
	}

	// Lançamentos só na primeira vez, como a origem (que olhava se o cliente já tinha algum).
	var manual int
	if err := d.Pool.QueryRow(ctx, `SELECT count(*) FROM client_entries WHERE company_id = $1 AND document_id IS NULL`,
		companyID).Scan(&manual); err != nil {
		return err
	}
	scenario, err := os.OpenRoot(o.ScenarioDir)
	if err != nil {
		return fmt.Errorf("pasta do cenário da Conciliação: %w", err)
	}
	defer func() { _ = scenario.Close() }()
	if manual == 0 {
		inputs, err := scenarioEntries(scenario, companyID)
		if err != nil {
			return err
		}
		svc := entries.NewService(d.Pool, d.Ops)
		for _, in := range inputs {
			if _, err := svc.Create(ctx, in); err != nil {
				return fmt.Errorf("lançamento %s de %s: %w", deref(in.DocumentRef), deref(in.Counterparty), err)
			}
			rep.Entries++
		}
	}

	files, err := scenarioFiles(scenario)
	if err != nil {
		return err
	}
	docs := documents.NewService(d.Pool, d.Ops, d.Queue, d.Originals, d.Logger)
	shas := make([]string, 0, len(files))
	for _, name := range files {
		data, err := scenario.ReadFile(name)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		shas = append(shas, hex.EncodeToString(sum[:]))
		_, err = docs.Upload(ctx, documents.UploadInput{FileName: baseName(name), Data: data, CompanyID: companyID, Competence: c.String()})
		var ae *apperr.Error
		switch {
		case err == nil:
			rep.Uploaded++
		case errors.As(err, &ae) && ae.Code == "DUPLICATE_DOCUMENT":
			rep.AlreadyUploaded++
		default:
			return fmt.Errorf("envio de %s: %w", name, err)
		}
	}
	if err := warnOtherCompanies(ctx, d.Pool, companyID, shas, out); err != nil {
		return err
	}
	if rep.Documents, err = waitProcessed(ctx, d.Pool, o, companyID, shas, out); err != nil {
		return err
	}
	sum, err := reconcile.NewService(d.Pool, d.Ops, d.Matching, d.Logger).Run(ctx, companyID, c)
	if err != nil {
		return fmt.Errorf("rodada da conciliação de %s: %w", c.Display(), err)
	}
	rep.Summary = &sum
	return nil
}

// scenarioFiles são os extratos e, em seguida, os comprovantes, relativos à pasta do cenário.
func scenarioFiles(root *os.Root) ([]string, error) {
	out := make([]string, 0, 16)
	for _, name := range statementFiles {
		out = append(out, "extratos/"+name)
	}
	items, err := fs.ReadDir(root.FS(), "comprovantes")
	if err != nil {
		return nil, fmt.Errorf("comprovantes do cenário: %w", err)
	}
	var receipts []string
	for _, e := range items {
		if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".pdf") {
			receipts = append(receipts, "comprovantes/"+e.Name())
		}
	}
	sort.Strings(receipts)
	return append(out, receipts...), nil
}

func baseName(path string) string {
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[i+1:]
	}
	return path
}

// paidOn acha a data do pagamento (re.search da origem: "06/08 e 09/08/2026" dá 09/08/2026).
var paidOn = regexp.MustCompile(`([0-9]{2}/[0-9]{2}/[0-9]{4})`)

// scenarioEntries lê os lançamentos do cliente (_lancamentos_pagar e _lancamentos_receber da
// origem): contas a pagar com a data e o valor pagos (saída) e contas a receber com o vencimento e
// o valor (entrada), com a contraparte, o documento e, nas notas, o CPF ou CNPJ do cliente.
func scenarioEntries(root *os.Root, companyID string) ([]entries.CreateInput, error) {
	var out []entries.CreateInput
	pay, err := readCSV(root, "contabil/contas_a_pagar.csv")
	if err != nil {
		return nil, err
	}
	for _, row := range pay {
		m := paidOn.FindString(row["data_pagamento"])
		if m == "" {
			continue
		}
		in, err := entryInput(companyID, m, row["valor_pago"], "D", row["fornecedor"], row["documento"], "")
		if err != nil {
			return nil, fmt.Errorf("contas a pagar, %s: %w", row["titulo"], err)
		}
		out = append(out, in)
	}
	receive, err := readCSV(root, "contabil/contas_a_receber.csv")
	if err != nil {
		return nil, err
	}
	for _, row := range receive {
		doc := onlyDigits(row["cpf_cnpj"])
		if len(doc) > 14 {
			doc = doc[:14]
		}
		in, err := entryInput(companyID, row["vencimento"], row["valor"], "C", row["cliente"], row["nf"], doc)
		if err != nil {
			return nil, fmt.Errorf("contas a receber, NF %s: %w", row["nf"], err)
		}
		out = append(out, in)
	}
	return out, nil
}

func entryInput(companyID, dateBR, amountBR, direction, counterparty, document, counterpartyDoc string) (entries.CreateInput, error) {
	date, err := statement.ParseDateBR(dateBR)
	if err != nil {
		return entries.CreateInput{}, err
	}
	amount, err := statement.ParseBRL(amountBR)
	if err != nil {
		return entries.CreateInput{}, err
	}
	in := entries.CreateInput{CompanyID: companyID, Date: date.Format(time.DateOnly), Amount: amount.Text(), Direction: direction,
		Counterparty: &counterparty, DocumentRef: &document}
	if counterpartyDoc != "" {
		in.CounterpartyDoc = &counterpartyDoc
	}
	return in, nil
}

// readCSV lê um CSV com cabeçalho e ponto e vírgula (csv.DictReader da origem).
func readCSV(root *os.Root, name string) ([]map[string]string, error) {
	f, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	r := csv.NewReader(f)
	r.Comma = ';'
	rows, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	if len(rows) < 2 {
		return nil, fmt.Errorf("%s: sem linhas", name)
	}
	out := make([]map[string]string, 0, len(rows)-1)
	for _, row := range rows[1:] {
		m := make(map[string]string, len(row))
		for i, h := range rows[0] {
			if i < len(row) {
				m[h] = row[i]
			}
		}
		out = append(out, m)
	}
	return out, nil
}

func onlyDigits(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, s)
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// warnOtherCompanies avisa quando um arquivo do cenário já está na Conciliação de outra empresa (o
// mesmo arquivo é recusado em qualquer empresa): o cenário da 111 fica sem ele.
func warnOtherCompanies(ctx context.Context, pool *pgxpool.Pool, companyID string, shas []string, out io.Writer) error {
	rows, err := pool.Query(ctx, `
		SELECT d.file_name, COALESCE(c.internal_code, '(sem empresa)') FROM reconciliation_documents d
		LEFT JOIN companies c ON c.id = d.company_id
		WHERE d.sha256 = ANY($2) AND d.company_id IS DISTINCT FROM $1::uuid
		ORDER BY d.file_name`, companyID, shas)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var name, code string
		if err := rows.Scan(&name, &code); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(out, "fixtures: aviso: %s já foi enviado à Conciliação da empresa %s e não entra no cenário da %s\n",
			name, code, seed.HorizonteCode)
	}
	return rows.Err()
}

// documentStates conta os documentos do cenário da empresa por estado.
func documentStates(ctx context.Context, pool *pgxpool.Pool, companyID string, shas []string) (map[string]int, error) {
	rows, err := pool.Query(ctx, `SELECT status, count(*)::int FROM reconciliation_documents
		WHERE company_id = $1 AND sha256 = ANY($2) GROUP BY status`, companyID, shas)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var status string
		var n int
		if err := rows.Scan(&status, &n); err != nil {
			return nil, err
		}
		out[status] = n
	}
	return out, rows.Err()
}

// waitProcessed espera os documentos do cenário saírem de pendente e triado: o worker tria, lê
// e grava cada um (os leitores determinísticos, sem o modelo).
func waitProcessed(ctx context.Context, pool *pgxpool.Pool, o Options, companyID string, shas []string, out io.Writer) (map[string]int, error) {
	timeout, poll := o.Timeout, o.Poll
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	if poll <= 0 {
		poll = 500 * time.Millisecond
	}
	wait, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	announced := false
	for {
		if o.Process != nil {
			if err := o.Process(wait); err != nil {
				return nil, fmt.Errorf("processar os documentos: %w", err)
			}
		}
		states, err := documentStates(ctx, pool, companyID, shas)
		if err != nil {
			return nil, err
		}
		waiting := states[reconciliation.DocPending] + states[reconciliation.DocTriaged]
		if waiting == 0 {
			return states, nil
		}
		if !announced {
			_, _ = fmt.Fprintf(out, "fixtures: aguardando o worker processar %d documentos da Conciliação...\n", waiting)
			announced = true
		}
		select {
		case <-wait.Done():
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, fmt.Errorf("o worker não processou %d documentos da Conciliação em %s: confira se ele está no ar (make health)", waiting, timeout)
		case <-time.After(poll):
		}
	}
}

// Write imprime o resumo da preparação (outDir é a pasta dos extratos do PGDAS-D, como foi
// informada).
func (r Report) Write(w io.Writer, outDir string) {
	if r.Locked == "" {
		_, _ = fmt.Fprintf(w, "fixtures: Conciliação %s da %s Construtora Horizonte: %d contas bancárias, %d contas no plano e %d lançamentos novos; "+
			"%d documentos enviados (%d já estavam)\n", comp.MustParse(seed.HorizonteCompetence).Display(), seed.HorizonteCode,
			r.BankAccounts, r.ChartAccounts, r.Entries, r.Uploaded, r.AlreadyUploaded)
		states := make([]string, 0, len(r.Documents))
		for status, n := range r.Documents {
			states = append(states, fmt.Sprintf("%s %d", status, n))
		}
		sort.Strings(states)
		_, _ = fmt.Fprintf(w, "fixtures: documentos do cenário: %s\n", strings.Join(states, ", "))
	}
	if s := r.Summary; s != nil {
		_, _ = fmt.Fprintf(w, "fixtures: rodada: %d linhas conciliadas (%d nesta rodada), %d com divergência ou caso de borda, %d sem comprovante, "+
			"%d comprovantes sem extrato, %d em revisão, %d a classificar\n", s.Reconciled, s.NewMatches, s.Divergent, s.WithoutReceipt,
			s.WithoutStatement, s.AwaitingReview, s.Classification.Pending)
	}
	names := make([]string, 0, len(r.Pgdas))
	for _, f := range r.Pgdas {
		names = append(names, f.Name)
	}
	_, _ = fmt.Fprintf(w, "fixtures: extratos do PGDAS-D em %s: %s (veja o %s)\n", outDir, strings.Join(names, ", "), readmeName)
}
