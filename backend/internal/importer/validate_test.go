package importer

import (
	"errors"
	"strings"
	"testing"

	"controle/internal/platform/apperr"
	"controle/internal/platform/cnpj"
)

func errMessage(err error) string {
	var ae *apperr.Error
	if errors.As(err, &ae) {
		return ae.Message
	}
	return err.Error()
}

type fakeLookup struct {
	byCnpj map[string]ExistingCompany
	byCode map[string]ExistingCompany
	users  map[string]string
}

func (f fakeLookup) CompanyByCnpj(c string) (ExistingCompany, bool) {
	e, ok := f.byCnpj[c]
	return e, ok
}
func (f fakeLookup) CompanyByCode(c string) (ExistingCompany, bool) {
	e, ok := f.byCode[strings.ToLower(c)]
	return e, ok
}
func (f fakeLookup) UserID(s string) (string, bool) {
	id, ok := f.users[strings.ToLower(s)]
	return id, ok
}

// CNPJs com dígitos verificadores válidos (os do seed de demonstração são fictícios e usam bypass).
var alfa = ExistingCompany{ID: "c-alfa", InternalCode: "101", Cnpj: cnpj.WithCheckDigits("000000010001"), LegalName: "Empresa Fictícia Alfa Comércio Ltda"}
var beta = ExistingCompany{ID: "c-beta", InternalCode: "102", Cnpj: cnpj.WithCheckDigits("000000020001"), LegalName: "Empresa Fictícia Beta Serviços Ltda"}

const anaID = "7f3c2a10-1b2c-4d5e-8f90-1234567890ab"

func lookup() fakeLookup {
	return fakeLookup{
		byCnpj: map[string]ExistingCompany{alfa.Cnpj: alfa, beta.Cnpj: beta},
		byCode: map[string]ExistingCompany{"101": alfa, "102": beta},
		users:  map[string]string{"ana souza": anaID, "ana@escritorio.exemplo": anaID},
	}
}

// rec monta uma linha válida (nova empresa) com sobrescritas.
func rec(line int, override map[string]string) Record {
	v := map[string]string{
		"internal_code": "301", "legal_name": "Nova Empresa Ltda", "cnpj": cnpj.Format(cnpj.WithCheckDigits("000000110001")),
		"tax_regime": "Simples Nacional", "service_start_date": "01/02/2026", "payroll_type": "Sem folha",
		"module_fiscal": "Sim", "module_declarations": "sim", "module_payroll": "Não", "state": "pr", "responsible_fiscal": "Ana Souza",
	}
	for k, val := range override {
		if val == "" {
			delete(v, k)
		} else {
			v[k] = val
		}
	}
	return Record{Line: line, Values: v}
}

func statusOf(rows []Row) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.Status
	}
	return out
}

func TestClassify_EachStatus(t *testing.T) {
	records := []Record{
		rec(2, nil), // nova
		rec(3, map[string]string{"internal_code": "101", "cnpj": cnpj.Format(alfa.Cnpj)}),                                                   // existente (mesmo CNPJ e código)
		rec(4, map[string]string{"internal_code": "304", "cnpj": "00.000.001/0001-99"}),                                                     // CNPJ inválido
		rec(5, map[string]string{"internal_code": "102", "cnpj": cnpj.WithCheckDigits("000000120001")}),                                     // código de outra empresa
		rec(6, map[string]string{"internal_code": "306", "legal_name": "", "cnpj": cnpj.WithCheckDigits("000000130001")}),                   // obrigatório ausente
		rec(7, map[string]string{"internal_code": "307", "cnpj": cnpj.WithCheckDigits("000000140001"), "service_start_date": "31/02/2026"}), // valor inválido
	}
	rows := Classify(records, lookup())
	want := []string{StatusNew, StatusExisting, StatusInvalidCnpj, StatusDuplicateCode, StatusMissingRequired, StatusInvalidValue}
	if got := statusOf(rows); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("status = %v\nwant %v", got, want)
	}
	if rows[0].StatusLabel != "Nova empresa" || rows[1].StatusLabel != "Empresa existente" || rows[2].StatusLabel != "CNPJ inválido" ||
		rows[3].StatusLabel != "Código duplicado" || rows[4].StatusLabel != "Campo obrigatório ausente" {
		t.Fatal("rótulos da prévia incorretos")
	}
	if rows[1].ExistingID == nil || *rows[1].ExistingID != "c-alfa" {
		t.Fatal("existente deveria apontar para a empresa cadastrada")
	}
	if e := rows[4].Errors; len(e) != 1 || e[0].Field != "legal_name" || e[0].Column != "Razão social" {
		t.Fatalf("erros da linha 6 = %+v", e)
	}
	if e := rows[3].Errors; len(e) != 1 || !strings.Contains(e[0].Message, "Beta Serviços") {
		t.Fatalf("erros da linha 5 = %+v", e)
	}
	if e := rows[5].Errors; len(e) != 1 || e[0].Field != "service_start_date" {
		t.Fatalf("erros da linha 7 = %+v", e)
	}
}

