// Package matching é o matching determinístico da Conciliação [REQ-024, REQ-030; TASK-018,
// D-029, ADR-008]: porte de app/domain/{conciliacao,similaridade,bordas}.py e do pré-filtro
// _ids_divergentes de app/servicos/matching.py do Conciliador (commit 2bc6e26), travados por golden
// gerado pelo próprio Python da origem (testdata/matching_gen.py).
//
// A cascata tem os quatro passes da origem: 1, mesma data, valor e sentido; 2, valor e sentido com
// a data em D±3; 3, valor e sentido com a contraparte parecida com o descritivo (Jaro-Winkler ≥
// limiar); 4, de 2 a 8 lançamentos em D±3 cuja soma é a linha (N:1). Ambiguidade vira divergência,
// nunca chute [AC-018]. Por decisão do usuário (ADR-008), a ambiguidade conta dos dois lados: a
// linha precisa de um único comprovante candidato, e o comprovante, de uma única linha.
package matching

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"controle/internal/platform/decimal"
)

// Statement é a linha livre do extrato (ItemExtrato da origem).
type Statement struct {
	ID          string
	Date        time.Time
	Amount      decimal.Decimal
	Direction   string
	Description string
}

// Entry é o lançamento livre e completo do cliente (ItemLancamento da origem).
type Entry struct {
	ID           string
	Date         time.Time
	Amount       decimal.Decimal
	Direction    string
	Counterparty string
}

// Match é uma conciliação automática: a linha, os lançamentos (mais de um no passe 4), o passe, o
// score e o método.
type Match struct {
	StatementID string
	EntryIDs    []string
	Pass        int
	Score       decimal.Decimal
	Method      string
}

// Divergence é a linha que o passe não resolveu por ambiguidade.
type Divergence struct {
	StatementID string
	// Candidates são os lançamentos candidatos, em ordem (no passe 3, do maior score; no passe 4,
	// a primeira solução).
	Candidates []string
	// Reason é ambiguo_passe_N (ou passe_4_extenso, quando o passe 4 passa do orçamento).
	Reason string
	// Rivals são as outras linhas que disputam os mesmos comprovantes (regra dos dois lados).
	Rivals []string
}

// Result é o resultado da cascata, nos baldes da origem.
type Result struct {
	Matches     []Match
	Divergences []Divergence
	// StatementsWithoutReceipt são as linhas que ficaram livres (extrato sem comprovante).
	StatementsWithoutReceipt []string
	// ReceiptsWithoutStatement são os lançamentos que ficaram livres (comprovante sem extrato).
	ReceiptsWithoutStatement []string
}

// Options são os parâmetros da cascata.
type Options struct {
	// FuzzyThreshold é o limiar do passe 3 (LIMIAR_FUZZY, 0,86 na origem).
	FuzzyThreshold float64
	// WindowDays é a tolerância de data dos passes 2 e 4 (3 dias).
	WindowDays int
	// MaxN é o tamanho máximo do subconjunto do passe 4 (8).
	MaxN int
	// OneSided reproduz a origem: a ordem do extrato decide quem leva o comprovante disputado.
	// Só o golden usa; o padrão é a regra dos dois lados (ADR-008).
	OneSided bool
	// Pass4Budget limita as combinações que o passe 4 examina por linha (0 = sem limite, como a
	// origem). Passou do limite, a linha vira divergência em vez de prender o job.
	Pass4Budget int
}

// Pass4Budget é o orçamento padrão do passe 4: combinações examinadas por linha.
const Pass4Budget = 2_000_000

// DefaultOptions são os parâmetros da origem com a regra dos dois lados.
func DefaultOptions() Options {
	return Options{FuzzyThreshold: 0.86, WindowDays: 3, MaxN: 8, Pass4Budget: Pass4Budget}
}

// Métodos e motivos.
const (
	ReasonSubsetBudget = "passe_4_extenso"
)

func method(pass int) string { return "passe_" + strconv.Itoa(pass) }

func ambiguous(pass int) string { return "ambiguo_passe_" + strconv.Itoa(pass) }

var (
	score1 = decimal.MustParse("1.0000")
	score2 = decimal.MustParse("0.9000")
	score4 = decimal.MustParse("0.8000")
)

