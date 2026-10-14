// Package objectstore é um cliente mínimo de armazenamento de objetos compatível com S3: o MinIO
// dos originais da Conciliação [REQ-043, ADR-002]. Grava e lê pela versão do objeto (o bucket é
// versionado) e não tem operação de apagar: o original do cliente é imutável [09 §5], como o
// Storage da origem (`app/storage.py`, sem delete). A assinatura é a AWS Signature V4 com a
// biblioteca padrão; o endereçamento é por caminho (http://host/bucket/chave), como na origem.
package objectstore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"controle/internal/platform/redact"
)

// Erros do armazenamento. ErrUnavailable é falha de rede ou do serviço (5xx): a API responde 503
// STORAGE_UNAVAILABLE sem gravar nada [11 §9 "MinIO fora"].
var (
	ErrNotConfigured = errors.New("objectstore: armazenamento não configurado")
	ErrUnavailable   = errors.New("objectstore: armazenamento indisponível")
	ErrNotFound      = errors.New("objectstore: objeto não encontrado")
	ErrDenied        = errors.New("objectstore: acesso negado")
)

// Config é a configuração do armazenamento; Endpoint vazio = não configurado.
type Config struct {
	Endpoint  string // ex.: http://minio:9000
	Region    string
	AccessKey string
	SecretKey string
	Bucket    string
	Timeout   time.Duration
}

// Store fala com um bucket.
type Store struct {
	cfg  Config
	base *url.URL
	http *http.Client
	now  func() time.Time
}

// New cria o cliente. Um endpoint inválido deixa o cliente não configurado (a validação de
// formato é da configuração da aplicação).
func New(cfg Config) *Store {
	if cfg.Region == "" {
		cfg.Region = "us-east-1"
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 60 * time.Second
	}
	s := &Store{cfg: cfg, http: &http.Client{Timeout: cfg.Timeout}, now: time.Now}
	if u, err := url.Parse(strings.TrimRight(cfg.Endpoint, "/")); err == nil && u.Scheme != "" && u.Host != "" {
		s.base = u
	}
	return s
}

// Configured indica se há endpoint, credencial e bucket.
func (s *Store) Configured() bool {
	return s != nil && s.base != nil && s.cfg.AccessKey != "" && s.cfg.SecretKey != "" && s.cfg.Bucket != ""
}

// Bucket é o nome do bucket.
func (s *Store) Bucket() string { return s.cfg.Bucket }

// PutResult é o objeto gravado: a versão criada (o bucket é versionado) e o ETag.
type PutResult struct {
	VersionID string
	ETag      string
}

// Object é um objeto lido; Body deve ser fechado.
type Object struct {
	Body        io.ReadCloser
	Size        int64
	ContentType string
	VersionID   string
}

// Put grava o objeto. Com o bucket versionado, gravar a mesma chave de novo cria outra versão e
// a anterior continua legível.
func (s *Store) Put(ctx context.Context, key string, body []byte, contentType string) (PutResult, error) {
	sum := sha256.Sum256(body)
	hdr := http.Header{}
	hdr.Set("Content-Type", contentType)
	resp, err := s.do(ctx, http.MethodPut, key, nil, hdr, body, hex.EncodeToString(sum[:]))
	if err != nil {
		return PutResult{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)
	return PutResult{VersionID: resp.Header.Get("x-amz-version-id"), ETag: strings.Trim(resp.Header.Get("ETag"), `"`)}, nil
}

// Get lê o objeto; versionID vazio = a versão mais recente.
func (s *Store) Get(ctx context.Context, key, versionID string) (Object, error) {
	q := url.Values{}
	if versionID != "" {
		q.Set("versionId", versionID)
	}
	resp, err := s.do(ctx, http.MethodGet, key, q, nil, nil, EmptySHA256) //nolint:bodyclose // o corpo vai em Object.Body e quem lê fecha
	if err != nil {
		return Object{}, err
	}
	size, _ := strconv.ParseInt(resp.Header.Get("Content-Length"), 10, 64)
	return Object{Body: resp.Body, Size: size, ContentType: resp.Header.Get("Content-Type"), VersionID: resp.Header.Get("x-amz-version-id")}, nil
}

// Ping confere se o bucket responde com a credencial da aplicação.
func (s *Store) Ping(ctx context.Context) error {
	resp, err := s.do(ctx, http.MethodHead, "", nil, nil, nil, EmptySHA256)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	return nil
}

// do assina e envia a requisição; respostas fora de 2xx viram os erros do pacote.
func (s *Store) do(ctx context.Context, method, key string, q url.Values, hdr http.Header, body []byte, payloadHash string) (*http.Response, error) {
	if !s.Configured() {
		return nil, ErrNotConfigured
	}
	path := "/" + s.cfg.Bucket
	if key != "" {
		path += "/" + key
	}
	encoded := encodeURIPath(path)
	u := *s.base
	u.Path = path
	u.RawPath = encoded
	u.RawQuery = canonicalQuery(q)
	req, err := http.NewRequestWithContext(ctx, method, u.String(), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("objectstore: montar a requisição: %w", err)
	}
	if body == nil {
		req.Body = http.NoBody
		req.ContentLength = 0
	}
	for k, v := range hdr {
		req.Header[k] = v
	}
	now := s.now().UTC()
	amzDate := now.Format(amzFormat)
	req.Header.Set("x-amz-date", amzDate)
	req.Header.Set("x-amz-content-sha256", payloadHash)
	signed := map[string]string{"host": u.Host, "x-amz-date": amzDate, "x-amz-content-sha256": payloadHash}
	if ct := req.Header.Get("Content-Type"); ct != "" {
		signed["content-type"] = ct
	}
	req.Header.Set("Authorization", authorization(signingRequest{
		method: method, path: encoded, query: q, headers: signed, payloadHash: payloadHash,
	}, s.cfg.AccessKey, s.cfg.SecretKey, s.cfg.Region, now))
	resp, err := s.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("%w: %s", ErrUnavailable, redact.Message(err.Error()))
	}
	if resp.StatusCode/100 == 2 {
		return resp, nil
	}
	defer func() { _ = resp.Body.Close() }()
	code := errorCode(resp)
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return nil, fmt.Errorf("%w (%s)", ErrNotFound, code)
	case resp.StatusCode == http.StatusForbidden:
		return nil, fmt.Errorf("%w (%s)", ErrDenied, code)
	case resp.StatusCode >= 500:
		return nil, fmt.Errorf("%w: HTTP %d %s", ErrUnavailable, resp.StatusCode, code)
	default:
		return nil, fmt.Errorf("objectstore: HTTP %d %s", resp.StatusCode, code)
	}
}

// errorCode lê o <Code> do corpo de erro do S3 (vazio em HEAD).
func errorCode(resp *http.Response) string {
	var e struct {
		Code string `xml:"Code"`
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err := xml.Unmarshal(raw, &e); err != nil || e.Code == "" {
		return resp.Status
	}
	return e.Code
}
