// Package cnpj sanitiza, valida e formata CNPJ. O armazenamento é sempre com
// 14 dígitos; copiar = só dígitos [RN-014]. A API valida os dígitos verificadores.
package cnpj

import "strings"

// Sanitize remove tudo que não for dígito.
func Sanitize(s string) string {
	var b strings.Builder
	b.Grow(14)
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Valid valida tamanho (após sanitizar) e dígitos verificadores.
func Valid(s string) bool {
	d := Sanitize(s)
	if len(d) != 14 || allSame(d) {
		return false
	}
	return d[12] == checkDigit(d[:12]) && d[13] == checkDigit(d[:13])
}

func allSame(d string) bool {
	for i := 1; i < len(d); i++ {
		if d[i] != d[0] {
			return false
		}
	}
	return true
}

// checkDigit calcula o DV (módulo 11) para 12 ou 13 dígitos.
func checkDigit(base string) byte {
	weights := []int{6, 5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}
	offset := len(weights) - len(base)
	sum := 0
	for i := range len(base) {
		sum += int(base[i]-'0') * weights[offset+i]
	}
	r := sum % 11
	if r < 2 {
		return '0'
	}
	return byte('0' + 11 - r)
}

// Format devolve "00.000.000/0000-00" para 14 dígitos; outros valores voltam inalterados.
func Format(s string) string {
	d := Sanitize(s)
	if len(d) != 14 {
		return s
	}
	return d[0:2] + "." + d[2:5] + "." + d[5:8] + "/" + d[8:12] + "-" + d[12:14]
}

// WithCheckDigits completa uma base de 12 dígitos com os dígitos verificadores
// (usado para gerar CNPJs fictícios válidos em testes e massa de carga).
func WithCheckDigits(base12 string) string {
	d := Sanitize(base12)
	if len(d) != 12 {
		return ""
	}
	first := checkDigit(d)
	return d + string(first) + string(checkDigit(d+string(first)))
}