// pass3Score é o Decimal(str(round(score, 4))) da origem.
func pass3Score(score float64) decimal.Decimal {
	rounded, _ := strconv.ParseFloat(strconv.FormatFloat(score, 'f', 4, 64), 64)
	d, _ := decimal.FromPythonFloat(rounded)
	return d
}

func days(a, b time.Time) int {
	d := int(a.Sub(b).Hours() / 24)
	if d < 0 {
		return -d
	}
	return d
}

func sameDay(a, b time.Time) bool {
	return a.Year() == b.Year() && a.YearDay() == b.YearDay()
}

// cascade é o estado de uma execução.
type cascade struct {
	o       Options
	stmts   []Statement
	entries []Entry
	usedS   map[string]bool
	usedE   map[string]bool
	result  Result
}

// Cascade executa os quatro passes sobre as linhas e os lançamentos livres, na ordem dada.
func Cascade(stmts []Statement, entries []Entry, o Options) Result {
	c := &cascade{o: o, stmts: stmts, entries: entries, usedS: map[string]bool{}, usedE: map[string]bool{}}
	c.result = Result{Matches: []Match{}, Divergences: []Divergence{}}
	exact := func(s Statement, e Entry) (bool, float64) {
		return sameDay(e.Date, s.Date) && e.Amount.Cmp(s.Amount) == 0 && e.Direction == s.Direction, 0
	}
	window := func(s Statement, e Entry) (bool, float64) {
		return e.Amount.Cmp(s.Amount) == 0 && e.Direction == s.Direction && days(e.Date, s.Date) <= o.WindowDays, 0
	}
	fuzzy := func(s Statement, e Entry) (bool, float64) {
		if e.Amount.Cmp(s.Amount) != 0 || e.Direction != s.Direction {
			return false, 0
		}
		score := JaroWinkler(e.Counterparty, s.Description)
		return score >= o.FuzzyThreshold, score
	}
	if o.OneSided {
		c.oneSided(1, exact, false)
		c.oneSided(2, window, false)
		c.oneSided(3, fuzzy, true)
		c.oneSidedPass4()
	} else {
		c.twoSided(1, exact, false)
		c.twoSided(2, window, false)
		c.twoSided(3, fuzzy, true)
		c.twoSidedPass4()
	}
	for _, s := range stmts {
		if !c.usedS[s.ID] {
			c.result.StatementsWithoutReceipt = append(c.result.StatementsWithoutReceipt, s.ID)
		}
	}
	for _, e := range entries {
		if !c.usedE[e.ID] {
			c.result.ReceiptsWithoutStatement = append(c.result.ReceiptsWithoutStatement, e.ID)
		}
	}
	return c.result
}

func (c *cascade) register(m Match) {
	c.usedS[m.StatementID] = true
	for _, id := range m.EntryIDs {
		c.usedE[id] = true
	}
	c.result.Matches = append(c.result.Matches, m)
}

func (c *cascade) freeStatements() []Statement {
	out := make([]Statement, 0, len(c.stmts))
	for _, s := range c.stmts {
		if !c.usedS[s.ID] {
			out = append(out, s)
		}
	}
	return out
}

func (c *cascade) freeEntries() []Entry {
	out := make([]Entry, 0, len(c.entries))
	for _, e := range c.entries {
		if !c.usedE[e.ID] {
			out = append(out, e)
		}
	}
	return out
}

type scored struct {
	entry Entry
	score float64
}

// byScore ordena do maior score para o menor, estável (o sort(reverse=True) do Python).
func byScore(list []scored) {
	sort.SliceStable(list, func(i, j int) bool { return list[i].score > list[j].score })
}

func ids(list []scored) []string {
	out := make([]string, len(list))
	for i, x := range list {
		out[i] = x.entry.ID
	}
	return out
}

func (c *cascade) match(s Statement, pass int, x scored) Match {
	m := Match{StatementID: s.ID, EntryIDs: []string{x.entry.ID}, Pass: pass, Method: method(pass)}
	switch pass {
	case 1:
		m.Score = score1
	case 2:
		m.Score = score2
	default:
		m.Score = pass3Score(x.score)
	}
	return m
}

