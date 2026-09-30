package competence

import (
	"reflect"
	"testing"
)

var (
	fiscalItemSet = []string{"NOT_STARTED", "DOWNLOADED", "POSTED", "CLOSED", "MISSING_DOCUMENTS", "NO_MOVEMENT", "NOT_APPLICABLE"}
	payrollSet    = []string{"NOT_STARTED", "IN_PROGRESS", "SENT_TO_CLIENT", "CLOSED", "ARCHIVED", "PENDING", "NO_MOVEMENT"}
	accountingSet = []string{"NOT_STARTED", "PENDING", "NO_MOVEMENT", "OK"}
)

var allRegimes = []string{"SIMPLES_NACIONAL", "LUCRO_PRESUMIDO", "LUCRO_REAL", "LUCRO_ARBITRADO", "IMUNE_ISENTA"}

func everyRegime(r RegimeRule) map[string]RegimeRule {
	out := map[string]RegimeRule{}
	for _, g := range allRegimes {
		out[g] = r
	}
	return out
}

var on = RegimeRule{DefaultApplicable: true, CountsInProgress: true, Active: true}

// testCatalog imita o catálogo do seed com os casos relevantes para a resolução.
func testCatalog() []CatalogType {
	return []CatalogType{
		{
			ID: "t-icms", Code: "ICMS", PrimaryModule: "FISCAL", Active: true,
			InitialStatusCode: "NOT_STARTED", StatusCodes: fiscalItemSet,
			RegimeRules: map[string]RegimeRule{
				"SIMPLES_NACIONAL": {DefaultApplicable: true, CountsInProgress: false, Active: true},
				"LUCRO_PRESUMIDO":  on,
				"LUCRO_REAL":       on,
			},
		},
		{
			ID: "t-ipi", Code: "IPI", PrimaryModule: "FISCAL", Active: true,
			InitialStatusCode: "NOT_STARTED", StatusCodes: fiscalItemSet,
			RegimeRules: map[string]RegimeRule{"LUCRO_PRESUMIDO": on, "LUCRO_REAL": on},
		},
		{
			ID: "t-pis", Code: "PIS_COFINS", PrimaryModule: "FISCAL", Active: true,
			InitialStatusCode: "NOT_STARTED", StatusCodes: fiscalItemSet,
			// Regra desativada no catálogo: como se não existisse.
			RegimeRules: map[string]RegimeRule{"LUCRO_PRESUMIDO": {DefaultApplicable: true, CountsInProgress: true, Active: false}},
		},
		{
			ID: "t-old", Code: "OLD_TYPE", PrimaryModule: "FISCAL", Active: false,
			InitialStatusCode: "NOT_STARTED", StatusCodes: fiscalItemSet,
			RegimeRules: everyRegime(on),
		},
		{
			ID: "t-folha", Code: "PAYROLL_PROCESSING", PrimaryModule: "PAYROLL", Active: true,
			InitialStatusCode: "NOT_STARTED", StatusCodes: payrollSet, StatusWhenNoPayroll: NoPayrollNoMovement,
			RegimeRules: everyRegime(on),
		},
		{
			ID: "t-fgts", Code: "FGTS", PrimaryModule: "PAYROLL", Active: true,
			InitialStatusCode: "NOT_STARTED", StatusCodes: payrollSet, StatusWhenNoPayroll: NoPayrollNotApplicable,
			RegimeRules: everyRegime(on),
		},
		{
			ID: "t-mov", Code: "MOVEMENT_RECEIVED", PrimaryModule: "ACCOUNTING", Active: true,
			InitialStatusCode: "NOT_STARTED", StatusCodes: accountingSet,
			RegimeRules: everyRegime(on),
		},
	}
}

func byCode(ps []PlannedInstance) map[string]PlannedInstance {
	out := map[string]PlannedInstance{}
	for _, p := range ps {
		out[p.Code] = p
	}
	return out
}

func codes(ps []PlannedInstance) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.Code
	}
	return out
}

func snapshot(regime, payroll string, modules ...string) Snapshot {
	return Snapshot{TaxRegime: regime, PayrollType: payroll, ActiveModules: modules}
}

