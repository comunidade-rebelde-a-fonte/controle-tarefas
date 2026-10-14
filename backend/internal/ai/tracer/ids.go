package tracer

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
)

var traceIDRe = regexp.MustCompile(`^[0-9a-f]{32}$`)

// ValidTraceID indica se s tem o formato do trace_id: 32 hex minúsculos.
func ValidTraceID(s string) bool { return traceIDRe.MatchString(s) }

// NewTraceID gera o trace_id: 16 bytes aleatórios em hex, o formato do OpenTelemetry que o
// Langfuse usa (Langfuse.create_trace_id na origem). O mesmo ID vale no banco e no Langfuse.
func NewTraceID() string { return randomHex(16) }

func newSpanID() string { return randomHex(8) }

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b) // nunca falha (crypto/rand)
	return hex.EncodeToString(b)
}

// RootSpanID é o ID do span raiz, derivado do trace: o mesmo em todo reenvio, para o Langfuse
// atualizar a observação em vez de criar outra.
func RootSpanID(traceID string) string {
	sum := sha256.Sum256([]byte("root:" + traceID))
	return hex.EncodeToString(sum[:8])
}

// derivedScoreID é o ID da nota agregada (gold_acerto) de um trace: um UUID versão 8 (formato
// livre da RFC 9562) tirado do SHA-256 de (trace, nome), o mesmo em todo reenvio.
func derivedScoreID(traceID, name string) string {
	sum := sha256.Sum256([]byte("score:" + traceID + "/" + name))
	b := sum[:16]
	b[6] = (b[6] & 0x0f) | 0x80 // versão 8
	b[8] = (b[8] & 0x3f) | 0x80 // variante RFC 9562
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