// oneSided é o passe 1, 2 ou 3 da origem: cada linha, na ordem, com os lançamentos ainda livres;
// um candidato concilia, mais de um é divergência.
func (c *cascade) oneSided(pass int, crit func(Statement, Entry) (bool, float64), sortByScore bool) {
	stmts := c.freeStatements()
	entries := c.freeEntries()
	for _, s := range stmts {
		if c.usedS[s.ID] {
			continue
		}
		var cands []scored
		for _, e := range entries {
			if c.usedE[e.ID] {
				continue
			}
			if ok, score := crit(s, e); ok {
				cands = append(cands, scored{e, score})
			}
		}
		if sortByScore {
			byScore(cands)
		}
		switch {
		case len(cands) == 1:
			c.register(c.match(s, pass, cands[0]))
		case len(cands) > 1:
			c.result.Divergences = append(c.result.Divergences, Divergence{StatementID: s.ID, Candidates: ids(cands), Reason: ambiguous(pass)})
		}
	}
}

// twoSided é o passe com a regra dos dois lados (ADR-008): a linha concilia só com um único
// candidato que, por sua vez, só tem essa linha como candidata. Os candidatos são calculados no
// início do passe; a ordem das linhas não decide nada.
func (c *cascade) twoSided(pass int, crit func(Statement, Entry) (bool, float64), sortByScore bool) {
	stmts := c.freeStatements()
	entries := c.freeEntries()
	cands := make(map[string][]scored, len(stmts))
	back := map[string][]string{}
	for _, s := range stmts {
		for _, e := range entries {
			if ok, score := crit(s, e); ok {
				cands[s.ID] = append(cands[s.ID], scored{e, score})
				back[e.ID] = append(back[e.ID], s.ID)
			}
		}
	}
	for _, s := range stmts {
		list := cands[s.ID]
		if sortByScore {
			byScore(list)
		}
		switch {
		case len(list) == 0:
		case len(list) == 1 && len(back[list[0].entry.ID]) == 1:
			c.register(c.match(s, pass, list[0]))
		default:
			d := Divergence{StatementID: s.ID, Candidates: ids(list), Reason: ambiguous(pass)}
			if len(list) == 1 {
				d.Rivals = without(back[list[0].entry.ID], s.ID)
			}
			c.result.Divergences = append(c.result.Divergences, d)
		}
	}
}

func without(list []string, id string) []string {
	out := []string{}
	for _, x := range list {
		if x != id {
			out = append(out, x)
		}
	}
	return out
}

// --- Passe 4 (N:1) ---

// cents converte o valor de duas casas em centavos inteiros (a soma do passe 4 é exata).
func cents(d decimal.Decimal) (int64, bool) {
	q, ok := d.TryQuantize(-2)
	if !ok || q.Cmp(d) != 0 {
		return 0, false
	}
	n, err := strconv.ParseInt(strings.Replace(q.Text(), ".", "", 1), 10, 64)
	return n, err == nil
}

// subsetSums enumera os subconjuntos de 2 a maxN posições de values cuja soma é target, na ordem
// do itertools.combinations da origem (por tamanho; dentro dele, lexicográfica). Os valores são
// positivos (o banco exige), então o ramo que passa do alvo é cortado sem mudar o resultado. Para
// em limit soluções (0 = todas); passando do orçamento de combinações, devolve truncated.
func subsetSums(values []int64, target int64, maxN, limit, budget int) (sols [][]int, truncated bool) {
	visited := 0
	stop := false
	for n := 2; n <= min(maxN, len(values)) && !stop; n++ {
		idx := make([]int, 0, n)
		var walk func(start int, sum int64)
		walk = func(start int, sum int64) {
			if len(idx) == n {
				if sum == target {
					sols = append(sols, append([]int(nil), idx...))
					if limit > 0 && len(sols) >= limit {
						stop = true
					}
				}
				return
			}
			for i := start; i <= len(values)-(n-len(idx)) && !stop; i++ {
				if visited++; budget > 0 && visited > budget {
					truncated, stop = true, true
					return
				}
				next := sum + values[i]
				if next > target {
					continue
				}
				idx = append(idx, i)
				walk(i+1, next)
				idx = idx[:len(idx)-1]
			}
		}
		walk(0, 0)
	}
	return sols, truncated
}

