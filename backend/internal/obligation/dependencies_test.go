package obligation

import (
	"strings"
	"testing"
)

// Cadeia contábil: 1 ← 2 ← 3 ← 4 ← 5 ← 6 (HARD) e DCTFWeb SOFT sobre MIT e eSocial.
var chain = []Dependency{
	{"s2", "s1", DependencyHard}, {"s3", "s2", DependencyHard}, {"s4", "s3", DependencyHard},
	{"s5", "s4", DependencyHard}, {"s6", "s5", DependencyHard},
	{"dctf", "mit", DependencySoft}, {"dctf", "esocial", DependencySoft},
}

func steps(statuses map[string]string, notApplicable ...string) map[string]DepInstance {
	names := map[string]string{"s1": "Movimento recebido", "s2": "Contabilização da folha", "s3": "Contabilização dos impostos",
		"s4": "Contábil digitado/processado", "s5": "Conciliação", "s6": "Fechamento do balancete"}
	out := map[string]DepInstance{}
	for i, id := range []string{"s1", "s2", "s3", "s4", "s5", "s6"} {
		st := statuses[id]
		if st == "" {
			st = "NOT_STARTED"
		}
		na := false
		for _, n := range notApplicable {
			na = na || n == id
		}
		out[id] = DepInstance{TypeID: id, Name: names[id], StatusCode: st, Applicable: !na, IsFinal: st == "OK" || st == "NO_MOVEMENT", Order: i}
	}
	return out
}

func TestCheckHard_TransitiveChain(t *testing.T) {
	// Etapas 1-3 OK, 4 A iniciar: Conciliação (5) bloqueada pela 4.
	pending := CheckHard("s5", chain, steps(map[string]string{"s1": "OK", "s2": "OK", "s3": "OK"}))
	if len(pending) != 1 || pending[0].TypeID != "s4" {
		t.Fatalf("pendentes = %+v, want [s4]", pending)
	}
	if msg := BlockedMessage("Conciliação", pending); msg != "Não é possível concluir Conciliação. A etapa Contábil digitado/processado ainda não foi concluída." {
		t.Fatalf("mensagem = %q", msg)
	}
	// Todas anteriores pendentes: lista em ordem.
	all := CheckHard("s5", chain, steps(nil))
	if len(all) != 4 || all[0].TypeID != "s1" || all[3].TypeID != "s4" {
		t.Fatalf("pendentes = %+v", all)
	}
	if msg := BlockedMessage("Conciliação", all); !strings.Contains(msg, "As etapas Movimento recebido, Contabilização da folha, Contabilização dos impostos e Contábil digitado/processado") {
		t.Fatalf("mensagem = %q", msg)
	}
}

func TestCheckHard_NoMovementSatisfiesAndNotApplicableIgnored(t *testing.T) {
	st := steps(map[string]string{"s1": "OK", "s2": "NO_MOVEMENT", "s3": "OK", "s4": "OK"})
	if p := CheckHard("s5", chain, st); len(p) != 0 {
		t.Fatalf("Sem movimento deveria satisfazer: %+v", p)
	}
	st = steps(map[string]string{"s1": "OK", "s3": "OK", "s4": "OK"}, "s2")
	if p := CheckHard("s5", chain, st); len(p) != 0 {
		t.Fatalf("pré-requisito não aplicável deveria ser ignorado: %+v", p)
	}
	if p := CheckHard("s1", chain, steps(nil)); len(p) != 0 {
		t.Fatal("primeira etapa não tem pré-requisitos")
	}
}

func TestCheckSoft_DirectComponents(t *testing.T) {
	insts := map[string]DepInstance{
		"mit":     {TypeID: "mit", Name: "MIT", Applicable: true, IsFinal: true, Order: 1},
		"esocial": {TypeID: "esocial", Name: "eSocial", Applicable: true, IsFinal: false, Order: 2},
	}
	pending := CheckSoft("dctf", chain, insts)
	if len(pending) != 1 || pending[0].Name != "eSocial" {
		t.Fatalf("pendentes = %+v", pending)
	}
	if msg := SoftWarningMessage("DCTFWeb", pending); msg != "DCTFWeb possui pré-requisitos pendentes: eSocial." {
		t.Fatalf("mensagem = %q", msg)
	}
	if p := CheckHard("dctf", chain, insts); len(p) != 0 {
		t.Fatal("dependência SOFT não bloqueia como HARD")
	}
}

func TestSoftDependencyStatus(t *testing.T) {
	insts := map[string]DepInstance{
		"mit":     {TypeID: "mit", Name: "MIT", Applicable: true, IsFinal: true},
		"esocial": {TypeID: "esocial", Name: "eSocial", Applicable: true, IsFinal: false},
	}
	st := SoftDependencyStatus("dctf", chain, insts)
	if st == nil || st.Ready || len(st.Pending) != 1 {
		t.Fatalf("status = %+v", st)
	}
	insts["esocial"] = DepInstance{TypeID: "esocial", Applicable: false}
	if st := SoftDependencyStatus("dctf", chain, insts); st == nil || !st.Ready {
		t.Fatalf("componente não aplicável não deve pender: %+v", st)
	}
	if SoftDependencyStatus("s3", chain, insts) != nil {
		t.Fatal("tipo sem dependência SOFT não tem indicador")
	}
}
