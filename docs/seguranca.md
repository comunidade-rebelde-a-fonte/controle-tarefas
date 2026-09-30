# Checklist de segurança

Verificação da tarefa T-M7-05 [PRD §45.3, RN-018], feita em 30/09/2026 sobre a branch de integração (`develop`, após T-M6-05).
Cada item traz a evidência reproduzível. Refaça a verificação sempre que surgir um handler, uma query ou uma dependência nova.

**Resultado:** nenhum item em aberto de severidade alta. Há recomendações de severidade baixa na seção 8, a tratar na T-M7-06 (implantação).

| # | Item | Situação |
|---|---|---|
| 1 | Segredos só em variáveis de ambiente | OK |
| 2 | Queries parametrizadas, sem SQL concatenado | OK |
| 3 | CORS explícito | OK |
| 4 | Validação de payload em todos os handlers | OK |
| 5 | Logs sem conteúdo de arquivos ou dados sensíveis | OK |
| 6 | Limites de upload e armazenamento fora do webroot | OK |
| 7 | Sem segredo digital (RN-018) | OK |
| 8 | `govulncheck` e `npm audit` | OK (2 achados corrigidos nesta tarefa) |

---

## 1. Segredos só em variáveis de ambiente

- A configuração vem só do ambiente (`backend/internal/platform/config/config.go`, `Load(getenv)`): `DATABASE_URL`, `CORS_ORIGINS`, `FILE_STORAGE_DIR`, `APP_ENV` etc. Não há arquivo de configuração com credenciais.
- `.env` e `.env.*` estão no `.gitignore` (exceto `.env.example` e `.env.prod.example`). Nenhum `.env` versionado: `git ls-files | grep -iE '\.env|\.pfx|\.p12|\.pem$|\.key$'` devolve só `.env.example`.
- A única senha versionada é a de exemplo de dev, `dev-only-change-me` (`.env.example`, `docker-compose.yml`, `Makefile`), com o nome indicando o uso. Busca por DSN com senha fora desse padrão: `git grep -nE "postgres(ql)?://[^:@/ ]+:[^@ ]+@"` só encontra variáveis do Makefile e DSNs fictícios (`u:p@`) em `cmd/loadgen/main_test.go`.
- A `DATABASE_URL` nunca é registrada em log: o boot registra só porta, ambiente e fuso (`cmd/api/main.go`); os erros de pool e migração não incluem a DSN (`internal/platform/db`, `internal/platform/migrate`).
- `APP_FIXED_TIME` e o seed de demonstração são recusados em produção (`config.go`, `cmd/api/main.go`), e a rota de demo só é registrada fora de produção (`internal/app/app.go`).

## 2. Queries parametrizadas e ausência de SQL concatenado

- O acesso a dados usa sqlc (`backend/queries/*.sql` → `internal/store`), sempre com parâmetros (`$n`, `sqlc.arg`).
- Três consultas escritas à mão, todas constantes com parâmetros posicionais:
  - `internal/obligation/progress.go`: `moduleProgressSQL`;
  - `internal/dashboard/backlog.go`: `backlogSQL`. A busca livre chega como parâmetro; `cnpj.Sanitize` só filtra dígitos;
  - `internal/loadgen/loadgen.go` e `cmd/loadgen`: SQL literal da massa de carga, só no banco `controle_perf`.
- Busca por concatenação: `grep -rnE 'Sprintf\([^)]*(SELECT|INSERT|UPDATE|DELETE|WHERE|FROM)|"(SELECT|INSERT|UPDATE|DELETE)[^"]*"\s*\+|\+\s*"\s*(WHERE|AND|ORDER|FROM)' --include='*.go' backend/internal backend/cmd | grep -v _test` não devolve nada.
- A única montagem dinâmica de SQL está em `internal/testutil/postgres.go`: `CREATE/DROP DATABASE` com nomes gerados internamente pelos testes, sem entrada do usuário. É código de teste e não entra no binário da API.

## 3. CORS explícito

- `httpx.CORS(allowed)` (`internal/platform/httpx/middleware.go`) só devolve `Access-Control-Allow-Origin` para origens listadas. Preflight de origem desconhecida recebe `403 CORS_FORBIDDEN`.
- `config.Load` recusa `*` em `CORS_ORIGINS`, com erro na inicialização. Métodos e cabeçalhos permitidos são fixos, sem credenciais (cookies).
- Os testes de `internal/platform/httpx` e `config` cobrem a origem permitida, a origem negada e a recusa do `*`.

## 4. Validação de payload

- Todo corpo JSON passa por `httpx.DecodeJSON`: exige `Content-Type` JSON, limita o corpo a 1 MiB (`MaxJSONBody`), recusa campos desconhecidos (`DisallowUnknownFields`) e mais de um objeto. O erro sai no envelope `400`.
- Handlers que decodificam JSON e validam as regras: `calendar`, `catalog`, `company` (cadastro, regime, configurações), `credential`, `importer`, `obligation` (status, detalhes, lote), `period` e `user` usam `validate.Struct` (go-playground/validator) no handler ou no serviço. `platform/settings` usa `Thresholds.Validate()`.
- Parâmetros de rota e query são validados no servidor: competência (`competence.Parse`), UUIDs, filtros de grade (`grid.ParseFilters`) e módulo de exportação (`404 EXPORT_NOT_FOUND`).
- O CNPJ é validado pelos dígitos verificadores na API (`platform/cnpj`). O bypass dos CNPJs fictícios existe só no seed.
- Um erro não tratado vira `500 INTERNAL_ERROR` com mensagem genérica: detalhes internos não vão ao cliente (`httpx.WriteError`).

