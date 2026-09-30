import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { jsonResponse, requestBody } from "@/test/fetch";
import { mockFetchRoutes } from "@/test/fetchRouter";
import { renderWithProviders } from "@/test/render";
import { NoteCell } from "./NoteCell";

afterEach(() => {
  vi.unstubAllGlobals();
});

const period = (note: string | null, version: number) => ({
  id: "p1", company_id: "c1", competence: "2026-09", module: "FISCAL", state: "OPEN", closed_at: null, closed_by: null, general_note: note, version,
});

describe("NoteCell", () => {
  it("edita inline e salva com Enter (otimista)", async () => {
    const user = userEvent.setup();
    let resolve: (r: Response) => void = () => undefined;
    const fetchFn = vi.fn<typeof fetch>().mockImplementation(() => new Promise<Response>((r) => (resolve = r)));
    vi.stubGlobal("fetch", fetchFn);
    const onSaved = vi.fn();
    renderWithProviders(<NoteCell periodId="p1" version={2} note={null} label="Alfa Comércio" onSaved={onSaved} />);

    await user.click(screen.getByRole("button", { name: "Observação de Alfa Comércio: adicionar" }));
    const input = screen.getByRole("textbox", { name: "Observação de Alfa Comércio" });
    await user.type(input, "  Aguardando retenções {Enter}");

    // Otimista: o texto aparece antes da resposta.
    expect(screen.getByRole("button", { name: "Observação de Alfa Comércio: Aguardando retenções" })).toHaveAttribute("aria-busy", "true");
    expect(requestBody(fetchFn)).toEqual({ version: 2, general_note: "Aguardando retenções" });
    resolve(jsonResponse(period("Aguardando retenções", 3)));
    await waitFor(() => expect(onSaved).toHaveBeenCalled());
    expect(screen.getByRole("button", { name: "Observação de Alfa Comércio: Aguardando retenções" })).not.toHaveAttribute("aria-busy");
  });

  it("Esc cancela sem chamar a API", async () => {
    const user = userEvent.setup();
    const fetchFn = mockFetchRoutes();
    renderWithProviders(<NoteCell periodId="p1" version={2} note="Nota antiga" label="Alfa" />);

    await user.click(screen.getByRole("button", { name: "Observação de Alfa: Nota antiga" }));
    const input = screen.getByRole("textbox", { name: "Observação de Alfa" });
    await user.clear(input);
    await user.type(input, "Outra coisa{Escape}");
    expect(screen.queryByRole("textbox")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Observação de Alfa: Nota antiga" })).toBeInTheDocument();
    expect(fetchFn).not.toHaveBeenCalled();
  });

  it("volta ao texto anterior quando a API recusa", async () => {
    const user = userEvent.setup();
    mockFetchRoutes(({ method, url }) =>
      method === "PATCH" && url.pathname === "/api/v1/company-module-periods/p1/note"
        ? jsonResponse({ code: "VERSION_CONFLICT", message: "Registro alterado por outro usuário.", details: {} }, 409)
        : undefined,
    );
    renderWithProviders(<NoteCell periodId="p1" version={2} note="Nota antiga" label="Alfa" />);

    await user.click(screen.getByRole("button", { name: "Observação de Alfa: Nota antiga" }));
    const input = screen.getByRole("textbox", { name: "Observação de Alfa" });
    await user.clear(input);
    await user.type(input, "Nova{Enter}");
    expect(await screen.findByText("Não foi possível salvar a observação")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Observação de Alfa: Nota antiga" })).toBeInTheDocument();
  });

  it("apagar o texto limpa a observação", async () => {
    const user = userEvent.setup();
    const fetchFn = mockFetchRoutes(({ method }) => (method === "PATCH" ? jsonResponse(period(null, 3)) : undefined));
    renderWithProviders(<NoteCell periodId="p1" version={2} note="Nota" label="Alfa" />);
    await user.click(screen.getByRole("button", { name: "Observação de Alfa: Nota" }));
    await user.clear(screen.getByRole("textbox", { name: "Observação de Alfa" }));
    await user.keyboard("{Enter}");
    await waitFor(() => expect(requestBody(fetchFn)).toEqual({ version: 2, general_note: null }));
  });
});
