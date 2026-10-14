package pgdas

import (
	"fmt"
	"maps"

	"controle/internal/platform/decimal"
)

// Stored é o resultado do parse como fica gravado no documento (o campos_json da origem): os
// campos lidos e, com prefixo "_", as confianças por campo, os motivos, a identificação para o
// cadastro e as séries dos 12 meses anteriores. É o que o agente usa para decidir e vincular.
type Stored struct {
	CNPJ             *string           `json:"cnpj"`
	PA               *string           `json:"pa"`
	RBT12            *string           `json:"rbt12"`
	RPA              *string           `json:"rpa"`
	FS12             *string           `json:"fs12"`
	FatorR           *string           `json:"fator_r"`
	DAS              *string           `json:"das"`
	Anexo            *string           `json:"anexo"`
	ConfiancaCampos  map[string]string `json:"_confianca_campos"`
	Motivos          []string          `json:"_motivos"`
	Identificacao    StoredID          `json:"_identificacao"`
	SeriesAnteriores StoredSeries      `json:"_series_anteriores"`
}

// StoredID é a identificação lida do extrato (fora da confiança e da nota ouro).
type StoredID struct {
	NomeEmpresarial *string `json:"nome_empresarial"`
	SujeitaFatorR   *bool   `json:"sujeita_fator_r"`
	InicioAtividade *string `json:"inicio_atividade"`
}

// StoredSeries são as séries mensais declaradas e se conferem com RBT12 e FS12.
type StoredSeries struct {
	Receitas         map[string]string `json:"receitas"`
	Folhas           map[string]string `json:"folhas"`
	ReceitasConferem bool              `json:"receitas_conferem"`
	FolhasConferem   bool              `json:"folhas_conferem"`
}

// Stored monta o JSON gravado (_campos_json).
func (r Result) Stored() Stored {
	conf := make(map[string]string, len(r.FieldConfidence))
	for f, v := range r.FieldConfidence {
		conf[f] = v.String()
	}
	motivos := append([]string{}, r.Reasons...)
	return Stored{
		CNPJ: r.Fields["cnpj"], PA: r.Fields["pa"], RBT12: r.Fields["rbt12"], RPA: r.Fields["rpa"],
		FS12: r.Fields["fs12"], FatorR: r.Fields["fator_r"], DAS: r.Fields["das"], Anexo: r.Fields["anexo"],
		ConfiancaCampos: conf, Motivos: motivos,
		Identificacao: StoredID{
			NomeEmpresarial: r.Identification.NomeEmpresarial,
			SujeitaFatorR:   r.Identification.SujeitaFatorR,
			InicioAtividade: r.Identification.InicioAtividade,
		},
		SeriesAnteriores: StoredSeries{
			Receitas: nonNil(r.Series.Receitas), Folhas: nonNil(r.Series.Folhas),
			ReceitasConferem: r.Series.ReceitasConferem, FolhasConferem: r.Series.FolhasConferem,
		},
	}
}

func nonNil(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	return maps.Clone(m)
}

// Field devolve o campo gravado pelo nome (nil = não lido).
func (s Stored) Field(name string) *string {
	switch name {
	case "cnpj":
		return s.CNPJ
	case "pa":
		return s.PA
	case "rbt12":
		return s.RBT12
	case "rpa":
		return s.RPA
	case "fs12":
		return s.FS12
	case "fator_r":
		return s.FatorR
	case "das":
		return s.DAS
	case "anexo":
		return s.Anexo
	}
	return nil
}

// MonthValues são os meses anteriores que podem ser gravados, com receita e folha
// (_meses_para_importar): só com a série de receitas conferida contra o RBT12. Folha: a série
// conferida contra a FS12 (comFolha); atividade que o extrato diz não ter fator r entra com
// folha zero; fora disso, nada é gravado e o motivo diz por quê (folha zero inventada derrubaria
// o Fator R).
type MonthValues struct {
	Receita decimal.Decimal
	Folha   decimal.Decimal
}

// Motivos de meses anteriores não lançados (MOTIVOS_ANTERIORES).
const (
	ReceitasNaoConferem = "receitas_anteriores_nao_conferem"
	FolhasNaoConferem   = "folhas_anteriores_nao_conferem"
)

// PriorMonths aplica a regra dos meses anteriores ao PA sobre o JSON gravado.
func (s Stored) PriorMonths() (months map[string]MonthValues, comFolha bool, motivo string, err error) {
	receitas := s.SeriesAnteriores.Receitas
	if len(receitas) == 0 {
		return nil, false, "", nil
	}
	if !s.SeriesAnteriores.ReceitasConferem {
		return nil, false, ReceitasNaoConferem, nil
	}
	parse := func(v string) (decimal.Decimal, error) {
		d, err := decimal.Parse(v)
		if err != nil {
			return decimal.Decimal{}, fmt.Errorf("pgdas: valor da série %q: %w", v, err)
		}
		return d, nil
	}
	out := make(map[string]MonthValues, len(receitas))
	switch {
	case s.SeriesAnteriores.FolhasConferem:
		for m, v := range receitas {
			rec, err := parse(v)
			if err != nil {
				return nil, false, "", err
			}
			fol, err := parse(s.SeriesAnteriores.Folhas[m])
			if err != nil {
				return nil, false, "", err
			}
			out[m] = MonthValues{Receita: rec, Folha: fol}
		}
		return out, true, "", nil
	case s.Identificacao.SujeitaFatorR != nil && !*s.Identificacao.SujeitaFatorR:
		for m, v := range receitas {
			rec, err := parse(v)
			if err != nil {
				return nil, false, "", err
			}
			out[m] = MonthValues{Receita: rec, Folha: decimal.Zero}
		}
		return out, false, "", nil
	}
	return nil, false, FolhasNaoConferem, nil
}
