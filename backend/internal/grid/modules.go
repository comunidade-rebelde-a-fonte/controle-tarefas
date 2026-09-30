package grid

import "controle/internal/company"

// Grades dos demais módulos [PRD §29-§32, §41.6]: todas sobre o mesmo construtor, com as
// colunas do contexto de exibição do catálogo e todos os regimes.

// DeclarationsSpec: colunas dinâmicas do catálogo; a célula traz vencimento, transmissão e
// protocolo [§29].
func DeclarationsSpec() Spec {
	return Spec{Module: "DECLARATIONS", PeriodModule: "DECLARATIONS", Regimes: company.Regimes, Closable: true}
}

// PayrollSpec: Tipo de Folha e Fator R vêm do snapshot (Fator R só no Simples) [§30]. O
// eSocial é a mesma instância exibida na DCTFWeb [CA-009].
func PayrollSpec() Spec {
	return Spec{Module: "PAYROLL", PeriodModule: "PAYROLL", Regimes: company.Regimes, Closable: true}
}

// AccountingSpec: colunas na ordem da sequência (sort_order do contexto) e blocked_by por
// célula [§18, §32].
func AccountingSpec() Spec {
	return Spec{Module: "ACCOUNTING", PeriodModule: "ACCOUNTING", Regimes: company.Regimes, Closable: true}
}

// DctfwebSpec: visão sobre instâncias de Declarações e Folha (contexto DCTFWEB), com a coluna
// "dependências" da DCTFWeb e sem fechamento próprio (plan R1). As linhas e a observação são
// as de Declarações, módulo da DCTFWeb [§16, §31].
func DctfwebSpec() Spec {
	return Spec{Module: "DCTFWEB", PeriodModule: "DECLARATIONS", Regimes: company.Regimes, DependencyColumn: "DCTFWEB"}
}
