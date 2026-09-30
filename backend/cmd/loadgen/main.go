// Comando loadgen: massa sintética e medição de desempenho [PRD §45.1, T-M3-10].
//
//	loadgen generate [-companies 300] [-competences 3] [-seed 42]
//	    migra o banco, carrega os dados de referência, cria N empresas fictícias com
//	    distribuição realista (~60% Simples, módulos e overrides variados), gera K
//	    competências e aplica status determinísticos pela semente;
//	loadgen bench [-iterations 20] [-patches 100]
//	    sobe a API em processo (HTTP real em 127.0.0.1) e mede a grade Fiscal e o PATCH
//	    de status, imprimindo a tabela para docs/perf.md.
//
// Só roda contra o banco local controle_perf (DATABASE_URL); recusa qualquer outro.
package main

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"controle/internal/app"
	"controle/internal/competence"
	"controle/internal/platform/clock"
	"controle/internal/platform/cnpj"
	comp "controle/internal/platform/competence"
	"controle/internal/platform/config"
	"controle/internal/platform/db"
	"controle/internal/platform/migrate"
	"controle/seed"
)

const perfDatabase = "controle_perf"

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "loadgen:", err)
		os.Exit(1)
	}
}

func run(args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("use: loadgen generate|bench [flags]")
	}
	dbURL := os.Getenv("DATABASE_URL")
	if err := requirePerfDatabase(dbURL); err != nil {
		return err
	}
	loc, err := time.LoadLocation(envOr("APP_TIMEZONE", "America/Sao_Paulo"))
	if err != nil {
		return err
	}
	ctx := context.Background()
	switch args[0] {
	case "generate":
		fs := flag.NewFlagSet("generate", flag.ContinueOnError)
		n := fs.Int("companies", 300, "quantidade de empresas sintéticas")
		k := fs.Int("competences", 3, "competências geradas (a corrente e as anteriores)")
		s := fs.Uint64("seed", 42, "semente dos dados determinísticos")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		return generate(ctx, dbURL, loc, *n, *k, *s, out)
	case "bench":
		fs := flag.NewFlagSet("bench", flag.ContinueOnError)
		iterations := fs.Int("iterations", 20, "requisições medidas por grade")
		patches := fs.Int("patches", 100, "PATCHs de status medidos")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		return bench(ctx, dbURL, loc, *iterations, *patches, out)
	}
	return fmt.Errorf("comando desconhecido %q (use: generate, bench)", args[0])
}

