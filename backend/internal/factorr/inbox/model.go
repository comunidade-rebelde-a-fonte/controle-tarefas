// Package inbox é o Inbox PGDAS-D do Fator R [REQ-012, REQ-014, REQ-015, REQ-046; 06 §3;
// TASK-012]: upload manual do extrato, processamento assíncrono pelo agente parser_pgdas
// (determinístico, sem LLM: ADR-006), revisão, vínculo por CNPJ, cadastro assistido da empresa e
// gravação segura da série. Porte de api/inbox.py, agents/parser_pgdas.py e
// services/ouro_pgdas.py do Fator R (commit 9dd7a74).
//
// Diferença da origem: o upload só guarda o arquivo e enfileira o job pgdas_parse (202); o
// worker extrai, faz o parse e decide numa corrida do agente, como o upload síncrono da origem.
package inbox

import (
	"time"

	"controle/internal/company"
	"controle/internal/platform/decimal"
)

// Estados do documento (StatusDocumento da origem).
const (
	StatusReceived    = "received"
	StatusParsed      = "parsed"
	StatusNeedsReview = "needs_review"
	StatusLinked      = "linked"
	StatusRejected    = "rejected"
)

// Statuses são os estados, na ordem do fluxo.
var Statuses = []string{StatusReceived, StatusParsed, StatusNeedsReview, StatusLinked, StatusRejected}

// Agente e gatilhos das corridas (como na origem).
const (
	AgentName            = "parser_pgdas"
	TriggerUpload        = "upload"
	TriggerManualLink    = "vinculo_manual"
	TriggerReject        = "rejeicao"
	TriggerCreateCompany = "cadastro_pelo_extrato"
)

// KindParse é o job que processa o documento enviado.
const KindParse = "pgdas_parse"

// MaxFileSize é o limite do upload (upload_max_mb da origem: 10 MB).
const MaxFileSize = 10 << 20

// Fields são os campos lidos do extrato.
type Fields struct {
	CNPJ   *string `json:"cnpj"`
	PA     *string `json:"pa"`
	RBT12  *string `json:"rbt12"`
	RPA    *string `json:"rpa"`
	FS12   *string `json:"fs12"`
	FatorR *string `json:"fator_r"`
	DAS    *string `json:"das"`
	Anexo  *string `json:"anexo"`
}

// Identification são os dados de cadastro lidos do extrato (fora da confiança e da nota ouro).
type Identification struct {
	NomeEmpresarial *string `json:"nome_empresarial"`
	SujeitaFatorR   *bool   `json:"sujeita_fator_r"`
	InicioAtividade *string `json:"inicio_atividade"`
}

// PriorSeries são a receita e a folha mês a mês dos 12 meses anteriores ao PA, como declaradas.
type PriorSeries struct {
	Receitas         map[string]string `json:"receitas"`
	Folhas           map[string]string `json:"folhas"`
	ReceitasConferem bool              `json:"receitas_conferem"`
	FolhasConferem   bool              `json:"folhas_conferem"`
}

// JobState é o estado do job de processamento do upload [AC-014].
type JobState struct {
	ID            string  `json:"id"`
	Status        string  `json:"status"`
	Attempts      int32   `json:"attempts"`
	MaxAttempts   int32   `json:"max_attempts"`
	LastErrorCode *string `json:"last_error_code"`
}

// Suggestion são os dados do extrato para pré-preencher o cadastro de uma empresa fora da
// carteira (SugestaoCadastroOut da origem).
type Suggestion struct {
	CNPJ            string  `json:"cnpj"`
	CNPJFormatado   string  `json:"cnpj_formatado"`
	NomeEmpresarial *string `json:"nome_empresarial"`
	SujeitaFatorR   *bool   `json:"sujeita_fator_r"`
	InicioAtividade *string `json:"inicio_atividade"`
}

// Document é o documento do inbox (DocumentoOut da origem).
type Document struct {
	ID           string  `json:"id"`
	Status       string  `json:"status"`
	Motivo       *string `json:"motivo"`
	CompanyID    *string `json:"company_id"`
	NomeOriginal *string `json:"nome_original"`
	Mime         string  `json:"mime"`
	TamanhoBytes int32   `json:"tamanho_bytes"`
	// Campos lidos pelo parser (nulos até o processamento e quando não encontrados).
	Campos           Fields            `json:"campos"`
	ConfiancaCampos  map[string]string `json:"confianca_campos"`
	Motivos          []string          `json:"motivos"`
	Identificacao    Identification    `json:"identificacao"`
	SeriesAnteriores PriorSeries       `json:"series_anteriores"`
	Confianca        *decimal.Decimal  `json:"confianca"`
	ParserVersion    *string           `json:"parser_version"`
	TraceID          *string           `json:"trace_id"`
	// TextoAgente é o texto da última corrida do agente sobre o documento, com o disclaimer.
	TextoAgente      *string     `json:"texto_agente"`
	Job              *JobState   `json:"job"`
	SugestaoCadastro *Suggestion `json:"sugestao_cadastro"`
	Version          int32       `json:"version"`
	CreatedAt        time.Time   `json:"created_at"`
	UpdatedAt        time.Time   `json:"updated_at"`
}

// DocumentList é a resposta de GET /factor-r/pgdas (DocumentoList da origem).
type DocumentList struct {
	Items             []Document       `json:"items"`
	Total             int64            `json:"total"`
	ContagemPorStatus map[string]int64 `json:"contagem_por_status"`
}

// UploadResult é a resposta do upload: 202 com o job enfileirado ou 200 com o documento que
// já existia para o mesmo arquivo.
type UploadResult struct {
	DocumentID string  `json:"document_id"`
	JobID      *string `json:"job_id"`
	Status     string  `json:"status"`
	Duplicate  bool    `json:"duplicate"`
}

// LinkInput é o corpo do vínculo manual (VinculoIn).
type LinkInput struct {
	CompanyID string `json:"company_id"`
}

// Decisões de revisão aceitas em POST /factor-r/pgdas/{id}/review.
const DecisionReject = "rejeitar"

// ReviewInput é o corpo da revisão: hoje só a rejeição (RejeicaoIn da origem).
type ReviewInput struct {
	Decisao string `json:"decisao"`
	Motivo  string `json:"motivo"`
}

// CreateCompanyInput é o corpo do cadastro pelo extrato: a empresa canônica e, opcional, o
// início da atividade (YYYY-MM) da configuração do Fator R.
type CreateCompanyInput struct {
	Empresa         company.CreateInput `json:"empresa"`
	InicioAtividade *string             `json:"inicio_atividade"`
}

// CreateCompanyResult é a resposta do cadastro pelo extrato (CadastroPeloExtratoOut).
type CreateCompanyResult struct {
	Documento Document        `json:"documento"`
	Empresa   company.Company `json:"empresa"`
}
