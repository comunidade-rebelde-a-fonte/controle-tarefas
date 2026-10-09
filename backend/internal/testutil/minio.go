package testutil

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	tcexec "github.com/testcontainers/testcontainers-go/exec"
	"github.com/testcontainers/testcontainers-go/wait"
)

// MinIOImage é a mesma imagem do docker-compose.yml (Chainguard, fixada por digest; D-013).
const MinIOImage = "cgr.dev/chainguard/minio:latest@sha256:e7ca559d9f7c0b5f24f5f669bb92f40f3ca88d56273b808bf3a7c116c17d2ffa"

// MinIO é o armazenamento de teste, inicializado pelo mesmo docker/minio/init.sh do ambiente:
// bucket versionado e usuário da aplicação sem permissão de apagar.
type MinIO struct {
	Endpoint     string
	AccessKey    string
	SecretKey    string
	RootUser     string
	RootPassword string
	Bucket       string
}

var (
	minioOnce   sync.Once
	sharedMinIO MinIO
	minioErr    error
)

// NewMinIO sobe o MinIO de teste, um por pacote (processo), e devolve o acesso.
func NewMinIO(t testing.TB) MinIO {
	t.Helper()
	minioOnce.Do(startMinIO)
	if minioErr != nil {
		t.Fatalf("testutil: %v", minioErr)
	}
	return sharedMinIO
}

// initScript é o caminho do docker/minio/init.sh a partir deste arquivo.
func initScript() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "docker", "minio", "init.sh")
}

func startMinIO() {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	m := MinIO{AccessKey: "conciliacao-app", SecretKey: "test-app-secret", RootUser: "minio-admin", RootPassword: "test-root-secret", Bucket: "conciliacao-originais"}
	var ctr testcontainers.Container
	var err error
	// Mesma tolerância do Postgres à detecção do provedor do Docker no Windows.
	for attempt := 1; ; attempt++ {
		ctr, err = testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
			ContainerRequest: testcontainers.ContainerRequest{
				Image:        MinIOImage,
				Cmd:          []string{"server", "/data"},
				Env:          map[string]string{"MINIO_ROOT_USER": m.RootUser, "MINIO_ROOT_PASSWORD": m.RootPassword},
				ExposedPorts: []string{"9000/tcp"},
				Files:        []testcontainers.ContainerFile{{HostFilePath: initScript(), ContainerFilePath: "/scripts/init.sh", FileMode: 0o755}},
				WaitingFor:   wait.ForHTTP("/minio/health/live").WithPort("9000/tcp").WithStartupTimeout(2 * time.Minute),
			},
			Started: true,
		})
		if err == nil || attempt == 3 || !strings.Contains(err.Error(), "Docker provider") {
			break
		}
		time.Sleep(time.Duration(attempt) * 3 * time.Second)
	}
	if err != nil {
		minioErr = fmt.Errorf("subir MinIO de teste (Docker está rodando?): %w", err)
		return
	}
	code, out, err := ctr.Exec(ctx, []string{"sh", "/scripts/init.sh"}, tcexec.Multiplexed(), tcexec.WithEnv([]string{
		"MINIO_URL=http://localhost:9000",
		"S3_ROOT_USER=" + m.RootUser, "S3_ROOT_PASSWORD=" + m.RootPassword,
		"S3_ACCESS_KEY=" + m.AccessKey, "S3_SECRET_KEY=" + m.SecretKey, "S3_BUCKET=" + m.Bucket,
	}))
	if err != nil {
		minioErr = fmt.Errorf("inicializar o MinIO de teste (init.sh): %w", err)
		return
	}
	if code != 0 {
		msg := ""
		if out != nil {
			b, _ := io.ReadAll(out)
			msg = string(b)
		}
		minioErr = fmt.Errorf("inicializar o MinIO de teste (init.sh, código %d): %s", code, msg)
		return
	}
	endpoint, err := ctr.PortEndpoint(ctx, "9000/tcp", "http")
	if err != nil {
		minioErr = err
		return
	}
	m.Endpoint = endpoint
	sharedMinIO = m
}
