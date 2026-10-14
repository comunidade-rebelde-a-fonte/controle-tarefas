package receipt

import (
	"reflect"
	"testing"
	"time"

	"controle/internal/platform/decimal"
)

func dec(s string) *decimal.Decimal {
	d := decimal.MustParse(s)
	return &d
}

func base() Receipt {
	d := time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC)
	nine := decimal.MustParse("0.9")
	return Receipt{
		Date: &d, Amount: dec("10.00"), Direction: str(Credit), Counterparty: str("Ana Lucia Ferreira"),
		Confidence: map[string]decimal.Decimal{FieldDate: nine, FieldAmount: nine, FieldDirection: nine, FieldCounterparty: nine},
	}
}

// Regras de campo da M3.4 e formatos do banco [REQ-023]: valor inválido e contraparte vazia ficam
// nulos e vão à revisão; documento da contraparte fora de CPF/CNPJ fica nulo sem mandar à revisão.
func TestCheck_FieldRules(t *testing.T) {
	cases := []struct {
		name   string
		change func(*Receipt)
		weak   []string
		check  func(Receipt) bool
	}{
		{"válido", func(*Receipt) {}, []string{}, func(r Receipt) bool { return r.Amount.String() == "10.00" }},
		{"valor zero", func(r *Receipt) { r.Amount = dec("0.00") }, []string{FieldAmount}, func(r Receipt) bool { return r.Amount == nil }},
		{"valor negativo", func(r *Receipt) { r.Amount = dec("-5.00") }, []string{FieldAmount}, func(r Receipt) bool { return r.Amount == nil }},
		{"valor fora de numeric(14,2)", func(r *Receipt) { r.Amount = dec("1000000000000.00") }, []string{FieldAmount}, func(r Receipt) bool { return r.Amount == nil }},
		{"maior valor do banco", func(r *Receipt) { r.Amount = dec("999999999999.99") }, []string{}, func(r Receipt) bool { return r.Amount != nil }},
		{"contraparte em branco", func(r *Receipt) { r.Counterparty = str("  ") }, []string{FieldCounterparty}, func(r Receipt) bool { return r.Counterparty == nil }},
		{"confiança no limiar", func(r *Receipt) { r.Confidence[FieldDate] = decimal.MustParse("0.70") }, []string{}, func(Receipt) bool { return true }},
		{"confiança abaixo do limiar", func(r *Receipt) { r.Confidence[FieldDirection] = decimal.MustParse("0.69") }, []string{FieldDirection}, func(Receipt) bool { return true }},
		{"sem confiança", func(r *Receipt) { delete(r.Confidence, FieldCounterparty) }, []string{FieldCounterparty}, func(Receipt) bool { return true }},
		{"CNPJ pontuado", func(r *Receipt) { r.CounterpartyDoc = str("12.345.678/0001-90") }, []string{},
			func(r Receipt) bool { return r.CounterpartyDoc != nil && *r.CounterpartyDoc == "12345678000190" }},
		{"CPF pontuado", func(r *Receipt) { r.CounterpartyDoc = str("123.456.789-01") }, []string{},
			func(r Receipt) bool { return r.CounterpartyDoc != nil && *r.CounterpartyDoc == "12345678901" }},
		{"CPF mascarado", func(r *Receipt) { r.CounterpartyDoc = str("***.456.789-**") }, []string{}, func(r Receipt) bool { return r.CounterpartyDoc == nil }},
		{"documento vazio", func(r *Receipt) { r.DocumentRef, r.OperationType = str(""), str(" ") }, []string{},
			func(r Receipt) bool { return r.DocumentRef == nil && r.OperationType == nil }},
	}
	for _, c := range cases {
		r := base()
		c.change(&r)
		got, weak := Check(r, originThreshold)
		if !reflect.DeepEqual(weak, c.weak) || !c.check(got) {
			t.Errorf("%s: campos a revisar %v (esperado %v), comprovante %+v", c.name, weak, c.weak, got)
		}
	}
}

// O leitor procura a empresa do documento pelo nome, sem a natureza jurídica, em palavras
// inteiras (a origem procurava "horizonte").
func TestClient_Names(t *testing.T) {
	c := NewClient("Construtora Horizonte Ltda", "Horizonte Obras", "")
	for s, want := range map[string]bool{
		"CONSTRUTORA HORIZONTE LTDA.":     true,
		"Construtora Horizonte":           true,
		"Construtora  Horizonte S/A":      true,
		"construtora horizonte-ltda":      true,
		"Horizonte Obras e Serviços":      true,
		"Construtora Horizontes Ltda":     false,
		"Horizonte":                       false,
		"Aco Brasil Distribuidora Ltda":   false,
		"Prefeitura de Belo Horizonte MG": false,
	} {
		if got := c.In(s); got != want {
			t.Errorf("In(%q) = %v; esperado %v", s, got, want)
		}
	}
	if !NewClient("Cerâmica São José ME").In("CERAMICA SAO JOSE") {
		t.Error("acento e natureza jurídica deviam ser ignorados")
	}
	if NewClient("", "  ").Known() || !NewClient("Ltda").Known() {
		t.Error("nome vazio não identifica; nome só com a natureza jurídica fica como está")
	}
}

