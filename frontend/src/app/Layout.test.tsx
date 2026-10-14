import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { renderWithProviders } from "@/test/render";
import { routes } from "./router";

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("Layout", () => {
  it("exibe as 15 áreas no menu, na ordem aprovada e nos grupos (TASK-029), e navega entre elas", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn<typeof fetch>().mockResolvedValue(new Response(JSON.stringify({ status: "ok", database: "ok" }), { status: 200 })),
    );
    renderWithProviders(<></>, { route: "/dashboard", routes });

    const nav = screen.getByRole("navigation");
    expect(within(nav).getAllByRole("link")).toHaveLength(15);
    // A ordem aprovada da navegação consolidada (08 §2) e a Importação/Exportação no fim.
    expect(within(nav).getAllByRole("link").map((l) => l.textContent)).toEqual([
      "Dashboard",
      "Empresas",
      "Fiscal",
      "Declarações",
      "Contábil",
      "Folha",
      "DCTFWeb",
      "Fator R",
      "Conciliação",
      "Procurações e Certificados",
      "Backlog",
      "Documentos",
      "Relatórios",
      "Cadastros e Configurações",
      "Importação/Exportação",
    ]);
    for (const group of ["Visão geral", "Operação mensal", "Fator R e Conciliação", "Acompanhamento", "Sistema"]) {
      expect(within(nav).getByText(group)).toBeInTheDocument();
    }
    expect(screen.getByRole("img", { name: "InovaSala by Ceath" })).toBeInTheDocument();
    expect(within(nav).getByRole("link", { name: "Dashboard" })).toHaveAttribute("aria-current", "page");
    expect(within(nav).getByRole("link", { name: "Conciliação" })).toHaveAttribute("href", "/reconciliation");
    expect(within(nav).getByRole("link", { name: "Documentos" })).toHaveAttribute("href", "/documents");
    expect(await screen.findByRole("status", { name: "API disponível" })).toBeInTheDocument();

    await userEvent.click(within(nav).getByRole("link", { name: "Empresas" }));
    expect(screen.getByRole("heading", { level: 1, name: "Empresas" })).toBeInTheDocument();
  });

  it("indica API indisponível com texto, não só cor", async () => {
    vi.stubGlobal("fetch", vi.fn<typeof fetch>().mockRejectedValue(new TypeError("offline")));
    renderWithProviders(<></>, { route: "/dashboard", routes });
    expect(await screen.findByRole("status", { name: "API indisponível" })).toHaveTextContent("API indisponível");
  });
});
