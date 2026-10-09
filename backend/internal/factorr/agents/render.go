package agents

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"controle/internal/factorr"
	"controle/internal/platform/decimal"
	"controle/internal/platform/pytext"
)

// Validação do texto do LLM contra a decisão estruturada: porte de agents/render.py da origem
// (CLAUDE §3.12): todo número e toda data do texto têm de estar na decisão; número por extenso e
// recomendação que contradiz o veredito também reprovam. Texto reprovado vira o template.

var (
	reData = pytext.MustCompile(`\b(\d{4})-(\d{2})\b|\b(\d{2})/(\d{4})\b`, false)
	// Sinal capturado e sem exigir borda de palavra: "x45000" e "R$-99.000,00" também são checados.
	reNumero = pytext.MustCompile(`-?(?<!\d)\d{1,3}(?:\.\d{3})+(?:,\d+)?|-?(?<!\d)\d+(?:[.,]\d+)?`, false)
	// _RE_EXTENSO = \b(…)\b: com \b dos dois lados, a correspondência é sempre uma palavra inteira;
	// aqui cada palavra é conferida por inteiro contra as alternativas (mesmo resultado).
	rePalavra = pytext.MustCompile(`\w+`, false)
	reExtenso = pytext.MustCompile(`^(mil|milh[aã]o|milh[oõ]es|bilh[aã]o|bilh[oõ]es|onze|doze|treze|quatorze|catorze|quinze|`+
		`dezesseis|dezessete|dezoito|dezenove|vinte|trinta|quarenta|cinquenta|sessenta|setenta|`+
		`oitenta|noventa|cem|cento|duzent\w*|trezent\w*|quatrocent\w*|quinhent\w*|seiscent\w*|`+
		`setecent\w*|oitocent\w*|novecent\w*)$`, true)
	reRecomendaCorrigir    = pytext.MustCompile(`\b(vale|recomend\w*|deve|convém|sugiro)\s+(a\s+)?corrig`, true)
	reRecomendaNaoCorrigir = pytext.MustCompile(`n[aã]o\s+(for[cç]ar|vale|compensa|recomend)`, true)
	reCompetencia          = pytext.MustCompile(`^\d{4}-\d{2}$`, false)
	reMilhares             = pytext.MustCompile(`^\d{1,3}(?:\.\d{3})+$`, false)
)

var (
	hundred = decimal.FromInt(100)
	one     = decimal.FromInt(1)
)

// orphanVerdict é o órfão de um texto que contradiz o veredito.
const orphanVerdict = "veredito_contraditorio"

// numeroBR lê um número do texto (_decimal da origem): com vírgula, a vírgula é o decimal; só
// com pontos em grupos de milhar, os pontos somem.
func numeroBR(token string) (decimal.Decimal, bool) {
	bruto := strings.TrimFunc(token, pytext.IsSpace)
	if strings.Contains(bruto, ",") {
		bruto = strings.ReplaceAll(strings.ReplaceAll(bruto, ".", ""), ",", ".")
	} else if reMilhares.MatchString(bruto) {
		bruto = strings.ReplaceAll(bruto, ".", "")
	}
	d, special, ok := decimal.ParsePython(bruto)
	return d, ok && !special
}

// leaves devolve as folhas da decisão (os valores, sem as chaves), como _folhas da origem.
func leaves(v any) []any {
	switch x := v.(type) {
	case map[string]any:
		var out []any
		for _, k := range sortedKeys(x) {
			out = append(out, leaves(x[k])...)
		}
		return out
	case []any:
		var out []any
		for _, e := range x {
			out = append(out, leaves(e)...)
		}
		return out
	}
	return []any{v}
}

// allowed são os números aceitos no texto (com as formas de exibição permitidas) e as
// competências (AAAA-MM) da decisão (valores_permitidos).
type allowed struct {
	numbers      map[string]bool
	competencias map[string]bool
}

