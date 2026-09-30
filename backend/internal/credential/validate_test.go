package credential

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/pem"
	"math/big"
	"testing"
	"time"

	"controle/internal/platform/apperr"
)

// Fixtures sintéticas geradas no teste: nenhuma chave é versionada [AGENTS §6.4, RN-018].
type fixtures struct {
	certDER  []byte
	certPEM  []byte
	keyPEM   []byte
	pkcs12   []byte
	pkcs12BE []byte
}

func newFixtures(t *testing.T) fixtures {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "Empresa Fictícia Alfa Comércio Ltda:00000001000101"},
		NotBefore:    time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		NotAfter:     time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return fixtures{
		certDER:  der,
		certPEM:  pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		keyPEM:   pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}),
		pkcs12:   pfxDER(t, keyDER),
		pkcs12BE: pfxBER(keyDER),
	}
}

// pfxDER monta um contêiner com a estrutura de um PFX (PKCS#12, RFC 7292):
// SEQUENCE { version 3, ContentInfo { id-data, [0] OCTET STRING }, MacData }.
func pfxDER(t *testing.T, payload []byte) []byte {
	t.Helper()
	type contentInfo struct {
		ContentType asn1.ObjectIdentifier
		Content     asn1.RawValue `asn1:"explicit,tag:0"`
	}
	type pfx struct {
		Version  int
		AuthSafe contentInfo
		MacData  asn1.RawValue
	}
	octets, err := asn1.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	mac, err := asn1.Marshal(struct{ Salt []byte }{Salt: []byte("salt")})
	if err != nil {
		t.Fatal(err)
	}
	out, err := asn1.Marshal(pfx{
		Version:  3,
		AuthSafe: contentInfo{ContentType: asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 7, 1}, Content: asn1.RawValue{FullBytes: octets}},
		MacData:  asn1.RawValue{FullBytes: mac},
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// pfxBER é o mesmo início em BER com comprimento indefinido (gerado por algumas ferramentas).
func pfxBER(payload []byte) []byte {
	b := []byte{0x30, 0x80, 0x02, 0x01, 0x03, 0x30, 0x80}
	b = append(b, pkcs7OIDPrefix...)
	b = append(b, 0x01)
	b = append(b, payload...)
	return append(b, 0x00, 0x00, 0x00, 0x00)
}

var (
	pdf  = []byte("%PDF-1.4\n1 0 obj << /Type /Catalog >> endobj\ntrailer << /Root 1 0 R >>\n%%EOF\n")
	png  = append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 32)...)
	jpeg = append([]byte{0xff, 0xd8, 0xff, 0xe0}, make([]byte, 32)...)
)

func errCode(err error) string {
	if e, ok := apperr.As(err); ok {
		return e.Code
	}
	return ""
}

