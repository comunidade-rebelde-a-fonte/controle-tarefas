import { vi } from "vitest";

/**
 * O jsdom não faz layout: a área rolável da DataGrid mediria 0 px e a virtualização não
 * renderizaria linhas. Simula uma viewport de 1280 × 600. Use em beforeAll e chame o
 * retorno em afterAll.
 */
export function mockGridLayout(): () => void {
  const spies = [
    vi.spyOn(HTMLElement.prototype, "offsetHeight", "get").mockReturnValue(600),
    vi.spyOn(HTMLElement.prototype, "offsetWidth", "get").mockReturnValue(1280),
    vi.spyOn(HTMLElement.prototype, "clientHeight", "get").mockReturnValue(600),
    vi.spyOn(HTMLElement.prototype, "scrollHeight", "get").mockReturnValue(100_000),
  ];
  return () => {
    for (const s of spies) s.mockRestore();
  };
}
