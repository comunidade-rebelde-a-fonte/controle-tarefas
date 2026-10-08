package factorr_test

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"controle/internal/platform/db"
	"controle/internal/platform/migrate"
	"controle/internal/store"
	"controle/internal/testutil"
	"controle/internal/testutil/seeded"
	"controle/internal/user"
	"controle/seed"
)

// TEST-006: migration do zero (todo banco de teste nasce do template migrado do zero),
// reversão, constraints e upsert de movimento.

const insertCompany = `
INSERT INTO companies (internal_code, legal_name, cnpj, service_start_date, payroll_type, created_by, updated_by)
VALUES ($1, $2, $3, '2025-01-01', 'PAYROLL', $4, $4) RETURNING id`

func newCompany(t *testing.T, pool *pgxpool.Pool, code, cnpj string) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(context.Background(), insertCompany, code, "Empresa "+code, cnpj, user.AdminID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func tableExists(t *testing.T, pool *pgxpool.Pool, name string) bool {
	t.Helper()
	var ok bool
	if err := pool.QueryRow(context.Background(), `SELECT to_regclass($1) IS NOT NULL`, name).Scan(&ok); err != nil {
		t.Fatal(err)
	}
	return ok
}

func TestTEST006_MigrationRevertsAndReapplies(t *testing.T) {
	pool := testutil.NewDB(t)
	ctx := context.Background()
	tables := []string{"company_factor_r_settings", "factor_r_movements", "factor_r_policy", "factor_r_simples_tables"}
	// Reverte a 0018 e as posteriores; o up reaplica todas.
	if err := migrate.DownTo(ctx, testutil.URLFor(pool), 17, io.Discard); err != nil {
		t.Fatal(err)
	}
	for _, name := range tables {
		if tableExists(t, pool, name) {
			t.Fatalf("%s continua existindo depois do down", name)
		}
	}
	if err := migrate.Up(ctx, testutil.URLFor(pool), io.Discard); err != nil {
		t.Fatal(err)
	}
	for _, name := range tables {
		if !tableExists(t, pool, name) {
			t.Fatalf("%s não voltou com o up", name)
		}
	}
}

func TestTEST006_PolicyIsSingleRowWithOriginDefaults(t *testing.T) {
	pool := testutil.NewDB(t)
	ctx := context.Background()
	p, err := store.New(pool).GetFactorRPolicy(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if p.MetaOperacional.String() != "0.300000" || !p.CppDasIntegraFs12 || p.PisoEconomiaAnual.String() != "6000.00" ||
		p.LimiarConfiancaParser.String() != "0.400000" || p.ToleranciaOuroPct.String() != "0.010000" {
		t.Fatalf("política = %+v; esperados os padrões da origem", p)
	}
	_, err = pool.Exec(ctx, `INSERT INTO factor_r_policy (id, meta_operacional, cpp_das_integra_fs12, piso_economia_anual,
		limiar_confianca_parser, tolerancia_ouro_pct) VALUES (true, 0.30, true, 0, 0.4, 0.01)`)
	if !db.IsConstraint(err, db.UniqueViolation, "") {
		t.Fatalf("segunda política deveria falhar: %v", err)
	}
	// O corte legal é 28%: a meta não pode ficar abaixo dele.
	_, err = pool.Exec(ctx, `UPDATE factor_r_policy SET meta_operacional = 0.27`)
	if !db.IsConstraint(err, db.CheckViolation, "") {
		t.Fatalf("meta abaixo de 28%% deveria falhar: %v", err)
	}
}

func TestTEST006_MovementConstraints(t *testing.T) {
	pool := testutil.NewDB(t)
	ctx := context.Background()
	id := newCompany(t, pool, "301", "11222333000181")
	ins := `INSERT INTO factor_r_movements (company_id, competence, receita_bruta, pro_labore, salarios_13_ferias,
	            cpp_recolhida, fgts_recolhido, folha_total_declarada_pgdas, origem)
	        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`
	if _, err := pool.Exec(ctx, ins, id, "2026-08-01", "50000.00", "10000.00", "2000.00", "500.00", "160.00", nil, "manual"); err != nil {
		t.Fatal(err)
	}
	var folha string
	if err := pool.QueryRow(ctx, `SELECT folha_mes::text FROM factor_r_movements WHERE company_id = $1`, id).Scan(&folha); err != nil || folha != "12660.00" {
		t.Fatalf("folha_mes = %s, %v; esperado 12660.00", folha, err)
	}
	cases := []struct {
		name string
		args []any
		code string
	}{
		{"competência repetida", []any{id, "2026-08-01", "1", "0", "0", "0", "0", nil, "manual"}, db.UniqueViolation},
		{"competência fora do dia 1", []any{id, "2026-07-15", "1", "0", "0", "0", "0", nil, "manual"}, db.CheckViolation},
		{"receita negativa", []any{id, "2026-07-01", "-1", "0", "0", "0", "0", nil, "manual"}, db.CheckViolation},
		{"origem inválida", []any{id, "2026-07-01", "1", "0", "0", "0", "0", nil, "planilha"}, db.CheckViolation},
		{"folha declarada fora do PGDAS-D", []any{id, "2026-07-01", "1", "0", "0", "0", "0", "1000.00", "manual"}, db.CheckViolation},
		{"folha declarada com divisão", []any{id, "2026-07-01", "1", "500.00", "0", "0", "0", "1000.00", "pgdas"}, db.CheckViolation},
		{"empresa inexistente", []any{"00000000-0000-0000-0000-00000000abcd", "2026-07-01", "1", "0", "0", "0", "0", nil, "manual"}, db.ForeignKeyViolation},
	}
	for _, c := range cases {
		if _, err := pool.Exec(ctx, ins, c.args...); !db.IsConstraint(err, c.code, "") {
			t.Errorf("%s: esperado erro %s, veio %v", c.name, c.code, err)
		}
	}
	// Folha do PGDAS-D sem divisão: a folha do mês é a declarada.
	if _, err := pool.Exec(ctx, ins, id, "2026-07-01", "50000.00", "0", "0", "0", "0", "14000.00", "pgdas"); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT folha_mes::text FROM factor_r_movements WHERE company_id = $1 AND competence = '2026-07-01'`, id).Scan(&folha); err != nil || folha != "14000.00" {
		t.Fatalf("folha_mes do PGDAS-D = %s, %v", folha, err)
	}
}

func TestTEST006_SettingsConstraints(t *testing.T) {
	pool := testutil.NewDB(t)
	ctx := context.Background()
	id := newCompany(t, pool, "301", "11222333000181")
	if _, err := pool.Exec(ctx, `INSERT INTO company_factor_r_settings (company_id) VALUES ($1)`, id); err != nil {
		t.Fatal(err)
	}
	var socios int
	var honorario string
	if err := pool.QueryRow(ctx, `SELECT quantidade_socios, honorario_mensal::text FROM company_factor_r_settings WHERE company_id = $1`, id).
		Scan(&socios, &honorario); err != nil || socios != 1 || honorario != "0.00" {
		t.Fatalf("padrões = %d sócios, honorário %s, %v", socios, honorario, err)
	}
	for name, set := range map[string]string{
		"pacote inválido":            `pacote_comercial = 'premium'`,
		"honorário negativo":         `honorario_mensal = -1`,
		"início fora do dia 1":       `inicio_atividade = '2026-03-15'`,
		"quantidade de sócios acima": `quantidade_socios = 1001`,
	} {
		if _, err := pool.Exec(ctx, `UPDATE company_factor_r_settings SET `+set+` WHERE company_id = $1`, id); !db.IsConstraint(err, db.CheckViolation, "") {
			t.Errorf("%s: esperado CHECK, veio %v", name, err)
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO company_factor_r_settings (company_id) VALUES ($1)`, id); !db.IsConstraint(err, db.UniqueViolation, "") {
		t.Fatalf("segunda configuração da mesma empresa deveria falhar: %v", err)
	}
}

func TestTEST006_SimplesTablesSeedIsIdempotentAndNeverOverwrites(t *testing.T) {
	pool := seeded.NewDB(t)
	ctx := context.Background()
	count := func() int {
		var n int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM factor_r_simples_tables`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if n := count(); n != 12 {
		t.Fatalf("faixas = %d; esperado 12 (Anexos III e V, 6 faixas cada)", n)
	}
	// O seed insere só linhas novas e nunca altera uma vigência gravada (origem: carregar_csv).
	if _, err := pool.Exec(ctx, `UPDATE factor_r_simples_tables SET parcela_deduzir = 1.00 WHERE anexo = 'III' AND faixa = 1`); err != nil {
		t.Fatal(err)
	}
	if err := seed.Reference(ctx, pool, seeded.DemoClock(), io.Discard); err != nil {
		t.Fatal(err)
	}
	var pd string
	if err := pool.QueryRow(ctx, `SELECT parcela_deduzir::text FROM factor_r_simples_tables WHERE anexo = 'III' AND faixa = 1`).Scan(&pd); err != nil || pd != "1.00" {
		t.Fatalf("seed alterou a vigência gravada: %s, %v", pd, err)
	}
	if n := count(); n != 12 {
		t.Fatalf("seed repetido duplicou faixas: %d", n)
	}
	_, err := pool.Exec(ctx, `INSERT INTO factor_r_simples_tables (anexo, faixa, rbt12_ate, aliquota_nominal, parcela_deduzir, vigencia_inicio, vigencia_fim)
		VALUES ('III', 1, 180000, 0.06, 0, '2027-01-01', '2026-01-01')`)
	if !db.IsConstraint(err, db.CheckViolation, "factor_r_simples_tables_vigencia_ck") {
		t.Fatalf("vigência invertida deveria falhar: %v", err)
	}
}

// O pgx devolve o zero de qualquer escala com expoente 0; os valores não nulos chegam com a
// escala da coluna. O serviço reaplica a escala ao montar o motor (money/rate).
func TestTEST006_NumericRoundTripKeepsScale(t *testing.T) {
	pool := testutil.NewDB(t)
	ctx := context.Background()
	id := newCompany(t, pool, "301", "11222333000181")
	q := store.New(pool)
	m, err := q.InsertFactorRMovementManual(ctx, store.InsertFactorRMovementManualParams{
		CompanyID: id, Competence: mustDate(t, "2026-08-01"), ReceitaBruta: dec("1234.5"), ProLabore: dec("0.00"),
		Salarios13Ferias: dec("10"), CppRecolhida: dec("0"), FgtsRecolhido: dec("0.10"), UpdatedAt: mustDate(t, "2026-10-01"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if m.ReceitaBruta.String() != "1234.50" || m.Salarios13Ferias.String() != "10.00" || m.FgtsRecolhido.String() != "0.10" {
		t.Fatalf("valores lidos = %s, %s, %s", m.ReceitaBruta, m.Salarios13Ferias, m.FgtsRecolhido)
	}
	if m.ProLabore.String() != "0" || m.ProLabore.Quantize(-2).String() != "0.00" {
		t.Fatalf("zero lido = %s (esperado 0 do pgx, 0.00 com a escala)", m.ProLabore)
	}
	if m.FolhaMes == nil || m.FolhaMes.String() != "10.10" {
		t.Fatalf("folha_mes = %v", m.FolhaMes)
	}
}

// TASK-006b: documentos do Inbox PGDAS-D e a FK dos movimentos criados a partir deles.
func TestTEST006b_PgdasDocumentConstraints(t *testing.T) {
	pool := testutil.NewDB(t)
	ctx := context.Background()
	id := newCompany(t, pool, "301", "11222333000181")
	sha := strings.Repeat("a", 64)
	ins := `INSERT INTO factor_r_pgdas_documents (sha256, file_ref, mime, size_bytes, status, motivo, company_id)
	        VALUES ($1, $2, $3, 10, $4, $5, $6) RETURNING id`
	var doc string
	if err := pool.QueryRow(ctx, ins, sha, "pgdas/"+sha, "text/plain", "received", nil, nil).Scan(&doc); err != nil {
		t.Fatal(err)
	}
	other := strings.Repeat("b", 64)
	cases := []struct {
		name string
		args []any
		code string
	}{
		{"mesmo arquivo de novo", []any{sha, "pgdas/" + sha, "text/plain", "received", nil, nil}, db.UniqueViolation},
		{"caminho que não é o do hash", []any{other, "pgdas/../x", "text/plain", "received", nil, nil}, db.CheckViolation},
		{"tipo fora de PDF e TXT", []any{other, "pgdas/" + other, "image/png", "received", nil, nil}, db.CheckViolation},
		{"vinculado sem empresa", []any{other, "pgdas/" + other, "text/plain", "linked", nil, nil}, db.CheckViolation},
		{"empresa sem vínculo", []any{other, "pgdas/" + other, "text/plain", "needs_review", "cnpj_nao_encontrado", id}, db.CheckViolation},
		{"revisão sem motivo", []any{other, "pgdas/" + other, "text/plain", "needs_review", nil, nil}, db.CheckViolation},
		// Depois do upload, toda decisão tem corrida do agente [AC-029].
		{"decisão sem trace", []any{other, "pgdas/" + other, "text/plain", "needs_review", "cnpj_nao_encontrado", nil}, db.CheckViolation},
	}
	for _, c := range cases {
		var got string
		if err := pool.QueryRow(ctx, ins, c.args...).Scan(&got); !db.IsConstraint(err, c.code, "") {
			t.Errorf("%s: esperado erro %s, veio %v", c.name, c.code, err)
		}
	}
	mov := `INSERT INTO factor_r_movements (company_id, competence, receita_bruta, origem, pgdas_document_id) VALUES ($1, '2026-09-01', 1, 'pgdas', $2)`
	if _, err := pool.Exec(ctx, mov, id, "00000000-0000-0000-0000-00000000abcd"); !db.IsConstraint(err, db.ForeignKeyViolation, "") {
		t.Errorf("movimento com documento inexistente: %v", err)
	}
	if _, err := pool.Exec(ctx, mov, id, doc); err != nil {
		t.Fatal(err)
	}
	// Sem exclusão física: o documento referenciado não sai.
	if _, err := pool.Exec(ctx, `DELETE FROM factor_r_pgdas_documents WHERE id = $1`, doc); !db.IsConstraint(err, db.ForeignKeyViolation, "") {
		t.Errorf("apagar documento com movimento: %v", err)
	}
}