// RN-010, D2: no Simples, o ICMS é gerado e visível, mas fora do progresso.
func TestRN010_ICMSSimplesOutOfProgress(t *testing.T) {
	got := byCode(Resolve(snapshot("SIMPLES_NACIONAL", "PAYROLL", "FISCAL"), testCatalog(), nil))
	icms, ok := got["ICMS"]
	if !ok {
		t.Fatal("ICMS deveria ser gerado para o Simples")
	}
	want := PlannedInstance{ObligationTypeID: "t-icms", Code: "ICMS", Applicable: true, CountsInProgress: false, StatusCode: "NOT_STARTED", RegimeSnapshot: "SIMPLES_NACIONAL"}
	if icms != want {
		t.Fatalf("got %+v, want %+v", icms, want)
	}

	normal := byCode(Resolve(snapshot("LUCRO_PRESUMIDO", "PAYROLL", "FISCAL"), testCatalog(), nil))
	if !normal["ICMS"].CountsInProgress {
		t.Fatal("no Presumido o ICMS conta no progresso")
	}
}

// D3: tipo sem regra para o regime não é gerado.
func TestResolve_TypeWithoutRegimeRuleIsNotGenerated(t *testing.T) {
	got := byCode(Resolve(snapshot("SIMPLES_NACIONAL", "PAYROLL", "FISCAL"), testCatalog(), nil))
	if _, ok := got["IPI"]; ok {
		t.Fatal("IPI não tem regra para o Simples e não deveria ser gerado")
	}
	// Override APPLICABLE não cria tipo incompatível com o regime (D6).
	got = byCode(Resolve(snapshot("SIMPLES_NACIONAL", "PAYROLL", "FISCAL"), testCatalog(), map[string]string{"t-ipi": "APPLICABLE"}))
	if _, ok := got["IPI"]; ok {
		t.Fatal("override não deveria gerar tipo sem regra para o regime")
	}
}

// D6, RN-015: empresa com IPI dispensado gera a instância não aplicável, visível.
func TestResolve_IPIExemptIsGeneratedNotApplicable(t *testing.T) {
	got := byCode(Resolve(snapshot("LUCRO_PRESUMIDO", "PAYROLL", "FISCAL"), testCatalog(), map[string]string{"t-ipi": "NOT_APPLICABLE"}))
	ipi, ok := got["IPI"]
	if !ok {
		t.Fatal("IPI dispensado deveria ser gerado (visível) com applicable=false")
	}
	if ipi.Applicable {
		t.Fatal("IPI dispensado: applicable deveria ser false")
	}
	if ipi.StatusCode != "NOT_APPLICABLE" {
		t.Fatalf("status = %s, want NOT_APPLICABLE (o conjunto FISCAL_ITEM tem esse código)", ipi.StatusCode)
	}
	if !ipi.CountsInProgress {
		t.Fatal("counts_in_progress vem da regra tipo × regime; applicable=false já tira do denominador")
	}
	if !got["ICMS"].Applicable {
		t.Fatal("override do IPI não deveria afetar o ICMS")
	}
}

func TestResolve_OverridePrecedence(t *testing.T) {
	cat := testCatalog()
	cat[0].RegimeRules["LUCRO_REAL"] = RegimeRule{DefaultApplicable: false, CountsInProgress: true, Active: true}
	snap := snapshot("LUCRO_REAL", "PAYROLL", "FISCAL")

	if byCode(Resolve(snap, cat, nil))["ICMS"].Applicable {
		t.Fatal("sem override vale a regra padrão (não aplicável)")
	}
	if byCode(Resolve(snap, cat, map[string]string{"t-icms": "DEFAULT"}))["ICMS"].Applicable {
		t.Fatal("override DEFAULT mantém a regra padrão")
	}
	icms := byCode(Resolve(snap, cat, map[string]string{"t-icms": "APPLICABLE"}))["ICMS"]
	if !icms.Applicable || icms.StatusCode != "NOT_STARTED" {
		t.Fatalf("override APPLICABLE vence a regra padrão: got %+v", icms)
	}
}

// Módulo desativado não gera os tipos do módulo [D3].
func TestResolve_DisabledModuleGeneratesNothing(t *testing.T) {
	got := Resolve(snapshot("LUCRO_PRESUMIDO", "PAYROLL", "PAYROLL", "ACCOUNTING"), testCatalog(), nil)
	for _, p := range got {
		if p.Code == "ICMS" || p.Code == "IPI" {
			t.Fatalf("Fiscal desativado não deveria gerar %s", p.Code)
		}
	}
	if want := []string{"PAYROLL_PROCESSING", "FGTS", "MOVEMENT_RECEIVED"}; !reflect.DeepEqual(codes(got), want) {
		t.Fatalf("got %v, want %v", codes(got), want)
	}
	if got := Resolve(snapshot("LUCRO_PRESUMIDO", "PAYROLL"), testCatalog(), nil); len(got) != 0 {
		t.Fatalf("sem módulos ativos, nada deveria ser gerado: %v", codes(got))
	}
}

