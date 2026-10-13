package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"controle/internal/testutil"
)

// A preparação dos fixtures pela linha de comando (TASK-027) recusa opção desconhecida, produção,
// falta do MinIO dos originais e worker fora do ar (é ele que lê os documentos do cenário). A
// preparação em si é o TEST-027 (seed/fixtures).
func TestSeedFixtures_Refusals(t *testing.T) {
	pool := testutil.NewDB(t)
	env := map[string]string{"DATABASE_URL": testutil.URLFor(pool), "APP_FIXED_TIME": "2026-10-12T12:00:00-03:00", "LOG_LEVEL": "error"}
	seedCmd := func(args ...string) string {
		t.Helper()
		var out bytes.Buffer
		err := run(context.Background(), append([]string{"seed"}, args...), func(k string) string { return env[k] }, &out)
		if err == nil {
			t.Fatalf("seed %v deveria recusar:\n%s", args, out.String())
		}
		return err.Error()
	}

	if msg := seedCmd("--fixture"); !strings.Contains(msg, "opção de seed desconhecida") {
		t.Fatalf("opção desconhecida: %s", msg)
	}
	if msg := seedCmd("--fixtures"); !strings.Contains(msg, "MinIO") {
		t.Fatalf("sem MinIO: %s", msg)
	}
	m := testutil.NewMinIO(t)
	env["S3_ENDPOINT"], env["S3_ACCESS_KEY"], env["S3_SECRET_KEY"], env["S3_BUCKET"] = m.Endpoint, m.AccessKey, m.SecretKey, m.Bucket
	if msg := seedCmd("--fixtures"); !strings.Contains(msg, "o worker não está no ar") {
		t.Fatalf("sem worker: %s", msg)
	}
	delete(env, "APP_FIXED_TIME")
	env["APP_ENV"] = "production"
	if msg := seedCmd("--fixtures"); !strings.Contains(msg, "não são carregados em produção") {
		t.Fatalf("produção: %s", msg)
	}
}
