package objectstore

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"sort"
	"strings"
	"time"
)

// Assinatura AWS Signature Version 4 para o S3 (cabeçalho Authorization, payload num pedaço só).
// Conferida contra os exemplos da documentação da AWS (sigv4_test.go) e contra o MinIO real.

const (
	algorithm = "AWS4-HMAC-SHA256"
	service   = "s3"
	amzFormat = "20060102T150405Z"
	dayFormat = "20060102"
)

// EmptySHA256 é o hash do corpo vazio (GET, HEAD).
const EmptySHA256 = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

// signingRequest é o que entra na assinatura: método, caminho já codificado, query e cabeçalhos
// assinados (nomes em minúsculas).
type signingRequest struct {
	method      string
	path        string
	query       url.Values
	headers     map[string]string
	payloadHash string
}

// encodeURIPath codifica o caminho como a AWS: tudo exceto os não reservados (A-Z a-z 0-9 - _ . ~),
// mantendo a barra entre segmentos.
func encodeURIPath(p string) string {
	var b strings.Builder
	for i := 0; i < len(p); i++ {
		c := p[i]
		if c == '/' || unreserved(c) {
			b.WriteByte(c)
			continue
		}
		b.WriteString("%")
		b.WriteString(strings.ToUpper(hex.EncodeToString([]byte{c})))
	}
	return b.String()
}

func unreserved(c byte) bool {
	return 'A' <= c && c <= 'Z' || 'a' <= c && c <= 'z' || '0' <= c && c <= '9' || c == '-' || c == '_' || c == '.' || c == '~'
}

// encodeQueryComponent codifica nome e valor da query como a AWS (espaço vira %20).
func encodeQueryComponent(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if unreserved(c) {
			b.WriteByte(c)
			continue
		}
		b.WriteString("%")
		b.WriteString(strings.ToUpper(hex.EncodeToString([]byte{c})))
	}
	return b.String()
}

func canonicalQuery(q url.Values) string {
	keys := make([]string, 0, len(q))
	for k := range q {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		vals := append([]string(nil), q[k]...)
		sort.Strings(vals)
		for _, v := range vals {
			parts = append(parts, encodeQueryComponent(k)+"="+encodeQueryComponent(v))
		}
	}
	return strings.Join(parts, "&")
}

// canonicalRequest monta a requisição canônica e devolve também a lista de cabeçalhos assinados.
func canonicalRequest(r signingRequest) (string, string) {
	names := make([]string, 0, len(r.headers))
	for k := range r.headers {
		names = append(names, k)
	}
	sort.Strings(names)
	var hdr strings.Builder
	for _, k := range names {
		hdr.WriteString(k)
		hdr.WriteString(":")
		hdr.WriteString(strings.Join(strings.Fields(r.headers[k]), " "))
		hdr.WriteString("\n")
	}
	signed := strings.Join(names, ";")
	return strings.Join([]string{r.method, r.path, canonicalQuery(r.query), hdr.String(), signed, r.payloadHash}, "\n"), signed
}

func hmacSHA256(key []byte, data string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(data))
	return h.Sum(nil)
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// authorization devolve o valor do cabeçalho Authorization da requisição no instante t.
func authorization(r signingRequest, accessKey, secretKey, region string, t time.Time) string {
	t = t.UTC()
	scope := t.Format(dayFormat) + "/" + region + "/" + service + "/aws4_request"
	creq, signed := canonicalRequest(r)
	toSign := strings.Join([]string{algorithm, t.Format(amzFormat), scope, sha256Hex(creq)}, "\n")
	key := hmacSHA256([]byte("AWS4"+secretKey), t.Format(dayFormat))
	key = hmacSHA256(key, region)
	key = hmacSHA256(key, service)
	key = hmacSHA256(key, "aws4_request")
	sig := hex.EncodeToString(hmacSHA256(key, toSign))
	return algorithm + " Credential=" + accessKey + "/" + scope + ", SignedHeaders=" + signed + ", Signature=" + sig
}
