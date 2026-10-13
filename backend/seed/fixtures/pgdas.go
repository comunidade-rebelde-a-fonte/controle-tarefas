package fixtures

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"controle/internal/factorr"
	"controle/internal/factorr/agents"
	"controle/internal/factorr/engine"
	"controle/internal/platform/cnpj"
	comp "controle/internal/platform/competence"
	"controle/internal/platform/decimal"
	"controle/internal/platform/filestore"
	"controle/internal/store"
)

// Extratos do PGDAS-D para a demonstração do inbox do Fator R (TASK-027, D-041), gerados a partir
// da fixture sintética do parser (txt_padrao, a mesma da origem), com o PA contado da competência
// corrente. Os CNPJs fictícios do seed não têm dígito verificador válido (o bypass de
// seed/companies.go), e o inbox só procura na carteira CNPJ válido. Por isso:
//  1. o extrato da empresa da carteira (a Íris, 208, com os números da série dela) cai em revisão e
//     se vincula pela ação Vincular; a nota ouro confere RBT12, FS12, fator r e anexo com o motor;
//  2. o de CNPJ válido fora do cadastro (o da fixture) cai em revisão com a sugestão de cadastro:
//     Cadastrar e vincular cria a empresa;
//  3. o da mesma empresa no PA seguinte, enviado depois do cadastro, se vincula sozinho.

// PgdasFile é um extrato gravado na pasta, com o que ele mostra no inbox.
type PgdasFile struct {
	Name  string
	About string
}

const (
	pgdasTemplate = "txt_padrao/documento.txt"
	// templateCNPJ é o CNPJ fictício (válido) da fixture: fica nos extratos de fora do cadastro.
	templateCNPJ = "11.222.333/0001-81"
	// portfolioCode é a empresa da carteira do primeiro extrato: a Íris Clínica Veterinária,
	// vermelha na carteira (Anexo V), com a série completa nos 12 meses antes do PA.
	portfolioCode = "208"
	readmeName    = "LEIA-ME.txt"
)

// pgdasValues são os campos do extrato que mudam em relação ao modelo (valores no formato
// brasileiro, como o PGDAS-D imprime).
type pgdasValues struct {
	pa                         comp.Competence
	cnpj, name                 string
	rpa, rbt12, rba, fs12, das string
	factorR, annex             string
}

// templateValues são os da fixture (PA 09/2026).
var templateValues = pgdasValues{pa: comp.MustParse("2026-09"), cnpj: templateCNPJ, name: "CLINICA EXEMPLO LTDA",
	rpa: "50.000,00", rbt12: "600.000,00", rba: "400.000,00", fs12: "180.000,00", das: "5.280,00", factorR: "0,30", annex: "III"}

// fill troca no modelo os campos do extrato. Cada trecho tem de aparecer o número de vezes
// esperado: se a fixture mudar, a geração falha em vez de gravar um extrato errado.
func fill(tpl string, v pgdasValues) (string, error) {
	t := templateValues
	pairs := []struct {
		old, new string
		n        int
	}{
		{"(PA): " + t.pa.Display(), "(PA): " + v.pa.Display(), 1},
		{t.cnpj, v.cnpj, 2},
		{"Nome empresarial: " + t.name, "Nome empresarial: " + v.name, 1},
		{"Competência   " + t.rpa + "   0,00   " + t.rpa, "Competência   " + v.rpa + "   0,00   " + v.rpa, 1},
		{"(RBT12)   " + t.rbt12 + "   0,00   " + t.rbt12, "(RBT12)   " + v.rbt12 + "   0,00   " + v.rbt12, 1},
		{"(RBA)   " + t.rba + "   0,00   " + t.rba, "(RBA)   " + v.rba + "   0,00   " + v.rba, 1},
		{"Anteriores (R$)   " + t.fs12, "Anteriores (R$)   " + v.fs12, 1},
		{"Fator r - Anexo " + t.annex, "Fator r - Anexo " + v.annex, 1},
		{"Fator r: " + t.factorR, "Fator r: " + v.factorR, 1},
		{"Declarado (R$): " + t.das, "Declarado (R$): " + v.das, 1},
	}
	for _, p := range pairs {
		if n := strings.Count(tpl, p.old); n != p.n {
			return "", fmt.Errorf("o modelo %s mudou: %q aparece %d vezes (esperado %d)", pgdasTemplate, p.old, n, p.n)
		}
		tpl = strings.ReplaceAll(tpl, p.old, p.new)
	}
	return tpl, nil
}

func brl(d decimal.Decimal) string {
	s := d.Text()
	return agents.FormatBRL(&s)
}