const pixRecebido = "Banco Digital Nuvem\nComprovante de transferência PIX\nData e hora\n05/08/2026 09:12\nValor\nR$ 2.000,00\n" +
	"Pagador\nAna Lucia Ferreira\nRecebedor\nConstrutora Horizonte Ltda\n"

const pixEnviado = "Banco A Fonte\nComprovante de transferência PIX\nData e hora\n06/08/2026 10:20\nValor\nR$ 20.000,00\n" +
	"Pagador\nConstrutora Horizonte Ltda\nCNPJ\n12.345.678/0001-90\nRecebedor\nAco Brasil Distribuidora Ltda\n" +
	"CNPJ\n89.012.345/0001-67\nDescrição\nNF 4410 vergalhoes - parte 1 de 2\n"

// O sentido segue a empresa do documento, qualquer que seja ela: o mesmo PIX é saída para quem
// paga e entrada para quem recebe; sem a empresa no comprovante, é suposição (0,80).
func TestFromText_DirectionFollowsTheDocumentCompany(t *testing.T) {
	cases := []struct {
		text, client, direction, counterparty, conf string
		named                                       bool
	}{
		{pixEnviado, "Construtora Horizonte Ltda", Debit, "Aco Brasil Distribuidora Ltda", "1.0000", true},
		{pixEnviado, "Aço Brasil Distribuidora EIRELI", Credit, "Construtora Horizonte Ltda", "1.0000", true},
		{pixRecebido, "Construtora Horizonte Ltda", Credit, "Ana Lucia Ferreira", "1.0000", true},
		{pixRecebido, "Padaria Pão Quente Ltda", Credit, "Ana Lucia Ferreira", "0.8000", false},
	}
	for _, c := range cases {
		client := NewClient(c.client)
		r, ok, err := FromText(c.text, client)
		if err != nil || !ok {
			t.Fatalf("%s: não leu (%v)", c.client, err)
		}
		if *r.Direction != c.direction || *r.Counterparty != c.counterparty || r.Confidence[FieldDirection].String() != c.conf {
			t.Errorf("%s: sentido %s, contraparte %q, confiança %s", c.client, *r.Direction, *r.Counterparty, r.Confidence[FieldDirection])
		}
		if ClientNamed(c.text, client) != c.named {
			t.Errorf("%s: ClientNamed = %v", c.client, !c.named)
		}
	}
	r, _, _ := FromText(pixEnviado, NewClient("Construtora Horizonte Ltda"))
	if !reflect.DeepEqual(r.Alerts, []string{FlagInstallment, "nf:NF 4410"}) || *r.DocumentRef != "NF 4410" {
		t.Errorf("marcas %v, documento %v", r.Alerts, *r.DocumentRef)
	}
}

func TestCNPJs(t *testing.T) {
	text := "CNPJ\n12.345.678/0001-90\nCNPJ 89.012.345/0001-67\n12345678000190\nE77700000202608050912000000101\n" +
		"CPF ***.456.789-**\n123.456.789-01\n9912.345.678/0001-90"
	if got := CNPJs(text); !reflect.DeepEqual(got, []string{"12345678000190", "89012345000167"}) {
		t.Errorf("CNPJs = %v", got)
	}
	if got := CNPJs("CNPJ ١٢.٣٤٥.٦٧٨/٠٠٠١-٩٠"); !reflect.DeepEqual(got, []string{"12345678000190"}) {
		t.Errorf("CNPJ em dígitos árabe-índicos = %v", got)
	}
}

func TestFlags(t *testing.T) {
	if got := KnownFlags([]string{"parcelado", " juros ", "nf:NF 1", "nf:", "nf: ", "outro", "iss_retido"}); !reflect.DeepEqual(got,
		[]string{"parcelado", "juros", "nf:NF 1", "iss_retido"}) {
		t.Errorf("KnownFlags = %v", got)
	}
	if got := MergeFlags([]string{"juros"}, []string{"juros", "parcelado"}, nil); !reflect.DeepEqual(got, []string{"juros", "parcelado"}) {
		t.Errorf("MergeFlags = %v", got)
	}
	for text, want := range map[string][]string{
		"(+) Juros\nR$ 0,00\n":                    {},
		"(+) Juros 1% a.m. (5 dias)\nR$ 30,50\n":  {FlagInterest},
		"Parcela 2 de 3\nNFS-e 2201 - serviço\n":  {FlagInstallment, "nf:NFS-E 2201"},
		"(-) ISS retido na fonte 5%\nR$ 5.000,00": {FlagISSWithheld},
	} {
		if got := TextFlags(text); !reflect.DeepEqual(got, want) {
			t.Errorf("TextFlags(%q) = %v; esperado %v", text, got, want)
		}
	}
	if !IsReceiptText("COMPROVANTE DE PAGAMENTO") || IsReceiptText("Extrato de conta corrente") {
		t.Error("IsReceiptText")
	}
}