func TestResolve_InactiveTypeOrRuleIsSkipped(t *testing.T) {
	got := byCode(Resolve(snapshot("LUCRO_PRESUMIDO", "PAYROLL", "FISCAL"), testCatalog(), nil))
	if _, ok := got["OLD_TYPE"]; ok {
		t.Fatal("tipo inativo no catálogo não deveria ser gerado")
	}
	if _, ok := got["PIS_COFINS"]; ok {
		t.Fatal("regra tipo × regime inativa não deveria gerar o tipo")
	}
}

// D7, Q5: empresa Sem folha aplica o status configurado no catálogo.
func TestResolve_NoPayrollAppliesConfiguredStatus(t *testing.T) {
	got := byCode(Resolve(snapshot("LUCRO_PRESUMIDO", "NO_PAYROLL", "PAYROLL"), testCatalog(), nil))

	folha := got["PAYROLL_PROCESSING"]
	if !folha.Applicable || folha.StatusCode != "NO_MOVEMENT" {
		t.Fatalf("Folha em empresa Sem folha: got %+v, want aplicável com NO_MOVEMENT", folha)
	}
	fgts := got["FGTS"]
	if fgts.Applicable {
		t.Fatal("FGTS em empresa Sem folha deveria ser não aplicável")
	}
	// O conjunto PAYROLL não tem NOT_APPLICABLE: fica o status inicial [migration 0009].
	if fgts.StatusCode != "NOT_STARTED" {
		t.Fatalf("FGTS: status = %s, want NOT_STARTED", fgts.StatusCode)
	}

	// Com folha, os mesmos tipos seguem o status inicial e são aplicáveis.
	withPayroll := byCode(Resolve(snapshot("LUCRO_PRESUMIDO", "PRO_LABORE", "PAYROLL"), testCatalog(), nil))
	for _, c := range []string{"PAYROLL_PROCESSING", "FGTS"} {
		if p := withPayroll[c]; !p.Applicable || p.StatusCode != "NOT_STARTED" {
			t.Fatalf("%s com pró-labore: got %+v", c, p)
		}
	}
}

func TestResolve_NoPayrollAndOverrides(t *testing.T) {
	snap := snapshot("LUCRO_PRESUMIDO", "NO_PAYROLL", "PAYROLL")

	// Override explícito APPLICABLE vence o "não se aplica" do Sem folha.
	fgts := byCode(Resolve(snap, testCatalog(), map[string]string{"t-fgts": "APPLICABLE"}))["FGTS"]
	if !fgts.Applicable || fgts.StatusCode != "NOT_STARTED" {
		t.Fatalf("FGTS com override APPLICABLE: got %+v", fgts)
	}
	// Override NOT_APPLICABLE vence o "sem movimento".
	folha := byCode(Resolve(snap, testCatalog(), map[string]string{"t-folha": "NOT_APPLICABLE"}))["PAYROLL_PROCESSING"]
	if folha.Applicable || folha.StatusCode != "NOT_STARTED" {
		t.Fatalf("Folha com override NOT_APPLICABLE: got %+v", folha)
	}
}

// Sem folha com NO_MOVEMENT num conjunto que não tem esse código: mantém o status inicial.
func TestResolve_NoMovementMissingFromSetFallsBack(t *testing.T) {
	cat := []CatalogType{{
		ID: "t-x", Code: "X", PrimaryModule: "PAYROLL", Active: true,
		InitialStatusCode: "NOT_STARTED", StatusCodes: []string{"NOT_STARTED", "CLOSED"},
		StatusWhenNoPayroll: NoPayrollNoMovement, RegimeRules: everyRegime(on),
	}}
	got := Resolve(snapshot("LUCRO_REAL", "NO_PAYROLL", "PAYROLL"), cat, nil)
	if len(got) != 1 || got[0].StatusCode != "NOT_STARTED" || !got[0].Applicable {
		t.Fatalf("got %+v", got)
	}
}

// A ordem do catálogo é preservada e o regime do snapshot vai em cada instância.
func TestResolve_OrderAndRegimeSnapshot(t *testing.T) {
	got := Resolve(snapshot("LUCRO_REAL", "PAYROLL", "FISCAL", "PAYROLL", "ACCOUNTING", "DECLARATIONS"), testCatalog(), nil)
	if want := []string{"ICMS", "IPI", "PAYROLL_PROCESSING", "FGTS", "MOVEMENT_RECEIVED"}; !reflect.DeepEqual(codes(got), want) {
		t.Fatalf("got %v, want %v", codes(got), want)
	}
	for _, p := range got {
		if p.RegimeSnapshot != "LUCRO_REAL" {
			t.Fatalf("%s: regime_snapshot = %q", p.Code, p.RegimeSnapshot)
		}
	}
}