// CA-020: .pfx, .p12 (mesmo renomeado) e qualquer chave privada são recusados com a
// mensagem exigida; certificado público e documentos da procuração são aceitos.
func TestCA020_RejectPrivateKey(t *testing.T) {
	f := newFixtures(t)
	certAndKey := append(append([]byte{}, f.certPEM...), f.keyPEM...)
	rsaKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: []byte{1, 2, 3}})
	encKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "ENCRYPTED PRIVATE KEY", Bytes: []byte{1, 2, 3}})

	rejected := []struct {
		name string
		typ  string
		file string
		data []byte
	}{
		{".pfx pela extensão", TypeCertificate, "certificado.pfx", f.pkcs12},
		{".p12 pela extensão", TypeCertificate, "certificado.p12", []byte("qualquer conteúdo")},
		{".p12 renomeado para .cer", TypeCertificate, "certificado.cer", f.pkcs12},
		{"PKCS#12 em BER renomeado para .crt", TypeCertificate, "certificado.crt", f.pkcs12BE},
		{"PEM só com a chave", TypeCertificate, "chave.pem", f.keyPEM},
		{"PEM com certificado + chave", TypeCertificate, "completo.pem", certAndKey},
		{"PEM com chave RSA", TypeCertificate, "rsa.pem", rsaKeyPEM},
		{"PEM com chave cifrada", TypeCertificate, "enc.pem", encKeyPEM},
		{"chave enviada como procuração", TypePowerOfAttorney, "procuracao.pdf", f.keyPEM},
		{".pfx enviado como procuração", TypePowerOfAttorney, "procuracao.pfx", pdf},
	}
	for _, c := range rejected {
		t.Run("recusa "+c.name, func(t *testing.T) {
			_, err := ValidateFile(c.typ, c.file, c.data)
			if errCode(err) != CodePrivateKeyRejected {
				t.Fatalf("err = %v, want %s", err, CodePrivateKeyRejected)
			}
			if e, _ := apperr.As(err); e.Message != "Chaves privadas não são armazenadas pelo sistema." || e.Status != 422 {
				t.Fatalf("erro = %d %q", e.Status, e.Message)
			}
		})
	}

	accepted := []struct {
		name string
		typ  string
		file string
		data []byte
		mime string
	}{
		{".cer válido (DER)", TypeCertificate, "certificado.cer", f.certDER, "application/pkix-cert"},
		{".pem com certificado", TypeCertificate, "certificado.pem", f.certPEM, "application/x-pem-file"},
		{"cadeia PEM", TypeCertificate, "cadeia.crt", append(append([]byte{}, f.certPEM...), f.certPEM...), "application/x-pem-file"},
		{"PDF válido", TypePowerOfAttorney, "procuracao.pdf", pdf, "application/pdf"},
		{"PNG", TypePowerOfAttorney, "procuracao.png", png, "image/png"},
		{"JPEG com extensão trocada", TypePowerOfAttorney, "procuracao.bin", jpeg, "image/jpeg"},
	}
	for _, c := range accepted {
		t.Run("aceita "+c.name, func(t *testing.T) {
			got, err := ValidateFile(c.typ, c.file, c.data)
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if got.MimeType != c.mime {
				t.Fatalf("mime = %s, want %s", got.MimeType, c.mime)
			}
		})
	}
}

func TestValidateFile_UnsupportedFormats(t *testing.T) {
	f := newFixtures(t)
	cases := []struct {
		name string
		typ  string
		data []byte
		code string
	}{
		{"PDF como certificado", TypeCertificate, pdf, CodeUnsupportedFile},
		{"texto como procuração", TypePowerOfAttorney, []byte("procuração em texto"), CodeUnsupportedFile},
		{"certificado DER como procuração", TypePowerOfAttorney, f.certDER, CodeUnsupportedFile},
		{"DER corrompido", TypeCertificate, f.certDER[:len(f.certDER)/2], CodeUnsupportedFile},
		{"PEM com outro bloco", TypeCertificate, pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: []byte{1}}), CodeUnsupportedFile},
		{"PEM com lixo depois", TypeCertificate, append(append([]byte{}, f.certPEM...), []byte("sobrou texto")...), CodeUnsupportedFile},
		{"PEM com certificado inválido", TypeCertificate, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte{1, 2}}), CodeUnsupportedFile},
		{"arquivo vazio", TypePowerOfAttorney, nil, "EMPTY_FILE"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := ValidateFile(c.typ, "arquivo.x", c.data); errCode(err) != c.code {
				t.Fatalf("err = %v, want %s", err, c.code)
			}
		})
	}
}

func TestLooksLikePKCS12(t *testing.T) {
	f := newFixtures(t)
	if !looksLikePKCS12(f.pkcs12) || !looksLikePKCS12(f.pkcs12BE) {
		t.Fatal("deveria reconhecer PKCS#12 em DER e BER")
	}
	// Certificado X.509 também começa com SEQUENCE, mas não com INTEGER 3 + PKCS#7.
	for _, b := range [][]byte{f.certDER, pdf, nil, {0x30}, bytes.Repeat([]byte{0x30, 0x82}, 4)} {
		if looksLikePKCS12(b) {
			t.Fatalf("falso positivo em % x", b[:min(len(b), 8)])
		}
	}
}
