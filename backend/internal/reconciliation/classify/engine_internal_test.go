package classify

import "testing"

// O CNPJ e a contraparte só entram quando os comprovantes da conciliação concordam (N:1 do mesmo
// fornecedor); misturados, vale o descritivo.
func TestInputOf(t *testing.T) {
	in := inputOf("PIX ENVIADO ALFA", []string{"Alfa Ltda", "ALFA  LTDA", ""}, []string{"12345678000199", "", "12345678000199"})
	if in.Counterparty == nil || *in.Counterparty != "Alfa Ltda" || in.CNPJ == nil || *in.CNPJ != "12345678000199" || *in.Description != "PIX ENVIADO ALFA" {
		t.Fatalf("N:1 do mesmo fornecedor = %+v", in)
	}
	mixed := inputOf("PIX ENVIADO VARIOS", []string{"Alfa", "Beta"}, []string{"12345678000199", "98765432000110"})
	if mixed.Counterparty != nil || mixed.CNPJ != nil {
		t.Fatalf("N:1 misto = %+v", mixed)
	}
	none := inputOf("TARIFA", nil, nil)
	if none.Counterparty != nil || none.CNPJ != nil || *none.Description != "TARIFA" {
		t.Fatalf("sem comprovante = %+v", none)
	}
}
