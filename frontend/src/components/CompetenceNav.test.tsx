import { fireEvent, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { jsonResponse } from "@/test/fetch";
import { mockFetchRoutes } from "@/test/fetchRouter";
import { renderWithProviders } from "@/test/render";
import { CompetenceNav } from "./CompetenceNav";

afterEach(() => {
  vi.unstubAllGlobals();
});

function mockLatest(competence: string) {
  return mockFetchRoutes(({ method, url }) =>
    method === "GET" && url.pathname === "/api/v1/competencies/latest" ? jsonResponse({ competence, current: competence }) : undefined,
  );
}

function renderNav(route: string) {
  return renderWithProviders(<CompetenceNav />, { route, path: "/fiscal" });
}

const search = (router: ReturnType<typeof renderNav>["router"]) => new URLSearchParams(router.state.location.search);

/** Espera a competência mais recente carregar (define o limite da navegação). */
async function latestLoaded() {
  await waitFor(() => expect(screen.getByLabelText("Selecionar competência")).toHaveAttribute("max"));
}

describe("CompetenceNav", () => {
  it("sem competência na URL, usa a mais recente da API", async () => {
    mockLatest("2026-09");
    renderNav("/fiscal");
    expect(await screen.findByRole("button", { name: "Competência anterior: 08/2026" })).toBeInTheDocument();
    expect(screen.getByLabelText("Selecionar competência")).toHaveValue("2026-09");
    expect(screen.getByRole("button", { name: "Próxima competência: 10/2026" })).toBeDisabled();
  });

  it("navegar para a anterior e a próxima atualiza a URL, preservando os filtros", async () => {
    mockLatest("2026-09");
    const user = userEvent.setup();
    const { router } = renderNav("/fiscal?competence=2026-07&group=simples");
    await latestLoaded();

    await user.click(screen.getByRole("button", { name: "Competência anterior: 06/2026" }));
    expect(search(router).get("competence")).toBe("2026-06");
    expect(search(router).get("group")).toBe("simples");

    await user.click(screen.getByRole("button", { name: "Próxima competência: 07/2026" }));
    await user.click(screen.getByRole("button", { name: "Próxima competência: 08/2026" }));
    expect(search(router).get("competence")).toBe("2026-08");
    expect(screen.getByLabelText("Selecionar competência")).toHaveValue("2026-08");
  });

  it("atravessa a virada de ano pelos botões e pelo seletor direto", async () => {
    mockLatest("2027-03");
    const user = userEvent.setup();
    const { router } = renderNav("/fiscal?competence=2027-01");
    await latestLoaded();

    await user.click(screen.getByRole("button", { name: "Competência anterior: 12/2026" }));
    expect(search(router).get("competence")).toBe("2026-12");
    await user.click(screen.getByRole("button", { name: "Próxima competência: 01/2027" }));
    expect(search(router).get("competence")).toBe("2027-01");

    fireEvent.change(screen.getByLabelText("Selecionar competência"), { target: { value: "2025-12" } });
    expect(search(router).get("competence")).toBe("2025-12");
    expect(screen.getByRole("button", { name: "Competência anterior: 11/2025" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Próxima competência: 01/2026" })).toBeEnabled();
  });

  it("não avança além da mais recente e 'Mais recente' volta para ela", async () => {
    mockLatest("2026-09");
    const user = userEvent.setup();
    const { router } = renderNav("/fiscal?competence=2026-03");

    await latestLoaded();
    const input = screen.getByLabelText("Selecionar competência");
    expect(input).toHaveAttribute("max", "2026-09");
    fireEvent.change(input, { target: { value: "2026-11" } });
    expect(search(router).get("competence")).toBe("2026-03");

    await user.click(screen.getByRole("button", { name: "Mais recente" }));
    expect(search(router).get("competence")).toBe("2026-09");
    expect(screen.getByRole("button", { name: "Mais recente" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Próxima competência: 10/2026" })).toBeDisabled();
  });

  it("ignora competência inválida na URL e usa a mais recente", async () => {
    mockLatest("2026-09");
    renderNav("/fiscal?competence=2026-13");
    expect(await screen.findByLabelText("Selecionar competência")).toHaveValue("2026-09");
  });
});
