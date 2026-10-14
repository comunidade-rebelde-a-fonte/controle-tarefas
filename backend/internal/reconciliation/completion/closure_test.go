package completion_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"controle/internal/store"
	"controle/internal/testutil"
	"controle/internal/user"
)

// TEST-023 [REQ-033, REQ-034, AC-025, AC-026]: o fechamento do Contábil, pela API do Controle, é
// a única trava da Conciliação; fecha exigindo a obrigação Conciliação final só quando a empresa
// usa a Conciliação na competência (decisão do usuário, D-037); trava toda mutação da Conciliação
// até a reabertura com motivo; não fecha os outros módulos.

// accountingPeriod devolve o período do Contábil da empresa em 10/2026 e a versão.
func (e *env) accountingPeriod(company string) (id string, version int) {
	e.t.Helper()
	if err := e.pool.QueryRow(ctx, `SELECT p.id, p.version FROM company_module_periods p JOIN competencies k ON k.id = p.competence_id
		WHERE p.company_id = $1 AND p.module_code = 'ACCOUNTING' AND k.competence_key = '2026-10'`, company).Scan(&id, &version); err != nil {
		e.t.Fatalf("período do Contábil: %v", err)
	}
	return id, version
}

func (e *env) closeAccounting(company string) testutil.Response {
	id, v := e.accountingPeriod(company)
	return e.srv.Post("/api/v1/company-module-periods/"+id+"/close", map[string]any{"version": v})
}

func (e *env) reopenAccounting(company, reason string) testutil.Response {
	id, v := e.accountingPeriod(company)
	return e.srv.Post("/api/v1/company-module-periods/"+id+"/reopen", map[string]any{"version": v, "reason": reason})
}

type closureOut struct {
	Period *struct {
		ID    string `json:"id"`
		State string `json:"state"`
	} `json:"period"`
	Events []struct {
		EventType string  `json:"event_type"`
		Reason    *string `json:"reason"`
	} `json:"events"`
	RequiresReconciliation bool `json:"requires_reconciliation"`
}

func (e *env) closure() closureOut {
	e.t.Helper()
	var out closureOut
	e.srv.Get("/api/v1/companies/"+e.company+"/reconciliation/closure?competence=2026-10").MustStatus(e.t, http.StatusOK).JSON(e.t, &out)
	return out
}

// Usando a Conciliação, o Contábil só fecha com a obrigação Conciliação final; sem Conciliação, ou
// com a obrigação não aplicável, fecha como sempre.
func TestTEST023_ClosingRequiresTheReconciliationWhenUsed(t *testing.T) {
	e := setup(t)
	if c := e.closure(); c.Period == nil || c.Period.State != "OPEN" || c.RequiresReconciliation {
		t.Fatalf("antes do extrato = %+v", c)
	}
	e.resolve(e.statement("2026-10", 5)[0], true)
	if c := e.closure(); !c.RequiresReconciliation {
		t.Fatalf("com o extrato = %+v", c)
	}
	res := e.closeAccounting(e.company)
	if res.Status != http.StatusUnprocessableEntity || res.ErrorCode(t) != "RECONCILIATION_NOT_FINAL" ||
		!strings.Contains(string(res.Body), "A Conciliação de 10/2026 não está concluída (a obrigação Conciliação está como A iniciar)") {
		t.Fatalf("fechar com a conciliação aberta = %d %s", res.Status, res.Body)
	}
	// Concluída a conciliação (a obrigação vai para OK), o Contábil fecha.
	e.prerequisites()
	e.complete("2026-10").MustStatus(t, http.StatusOK)
	e.closeAccounting(e.company).MustStatus(t, http.StatusOK)
	if c := e.closure(); c.Period.State != "CLOSED" || len(c.Events) < 2 || c.Events[0].EventType != "CLOSED" {
		t.Fatalf("depois de fechar = %+v", c)
	}

	// Outra empresa, sem Conciliação na competência: fecha como sempre (o comportamento do Controle).
	other := e.scalar(`SELECT id FROM companies WHERE internal_code = '102'`)
	e.closeAccounting(other).MustStatus(t, http.StatusOK)
	// Usando a Conciliação, mas com a obrigação não aplicável: também fecha.
	e.reopenAccounting(e.company, "teste da obrigação não aplicável").MustStatus(t, http.StatusOK)
	e.reopen("2026-10", "teste").MustStatus(t, http.StatusOK)
	id, _ := e.instance("ACCOUNTING_RECONCILIATION")
	e.exec(`UPDATE obligation_instances SET applicable = false WHERE id = $1`, id)
	if c := e.closure(); c.RequiresReconciliation {
		t.Fatalf("com a obrigação não aplicável = %+v", c)
	}
	e.closeAccounting(e.company).MustStatus(t, http.StatusOK)
}