## 5. Logs sem conteúdo de arquivos ou dados sensíveis

- O log de acesso (`httpx.AccessLog`) registra método, caminho, status, bytes e duração. Não registra corpo de requisição ou resposta nem query string.
- Os demais logs (`grep -rn 'slog\.\|\.Info(\|\.Warn(\|\.Error(' --include='*.go' backend/internal backend/cmd`) registram só mensagens de erro e metadados: agendador de competência, health, remoção de documento substituído e erro interno. Nenhum registra conteúdo de arquivo, planilha importada ou corpo de requisição.
- O `request_id` é gerado com `crypto/rand` ou reaproveitado se for válido (máximo de 64 caracteres, alfanumérico, `-` e `_`), o que impede injeção de linhas no log.

## 6. Limites de upload e armazenamento fora do webroot

| Entrada | Limite | Onde |
|---|---|---|
| Corpo JSON | 1 MiB | `httpx.MaxJSONBody` |
| Documento de procuração/certificado | 10 MiB (`MaxBytesReader` + `LimitReader`) | `credential.MaxFileSize`, `credential/handler.go` |
| Planilha de importação | 10 MiB (`MaxBytesReader` + `LimitReader`) | `importer.MaxUploadSize`, `importer/handler.go` |
| Nome de arquivo | 200 caracteres; só o nome-base (`filepath.Base`) | `credential/upload.go` |

- Os documentos ficam em `FILE_STORAGE_DIR` (padrão `./storage`, em dev montado em `/app/storage`), fora de qualquer diretório servido. Nenhuma rota serve esse diretório.
- O nome no disco é uma chave aleatória (`newStorageKey`), nunca o nome enviado. A escrita é atômica (temporário `0600` + `rename`), o que impede path traversal. `storage/` está no `.gitignore`.
- O arquivo só é gravado depois de passar pela validação de tipo e conteúdo (T-M4-08).
- A prévia de importação não grava arquivo: o conteúdo classificado fica em `import_previews`, com token UUID e validade de 1 h (`410` depois disso).

## 7. Sem segredo digital (RN-018)

- Nenhuma tabela tem senha, chave privada ou segredo. `migrations/0008_credentials_settings.sql` documenta a regra, e `grep -rniE "password|senha|private_key|secret" backend/migrations backend/queries` só encontra esse comentário.
- São recusados antes da gravação, com a resposta `PRIVATE_KEY_REJECTED`, os contêineres PKCS#12 (`.pfx`/`.p12`, reconhecidos pela extensão ou pela assinatura) e qualquer bloco `PRIVATE KEY` (PEM, inclusive PKCS#8 e chave cifrada). Os demais formatos também são recusados, entre eles uma chave DER solta: a procuração só aceita PDF, PNG ou JPEG, e o certificado só aceita X.509 que o `crypto/x509` consiga ler (`credential/validate.go`). Evidências: `TestCA020_RejectPrivateKey` (`credential/validate_test.go`), `TestCA020_UploadEndpointRejectsPrivateKeys` (`credential/upload_test.go`) e o E2E `credentials-uc05.spec.ts`.
- As chaves usadas nos testes são sintéticas, geradas em tempo de execução. Não há `BEGIN ... PRIVATE KEY` versionado no repositório.
- As exportações XLSX gravam os valores como texto ou número, nunca como fórmula. A leitura das planilhas importadas usa o valor bruto da célula, sem avaliar fórmulas.

## 8. Vulnerabilidades conhecidas

Comando reproduzível: `make audit` (`govulncheck ./...` + `npm audit --audit-level=high` em `frontend` e `e2e`). O job `audit` do CI roda o mesmo comando.

| Ferramenta | Resultado inicial | Ação | Resultado final |
|---|---|---|---|
| `govulncheck` v1.8.0 | GO-2026-6253 (`github.com/moby/go-archive` v0.2.0, via testcontainers, só testes) | atualizado para v0.3.0 | corrigido |
| `govulncheck` | GO-2026-5061 (`golang.org/x/image` v0.41.0, via maroto) | atualizado para v0.45.0 (também fecha a GO-2026-6222) | corrigido |
| `govulncheck` | GO-2026-5932 (`golang.org/x/crypto` v0.57.0, sem versão corrigida) | nenhuma possível; o código não chama a função afetada | monitorar |
| `npm audit` (frontend) | 0 vulnerabilidades | — | OK |
| `npm audit` (e2e) | 0 vulnerabilidades | — | OK |

Saída final: `Your code is affected by 0 vulnerabilities.`

## 9. Recomendações (severidade baixa, para a T-M7-06)

1. **Senha de exemplo em produção — aplicada (T-M7-06).** `docker-compose.prod.yml` exige `POSTGRES_USER` e `POSTGRES_PASSWORD` com `${VAR:?}`, sem valor padrão; `.env.prod.example` vem com a senha vazia; a porta do Postgres não é publicada [AGENTS §6.3].
2. **Cabeçalhos de segurança — aplicada (T-M7-06).** O middleware `httpx.SecurityHeaders` envia `X-Content-Type-Options: nosniff`, `Referrer-Policy: same-origin` e `X-Frame-Options: DENY` em todas as respostas, da API e da SPA servida pelo binário.
3. **Imagem final — aplicada (T-M7-06).** Usuário não-root (UID 10001) e `FILE_STORAGE_DIR=/data/storage` num volume separado dos estáticos (`/app/web`).
4. **x/crypto GO-2026-5932:** refazer `make audit` quando sair a correção e atualizar o módulo.
