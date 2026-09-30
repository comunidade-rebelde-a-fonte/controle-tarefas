import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterAll, afterEach, beforeAll, describe, expect, it, vi } from "vitest";
import type { CredentialControl, CredentialRow } from "@/services/credentials";
import { jsonResponse } from "@/test/fetch";
import { mockFetchRoutes } from "@/test/fetchRouter";
import { mockGridLayout } from "@/test/gridLayout";
import { renderWithProviders } from "@/test/render";
import { CredentialsPage } from "./CredentialsPage";

let restoreLayout: () => void;
beforeAll(() => {
  restoreLayout = mockGridLayout();
});
afterAll(() => restoreLayout());
afterEach(() => {
  vi.unstubAllGlobals();
});

const control = (patch: Partial<CredentialControl>): CredentialControl => ({
  type: "RFB_POWER_OF_ATTORNEY",
  expires_at: null,
  renewal_status: "CURRENT",
  renewal_label: "Vigente",
  situation: { code: "NO_DATE", label: "Sem data cadastrada", threshold_days: null, days_left: null },
  notes: null,
  file: null,
  version: 0,
  updated_at: null,
  ...patch,
});

const ALFA = "11111111-1111-4111-8111-111111111111";
const BETA = "22222222-2222-4222-8222-222222222222";

const rows: CredentialRow[] = [
  {
    company: { id: ALFA, internal_code: "101", legal_name: "Empresa Fictícia Alfa Comércio Ltda", trade_name: "Alfa", cnpj: "00000001000101", tax_regime: "LUCRO_PRESUMIDO" },
    rfb_power_of_attorney: control({
      expires_at: "2026-10-05",
      version: 3,
      situation: { code: "DUE_SOON", label: "Vence em até 7 dias", threshold_days: 7, days_left: 4 },
      notes: "Procuração eletrônica",
    }),
    digital_certificate: control({ type: "DIGITAL_CERTIFICATE" }),
  },
  {
    company: { id: BETA, internal_code: "102", legal_name: "Empresa Fictícia Beta Serviços Ltda", trade_name: null, cnpj: "00000002000102", tax_regime: "LUCRO_REAL" },
    rfb_power_of_attorney: control({}),
    digital_certificate: control({
      type: "DIGITAL_CERTIFICATE",
      expires_at: "2026-09-15",
      version: 1,
      situation: { code: "EXPIRED", label: "Vencido", threshold_days: null, days_left: -16 },
    }),
  },
];

interface Captured {
  method: string;
  path: string;
  query: string;
  body: unknown;
}

function mockApi(overrides: { put?: (body: unknown) => Response; upload?: () => Response } = {}) {
  const calls: Captured[] = [];
  mockFetchRoutes(({ method, url, body }) => {
    calls.push({ method, path: url.pathname, query: url.search, body });
    if (method === "GET" && url.pathname === "/api/v1/credential-controls") {
      const filter = url.searchParams.get("filter");
      const items = filter === "expired" ? rows.filter((r) => r.digital_certificate.situation.code === "EXPIRED") : rows;
      return jsonResponse({ items, total: items.length, thresholds: [7, 15, 30, 60] });
    }
    if (method === "GET" && url.pathname === `/api/v1/companies/${ALFA}/credential-controls`) return jsonResponse(rows[0]);
    if (method === "PUT" && url.pathname.endsWith("/file")) return overrides.upload?.() ?? jsonResponse(rows[0]?.digital_certificate);
    if (method === "PUT" && url.pathname.startsWith("/api/v1/companies/")) return overrides.put?.(body) ?? jsonResponse(rows[0]?.rfb_power_of_attorney);
    return undefined;
  });
  return calls;
}

function renderPage(route = "/credentials") {
  return renderWithProviders(<CredentialsPage />, { route, path: "/credentials" });
}

