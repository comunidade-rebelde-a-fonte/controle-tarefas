package jobs

import (
	"errors"
	"testing"
	"time"
)

// Transições iguais às da origem (tests/unit/test_fila.py): lá a tentativa conta na falha; aqui
// conta na aquisição, então "attempts" já inclui a tentativa que falhou.
func TestNextAfterFailure_MatchesOrigin(t *testing.T) {
	now := time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)
	// proximo_estado_falha(0, 3) → retry, tentativas 1, available_at = agora + backoff(1).
	if got := NextAfterFailure(1, 3, now, false); got.Status != StatusRetry || !got.RunAfter.Equal(now.Add(2*time.Second)) {
		t.Fatalf("1ª falha = %+v", got)
	}
	if got := NextAfterFailure(2, 3, now, false); got.Status != StatusRetry || !got.RunAfter.Equal(now.Add(4*time.Second)) {
		t.Fatalf("2ª falha = %+v", got)
	}
	// proximo_estado_falha(2, 3) → dead_letter, tentativas 3.
	if got := NextAfterFailure(3, 3, now, false); got.Status != StatusDead {
		t.Fatalf("3ª falha = %+v", got)
	}
	if got := NextAfterFailure(1, 3, now, true); got.Status != StatusDead {
		t.Fatalf("falha permanente = %+v", got)
	}
}

func TestBackoff(t *testing.T) {
	for attempt, want := range map[int32]time.Duration{0: time.Second, 1: 2 * time.Second, 3: 8 * time.Second, 12: time.Hour, 40: time.Hour} {
		if got := Backoff(attempt); got != want {
			t.Errorf("Backoff(%d) = %s; esperado %s", attempt, got, want)
		}
	}
}

func TestClassify(t *testing.T) {
	code, permanent, msg := classify(errors.New("Bearer sk-abc123xyz\nrecusado"))
	if code != CodeHandlerError || permanent || msg != "Bearer [oculto] recusado" {
		t.Fatalf("erro comum = %s, %v, %q", code, permanent, msg)
	}
	code, permanent, _ = classify(Coded("LLM_UNAVAILABLE", errors.New("x")))
	if code != "LLM_UNAVAILABLE" || permanent {
		t.Fatalf("Coded = %s, %v", code, permanent)
	}
	code, permanent, _ = classify(Permanent("DOCUMENT_UNREADABLE", errors.New("x")))
	if code != "DOCUMENT_UNREADABLE" || !permanent {
		t.Fatalf("Permanent = %s, %v", code, permanent)
	}
}
