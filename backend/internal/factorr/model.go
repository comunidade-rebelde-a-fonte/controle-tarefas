package factorr

import (
	"time"

	"controle/internal/factorr/engine"
	"controle/internal/platform/competence"
	"controle/internal/platform/decimal"
)

// Pacotes comerciais da carteira; sem pacote = null (o "none" do 05 §4).
var packages = []string{"monitoramento", "correcao", "retainer"}

// Settings é a configuração do Fator R de uma empresa (company_factor_r_settings). Sem
// configuração gravada, valem os padrões e version 0 (o PATCH com version 0 cria). "Sujeita ao
// Fator R" não fica aqui: é o factor_r do cadastro da empresa (D-017).
type Settings struct {
	CompanyID        string                 `json:"company_id"`
	Cnae             *string                `json:"cnae"`
	Atividade        *string                `json:"atividade"`
	QuantidadeSocios int32                  `json:"quantidade_socios"`
	Contato          *string                `json:"contato"`
	PacoteComercial  *string                `json:"pacote_comercial"`
	HonorarioMensal  decimal.Decimal        `json:"honorario_mensal"`
	InicioAtividade  *competence.Competence `json:"inicio_atividade"`
	Notas            *string                `json:"notas"`
	Version          int32                  `json:"version"`
	UpdatedAt        *time.Time             `json:"updated_at"`
}

// Movement é um mês da série de receita e folha (factor_r_movements).
type Movement struct {
	Competence               competence.Competence `json:"competence"`
	ReceitaBruta             decimal.Decimal       `json:"receita_bruta"`
	ProLabore                decimal.Decimal       `json:"pro_labore"`
	Salarios13Ferias         decimal.Decimal       `json:"salarios_13_ferias"`
	CppRecolhida             decimal.Decimal       `json:"cpp_recolhida"`
	FgtsRecolhido            decimal.Decimal       `json:"fgts_recolhido"`
	FolhaTotalDeclaradaPgdas *decimal.Decimal      `json:"folha_total_declarada_pgdas"`
	// FolhaMes é a folha do mês: a total declarada no PGDAS-D ou a soma de pró-labore,
	// salários, CPP e FGTS (coluna gerada).
	FolhaMes        decimal.Decimal `json:"folha_mes"`
	Origem          string          `json:"origem"`
	Observacao      *string         `json:"observacao"`
	PgdasDocumentID *string         `json:"pgdas_document_id"`
	Version         int32           `json:"version"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

// MovementList é a resposta de GET /companies/{id}/factor-r/movements.
type MovementList struct {
	Items []Movement `json:"items"`
}

// Calc é a resposta de GET /companies/{id}/factor-r/calc: o resultado do motor mais a situação
// da empresa no PA.
type Calc struct {
	engine.Result
	// RegimeNoPA é o regime tributário vigente no PA (null sem regime vigente).
	RegimeNoPA *string `json:"regime_no_pa"`
	// Monitorada: ativa, sujeita ao Fator R e no Simples Nacional no PA, ou seja, na carteira.
	Monitorada bool `json:"monitorada"`
}

// CompanyRef identifica a empresa numa linha da carteira.
type CompanyRef struct {
	ID           string  `json:"id"`
	InternalCode string  `json:"internal_code"`
	LegalName    string  `json:"legal_name"`
	TradeName    *string `json:"trade_name"`
	Cnpj         string  `json:"cnpj"`
}

// PortfolioLine é uma linha da carteira (LinhaCarteiraOut da origem).
type PortfolioLine struct {
	Company           CompanyRef              `json:"company"`
	PacoteComercial   *string                 `json:"pacote_comercial"`
	HonorarioMensal   decimal.Decimal         `json:"honorario_mensal"`
	Acao              engine.Action           `json:"acao"`
	AcaoTexto         string                  `json:"acao_texto"`
	Status            engine.Status           `json:"status"`
	Motivo            *engine.Reason          `json:"motivo"`
	Semaforo          *engine.Light           `json:"semaforo"`
	FatorR            *decimal.Decimal        `json:"fator_r"`
	Anexo             *engine.Annex           `json:"anexo"`
	RBT12             decimal.Decimal         `json:"rbt12"`
	FS12              decimal.Decimal         `json:"fs12"`
	ReforcoMensal28   *decimal.Decimal        `json:"reforco_mensal_28"`
	ReforcoMensalMeta *decimal.Decimal        `json:"reforco_mensal_meta"`
	MetaOperacional   decimal.Decimal         `json:"meta_operacional"`
	Economia12m       *decimal.Decimal        `json:"economia_12m"`
	MesesFaltantes    []competence.Competence `json:"meses_faltantes"`
}

// Portfolio é a resposta de GET /factor-r/portfolio.
type Portfolio struct {
	PA              competence.Competence `json:"pa"`
	MetaOperacional decimal.Decimal       `json:"meta_operacional"`
	KPIs            engine.KPIs           `json:"kpis"`
	Linhas          []PortfolioLine       `json:"linhas"`
}
