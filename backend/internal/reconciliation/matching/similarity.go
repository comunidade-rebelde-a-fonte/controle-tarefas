package matching

import "controle/internal/reconciliation"

// Jaro é o jaro da origem (app/domain/similaridade.py) sobre os caracteres (code points) dos dois
// textos, com as mesmas operações em ponto flutuante, na mesma ordem.
func Jaro(s1, s2 string) float64 {
	if s1 == s2 {
		return 1.0
	}
	a, b := []rune(s1), []rune(s2)
	if len(a) == 0 || len(b) == 0 {
		return 0.0
	}
	dist := max(max(len(a), len(b))/2-1, 0)
	matchedA, matchedB := make([]bool, len(a)), make([]bool, len(b))
	matches := 0
	for i, ch := range a {
		for j := max(0, i-dist); j < min(i+dist+1, len(b)); j++ {
			if matchedB[j] || b[j] != ch {
				continue
			}
			matchedA[i], matchedB[j] = true, true
			matches++
			break
		}
	}
	if matches == 0 {
		return 0.0
	}
	transpositions, k := 0, 0
	for i, matched := range matchedA {
		if !matched {
			continue
		}
		for !matchedB[k] {
			k++
		}
		if a[i] != b[k] {
			transpositions++
		}
		k++
	}
	m := float64(matches)
	return (m/float64(len(a)) + m/float64(len(b)) + (m-float64(transpositions)/2)/m) / 3.0
}

// JaroWinkler é o jaro_winkler da origem: o Jaro das descrições normalizadas (a normalização da
// chave de dedupe) com o bônus de até 4 caracteres de prefixo comum (p = 0,1).
func JaroWinkler(s1, s2 string) float64 {
	a := reconciliation.NormalizeDescription(s1)
	b := reconciliation.NormalizeDescription(s2)
	j := Jaro(a, b)
	ra, rb := []rune(a), []rune(b)
	prefix := 0
	for i := 0; i < len(ra) && i < len(rb); i++ {
		if ra[i] != rb[i] || prefix == 4 {
			break
		}
		prefix++
	}
	return j + float64(prefix)*0.1*(1-j)
}
