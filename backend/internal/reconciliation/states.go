package reconciliation

import "encoding/json"

// Estados do documento da Conciliação (contrato congelado da origem, D-020): pendente → triado →
// importado, com os desvios de revisão e de período fechado.
const (
	DocPending       = "pendente"
	DocTriaged       = "triado"
	DocExtracted     = "extraido"
	DocImported      = "importado"
	DocTriageReview  = "revisao_triagem"
	DocExtractReview = "revisao_extracao"
	DocDuplicate     = "duplicado"
	DocClosedPeriod  = "pendencia_periodo_fechado"
)

// Jobs do processamento dos documentos [04 §7; TASK-015, TASK-016]: a triagem nasce no envio; o
// extrato triado ganha a extração; o comprovante, a extração de comprovante (TASK-017). A rodada
// do matching (TASK-018) é enfileirada quando chega extrato importado ou lançamento novo.
const (
	JobTriage           = "reconciliation_triage"
	JobStatementExtract = "statement_extract"
	JobReceiptExtract   = "receipt_extract"
	JobRun              = "reconciliation_run"
	// JobResolve pede ao resolvedor uma hipótese para a linha (TASK-019): o agente grava só a
	// proposta.
	JobResolve = "agent_resolver"
	// JobClassify pede ao classificador uma conta para a linha conciliada sem regra nem histórico
	// (TASK-020): o agente grava só a sugestão.
	JobClassify = "agent_classifier"
)

// MarkUndone é a marca da linha cuja conciliação foi desfeita (TASK-021a): fica na linha (a
// rodada não a troca) e tira a linha do automático, que não volta a casar sozinho o que o
// contador desfez. A linha vai para a fila como divergência, com o motivo do desfazer.
const MarkUndone = "desfeita"

// HasMark informa se a lista de marcas (statement_transactions.flags) tem uma marca do tipo dado.
func HasMark(flags []byte, kind string) bool {
	var list []any
	if json.Unmarshal(flags, &list) != nil {
		return false
	}
	for _, x := range list {
		if obj, ok := x.(map[string]any); ok && obj["tipo"] == kind {
			return true
		}
	}
	return false
}
