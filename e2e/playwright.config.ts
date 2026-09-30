import { defineConfig, devices } from "@playwright/test";

/**
 * E2E contra uma instância própria da API (porta E2E_API_PORT) apontada para o
 * banco controle_e2e e uma SPA própria (porta E2E_WEB_PORT). Nunca usa o banco de dev.
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
        FILE_STORAGE_DIR: process.env.E2E_STORAGE_DIR ?? "../e2e/.cache/storage",
        LOG_LEVEL: "warn",
        APP_FIXED_TIME: fixedTime,
      },
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
