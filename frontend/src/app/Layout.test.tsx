import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { renderWithProviders } from "@/test/render";
import { routes } from "./router";

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("Layout", () => {
  it("exibe as 14 áreas no menu (12 do PRD + Fator R + Conciliação) e navega entre elas", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn<typeof fetch>().mockResolvedValue(new Response(JSON.stringify({ status: "ok", database: "ok" }), { status: 200 })),
    );
    renderWithProviders(<></>, { route: "/dashboard", routes });

    const nav = screen.getByRole("navigation");
    expect(within(nav).getAllByRole("link")).toHaveLength(14);
    expect(within(nav).getByRole("link", { name: "Conciliação" })).toHaveAttribute("href", "/reconciliation");
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
