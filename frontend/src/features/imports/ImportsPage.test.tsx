import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ImportPreview } from "@/services/imports";
import { jsonResponse } from "@/test/fetch";
import { mockFetchRoutes } from "@/test/fetchRouter";
import { renderWithProviders } from "@/test/render";
import { ImportsPage } from "./ImportsPage";

afterEach(() => {
  vi.unstubAllGlobals();
});

const TOKEN = "5a4f7c1e-2b3d-4e5f-8a9b-0c1d2e3f4a5b";
const EXISTING = "9f8e7d6c-5b4a-4321-8fed-cba987654321";

const preview: ImportPreview = {
  token: TOKEN,
  file_name: "empresas.csv",
  expires_at: "2026-10-01T13:00:00Z",
  unknown_headers: [],
  summary: { total: 3, importable: 2, by_status: { NEW: 1, EXISTING: 1, INVALID_CNPJ: 1, DUPLICATE_CODE: 0, MISSING_REQUIRED: 0, INVALID_VALUE: 0 } },
  rows: [
    { line: 2, status: "NEW", status_label: "Nova empresa", errors: [], internal_code: "401", legal_name: "Nova Alfa Ltda", cnpj: "00000991000150", existing_company_id: null },
    {
      line: 3,
      status: "INVALID_CNPJ",
      status_label: "CNPJ inválido",
      errors: [{ field: "cnpj", column: "CNPJ", message: "CNPJ inválido: confira os 14 dígitos e os dígitos verificadores." }],
      internal_code: "402",
      legal_name: "CNPJ Errado Ltda",
      cnpj: "00000001000199",
      existing_company_id: null,
    },
    { line: 4, status: "EXISTING", status_label: "Empresa existente", errors: [], internal_code: "499", legal_name: "Empresa Existente Ltda", cnpj: "00000999000125", existing_company_id: EXISTING },
  ],
};

function mockImportApi(commit: (body: unknown) => Response) {
  const bodies: unknown[] = [];
  mockFetchRoutes(({ method, url, body }) => {
    if (method === "POST" && url.pathname === "/api/v1/imports/companies/preview") return jsonResponse(preview);
    if (method === "POST" && url.pathname === "/api/v1/imports/companies/commit") {
      bodies.push(body);
      return commit(body);
    }
    return undefined;
  });
  return bodies;
}

async function uploadSheet(user: ReturnType<typeof userEvent.setup>) {
  expect(screen.getByRole("link", { name: /Baixar modelo/ })).toHaveAttribute("href", "/api/v1/imports/companies/template");
  await user.upload(screen.getByLabelText(/Planilha \(\.xlsx ou \.csv/), new File(["x"], "empresas.csv", { type: "text/csv" }));
  await user.click(screen.getByRole("button", { name: "Validar planilha" }));
  return screen.findByRole("table", { name: "Prévia da importação" });
}

describe("ImportsPage", () => {
  it("mostra a prévia com erros por linha, filtra por classificação e importa só as válidas + existentes confirmadas", async () => {
    const bodies = mockImportApi(() =>
      jsonResponse({
        created: 1,
        updated: 1,
        skipped_existing: 0,
        skipped_invalid: 1,
        companies: [
          { line: 2, company_id: "11111111-1111-4111-8111-111111111111", internal_code: "401", legal_name: "Nova Alfa Ltda", action: "CREATED" },
          { line: 4, company_id: EXISTING, internal_code: "499", legal_name: "Empresa Existente Ltda", action: "UPDATED" },
        ],
      }),
    );
    const user = userEvent.setup();
    renderWithProviders(<ImportsPage />);

    const table = await uploadSheet(user);
    expect(within(table).getByText("CNPJ inválido: confira os 14 dígitos e os dígitos verificadores.")).toBeInTheDocument();
    expect(screen.getByText(/1 com erro \(não serão importadas\)/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Importar 1 empresa" })).toBeEnabled();

    await user.click(screen.getByRole("button", { name: "CNPJ inválido (1)" }));
    expect(within(table).getAllByRole("row")).toHaveLength(2); // cabeçalho + 1
    await user.click(screen.getByRole("button", { name: "Todas (3)" }));

    await user.click(screen.getByRole("checkbox", { name: "Atualizar o cadastro de Empresa Existente Ltda (linha 4)" }));
    await user.click(screen.getByRole("button", { name: "Importar 2 empresas" }));

    expect(await screen.findByText("Importação concluída")).toBeInTheDocument();
    expect(bodies).toEqual([{ token: TOKEN, update_existing_rows: [4] }]);
    expect(screen.getByRole("link", { name: "401 — Nova Alfa Ltda" })).toHaveAttribute("href", "/companies/11111111-1111-4111-8111-111111111111");
    expect(screen.getByText(/1 com erro e 0 existente\(s\) sem confirmação ficaram de fora/)).toBeInTheDocument();
  });

  it("prévia expirada (410) mostra a mensagem e permite enviar de novo", async () => {
    mockImportApi(() => jsonResponse({ code: "IMPORT_TOKEN_EXPIRED", message: "A prévia expirou (validade de 1 hora). Envie a planilha de novo." }, 410));
    const user = userEvent.setup();
    renderWithProviders(<ImportsPage />);
    await uploadSheet(user);
    await user.click(screen.getByRole("button", { name: "Importar 1 empresa" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("A prévia expirou");
    await user.click(screen.getByRole("button", { name: "Enviar outra planilha" }));
    await waitFor(() => expect(screen.getByRole("button", { name: "Validar planilha" })).toBeDisabled());
  });
});
