package reconciliation

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
