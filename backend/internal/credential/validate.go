package credential

import (
	"bytes"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"path/filepath"
	"strings"

	"controle/internal/platform/apperr"
)

// MaxFileSize é o limite de upload de documentos [T-M4-08].
const MaxFileSize = 10 << 20

// PrivateKeyMessage é a mensagem de recusa exigida pelo CA-020.
const PrivateKeyMessage = "Chaves privadas não são armazenadas pelo sistema."

// Códigos de recusa de arquivo.
const (
	CodePrivateKeyRejected = "PRIVATE_KEY_REJECTED"
	CodeUnsupportedFile    = "UNSUPPORTED_FILE"
)

// Accepted descreve o arquivo aceito, com o tipo detectado pelo conteúdo.
type Accepted struct {
	MimeType string
}

func rejectPrivateKey() error {
	return apperr.Unprocessable(CodePrivateKeyRejected, PrivateKeyMessage)
}

// pkcs7OIDPrefix é o prefixo DER do OID 1.2.840.113549.1.7 (tipos de conteúdo PKCS#7),
// usado no authSafe de todo contêiner PKCS#12.
var pkcs7OIDPrefix = []byte{0x06, 0x09, 0x2a, 0x86, 0x48, 0x86, 0xf7, 0x0d, 0x01, 0x07}

// looksLikePKCS12 reconhece a assinatura de um PFX (PKCS#12) em DER ou BER:
// SEQUENCE { INTEGER 3, ContentInfo { OID pkcs7-*, ... }, ... }.
func looksLikePKCS12(b []byte) bool {
	if len(b) < 8 || b[0] != 0x30 {
		return false
	}
	var i int
	switch l := b[1]; {
	case l == 0x80: // BER, comprimento indefinido
		i = 2
	case l&0x80 == 0: // forma curta
		i = 2
	default: // forma longa: 0x8N seguido de N bytes
		i = 2 + int(l&0x7f)
	}
	if i+3 > len(b) || b[i] != 0x02 || b[i+1] != 0x01 || b[i+2] != 0x03 {
		return false
	}
	head := b[i+3:]
	if len(head) > 64 {
		head = head[:64]
	}
	return bytes.Contains(head, pkcs7OIDPrefix)
}

// ValidateFile aplica as regras de segurança e de formato ao documento [PRD §19.1-§19.2,
// §45.3, CA-020, RN-018]. Em qualquer tipo, contêineres PKCS#12 (.pfx/.p12, por extensão
// ou assinatura) e qualquer bloco "PRIVATE KEY" são recusados. Procuração: PDF, PNG ou
// JPEG pelo conteúdo. Certificado: só certificado público (PEM com blocos CERTIFICATE
// válidos ou DER que o x509 aceite).
func ValidateFile(typ, name string, data []byte) (Accepted, error) {
	ext := strings.ToLower(filepath.Ext(name))
	if ext == ".pfx" || ext == ".p12" || looksLikePKCS12(data) {
		return Accepted{}, rejectPrivateKey()
	}
	if bytes.Contains(data, []byte("PRIVATE KEY")) {
		return Accepted{}, rejectPrivateKey()
	}
	if len(data) == 0 {
		return Accepted{}, apperr.BadRequest("EMPTY_FILE", "O arquivo está vazio.")
	}

	switch typ {
	case TypePowerOfAttorney:
		switch mime := http.DetectContentType(data); mime {
		case "application/pdf", "image/png", "image/jpeg":
			return Accepted{MimeType: mime}, nil
		}
		return Accepted{}, apperr.Unprocessable(CodeUnsupportedFile, "Formato não aceito para a procuração: envie PDF, PNG ou JPEG.")
	case TypeCertificate:
		if bytes.Contains(data, []byte("-----BEGIN")) {
			return validatePEM(data)
		}
		if _, err := x509.ParseCertificate(data); err == nil {
			return Accepted{MimeType: "application/pkix-cert"}, nil
		}
		return Accepted{}, apperr.Unprocessable(CodeUnsupportedFile,
			"Formato não aceito para o certificado: envie apenas o certificado público (.cer, .crt ou .pem).")
	}
	return Accepted{}, apperr.Unprocessable(CodeUnsupportedFile, "Tipo de documento inválido.")
}

// validatePEM aceita só blocos CERTIFICATE válidos; qualquer outro bloco é recusado.
func validatePEM(data []byte) (Accepted, error) {
	rest := data
	certs := 0
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if strings.Contains(block.Type, "PRIVATE KEY") {
			return Accepted{}, rejectPrivateKey()
		}
		if block.Type != "CERTIFICATE" {
			return Accepted{}, apperr.Unprocessable(CodeUnsupportedFile, "O arquivo PEM deve conter apenas certificados públicos.")
		}
		if _, err := x509.ParseCertificate(block.Bytes); err != nil {
			return Accepted{}, apperr.Unprocessable(CodeUnsupportedFile, "Certificado inválido no arquivo PEM.")
		}
		certs++
	}
	if certs == 0 || len(bytes.TrimSpace(rest)) > 0 {
		return Accepted{}, apperr.Unprocessable(CodeUnsupportedFile, "O arquivo PEM deve conter apenas certificados públicos.")
	}
	return Accepted{MimeType: "application/x-pem-file"}, nil
}
