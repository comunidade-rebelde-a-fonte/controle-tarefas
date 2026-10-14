package engine

import (
	"errors"
	"slices"
	"time"

	"controle/internal/platform/decimal"
)

// Annex é o anexo do Simples Nacional em jogo no Fator R (Anexo).
type Annex string

// Anexos.
const (
	AnnexIII Annex = "III"
	AnnexV   Annex = "V"
)

// Bracket é uma faixa de um anexo (Faixa).
type Bracket struct {
	Number      int             // faixa
	RBT12UpTo   decimal.Decimal // rbt12_ate
	NominalRate decimal.Decimal // aliquota_nominal
	Deduction   decimal.Decimal // parcela_deduzir
}

// AnnexTable é a tabela de um anexo numa vigência (TabelaAnexo). Os valores vêm do banco;
// nenhum valor fixo no código.
type AnnexTable struct {
	Annex     Annex
	ValidFrom time.Time // vigencia_inicio
	Brackets  []Bracket
}

// BracketFor é a primeira faixa, por limite crescente, que comporta o RBT12; nil acima do
// limite do Simples (faixa_para).
func (t AnnexTable) BracketFor(rbt12 decimal.Decimal) *Bracket {
	sorted := slices.Clone(t.Brackets)
	slices.SortStableFunc(sorted, func(a, b Bracket) int { return a.RBT12UpTo.Cmp(b.RBT12UpTo) })
	for i := range sorted {
		if rbt12.Cmp(sorted[i].RBT12UpTo) <= 0 {
			return &sorted[i]
		}
	}
	return nil
}

// Tables são os Anexos III e V vigentes no PA (TabelasVigentes).
type Tables struct {
	III AnnexTable
	V   AnnexTable
}

// ErrTableNotInForce: não há tabela do Simples vigente para a data pedida (TabelaNaoVigente).
var ErrTableNotInForce = errors.New("não há tabela do Simples vigente para esse PA")
