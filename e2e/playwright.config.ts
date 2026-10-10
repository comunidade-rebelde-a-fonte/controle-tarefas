import { defineConfig, devices } from "@playwright/test";
import { E2E_GATEWAY_KEY } from "./support/ai";

/**
 * E2E contra uma instância própria da API (porta E2E_API_PORT) e do worker, apontados para o
 * banco controle_e2e, e uma SPA própria (porta E2E_WEB_PORT). Nunca usa o banco de dev.
 * `make e2e` recria o controle_e2e (reset + migrate + seed) antes da suíte, com o relógio
 * fixo em E2E_FIXED_TIME tanto no seed quanto na API.
 */
const apiPort = process.env.E2E_API_PORT ?? "8091";
const webPort = process.env.E2E_WEB_PORT ?? "5174";
const databaseUrl = process.env.E2E_DATABASE_URL ?? "";

if (!/\/controle_e2e(\?|$)/.test(databaseUrl)) {
  throw new Error("E2E_DATABASE_URL deve apontar para o banco controle_e2e (use `make e2e`).");
}
// Relógio fixo da API (APP_FIXED_TIME), o mesmo com que o seed de demonstração foi carregado.
const fixedTime = process.env.E2E_FIXED_TIME ?? "";
if (Number.isNaN(Date.parse(fixedTime))) {
  throw new Error("E2E_FIXED_TIME deve ser um instante RFC 3339, o mesmo do seed (use `make e2e`).");
}

const webURL = `http://localhost:${webPort}`;
// MinIO só do E2E (make e2e: projeto compose controle-e2e), para os originais da Conciliação
// (TASK-021a). Sem ele, o envio da Conciliação responde 503 e os testes dela não rodam.
const s3 = {
  S3_ENDPOINT: process.env.E2E_S3_ENDPOINT ?? "",
  S3_REGION: "us-east-1",
  S3_BUCKET: process.env.E2E_S3_BUCKET ?? "",
  S3_ACCESS_KEY: process.env.E2E_S3_ACCESS_KEY ?? "",
  S3_SECRET_KEY: process.env.E2E_S3_SECRET_KEY ?? "",
};
// Worker dos jobs (TASK-014b): lê os extratos do inbox PGDAS-D enviados pela tela. Mesmo banco e
// mesmo armazenamento da API; o healthcheck dele avisa o Playwright de que está no ar.
const workerHealthPort = process.env.E2E_WORKER_HEALTH_PORT ?? "8094";
const storageDir = process.env.E2E_STORAGE_DIR ?? "../e2e/.cache/storage";

// Gateway LiteLLM simulado (support/fake-litellm.mjs) para a tela Configurações → IA e para os
// agentes do Fator R (LLM determinístico, FR-20). A chave é um valor de teste do E2E, conferido
// pelo simulado: prova que a API usa a chave do gateway.
const litellmPort = process.env.E2E_LITELLM_PORT ?? "8093";
const litellmURL = `http://127.0.0.1:${litellmPort}`;
// Padrões dos aliases como no .env.example (D-015).
const aiModels = {
  TRIADOR_MODEL: "openai/gpt-6-luna",
  EXTRATOR_MODEL: "openai/gpt-6-sol",
  EXTRATOR_FORTE_MODEL: "openai/gpt-6-astra",
  RESOLVEDOR_MODEL: "openai/gpt-6-sol",
  CLASSIFICADOR_MODEL: "openai/gpt-6-sol",
  CONSULTOR_MODEL: "openai/gpt-6-luna",
  PRIORIZADOR_MODEL: "openai/gpt-6-luna",
};

export default defineConfig({
  testDir: "./specs",
  fullyParallel: false,
  workers: 1,
  forbidOnly: !!process.env.CI,
  retries: 0,
  reporter: process.env.CI ? [["list"], ["html", { open: "never" }]] : "list",
  timeout: 30_000,
  expect: { timeout: 7_000 },
  use: {
    baseURL: webURL,
    locale: "pt-BR",
    timezoneId: "America/Sao_Paulo",
    viewport: { width: 1440, height: 900 },
    trace: "retain-on-failure",
  },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"], viewport: { width: 1440, height: 900 } } }],
  webServer: [
    {
      command: "go run ./cmd/api serve",
      cwd: "../backend",
      url: `http://localhost:${apiPort}/api/v1/health`,
      reuseExistingServer: false,
      timeout: 180_000,
      stdout: "ignore",
      stderr: "pipe",
      env: {
        DATABASE_URL: databaseUrl,
        APP_ENV: "test",
        HTTP_PORT: apiPort,
        APP_TIMEZONE: "America/Sao_Paulo",
        CORS_ORIGINS: webURL,
        FILE_STORAGE_DIR: storageDir,
        LOG_LEVEL: "warn",
        APP_FIXED_TIME: fixedTime,
        LITELLM_BASE_URL: litellmURL,
        LITELLM_API_KEY: E2E_GATEWAY_KEY,
        ...aiModels,
        ...s3,
      },
    },
    {
      command: "go run ./cmd/api worker",
      cwd: "../backend",
      url: `http://localhost:${workerHealthPort}/health`,
      reuseExistingServer: false,
      timeout: 180_000,
      stdout: "ignore",
      stderr: "pipe",
      env: {
        DATABASE_URL: databaseUrl,
        APP_ENV: "test",
        APP_TIMEZONE: "America/Sao_Paulo",
        WORKER_ID: "worker-e2e",
        WORKER_HEALTH_PORT: workerHealthPort,
        FILE_STORAGE_DIR: storageDir,
        LOG_LEVEL: "warn",
        // Os documentos da Conciliação nos E2E seguem os leitores determinísticos (sem LLM).
        ...s3,
      },
    },
    {
      command: "node support/fake-litellm.mjs",
      url: `${litellmURL}/health/liveliness`,
      reuseExistingServer: false,
      timeout: 30_000,
      stdout: "ignore",
      stderr: "pipe",
      env: { FAKE_LITELLM_PORT: litellmPort, FAKE_LITELLM_KEY: E2E_GATEWAY_KEY },
    },
    {
      command: "npx vite --strictPort",
      cwd: "../frontend",
      url: webURL,
      reuseExistingServer: false,
      timeout: 120_000,
      stdout: "ignore",
      stderr: "pipe",
      env: {
        VITE_PORT: webPort,
        VITE_API_PROXY_TARGET: `http://localhost:${apiPort}`,
      },
    },
  ],
});
