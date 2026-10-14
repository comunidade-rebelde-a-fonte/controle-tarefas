package matching

import "controle/internal/platform/decimal"

// Marcações de casos de borda do PRD §12 da origem (app/domain/bordas.py). A origem só as
// definiu; aqui elas marcam as linhas, que saem do automático e vão à fila com o motivo
// (ADR-008): nenhum caso de borda é resolvido pelo matching automático [REQ-030].

// Tipos de marca de borda.
const (
	EdgeInternalTransfer = "transferencia_interna"
	EdgeReversal         = "estorno"
	EdgeGrossNet         = "bruto_liquido"
)

// Parâmetros da origem.
const (
	// ReversalWindow é a janela do estorno (7 dias).
	ReversalWindow = 7
)

// GrossNetMaxDiff é a diferença máxima do bruto × líquido (R$ 20,00).
var GrossNetMaxDiff = decimal.MustParse("20.00")

// Mark é uma marca de borda: o tipo, os ids envolvidos e o detalhe.
type Mark struct {
	Kind   string
	IDs    []string
	Detail string
}

// InternalTransfers é o marcar_transferencia_interna: par C/D de mesmo valor no mesmo dia, uma
// possível transferência entre contas da empresa.
func InternalTransfers(stmts []Statement) []Mark {
	var out []Mark
	for _, a := range stmts {
		for _, b := range stmts {
			if a.ID >= b.ID {
				continue
			}
			if a.Amount.Cmp(b.Amount) == 0 && sameDay(a.Date, b.Date) && a.Direction != b.Direction &&
				(a.Direction == "C" || a.Direction == "D") {
				out = append(out, Mark{Kind: EdgeInternalTransfer, IDs: []string{a.ID, b.ID}, Detail: "par C/D mesmo valor e data"})
			}
		}
	}
	return out
}

// Reversals é o marcar_estorno: par de mesmo valor e sentidos opostos a até window dias.
func Reversals(stmts []Statement, window int) []Mark {
	var out []Mark
	for _, a := range stmts {
		for _, b := range stmts {
			if a.ID >= b.ID {
				continue
			}
			if a.Amount.Cmp(b.Amount) == 0 && a.Direction != b.Direction && days(a.Date, b.Date) <= window {
				out = append(out, Mark{Kind: EdgeReversal, IDs: []string{a.ID, b.ID}, Detail: "par simétrico próximo"})
			}
		}
	}
	return out
}

// GrossNet é o marcar_bruto_liquido: o comprovante maior que a linha do mesmo sentido, com
// diferença de até maxDiff. A origem não limitava a data; window ≥ 0 limita a D±window (ADR-008),
// e window < 0 reproduz a origem.
func GrossNet(stmts []Statement, entries []Entry, maxDiff decimal.Decimal, window int) []Mark {
	var out []Mark
	for _, e := range entries {
		for _, s := range stmts {
			if e.Direction != s.Direction || (window >= 0 && days(e.Date, s.Date) > window) {
				continue
			}
			if e.Amount.Cmp(s.Amount) > 0 && e.Amount.Sub(s.Amount).Cmp(maxDiff) <= 0 {
				out = append(out, Mark{Kind: EdgeGrossNet, IDs: []string{s.ID, e.ID}, Detail: "comprovante maior que extrato, diferença pequena"})
			}
		}
	}
	return out
}