// portfolioValues monta o extrato da empresa da carteira no PA: a receita do mês e a do ano até o
// PA vêm da série lançada; RBT12, FS12, fator r e anexo, do motor (a mesma referência da nota
// ouro); o DAS é a receita do PA pela alíquota efetiva do anexo. Sem a empresa, sem o mês ou sem
// referência do motor, devolve o motivo e nenhum extrato.
func portfolioValues(ctx context.Context, pool *pgxpool.Pool, pa comp.Competence) (*pgdasValues, string, error) {
	var id, digits, legal string
	err := pool.QueryRow(ctx, `SELECT id, cnpj, legal_name FROM companies WHERE internal_code = $1`, portfolioCode).Scan(&id, &digits, &legal)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, "a empresa " + portfolioCode + " não está no banco", nil
	}
	if err != nil {
		return nil, "", err
	}
	q := store.New(pool)
	first := comp.New(pa.Year, 1)
	months, err := q.ListFactorRMovements(ctx, store.ListFactorRMovementsParams{CompanyID: id, FromCompetence: first.FirstDay(), ToCompetence: pa.FirstDay()})
	if err != nil {
		return nil, "", err
	}
	var rpa *decimal.Decimal
	rba := decimal.Zero
	for _, m := range months {
		if comp.FromDate(m.Competence) == pa {
			v := m.ReceitaBruta
			rpa = &v
			continue
		}
		rba = rba.Add(m.ReceitaBruta)
	}
	if rpa == nil {
		return nil, fmt.Sprintf("a %s não tem movimento em %s", portfolioCode, pa.Display()), nil
	}
	ref, err := factorr.GoldReference(ctx, q, id, pa)
	if err != nil {
		return nil, "", err
	}
	if ref == nil || ref.FactorR == nil || ref.Annex == nil {
		return nil, fmt.Sprintf("o motor não tem referência para a %s em %s", portfolioCode, pa.Display()), nil
	}
	rate := ref.EffectiveRateIII
	if *ref.Annex == engine.AnnexV {
		rate = ref.EffectiveRateV
	}
	if rate == nil {
		return nil, fmt.Sprintf("sem alíquota efetiva para a %s em %s", portfolioCode, pa.Display()), nil
	}
	return &pgdasValues{
		pa: pa, cnpj: cnpj.Format(digits), name: strings.ToUpper(legal),
		rpa: brl(*rpa), rbt12: brl(ref.RBT12), rba: brl(rba), fs12: brl(ref.FS12), das: brl(rpa.Mul(*rate).Quantize(-2)),
		factorR: strings.Replace(ref.FactorR.Quantize(-4).Text(), ".", ",", 1), annex: string(*ref.Annex),
	}, "", nil
}

// writePgdas grava os extratos e o LEIA-ME na pasta de saída (sobrescreve os da mesma execução;
// nunca apaga nada).
func writePgdas(ctx context.Context, d Deps, o Options, out io.Writer) ([]PgdasFile, error) {
	templates, err := os.OpenRoot(o.PgdasTemplates)
	if err != nil {
		return nil, fmt.Errorf("fixtures do parser do PGDAS-D: %w", err)
	}
	defer func() { _ = templates.Close() }()
	raw, err := templates.ReadFile(pgdasTemplate)
	if err != nil {
		return nil, err
	}
	tpl := string(raw)
	current := comp.Current(d.Clock)

	type file struct {
		PgdasFile
		values pgdasValues
	}
	var files []file
	portfolioPA := current.AddMonths(-1)
	v, why, err := portfolioValues(ctx, d.Pool, portfolioPA)
	if err != nil {
		return nil, err
	}
	if v != nil {
		files = append(files, file{PgdasFile{
			Name: fmt.Sprintf("1_iris_veterinaria_pa_%s.txt", portfolioPA),
			About: "Empresa da carteira (208 Íris Clínica Veterinária), PA " + portfolioPA.Display() + ", com os números da série dela. " +
				"Os CNPJs fictícios do seed não têm dígito verificador válido, então o extrato cai em revisão (CNPJ não encontrado); " +
				"vincule pela ação Vincular, escolhendo a 208. A nota ouro confere RBT12, FS12, fator r e anexo com o motor, e o " +
				"movimento do mês, já lançado, não é sobrescrito.",
		}, *v})
	} else {
		_, _ = fmt.Fprintf(out, "fixtures: extrato da empresa da carteira não gerado: %s\n", why)
	}
	outside := templateValues
	outside.pa = current.AddMonths(-2)
	files = append(files, file{PgdasFile{
		Name: fmt.Sprintf("2_cnpj_fora_do_cadastro_pa_%s.txt", outside.pa),
		About: "CNPJ válido fora do cadastro (" + templateCNPJ + ", CLINICA EXEMPLO LTDA), PA " + outside.pa.Display() + ". " +
			"Cai em revisão com a sugestão de cadastro: Cadastrar e vincular abre o cadastro com os dados do extrato, e a empresa " +
			"nova (Simples Nacional, sujeita ao Fator R) entra na carteira com o movimento do PA.",
	}, outside})
	next := templateValues
	next.pa = current.AddMonths(-1)
	files = append(files, file{PgdasFile{
		Name: fmt.Sprintf("3_cnpj_cadastrado_pa_%s.txt", next.pa),
		About: "A mesma empresa no PA seguinte (" + next.pa.Display() + "). Envie depois do arquivo 2 e do cadastro: " +
			"o extrato se vincula sozinho pelo CNPJ.",
	}, next})

	readme := []string{
		"Extratos do PGDAS-D da demonstração (make demo-fixtures, TASK-027).",
		"Envie pela tela Fator R > Inbox PGDAS-D, na ordem dos números. Dados fictícios.",
		"",
	}
	written := make([]PgdasFile, 0, len(files))
	for _, f := range files {
		text, err := fill(tpl, f.values)
		if err != nil {
			return nil, err
		}
		if err := filestore.WriteAtomic(o.OutDir, f.Name, []byte(text)); err != nil {
			return nil, err
		}
		readme = append(readme, f.Name, "  "+f.About, "")
		written = append(written, f.PgdasFile)
	}
	if err := filestore.WriteAtomic(o.OutDir, readmeName, []byte(strings.Join(readme, "\n")+"\n")); err != nil {
		return nil, err
	}
	return written, nil
}