// pool são os lançamentos que podem compor a linha no passe 4: mesmo sentido, data em D±janela e
// valor que cabe na linha.
func (c *cascade) pool(s Statement, entries []Entry) ([]Entry, []int64, int64, bool) {
	target, ok := cents(s.Amount)
	if !ok {
		return nil, nil, 0, false
	}
	var pool []Entry
	var values []int64
	for _, e := range entries {
		if c.o.OneSided && c.usedE[e.ID] {
			continue
		}
		if e.Direction != s.Direction || days(e.Date, s.Date) > c.o.WindowDays {
			continue
		}
		v, ok := cents(e.Amount)
		if !ok {
			return nil, nil, 0, false
		}
		if v > target || v <= 0 {
			continue
		}
		pool = append(pool, e)
		values = append(values, v)
	}
	return pool, values, target, true
}

func entryIDs(pool []Entry, sol []int) []string {
	out := make([]string, len(sol))
	for i, k := range sol {
		out[i] = pool[k].ID
	}
	return out
}

// oneSidedPass4 é o passe 4 da origem: cada linha, na ordem, com os lançamentos ainda livres;
// solução única concilia, mais de uma é divergência (com a primeira solução).
func (c *cascade) oneSidedPass4() {
	stmts := c.freeStatements()
	entries := c.freeEntries()
	for _, s := range stmts {
		pool, values, target, ok := c.pool(s, entries)
		if !ok {
			continue
		}
		sols, truncated := subsetSums(values, target, c.o.MaxN, 2, c.o.Pass4Budget)
		switch {
		case truncated:
			c.result.Divergences = append(c.result.Divergences, Divergence{StatementID: s.ID, Candidates: []string{}, Reason: ReasonSubsetBudget})
		case len(sols) == 1:
			c.register(Match{StatementID: s.ID, EntryIDs: entryIDs(pool, sols[0]), Pass: 4, Score: score4, Method: method(4)})
		case len(sols) > 1:
			c.result.Divergences = append(c.result.Divergences, Divergence{StatementID: s.ID, Candidates: entryIDs(pool, sols[0]), Reason: ambiguous(4)})
		}
	}
}

// twoSidedPass4 é o passe 4 com a regra dos dois lados: a linha concilia só com solução única e
// se nenhum lançamento dela entra em solução de outra linha. As soluções são calculadas no início
// do passe; a linha que passa do orçamento vira divergência e torna disputados os lançamentos do
// pool dela.
func (c *cascade) twoSidedPass4() {
	stmts := c.freeStatements()
	entries := c.freeEntries()
	type plan struct {
		pool      []Entry
		sols      [][]int
		truncated bool
	}
	plans := make([]plan, len(stmts))
	users := map[string]map[string]bool{}
	use := func(entryID, stmtID string) {
		if users[entryID] == nil {
			users[entryID] = map[string]bool{}
		}
		users[entryID][stmtID] = true
	}
	for i, s := range stmts {
		pool, values, target, ok := c.pool(s, entries)
		if !ok {
			continue
		}
		sols, truncated := subsetSums(values, target, c.o.MaxN, 0, c.o.Pass4Budget)
		plans[i] = plan{pool: pool, sols: sols, truncated: truncated}
		switch {
		case truncated:
			for _, e := range pool {
				use(e.ID, s.ID)
			}
		default:
			for _, sol := range sols {
				for _, k := range sol {
					use(pool[k].ID, s.ID)
				}
			}
		}
	}
	for i, s := range stmts {
		p := plans[i]
		switch {
		case p.truncated:
			c.result.Divergences = append(c.result.Divergences, Divergence{StatementID: s.ID, Candidates: []string{}, Reason: ReasonSubsetBudget})
		case len(p.sols) == 0:
		default:
			first := entryIDs(p.pool, p.sols[0])
			var rivals []string
			seen := map[string]bool{s.ID: true}
			for _, id := range first {
				for other := range users[id] {
					if !seen[other] {
						seen[other] = true
						rivals = append(rivals, other)
					}
				}
			}
			if len(p.sols) == 1 && len(rivals) == 0 {
				c.register(Match{StatementID: s.ID, EntryIDs: first, Pass: 4, Score: score4, Method: method(4)})
				continue
			}
			sort.Strings(rivals)
			c.result.Divergences = append(c.result.Divergences, Divergence{StatementID: s.ID, Candidates: first, Reason: ambiguous(4), Rivals: rivals})
		}
	}
}
