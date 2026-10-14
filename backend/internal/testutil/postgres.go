// Package testutil oferece infraestrutura para testes de integração:
// Postgres real via testcontainers, bancos isolados por teste, servidor HTTP
// de teste e relógio fixo.
//
// Estratégia: um container Postgres por pacote de teste (processo). Nele, um
// banco "template" recebe as migrations uma única vez; cada teste ganha um
// banco novo clonado do template (CREATE DATABASE ... TEMPLATE), o que é
// rápido e garante isolamento total, inclusive com t.Parallel().
package testutil

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib" // driver database/sql "pgx" (goose)
	"github.com/pressly/goose/v3"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"controle/internal/platform/db"
	"controle/migrations"
)

// PostgresImage é a mesma imagem do docker-compose.yml.
const PostgresImage = "postgres:16.15-bookworm"

const (
	pgUser     = "test"
	pgPassword = "test"
	templateDB = "tmpl_migrated"
)

type cluster struct {
	baseURL string // sem o nome do banco: postgres://user:pass@host:port/
	err     error
}

var (
	clusterOnce sync.Once
	shared      cluster
	dbCounter   atomic.Int64
	templates   sync.Map // nome → *sync.Once, para templates adicionais
	templateErr sync.Map // nome → error
)

// runPostgres sobe o container. Com dezenas de pacotes subindo Postgres ao mesmo tempo, o
// Docker Desktop no Windows às vezes não responde à detecção do provedor ("failed to create
// Docker provider", antes de criar qualquer container): espera um pouco e tenta de novo.
func runPostgres(ctx context.Context) (*tcpostgres.PostgresContainer, error) {
	for attempt := 1; ; attempt++ {
		ctr, err := startPostgres(ctx)
		if err == nil || attempt == 3 || !strings.Contains(err.Error(), "Docker provider") {
			return ctr, err
		}
		time.Sleep(time.Duration(attempt) * 3 * time.Second)
	}
}

func startCluster() {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	ctr, err := runPostgres(ctx)
	if err != nil {
		shared.err = fmt.Errorf("subir postgres de teste (Docker está rodando?): %w", err)
		return
	}
	connStr, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		shared.err = err
		return
	}
	// connStr termina em /postgres?sslmode=disable
	idx := strings.LastIndex(connStr, "/postgres")
	shared.baseURL = connStr[:idx+1]

	if err := createMigratedTemplate(ctx); err != nil {
		shared.err = err
	}
}

func startPostgres(ctx context.Context) (*tcpostgres.PostgresContainer, error) {
	return tcpostgres.Run(ctx, PostgresImage,
		tcpostgres.WithUsername(pgUser),
		tcpostgres.WithPassword(pgPassword),
		tcpostgres.WithDatabase("postgres"),
		// Mesmos INITDB_ARGS e fuso do docker-compose.yml.
		testcontainers.WithEnv(map[string]string{
			"POSTGRES_INITDB_ARGS": "--encoding=UTF8 --locale-provider=icu --icu-locale=pt-BR",
			"TZ":                   "UTC",
			"PGTZ":                 "UTC",
		}),
		// As esperas de tcpostgres.BasicWaitStrategies, com prazo de 3 min em vez do padrão de
		// 60 s: no `make test`, dezenas de pacotes sobem Postgres ao mesmo tempo e, no Windows, o
		// initdb pode passar de 60 s (BL-005).
		testcontainers.WithWaitStrategyAndDeadline(3*time.Minute,
			wait.ForLog("database system is ready to accept connections").WithOccurrence(2),
			wait.ForListeningPort("5432/tcp"),
		),
	)
}

func dbURL(name string) string {
	return shared.baseURL + name + "?sslmode=disable&timezone=UTC"
}

func adminExec(ctx context.Context, sql string) error {
	conn, err := pgx.Connect(ctx, dbURL("postgres"))
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close(ctx) }()
	_, err = conn.Exec(ctx, sql)
	return err
}