func allowedValues(decision map[string]any) allowed {
	a := allowed{numbers: map[string]bool{}, competencias: map[string]bool{}}
	for _, leaf := range leaves(plain(decision)) {
		var text string
		switch x := leaf.(type) {
		case nil, bool:
			continue
		case string:
			if reCompetencia.MatchString(x) {
				a.competencias[x] = true
				continue
			}
			text = x
		case json.Number:
			text = x.String()
		default:
			text = fmt.Sprint(x)
		}
		valor, special, ok := decimal.ParsePython(text)
		if !ok || special {
			// Infinity ou NaN: o NaN não muda nada; com Infinity a origem levantava exceção no
			// quantize, e aqui o valor só não entra (D-024).
			continue
		}
		a.numbers[valor.Key()] = true
		if q, ok := valor.TryQuantize(-2); ok {
			a.numbers[q.Key()] = true
		}
		if valor.Abs().Cmp(one) <= 0 { // frações exibidas como percentual
			pct := valor.Mul(hundred)
			a.numbers[pct.Quantize(-2).Key()] = true
			if whole := pct.Quantize(0); pct.Cmp(whole) == 0 {
				a.numbers[whole.Key()] = true
			} else {
				a.numbers[pct.Quantize(-2).Key()] = true
			}
		}
	}
	return a
}

// ValidateText confere o texto do LLM contra a decisão (validar_texto). Devolve se é válido e
// os órfãos: números, datas e palavras que não vêm da decisão, ou a contradição do veredito.
func ValidateText(text string, decision map[string]any) (bool, []string) {
	a := allowedValues(decision)
	var orphans []string
	sem := strings.ReplaceAll(text, factorr.Disclaimer, "")
	for _, m := range rePalavra.FindAll(sem) {
		if w, _ := m.Group(0); reExtenso.MatchString(w) {
			orphans = append(orphans, w)
		}
	}
	pm, _ := plain(decision).(map[string]any)
	verdict, _ := pm["veredito"].(string)
	if (verdict == "nao_forcar" || verdict == "ja_na_meta") && reRecomendaCorrigir.MatchString(sem) {
		orphans = append(orphans, orphanVerdict)
	}
	if verdict == "corrigir" && reRecomendaNaoCorrigir.MatchString(sem) {
		orphans = append(orphans, orphanVerdict)
	}
	for _, m := range reData.FindAll(sem) {
		comp := ""
		if g1, ok := m.Group(1); ok && g1 != "" {
			g2, _ := m.Group(2)
			comp = g1 + "-" + g2
		} else {
			g4, _ := m.Group(4)
			g3, _ := m.Group(3)
			comp = g4 + "-" + g3
		}
		if !a.competencias[comp] {
			g0, _ := m.Group(0)
			orphans = append(orphans, g0)
		}
	}
	rest := reData.ReplaceAllString(sem, " ")
	for _, m := range reNumero.FindAll(rest) {
		token, _ := m.Group(0)
		if v, ok := numeroBR(token); !ok || !a.numbers[v.Key()] {
			orphans = append(orphans, token)
		}
	}
	return len(orphans) == 0, orphans
}

// FormatBRL formata reais (formatar_brl): 600000.00 → 600.000,00; nulo → "-".
func FormatBRL(v *string) string {
	if v == nil {
		return "-"
	}
	d, _, ok := decimal.ParsePython(*v)
	if !ok {
		return "-"
	}
	q := d.Quantize(-2)
	text := q.Abs().Text()
	intPart, frac, _ := strings.Cut(text, ".")
	var groups []string
	for len(intPart) > 3 {
		groups = append([]string{intPart[len(intPart)-3:]}, groups...)
		intPart = intPart[:len(intPart)-3]
	}
	groups = append([]string{intPart}, groups...)
	sign := ""
	if q.Sign() < 0 {
		sign = "-"
	}
	return sign + strings.Join(groups, ".") + "," + frac
}

// FormatPercentBR formata a fração como percentual (formatar_percentual_br): 0.240000 → 24,00%.
func FormatPercentBR(v *string) string {
	if v == nil {
		return "-"
	}
	d, _, ok := decimal.ParsePython(*v)
	if !ok {
		return "-"
	}
	return strings.ReplaceAll(d.Mul(hundred).Quantize(-2).Text(), ".", ",") + "%"
}

// plain normaliza a decisão para os tipos do JSON (mapa, lista, texto, número, booleano, nulo),
// como a origem a enxerga ao validar e ao mandar ao LLM.
func plain(v any) any {
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.UseNumber()
	var out any
	if err := dec.Decode(&out); err != nil {
		return nil
	}
	return out
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
