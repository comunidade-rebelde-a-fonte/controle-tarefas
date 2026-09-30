// Package importer importa o cadastro de empresas por planilha (.xlsx/.csv): modelo,
// leitura, validação, prévia e confirmação [PRD §35, §41.12].
package importer

// Opções de lista aceitas nas colunas (rótulos da interface; o parser também aceita os códigos).
var (
	RegimeOptions = []Option{
		{"Simples Nacional", "SIMPLES_NACIONAL"},
		{"Lucro Presumido", "LUCRO_PRESUMIDO"},
		{"Lucro Real", "LUCRO_REAL"},
		{"Lucro Arbitrado", "LUCRO_ARBITRADO"},
		{"Imune/Isenta", "IMUNE_ISENTA"},
	}
	PayrollOptions = []Option{
		{"Sem folha", "NO_PAYROLL"},
		{"Pró-labore", "PRO_LABORE"},
		{"Folha", "PAYROLL"},
	}
	YesNoOptions  = []Option{{"Sim", "true"}, {"Não", "false"}}
	ActiveOptions = []Option{{"Ativa", "true"}, {"Inativa", "false"}}
	UFs           = []string{"AC", "AL", "AP", "AM", "BA", "CE", "DF", "ES", "GO", "MA", "MT", "MS", "MG", "PA", "PB",
		"PR", "PE", "PI", "RJ", "RN", "RS", "RO", "RR", "SC", "SP", "SE", "TO"}
)

// Option é um valor de lista: rótulo na planilha e código na API.
type Option struct {
	Label string
	Code  string
}

// Column descreve uma coluna do modelo; o parser mapeia pelo cabeçalho [PRD §7.1].
type Column struct {
	Key      string
	Header   string
	Required bool
	Help     string
	Options  []string // lista de validação (rótulos)
	Width    float64
}

func labels(opts []Option) []string {
	out := make([]string, len(opts))
	for i, o := range opts {
		out[i] = o.Label
	}
	return out
}

// Columns são as colunas do modelo de importação, na ordem da planilha.
var Columns = []Column{
	{Key: "internal_code", Header: "Código interno", Required: true, Help: "Código do escritório, único por empresa.", Width: 16},
	{Key: "legal_name", Header: "Razão social", Required: true, Help: "Nome empresarial completo.", Width: 40},
	{Key: "trade_name", Header: "Nome fantasia", Help: "Opcional.", Width: 28},
	{Key: "cnpj", Header: "CNPJ", Required: true, Help: "Com ou sem máscara (00.000.000/0000-00). Chave para detectar empresas já cadastradas.", Width: 22},
	{Key: "tax_regime", Header: "Regime tributário inicial", Required: true, Help: "Regime vigente a partir do início do atendimento.", Options: labels(RegimeOptions), Width: 24},
	{Key: "service_start_date", Header: "Início do atendimento", Required: true, Help: "Data no formato dd/mm/aaaa.", Width: 18},
	{Key: "service_end_date", Header: "Encerramento do atendimento", Help: "Data no formato dd/mm/aaaa, quando houver.", Width: 18},
	{Key: "active", Header: "Situação", Help: "Ativa ou Inativa. Vazio = Ativa.", Options: labels(ActiveOptions), Width: 12},
	{Key: "city", Header: "Município", Help: "Opcional.", Width: 22},
	{Key: "state", Header: "UF", Help: "Sigla da UF.", Options: UFs, Width: 8},
	{Key: "state_registration", Header: "Inscrição estadual", Help: "Quando houver.", Width: 18},
	{Key: "municipal_registration", Header: "Inscrição municipal", Help: "Quando houver.", Width: 18},
	{Key: "payroll_type", Header: "Tipo de folha", Required: true, Help: "Sem folha, Pró-labore ou Folha.", Options: labels(PayrollOptions), Width: 14},
	{Key: "factor_r", Header: "Fator R", Help: "Sim ou Não (empresas do Simples Nacional).", Options: labels(YesNoOptions), Width: 10},
	{Key: "module_fiscal", Header: "Módulo Fiscal", Help: "Sim ou Não. Vazio = Não.", Options: labels(YesNoOptions), Width: 14},
	{Key: "module_declarations", Header: "Módulo Declarações", Help: "Sim ou Não. Vazio = Não.", Options: labels(YesNoOptions), Width: 18},
	{Key: "module_accounting", Header: "Módulo Contábil", Help: "Sim ou Não. Vazio = Não.", Options: labels(YesNoOptions), Width: 15},
	{Key: "module_payroll", Header: "Módulo Folha", Help: "Sim ou Não. Vazio = Não.", Options: labels(YesNoOptions), Width: 13},
	{Key: "module_credentials", Header: "Módulo Procurações/Certificados", Help: "Sim ou Não. Vazio = Não.", Options: labels(YesNoOptions), Width: 28},
	{Key: "responsible_fiscal", Header: "Responsável Fiscal", Help: "Nome ou e-mail de um responsável cadastrado.", Width: 22},
	{Key: "responsible_accounting", Header: "Responsável Contábil", Help: "Nome ou e-mail de um responsável cadastrado.", Width: 22},
	{Key: "responsible_payroll", Header: "Responsável Folha", Help: "Nome ou e-mail de um responsável cadastrado.", Width: 22},
	{Key: "responsible_declarations", Header: "Responsável Declarações", Help: "Nome ou e-mail de um responsável cadastrado.", Width: 24},
	{Key: "notes", Header: "Observações", Help: "Observações cadastrais gerais.", Width: 40},
}
