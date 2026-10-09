import { screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { renderWithProviders } from "@/test/render";
import { FactorRLayout } from "./FactorRLayout";

function render(route: string) {
  return renderWithProviders(<FactorRLayout />, {
    route,
    routes: [
      {
        path: "/factor-r",
        element: <FactorRLayout />,
        children: [
          { index: true, element: <h1>Carteira</h1> },
          { path: "inbox/:id", element: <h1>Documento</h1> },
          { path: "agents", element: <h1>Agentes</h1> },
        ],
      },
    ],
  });
}

describe("FactorRLayout", () => {
  it("D-025: submenu da área Fator R; carteira e agentes levam o PA escolhido", async () => {
    render("/factor-r/agents?pa=2026-10");
    expect(await screen.findByRole("heading", { name: "Agentes" })).toBeInTheDocument();
    const nav = screen.getByRole("navigation", { name: "Seções do Fator R" });
    expect(nav).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Agentes" })).toHaveAttribute("aria-current", "page");
    expect(screen.getByRole("link", { name: "Carteira" })).toHaveAttribute("href", "/factor-r?pa=2026-10");
    expect(screen.getByRole("link", { name: "Carteira" })).not.toHaveAttribute("aria-current");
    expect(screen.getByRole("link", { name: "Inbox PGDAS-D" })).toHaveAttribute("href", "/factor-r/inbox");
    expect(screen.getByRole("link", { name: "Observabilidade" })).toHaveAttribute("href", "/factor-r/runs");
  });

  it("o detalhe de um documento mantém a seção Inbox ativa", async () => {
    render("/factor-r/inbox/abc");
    expect(await screen.findByRole("heading", { name: "Documento" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Inbox PGDAS-D" })).toHaveAttribute("aria-current", "page");
    expect(screen.getByRole("link", { name: "Carteira" })).toHaveAttribute("href", "/factor-r");
  });
});