// Com o Contábil fechado pela API, toda mutação da Conciliação na competência é recusada (423
// PERIOD_CLOSED) e a leitura segue; os outros módulos continuam abertos; reaberto com motivo,
// tudo volta, e o documento que chegou fechado continua pendente.
func TestTEST023_ClosedAccountingLocksTheReconciliation(t *testing.T) {
	e := setup(t)
	lines := e.statement("2026-10", 3, 6, 9)
	free, classified, unclassified := lines[0], lines[1], lines[2]
	rec := e.resolve(classified, true)
	e.resolve(unclassified, false)
	manual := e.scalar(`INSERT INTO client_entries (company_id, entry_date, amount, direction, counterparty, status)
		VALUES ($1, '2026-10-03', 10.00, 'D', 'Fornecedor', 'manual') RETURNING id`, e.company)
	doc := e.scalar(`INSERT INTO reconciliation_documents (company_id, bucket, object_key, sha256, file_name, content_type, kind, status)
		VALUES ($1, 'originais', 'originais/r', repeat('e', 64), 'comprovante.pdf', 'application/pdf', 'comprovante', 'importado') RETURNING id`, e.company)
	receipt := e.scalar(`INSERT INTO client_entries (company_id, document_id, entry_date, amount, direction, counterparty, status)
		VALUES ($1, $2, '2026-10-12', 20.00, 'D', 'Outro', 'extraido') RETURNING id`, e.company, doc)
	late := e.scalar(`INSERT INTO reconciliation_documents (company_id, competence, bucket, object_key, sha256, file_name, content_type, status, diagnosis)
		VALUES ($1, '2026-10-01', 'originais', 'originais/t', repeat('d', 64), 'tardio.pdf', 'application/pdf', 'pendencia_periodo_fechado',
		'{"motivo": "periodo_fechado"}') RETURNING id`, e.company)
	// O contador marca a obrigação Conciliação no Controle (a guarda olha a obrigação) e fecha.
	e.prerequisites()
	e.setStatus("ACCOUNTING_RECONCILIATION", "OK")
	e.closeAccounting(e.company).MustStatus(t, http.StatusOK)

	base := "/api/v1/reconciliation/review/"
	for name, res := range map[string]testutil.Response{
		"lançamento manual":      e.srv.Post("/api/v1/reconciliation/entries", map[string]any{"company_id": e.company, "date": "2026-10-20", "amount": "5,00", "direction": "D"}),
		"correção do campo":      e.srv.Patch("/api/v1/reconciliation/entries/"+receipt, map[string]any{"amount": "21,00"}),
		"rodada":                 e.srv.Post("/api/v1/companies/"+e.company+"/reconciliation/run?competence=2026-10", nil),
		"classificação":          e.srv.Post("/api/v1/companies/"+e.company+"/classification/run?competence=2026-10", nil),
		"corrigir a conta":       e.srv.Put(base+classified+"/classification", map[string]any{"account_code": "1.01"}),
		"conciliar à mão":        e.srv.Post(base+free+"/manual-match", map[string]any{"entry_ids": []string{manual}}),
		"sem comprovante":        e.srv.Post(base+free+"/confirm-no-receipt", nil),
		"desfazer":               e.srv.Post("/api/v1/reconciliation/reconciliations/"+rec+"/undo", map[string]string{"reason": "teste"}),
		"pedir hipótese":         e.srv.Post(base+free+"/proposal", nil),
		"pedir sugestão":         e.srv.Post(base+unclassified+"/classification-suggestion", nil),
		"concluir a conciliação": e.complete("2026-10"),
	} {
		if res.Status != http.StatusLocked || res.ErrorCode(t) != "PERIOD_CLOSED" {
			t.Errorf("%s com o Contábil fechado = %d %s", name, res.Status, res.Body)
		}
	}
	// A leitura segue.
	if st := e.summary("2026-10"); st.State == "" {
		t.Error("resumo sem a conclusão")
	}
	for _, path := range []string{
		"/api/v1/reconciliation/review-queue?company_id=" + e.company + "&competence=2026-10",
		"/api/v1/companies/" + e.company + "/ledger?from=2026-10&to=2026-10",
	} {
		e.srv.Get(path).MustStatus(t, http.StatusOK)
	}
	// Fechamentos independentes (AC-026): os outros módulos da empresa seguem abertos e editáveis.
	if n := e.scalar(`SELECT count(*) FROM company_module_periods p JOIN competencies k ON k.id = p.competence_id
		WHERE p.company_id = $1 AND k.competence_key = '2026-10' AND p.module_code <> 'ACCOUNTING' AND p.state <> 'OPEN'`, e.company); n != "0" {
		t.Errorf("outros módulos fechados = %s", n)
	}
	fiscal := e.scalar(`SELECT t.code FROM obligation_instances i JOIN obligation_types t ON t.id = i.obligation_type_id
		JOIN competencies k ON k.id = i.competence_id
		WHERE i.company_id = $1 AND k.competence_key = '2026-10' AND t.primary_module = 'FISCAL' AND i.applicable ORDER BY t.sort_order LIMIT 1`, e.company)
	e.setStatus(fiscal, "DOWNLOADED")

	// Reabrir pede motivo; reaberto, as mutações voltam, e o documento tardio segue pendente.
	if res := e.reopenAccounting(e.company, "  "); res.Status != http.StatusUnprocessableEntity || res.ErrorCode(t) != "REASON_REQUIRED" {
		t.Fatalf("reabrir sem motivo = %d %s", res.Status, res.Body)
	}
	e.reopenAccounting(e.company, "Faltou lançar a tarifa de outubro").MustStatus(t, http.StatusOK)
	c := e.closure()
	if c.Period.State != "OPEN" || c.Events[0].EventType != "REOPENED" || c.Events[0].Reason == nil ||
		*c.Events[0].Reason != "Faltou lançar a tarifa de outubro" {
		t.Fatalf("depois de reabrir = %+v", c)
	}
	e.srv.Post("/api/v1/reconciliation/entries", map[string]any{"company_id": e.company, "date": "2026-10-20", "amount": "5,00", "direction": "D"}).
		MustStatus(t, http.StatusCreated)
	e.srv.Post(base+free+"/confirm-no-receipt", nil).MustStatus(t, http.StatusCreated)
	if st := e.scalar(`SELECT status FROM reconciliation_documents WHERE id = $1`, late); st != "pendencia_periodo_fechado" {
		t.Errorf("documento tardio depois de reabrir = %s", st)
	}
	if n := e.scalar(`SELECT count(*) FROM company_module_period_events WHERE event_type = 'REOPENED' AND reason = 'Faltou lançar a tarifa de outubro'
		AND user_id = $1`, user.AdminID); n != "1" {
		t.Errorf("evento da reabertura = %s", n)
	}
}

// O fechamento do Contábil espera a conclusão ou a reabertura da conciliação em andamento (a trava
// exclusiva da empresa) antes de olhar a obrigação.
func TestTEST023_ClosingWaitsForTheReconciliation(t *testing.T) {
	e := setup(t)
	e.resolve(e.statement("2026-10", 5)[0], true)
	e.prerequisites()
	e.setStatus("ACCOUNTING_RECONCILIATION", "OK")
	tx, err := e.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := store.New(tx).LockReconciliationCompany(ctx, e.company); err != nil {
		t.Fatal(err)
	}
	done := make(chan int, 1)
	go func() { done <- e.closeAccounting(e.company).Status }()
	select {
	case code := <-done:
		t.Fatalf("o fechamento não esperou a conciliação em andamento: %d", code)
	case <-time.After(500 * time.Millisecond):
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if code := <-done; code != http.StatusOK {
		t.Fatalf("fechamento depois da espera = %d", code)
	}
}