// requirePerfDatabase garante que a carga nunca atinja o banco de dev ou outro banco.
func requirePerfDatabase(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || raw == "" {
		return errors.New("DATABASE_URL inválida ou ausente (use make perf-generate / perf-bench)")
	}
	if name := strings.TrimPrefix(u.Path, "/"); name != perfDatabase {
		return fmt.Errorf("DATABASE_URL aponta para %q; o loadgen só roda no banco %s", name, perfDatabase)
	}
	if h := u.Hostname(); h != "localhost" && h != "127.0.0.1" && h != "db" {
		return fmt.Errorf("DATABASE_URL aponta para o host %q; o loadgen só roda em banco local", h)
	}
	return nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// ---------------------------------------------------------------------------------------
// generate

var (
	cities = []struct{ City, State string }{
		{"Curitiba", "PR"}, {"Londrina", "PR"}, {"Maringá", "PR"}, {"Joinville", "SC"}, {"Florianópolis", "SC"},
		{"São Paulo", "SP"}, {"Campinas", "SP"}, {"Porto Alegre", "RS"}, {"Belo Horizonte", "MG"}, {"Rio de Janeiro", "RJ"},
	}
	normalRegimes = []string{"LUCRO_PRESUMIDO", "LUCRO_PRESUMIDO", "LUCRO_PRESUMIDO", "LUCRO_REAL", "LUCRO_REAL", "LUCRO_ARBITRADO", "IMUNE_ISENTA"}
	payrollTypes  = []string{"PAYROLL", "PAYROLL", "PRO_LABORE", "NO_PAYROLL"}
	responsibles  = []string{"ana", "bruno", "carla", "diego"}
	overrides     = []string{"FISCAL_IPI", "FISCAL_ICMS", "FISCAL_ISSQN", "SINTEGRA", "FISCAL_RETIDOS"}
)

// prng é um gerador determinístico (splitmix64): a massa sintética precisa ser reproduzível
// pela semente, não imprevisível.
type prng struct{ state uint64 }

func (p *prng) next() uint64 {
	p.state += 0x9e3779b97f4a7c15
	z := p.state
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	return z ^ (z >> 31)
}

// IntN devolve um inteiro em [0, n) para n pequeno (n > 0).
func (p *prng) IntN(n int) int {
	return int(p.next()>>33) % n
}

// syntheticCompanies monta N empresas fictícias determinísticas pela semente. Os CNPJs têm
// dígitos verificadores válidos, com raiz 99xxxxxx (faixa sintética).
func syntheticCompanies(n int, s uint64) []seed.CompanySpec {
	r := &prng{state: s}
	out := make([]seed.CompanySpec, 0, n)
	for i := 1; i <= n; i++ {
		regime := "SIMPLES_NACIONAL"
		if r.IntN(100) >= 60 {
			regime = normalRegimes[r.IntN(len(normalRegimes))]
		}
		modules := []string{"FISCAL", "DECLARATIONS", "CREDENTIALS"}
		if r.IntN(100) < 80 {
			modules = append(modules, "ACCOUNTING")
		}
		payroll := payrollTypes[r.IntN(len(payrollTypes))]
		if payroll != "NO_PAYROLL" || r.IntN(2) == 0 {
			modules = append(modules, "PAYROLL")
		}
		var factorR *bool
		if regime == "SIMPLES_NACIONAL" {
			v := r.IntN(2) == 0
			factorR = &v
		}
		var na []string
		for _, o := range overrides {
			if r.IntN(100) < 15 {
				na = append(na, o)
			}
		}
		place := cities[r.IntN(len(cities))]
		who := responsibles[r.IntN(len(responsibles))]
		out = append(out, seed.CompanySpec{
			Code:          fmt.Sprintf("P%04d", i),
			LegalName:     fmt.Sprintf("Empresa Sintética %04d Ltda", i),
			TradeName:     fmt.Sprintf("Sintética %04d", i),
			Cnpj:          cnpj.WithCheckDigits(fmt.Sprintf("99%06d0001", i)),
			City:          place.City,
			State:         place.State,
			Regimes:       []seed.RegimeSpec{{Regime: regime, From: "2025-01-01"}},
			Modules:       modules,
			PayrollType:   payroll,
			FactorR:       factorR,
			Fiscal:        who,
			Accounting:    responsibles[(i+1)%len(responsibles)],
			Payroll:       responsibles[(i+2)%len(responsibles)],
			Declarations:  responsibles[(i+3)%len(responsibles)],
			NotApplicable: na,
		})
	}
	return out
}

func generate(ctx context.Context, dbURL string, loc *time.Location, n, k int, s uint64, out io.Writer) error {
	if n < 1 || k < 1 {
		return errors.New("companies e competences devem ser positivos")
	}
	started := time.Now()
	if err := migrate.Up(ctx, dbURL, io.Discard); err != nil {
		return err
	}
	pool, err := db.NewPool(ctx, dbURL, 8)
	if err != nil {
		return err
	}
	defer pool.Close()
	clk := clock.NewSystem(loc)
	if err := seed.Reference(ctx, pool, clk, io.Discard); err != nil {
		return err
	}
	if err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		return seed.InsertCompanies(ctx, tx, syntheticCompanies(n, s))
	}); err != nil {
		return err
	}
	engine := competence.NewEngine(pool, clk)
	cur := comp.Current(clk)
	var keys []string
	for age := k - 1; age >= 0; age-- {
		c := cur.AddMonths(-age)
		sum, err := engine.Ensure(ctx, c)
		if err != nil {
			return fmt.Errorf("competência %s: %w", c, err)
		}
		keys = append(keys, c.String())
		_, _ = fmt.Fprintf(out, "competência %s: %d empresas, %d instâncias novas\n", c.Display(), sum.CompaniesProcessed, sum.InstancesCreated)
	}
	if err := applyStatuses(ctx, pool, cur.String(), s); err != nil {
		return err
	}
	var companies, instances int
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM companies WHERE internal_code LIKE 'P%'),
		(SELECT count(*) FROM obligation_instances)`).Scan(&companies, &instances); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "controle_perf: %d empresas sintéticas, %d instâncias em %s (%.1fs)\n",
		companies, instances, strings.Join(keys, ", "), time.Since(started).Seconds())
	return nil
}

// applyStatuses aplica status determinísticos (hash da empresa, tipo, competência e semente)
// às instâncias ainda na versão 1, e fecha parte dos períodos das competências anteriores.
// Competências anteriores ficam majoritariamente concluídas; a corrente, em andamento.
func applyStatuses(ctx context.Context, pool *pgxpool.Pool, current string, s uint64) error {
	return pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			WITH defs AS (
			    SELECT status_set_id,
			           array_agg(code ORDER BY sort_order) FILTER (WHERE code <> 'NOT_APPLICABLE') AS codes,
			           array_agg(code ORDER BY sort_order) FILTER (WHERE is_final AND code NOT IN ('NOT_APPLICABLE', 'NO_MOVEMENT')) AS finals
			    FROM status_definitions GROUP BY status_set_id
			), cand AS (
			    SELECT i.id, t.status_set_id, k.competence_key,
			           abs(hashtextextended(c.internal_code || '|' || t.code || '|' || k.competence_key, $1::bigint)) AS h
			    FROM obligation_instances i
			    JOIN companies c ON c.id = i.company_id
			    JOIN obligation_types t ON t.id = i.obligation_type_id
			    JOIN competencies k ON k.id = i.competence_id
			    WHERE i.applicable AND i.version = 1 AND c.internal_code LIKE 'P%'
			)
			UPDATE obligation_instances i SET
			    status_code = CASE
			        WHEN cand.competence_key < $2 AND cand.h % 10 < 8 AND cardinality(defs.finals) > 0
			            THEN defs.finals[1 + (cand.h / 10) % cardinality(defs.finals)]
			        ELSE defs.codes[1 + (cand.h / 10) % cardinality(defs.codes)]
			    END,
			    version = i.version + 1
			FROM cand JOIN defs USING (status_set_id)
			WHERE i.id = cand.id`, int64(s&0x7fffffffffffffff), current); err != nil {
			return fmt.Errorf("status sintéticos: %w", err)
		}
		const admin = "00000000-0000-0000-0000-000000000001"
		if _, err := tx.Exec(ctx, `
			WITH closed AS (
			    UPDATE company_module_periods p SET state = 'CLOSED', closed_at = now(), closed_by = $3, version = p.version + 1, updated_at = now()
			    FROM competencies k, companies c
			    WHERE k.id = p.competence_id AND c.id = p.company_id AND p.state = 'OPEN' AND k.competence_key < $2
			      AND c.internal_code LIKE 'P%'
			      AND abs(hashtextextended(c.internal_code || '|' || p.module_code || '|' || k.competence_key, $1::bigint)) % 10 < 7
			    RETURNING p.id
			)
			INSERT INTO company_module_period_events (company_module_period_id, event_type, user_id, created_at)
			SELECT id, 'CLOSED', $3, now() FROM closed`, int64(s&0x7fffffffffffffff), current, admin); err != nil {
			return fmt.Errorf("fechamentos sintéticos: %w", err)
		}
		return nil
	})
}