func TestClassify_NormalizesValidRow(t *testing.T) {
	// 46387 é a data serial do Excel para 31/12/2026 (células de data lidas sem formatação).
	rows := Classify([]Record{rec(2, map[string]string{"factor_r": "Não", "active": "Inativa", "service_end_date": "46387"})}, lookup())
	r := rows[0]
	if r.Status != StatusNew || len(r.Errors) != 0 {
		t.Fatalf("linha = %+v", r)
	}
	in := r.Input
	if in.Cnpj != cnpj.WithCheckDigits("000000110001") || r.Cnpj != in.Cnpj {
		t.Fatalf("CNPJ não sanitizado: %q", in.Cnpj)
	}
	if in.TaxRegime != "SIMPLES_NACIONAL" || in.PayrollType != "NO_PAYROLL" || *in.State != "PR" || *in.FactorR {
		t.Fatalf("listas não normalizadas: %+v", in)
	}
	if in.ServiceStartDate.String() != "2026-02-01" || in.ServiceEndDate == nil || in.ServiceEndDate.String() != "2026-12-31" {
		t.Fatalf("datas = %s / %v", in.ServiceStartDate, in.ServiceEndDate)
	}
	if in.Modules == nil || strings.Join(*in.Modules, ",") != "FISCAL,DECLARATIONS" {
		t.Fatalf("módulos = %v", in.Modules)
	}
	if in.FiscalResponsibleUserID == nil || *in.FiscalResponsibleUserID != anaID || r.Active {
		t.Fatalf("responsável/situação = %+v / ativa=%v", in.FiscalResponsibleUserID, r.Active)
	}
}

func TestClassify_DuplicatesInsideTheFile(t *testing.T) {
	c2 := cnpj.WithCheckDigits("000000150001")
	rows := Classify([]Record{
		rec(2, nil),
		rec(3, map[string]string{"cnpj": c2}), // código 301 repetido
		rec(4, map[string]string{"internal_code": "309", "cnpj": c2}), // CNPJ repetido no arquivo
	}, lookup())
	if got := statusOf(rows); strings.Join(got, ",") != StatusNew+","+StatusDuplicateCode+","+StatusInvalidValue {
		t.Fatalf("status = %v", got)
	}
	if !strings.Contains(rows[1].Errors[0].Message, "linha 2") || !strings.Contains(rows[2].Errors[0].Message, "linha 3") {
		t.Fatalf("mensagens = %+v / %+v", rows[1].Errors, rows[2].Errors)
	}
}

func TestClassify_InvalidValues(t *testing.T) {
	cases := map[string]map[string]string{
		"regime":        {"tax_regime": "MEI"},
		"tipo de folha": {"payroll_type": "Às vezes"},
		"módulo":        {"module_fiscal": "talvez"},
		"fator R":       {"factor_r": "?"},
		"situação":      {"active": "Suspensa"},
		"responsável":   {"responsible_fiscal": "Fulano"},
		"UF":            {"state": "XX"},
		"encerramento":  {"service_end_date": "01/01/2026"}, // antes do início (01/02/2026)
		"código longo":  {"internal_code": strings.Repeat("9", 31)},
	}
	for name, override := range cases {
		t.Run(name, func(t *testing.T) {
			r := Classify([]Record{rec(2, override)}, lookup())[0]
			if r.Status != StatusInvalidValue || len(r.Errors) == 0 {
				t.Fatalf("linha = %+v", r)
			}
		})
	}
}

func TestParseDate(t *testing.T) {
	for in, want := range map[string]string{"01/02/2026": "2026-02-01", "1/2/2026": "2026-02-01", "2026-02-01": "2026-02-01", "46054": "2026-02-01"} {
		d, ok := parseDate(in)
		if !ok || d.String() != want {
			t.Errorf("parseDate(%q) = %s, %v; want %s", in, d, ok, want)
		}
	}
	for _, in := range []string{"31/02/2026", "2026/02/01", "fev/2026", "0", "1.5"} {
		if _, ok := parseDate(in); ok {
			t.Errorf("parseDate(%q) deveria falhar", in)
		}
	}
}
