// Comando loadgen: massa sintética e medição de desempenho [PRD §45.1, T-M3-10].
//
//	loadgen generate [-companies 300] [-competences 3] [-seed 42]
//	    migra o banco, carrega os dados de referência, cria N empresas fictícias com
//	    distribuição realista (~60% Simples, módulos e overrides variados), gera K
//	    competências e aplica status determinísticos pela semente;
//	loadgen bench [-iterations 20] [-patches 100] [-bulks 10] [-bulk-size 300]
//	    sobe a API em processo (HTTP real em 127.0.0.1) e mede todas as grades, dashboard,
//	    attention, backlog, exportações XLSX, ensure-current, PATCH e bulk de status,
//	    imprimindo a tabela para docs/perf.md [T-M7-01].
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
		var bc benchConfig
		fs.IntVar(&bc.iterations, "iterations", 20, "requisições medidas por rota de leitura")
		fs.IntVar(&bc.patches, "patches", 100, "PATCHs de status medidos")
		fs.IntVar(&bc.bulks, "bulks", 10, "lotes de bulk-status medidos")
		fs.IntVar(&bc.bulkSize, "bulk-size", 300, "instâncias por lote de bulk-status")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		return bench(ctx, dbURL, loc, bc, out)
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
	// Estatísticas do planejador após a carga em massa, como o autovacuum faria num banco em
	// uso; sem isso as estimativas ficam erradas e a medição reflete um estado irreal.
	if _, err := pool.Exec(ctx, "ANALYZE"); err != nil {
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

// listBody cobre as respostas medidas por GET: grades (rows), backlog (items/total) e
// attention (categories); o que não se aplica fica vazio.
type listBody struct {
	Rows       []json.RawMessage `json:"rows"`
	Items      []json.RawMessage `json:"items"`
	Categories []json.RawMessage `json:"categories"`
	Total      *int              `json:"total"`
}

func (b listBody) count() string {
	switch {
	case b.Rows != nil:
		return fmt.Sprint(len(b.Rows))
	case b.Total != nil:
		return fmt.Sprintf("%d de %d", len(b.Items), *b.Total)
	case b.Categories != nil:
		return fmt.Sprintf("%d categorias", len(b.Categories))
	}
	return "—"
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

func row(out io.Writer, label, count, size string, st stats) {
	_, _ = fmt.Fprintf(out, "| %s | %s | %s | %d | %s | %s | %s |\n", label, count, size, st.n, ms(st.p50), ms(st.p95), ms(st.max))
}

// measure repete fn (após 2 aquecimentos) e devolve as durações.
func measure(iterations int, fn func() error) ([]time.Duration, error) {
	for range 2 {
		if err := fn(); err != nil {
			return nil, err
		}
	}
	d := make([]time.Duration, 0, iterations)
	for range iterations {
		start := time.Now()
		if err := fn(); err != nil {
			return nil, err
		}
		d = append(d, time.Since(start))
	}
	return d, nil
}

type benchConfig struct {
	iterations, patches, bulks, bulkSize int
}

// bench sobe a API em processo e mede, na competência mais recente: todas as grades, o
// dashboard, attention, histórico, backlog, as exportações XLSX (quando existirem), o
// ensure-current idempotente, o PATCH de status e o bulk-status [T-M3-10, T-M7-01].
func bench(ctx context.Context, dbURL string, loc *time.Location, bc benchConfig, out io.Writer) error {
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
	client := &http.Client{Timeout: 60 * time.Second}

	var latest struct {
		Competence string `json:"competence"`
	}
	if _, _, err := get(ctx, client, base+"/competencies/latest", &latest); err != nil {
		return err
	}
	c := latest.Competence
	_, _ = fmt.Fprintf(out, "Competência medida: %s\n\n", c)
	_, _ = fmt.Fprintf(out, "| Medição | Linhas/itens | Tamanho (gzip) | n | p50 | p95 | máx |\n|---|---|---|---|---|---|---|\n")

	gets := []struct{ label, path string }{
		{"GET grade Fiscal simples", "/modules/fiscal/grid?group=simples&competence=" + c},
		{"GET grade Fiscal normal", "/modules/fiscal/grid?group=normal&competence=" + c},
		{"GET grade Declarações", "/modules/declarations/grid?competence=" + c},
		{"GET grade Folha", "/modules/payroll/grid?competence=" + c},
		{"GET grade Contábil", "/modules/accounting/grid?competence=" + c},
		{"GET grade DCTFWeb", "/modules/dctfweb/grid?competence=" + c},
		{"GET dashboard", "/dashboard?competence=" + c},
		{"GET dashboard/attention", "/dashboard/attention?competence=" + c},
		{"GET dashboard/history", "/dashboard/history?competence=" + c},
		{"GET backlog (página de 100)", "/backlog?before=" + c},
		{"GET backlog (página de 500)", "/backlog?page_size=500&before=" + c},
		{"GET backlog atrasadas da competência", "/backlog?state=overdue&competence=" + c},
	}
	for _, g := range gets {
		var b listBody
		var size int64
		d, err := measure(bc.iterations, func() error {
			b = listBody{}
			n, _, err := get(ctx, client, base+g.path, &b)
			size = n
			return err
		})
		if err != nil {
			return err
		}
		row(out, g.label, b.count(), fmt.Sprintf("%d KB", size/1024), summarize(d))
	}

	// Exportações XLSX (T-M6-05): medidas só se a rota existir nesta build.
	exports := []struct{ label, path string }{
		{"GET exportação XLSX Fiscal simples", "/exports/fiscal.xlsx?group=simples&competence=" + c},
		{"GET exportação XLSX Fiscal normal", "/exports/fiscal.xlsx?group=normal&competence=" + c},
		{"GET exportação XLSX Declarações", "/exports/declarations.xlsx?competence=" + c},
		{"GET exportação XLSX Folha", "/exports/payroll.xlsx?competence=" + c},
		{"GET exportação XLSX Contábil", "/exports/accounting.xlsx?competence=" + c},
		{"GET exportação XLSX DCTFWeb", "/exports/dctfweb.xlsx?competence=" + c},
	}
	for _, e := range exports {
		if _, status, err := get(ctx, client, base+e.path, nil); status == http.StatusNotFound {
			_, _ = fmt.Fprintf(out, "| %s | — | — | 0 | rota indisponível nesta build | — | — |\n", e.label)
			continue
		} else if err != nil {
			return err
		}
		var size int64
		d, err := measure(bc.iterations, func() error {
			n, _, err := get(ctx, client, base+e.path, nil)
			size = n
			return err
		})
		if err != nil {
			return err
		}
		row(out, e.label, "—", fmt.Sprintf("%d KB", size/1024), summarize(d))
	}

	// ensure-current com a competência já gerada (caminho de toda abertura de tela). A geração
	// de uma competência nova é medida pelo loadgen generate.
	d, err := measure(bc.iterations, func() error {
		return send(ctx, client, http.MethodPost, base+"/competencies/ensure-current", "", nil)
	})
	if err != nil {
		return err
	}
	row(out, "POST ensure-current (já gerada)", "—", "—", summarize(d))

	// Células abertas e não finais das duas grades Fiscais (o Fiscal não tem dependências).
	type target struct {
		id      string
		version int
		status  string
	}
	var targets []target
	for _, group := range []string{"simples", "normal"} {
		var g gridBody
		if _, _, err := get(ctx, client, fmt.Sprintf("%s/modules/fiscal/grid?group=%s&competence=%s", base, group, c), &g); err != nil {
			return err
		}
		for _, r := range g.Rows {
			if r.Period.State != "OPEN" {
				continue
			}
			for code, cell := range r.Cells {
				if code != "FISCAL_FECHAMENTO" && cell.Applicable && !cell.IsFinal && (cell.StatusCode == "NOT_STARTED" || cell.StatusCode == "DOWNLOADED") {
					targets = append(targets, target{cell.ID, cell.Version, cell.StatusCode})
				}
			}
		}
	}
	if len(targets) == 0 {
		return errors.New("nenhuma célula aberta para o PATCH (gere a massa com loadgen generate)")
	}

	// PATCH de status alternando entre "A iniciar" e "Baixadas" com a versão atual.
	durations := make([]time.Duration, 0, bc.patches)
	for i := range bc.patches {
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
		if err := send(ctx, client, http.MethodPatch, base+"/obligation-instances/"+t.id+"/status", body, &res); err != nil {
			return err
		}
		durations = append(durations, time.Since(start))
		t.version, t.status = res.Instance.Version, next
	}
	row(out, "PATCH status (com agregado da linha)", "—", "—", summarize(durations))

	// bulk-status com bulkSize instâncias, alternando o status do lote inteiro.
	size := min(bc.bulkSize, len(targets))
	ids := make([]string, size)
	for i := range size {
		ids[i] = targets[i].id
	}
	idsJSON, _ := json.Marshal(ids)
	durations = durations[:0]
	var updated int
	for i := range bc.bulks {
		next := "DOWNLOADED"
		if i%2 == 1 {
			next = "NOT_STARTED"
		}
		body := fmt.Sprintf(`{"instance_ids":%s,"status_code":%q}`, idsJSON, next)
		var res struct {
			Updated int `json:"updated"`
		}
		start := time.Now()
		if err := send(ctx, client, http.MethodPost, base+"/obligation-instances/bulk-status", body, &res); err != nil {
			return err
		}
		durations = append(durations, time.Since(start))
		updated = res.Updated
	}
	row(out, fmt.Sprintf("POST bulk-status (%d itens)", size), fmt.Sprintf("%d alteradas", updated), "—", summarize(durations))
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

// get faz o GET como o navegador (Accept-Encoding: gzip) e devolve o tamanho transferido e o
// status. Com v nil, só consome o corpo (ex.: XLSX).
func get(ctx context.Context, client *http.Client, u string, v any) (int64, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return 0, 0, err
	}
	req.Header.Set("Accept-Encoding", "gzip")
	res, err := client.Do(req)
	if err != nil {
		return 0, 0, err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		return 0, res.StatusCode, fmt.Errorf("GET %s: %d %s", u, res.StatusCode, b)
	}
	cr := &countingReader{r: res.Body}
	var body io.Reader = cr
	if res.Header.Get("Content-Encoding") == "gzip" {
		gz, err := gzip.NewReader(cr)
		if err != nil {
			return 0, res.StatusCode, err
		}
		body = gz
	}
	if v != nil {
		if err := json.NewDecoder(body).Decode(v); err != nil {
			return 0, res.StatusCode, err
		}
	}
	_, _ = io.Copy(io.Discard, body)
	return cr.n, res.StatusCode, nil
}

// send faz PATCH/POST com corpo JSON e decodifica a resposta em v (se não for nil).
func send(ctx context.Context, client *http.Client, method, u, body string, v any) error {
	req, err := http.NewRequestWithContext(ctx, method, u, strings.NewReader(body))
	if err != nil {
		return err
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		return fmt.Errorf("%s %s: %d %s", method, u, res.StatusCode, b)
	}
	if v == nil {
		_, _ = io.Copy(io.Discard, res.Body)
		return nil
	}
	return json.NewDecoder(res.Body).Decode(v)
}