// ---------------------------------------------------------------------------------------
// bench

type gridCell struct {
	ID         string `json:"id"`
	StatusCode string `json:"status_code"`
	Applicable bool   `json:"applicable"`
	IsFinal    bool   `json:"is_final"`
	Version    int    `json:"version"`
}

type gridBody struct {
	Rows []struct {
		Period struct {
			State string `json:"state"`
		} `json:"period"`
		Cells map[string]gridCell `json:"cells"`
	} `json:"rows"`
}

type stats struct {
	n                  int
	p50, p95, max, avg time.Duration
}

func summarize(d []time.Duration) stats {
	slices.Sort(d)
	var sum time.Duration
	for _, v := range d {
		sum += v
	}
	at := func(q float64) time.Duration { return d[min(len(d)-1, int(q*float64(len(d))))] }
	return stats{n: len(d), p50: at(0.50), p95: at(0.95), max: d[len(d)-1], avg: sum / time.Duration(len(d))}
}

func ms(d time.Duration) string { return fmt.Sprintf("%.0f ms", float64(d.Microseconds())/1000) }

func bench(ctx context.Context, dbURL string, loc *time.Location, iterations, patches int, out io.Writer) error {
	pool, err := db.NewPool(ctx, dbURL, 10)
	if err != nil {
		return err
	}
	defer pool.Close()
	cfg := config.Config{Env: config.EnvTest, DatabaseURL: dbURL, DBMaxConns: 10, TimezoneName: loc.String(), Location: loc,
		CORSOrigins: []string{"http://localhost"}, FileStorageDir: os.TempDir(), LogLevel: slog.LevelError}
	handler := app.New(app.Deps{Config: cfg, Pool: pool, Clock: clock.NewSystem(loc), Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}).Handler
	ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	srv := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	defer func() { _ = srv.Shutdown(ctx) }()
	base := "http://" + ln.Addr().String() + "/api/v1"
	client := &http.Client{Timeout: 30 * time.Second}

	var latest struct {
		Competence string `json:"competence"`
	}
	if _, err := getJSON(ctx, client, base+"/competencies/latest", &latest); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "| Medição | Linhas | Tamanho (gzip) | n | p50 | p95 | máx |\n|---|---|---|---|---|---|---|\n")

	var sample gridBody
	for _, group := range []string{"simples", "normal"} {
		u := fmt.Sprintf("%s/modules/fiscal/grid?competence=%s&group=%s", base, latest.Competence, group)
		var g gridBody
		for range 2 { // aquecimento
			if _, err := getJSON(ctx, client, u, &g); err != nil {
				return err
			}
		}
		durations := make([]time.Duration, 0, iterations)
		var size int64
		for range iterations {
			start := time.Now()
			n, err := getJSON(ctx, client, u, &g)
			if err != nil {
				return err
			}
			durations = append(durations, time.Since(start))
			size = n
		}
		st := summarize(durations)
		_, _ = fmt.Fprintf(out, "| GET grade Fiscal %s | %d | %d KB | %d | %s | %s | %s |\n", group, len(g.Rows), size/1024, st.n, ms(st.p50), ms(st.p95), ms(st.max))
		if len(g.Rows) > len(sample.Rows) {
			sample = g
		}
	}

	// PATCH de status em células abertas e não finais (Fiscal não tem dependências), alternando
	// entre "A iniciar" e "Baixadas" com a versão atual.
	type target struct {
		id      string
		version int
		status  string
	}
	var targets []target
	for _, row := range sample.Rows {
		if row.Period.State != "OPEN" {
			continue
		}
		for code, c := range row.Cells {
			if code != "FISCAL_FECHAMENTO" && c.Applicable && !c.IsFinal && (c.StatusCode == "NOT_STARTED" || c.StatusCode == "DOWNLOADED") {
				targets = append(targets, target{c.ID, c.Version, c.StatusCode})
			}
		}
	}
	if len(targets) == 0 {
		return errors.New("nenhuma célula aberta para o PATCH (gere a massa com loadgen generate)")
	}
	durations := make([]time.Duration, 0, patches)
	for i := range patches {
		t := &targets[i%len(targets)]
		next := "DOWNLOADED"
		if t.status == "DOWNLOADED" {
			next = "NOT_STARTED"
		}
		body := fmt.Sprintf(`{"status_code":%q,"version":%d}`, next, t.version)
		start := time.Now()
		var res struct {
			Instance struct {
				Version int `json:"version"`
			} `json:"instance"`
		}
		if err := patchJSON(ctx, client, base+"/obligation-instances/"+t.id+"/status", body, &res); err != nil {
			return err
		}
		durations = append(durations, time.Since(start))
		t.version, t.status = res.Instance.Version, next
	}
	st := summarize(durations)
	_, _ = fmt.Fprintf(out, "| PATCH status (com agregado da linha) | — | — | %d | %s | %s | %s |\n", st.n, ms(st.p50), ms(st.p95), ms(st.max))
	return nil
}

// countingReader conta os bytes recebidos (comprimidos quando o servidor usa gzip).
type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

// getJSON faz o GET como o navegador (Accept-Encoding: gzip) e devolve o tamanho transferido.
func getJSON(ctx context.Context, client *http.Client, u string, v any) (int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Accept-Encoding", "gzip")
	res, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		return 0, fmt.Errorf("GET %s: %d %s", u, res.StatusCode, b)
	}
	cr := &countingReader{r: res.Body}
	var body io.Reader = cr
	if res.Header.Get("Content-Encoding") == "gzip" {
		gz, err := gzip.NewReader(cr)
		if err != nil {
			return 0, err
		}
		body = gz
	}
	if err := json.NewDecoder(body).Decode(v); err != nil {
		return 0, err
	}
	_, _ = io.Copy(io.Discard, cr)
	return cr.n, nil
}

func patchJSON(ctx context.Context, client *http.Client, u, body string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, u, strings.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		return fmt.Errorf("PATCH %s: %d %s", u, res.StatusCode, b)
	}
	return json.NewDecoder(res.Body).Decode(v)
}