func createMigratedTemplate(ctx context.Context) error {
	if err := adminExec(ctx, "CREATE DATABASE "+templateDB); err != nil {
		return fmt.Errorf("criar template: %w", err)
	}
	sqlDB, err := sql.Open("pgx", dbURL(templateDB))
	if err != nil {
		return err
	}
	defer func() { _ = sqlDB.Close() }()
	p, err := goose.NewProvider(goose.DialectPostgres, sqlDB, migrations.FS)
	if err != nil {
		return err
	}
	if _, err := p.Up(ctx); err != nil {
		return fmt.Errorf("aplicar migrations no template: %w", err)
	}
	return nil
}

func ensureCluster(t testing.TB) {
	t.Helper()
	clusterOnce.Do(startCluster)
	if shared.err != nil {
		t.Fatalf("testutil: %v", shared.err)
	}
}

// TemplateInit popula um template derivado do template migrado (ex.: seeds).
type TemplateInit func(ctx context.Context, pool *pgxpool.Pool) error

// EnsureTemplate cria (uma vez por processo) o template `name` a partir do
// template migrado e executa init nele.
func EnsureTemplate(t testing.TB, name string, init TemplateInit) {
	t.Helper()
	ensureCluster(t)
	onceAny, _ := templates.LoadOrStore(name, &sync.Once{})
	onceAny.(*sync.Once).Do(func() {
		ctx := context.Background()
		if err := adminExec(ctx, fmt.Sprintf("CREATE DATABASE %s TEMPLATE %s", name, templateDB)); err != nil {
			templateErr.Store(name, err)
			return
		}
		pool, err := db.NewPool(ctx, dbURL(name), 4)
		if err != nil {
			templateErr.Store(name, err)
			return
		}
		defer pool.Close()
		if err := init(ctx, pool); err != nil {
			templateErr.Store(name, err)
		}
	})
	if err, ok := templateErr.Load(name); ok {
		t.Fatalf("testutil: template %s: %v", name, err)
	}
}

// NewDB cria um banco isolado com todas as migrations aplicadas e devolve o pool.
// O banco é removido ao fim do teste.
func NewDB(t testing.TB) *pgxpool.Pool {
	t.Helper()
	ensureCluster(t)
	return newDBFrom(t, templateDB)
}

// NewDBFromTemplate cria um banco isolado a partir de um template criado com EnsureTemplate.
func NewDBFromTemplate(t testing.TB, template string) *pgxpool.Pool {
	t.Helper()
	ensureCluster(t)
	return newDBFrom(t, template)
}

// URLFor devolve a URL de conexão de um banco criado por NewDB (útil para goose/sql).
func URLFor(pool *pgxpool.Pool) string {
	return dbURL(pool.Config().ConnConfig.Database)
}

func newDBFrom(t testing.TB, template string) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()
	name := fmt.Sprintf("t_%d_%d", os.Getpid(), dbCounter.Add(1))
	if err := adminExec(ctx, fmt.Sprintf("CREATE DATABASE %s TEMPLATE %s", name, template)); err != nil {
		t.Fatalf("testutil: criar banco de teste: %v", err)
	}
	pool, err := db.NewPool(ctx, dbURL(name), 10)
	if err != nil {
		t.Fatalf("testutil: pool: %v", err)
	}
	t.Cleanup(func() {
		pool.Close()
		_ = adminExec(context.Background(), fmt.Sprintf("DROP DATABASE IF EXISTS %s WITH (FORCE)", name))
	})
	return pool
}

// Truncate esvazia as tabelas informadas (RESTART IDENTITY CASCADE).
func Truncate(t testing.TB, pool *pgxpool.Pool, tables ...string) {
	t.Helper()
	if len(tables) == 0 {
		return
	}
	idents := make([]string, len(tables))
	for i, tbl := range tables {
		idents[i] = pgx.Identifier{tbl}.Sanitize()
	}
	if _, err := pool.Exec(context.Background(), "TRUNCATE "+strings.Join(idents, ", ")+" RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("testutil: truncate: %v", err)
	}
}

// WithTx executa fn numa transação que é sempre revertida ao final.
func WithTx(t testing.TB, pool *pgxpool.Pool, fn func(tx pgx.Tx)) {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("testutil: begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	fn(tx)
}
