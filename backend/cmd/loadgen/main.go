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

	"controle/internal/app"
	"controle/internal/loadgen"
	"controle/internal/platform/clock"
	"controle/internal/platform/config"
	"controle/internal/platform/db"
	"controle/internal/platform/migrate"
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

func generate(ctx context.Context, dbURL string, loc *time.Location, n, k int, s uint64, out io.Writer) error {
	started := time.Now()
	if err := migrate.Up(ctx, dbURL, io.Discard); err != nil {
		return err
	}
	pool, err := db.NewPool(ctx, dbURL, 8)
	if err != nil {
		return err
	}
	defer pool.Close()
	keys, err := loadgen.Generate(ctx, pool, clock.NewSystem(loc), n, k, s, out)
	if err != nil {
		return err
	}
	var companies, instances int
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM companies WHERE internal_code LIKE 'P%'),
		(SELECT count(*) FROM obligation_instances)`).Scan(&companies, &instances); err != nil {
		return err
	}
	names := make([]string, len(keys))
	for i, c := range keys {
		names[i] = c.String()
	}
	_, _ = fmt.Fprintf(out, "controle_perf: %d empresas sintéticas, %d instâncias em %s (%.1fs)\n",
		companies, instances, strings.Join(names, ", "), time.Since(started).Seconds())
	return nil
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