describe("CredentialsPage", () => {
  it("mostra as colunas do §33 com situação em texto e ícone", async () => {
    mockApi();
    renderPage();
    const grid = await screen.findByRole("grid", { name: "Procurações e Certificados" });
    const headers = within(grid).getAllByRole("columnheader").map((h) => h.textContent);
    expect(headers).toEqual([
      "Empresa",
      "Código",
      "CNPJ",
      "Regime",
      "Vencimento procuração",
      "Situação procuração",
      "Renovação procuração",
      "Vencimento certificado",
      "Situação certificado",
      "Renovação certificado",
      "Observação",
      "Documentos",
    ]);
    expect(await within(grid).findByText("Vence em até 7 dias")).toBeInTheDocument();
    expect(within(grid).getByText("Vencido")).toBeInTheDocument();
    expect(within(grid).getByText("00.000.001/0001-01")).toBeInTheDocument();
    expect(within(grid).getByText("Procuração: Procuração eletrônica")).toBeInTheDocument();
  });

  it("filtro rápido vai para a URL e para a API", async () => {
    const calls = mockApi();
    const user = userEvent.setup();
    const { router } = renderPage();
    await screen.findByRole("grid");
    await user.click(screen.getByRole("button", { name: "Vencidos" }));
    expect(new URLSearchParams(router.state.location.search).get("filter")).toBe("expired");
    await waitFor(() => expect(calls.some((c) => c.query === "?filter=expired")).toBe(true));
    expect(screen.getByRole("button", { name: "Vencidos" })).toHaveAttribute("aria-pressed", "true");
    expect(await screen.findAllByRole("rowheader")).toHaveLength(1);
    expect(screen.getByRole("button", { name: "Vencem em até 30 dias" })).toBeInTheDocument();
  });

  it("busca livre no cliente por CNPJ sem máscara", async () => {
    mockApi();
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("grid");
    await user.type(screen.getByRole("searchbox", { name: "Buscar empresa" }), "00000002000102");
    await waitFor(() => expect(screen.getAllByRole("rowheader").map((h) => h.textContent)).toEqual(["Empresa Fictícia Beta Serviços Ltda"]));
  });

  it("muda a etapa na célula e grava mantendo os demais campos", async () => {
    const calls = mockApi();
    const user = userEvent.setup();
    renderPage();
    const select = await screen.findByLabelText("Etapa de renovação — Procuração RFB — Empresa Fictícia Alfa Comércio Ltda");
    await user.selectOptions(select, "Aguardando cliente");
    await waitFor(() => expect(calls.some((c) => c.method === "PUT")).toBe(true));
    const put = calls.find((c) => c.method === "PUT");
    expect(put?.path).toBe(`/api/v1/companies/${ALFA}/credential-controls/RFB_POWER_OF_ATTORNEY`);
    expect(put?.body).toEqual({ version: 3, expires_at: "2026-10-05", renewal_status: "AWAITING_CLIENT", notes: "Procuração eletrônica" });
  });

  it("Renovado pede a nova validade antes de gravar (UC-05)", async () => {
    const calls = mockApi({
      put: () => jsonResponse(control({ expires_at: "2027-10-05", version: 4, situation: { code: "CURRENT", label: "Vigente", threshold_days: null, days_left: 370 } })),
    });
    const user = userEvent.setup();
    renderPage();
    await user.selectOptions(await screen.findByLabelText("Etapa de renovação — Procuração RFB — Empresa Fictícia Alfa Comércio Ltda"), "Renovado");

    const dialog = await screen.findByRole("dialog", { name: "Renovar procuração rfb" });
    expect(calls.some((c) => c.method === "PUT")).toBe(false);
    await user.click(within(dialog).getByRole("button", { name: "Confirmar renovação" }));
    expect(within(dialog).getByText("Informe a nova data de validade.")).toBeInTheDocument();

    await user.type(within(dialog).getByLabelText(/Nova data de validade/), "2027-10-05");
    await user.click(within(dialog).getByRole("button", { name: "Confirmar renovação" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    const put = calls.find((c) => c.method === "PUT");
    expect(put?.body).toEqual({ version: 3, expires_at: "2027-10-05", renewal_status: "RENEWED", notes: "Procuração eletrônica" });
    expect(await screen.findByText("Renovação registrada")).toBeInTheDocument();
  });

  it("CA-020: upload recusado mostra a mensagem do backend", async () => {
    mockApi({
      upload: () => jsonResponse({ code: "PRIVATE_KEY_REJECTED", message: "Chaves privadas não são armazenadas pelo sistema." }, 422),
    });
    // `accept` é só dica ao seletor de arquivos: o usuário pode escolher "Todos os arquivos".
    const user = userEvent.setup({ applyAccept: false });
    renderPage();
    await user.click(await screen.findByRole("button", { name: /Documentos e observações — Empresa Fictícia Alfa Comércio Ltda/ }));
    const dialog = await screen.findByRole("dialog", { name: "Documentos — Empresa Fictícia Alfa Comércio Ltda" });
    const input = within(dialog).getByLabelText(/Enviar documento — Certificado digital/);
    await user.upload(input, new File([new Uint8Array([0x30, 0x80, 0x02, 0x01, 0x03])], "certificado.pfx", { type: "application/x-pkcs12" }));
    expect(await within(dialog).findByRole("alert")).toHaveTextContent("Chaves privadas não são armazenadas pelo sistema.");
  });
});
