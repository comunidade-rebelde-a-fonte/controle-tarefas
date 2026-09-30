import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { jsonResponse } from "@/test/fetch";
import { mockFetchRoutes, usersRoute } from "@/test/fetchRouter";
import { renderWithProviders } from "@/test/render";
import { CompanyFormDialog } from "./CompanyForm";

afterEach(() => {
  vi.unstubAllGlobals();
});

function renderCreate() {
  return renderWithProviders(<CompanyFormDialog open onOpenChange={() => undefined} />, { route: "/companies", path: "/companies" });
}

async function fillValid(user: ReturnType<typeof userEvent.setup>) {
  await user.type(screen.getByLabelText(/Código interno/), "301");
  await user.type(screen.getByLabelText(/CNPJ/), "11222333000181");
  await user.type(screen.getByLabelText(/Razão social/), "Nova Empresa Ltda");
  await user.type(screen.getByLabelText(/Início do atendimento/), "2026-10-01");
}

describe("CompanyForm", () => {
  it("CA-001 valida os campos obrigatórios antes de enviar", async () => {
    const fetchFn = mockFetchRoutes(usersRoute);
    const user = userEvent.setup();
    renderCreate();

    await user.click(screen.getByRole("button", { name: "Cadastrar empresa" }));

    expect(await screen.findByText("Informe o código interno.")).toBeInTheDocument();
    expect(screen.getByText("Informe a razão social.")).toBeInTheDocument();
    expect(screen.getByText("O CNPJ deve ter 14 dígitos.")).toBeInTheDocument();
    expect(screen.getByText("Informe uma data válida.")).toBeInTheDocument();
    expect(screen.getByLabelText(/Razão social/)).toHaveAttribute("aria-invalid", "true");
    expect(fetchFn.mock.calls.every((c) => (c[1]?.method ?? "GET") === "GET")).toBe(true);
  });

  it("aplica a máscara de CNPJ e envia só os dígitos", async () => {
    let sent: unknown;
    mockFetchRoutes(usersRoute, ({ method, url, body }) => {
      if (method === "POST" && url.pathname === "/api/v1/companies") {
        sent = body;
        return jsonResponse({ id: "new-id" }, 201);
      }
      return undefined;
    });
    const user = userEvent.setup();
    renderCreate();
    await fillValid(user);
    expect(screen.getByLabelText(/CNPJ/)).toHaveValue("11.222.333/0001-81");

    await user.click(screen.getByRole("button", { name: "Cadastrar empresa" }));

    await waitFor(() => {
      expect(sent).toMatchObject({ cnpj: "11222333000181", internal_code: "301", tax_regime: "SIMPLES_NACIONAL", trade_name: null });
    });
  });

  it("CA-002 exibe a empresa conflitante com link para a ficha", async () => {
    mockFetchRoutes(usersRoute, ({ method, url }) =>
      method === "POST" && url.pathname === "/api/v1/companies"
        ? jsonResponse(
            {
              code: "CNPJ_CONFLICT",
              message: "Já existe uma empresa com este CNPJ: 101 — Empresa Fictícia Alfa Comércio Ltda.",
              details: { field: "cnpj", conflicting_company: { id: "c-101", internal_code: "101", legal_name: "Empresa Fictícia Alfa Comércio Ltda" } },
            },
            409,
          )
        : undefined,
    );
    const user = userEvent.setup();
    renderCreate();
    await fillValid(user);
    await user.click(screen.getByRole("button", { name: "Cadastrar empresa" }));

    expect(await screen.findByText(/Já existe uma empresa com este CNPJ/)).toBeInTheDocument();
    const link = screen.getByRole("link", { name: /Abrir ficha de 101 — Empresa Fictícia Alfa Comércio Ltda/ });
    expect(link).toHaveAttribute("href", "/companies/c-101");
  });

  it("mostra no campo o erro de validação devolvido pela API", async () => {
    mockFetchRoutes(usersRoute, ({ method, url }) =>
      method === "POST" && url.pathname === "/api/v1/companies"
        ? jsonResponse({ code: "VALIDATION_ERROR", message: "Dados inválidos.", details: { fields: { cnpj: "CNPJ inválido: confira os dígitos verificadores." } } }, 400)
        : undefined,
    );
    const user = userEvent.setup();
    renderCreate();
    await fillValid(user);
    await user.click(screen.getByRole("button", { name: "Cadastrar empresa" }));
    expect(await screen.findByText("CNPJ inválido: confira os dígitos verificadores.")).toBeInTheDocument();
  });
});
