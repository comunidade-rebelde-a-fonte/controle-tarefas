import { vi } from "vitest";
import { jsonResponse, urlOf } from "./fetch";

export type Route = (req: { method: string; url: URL; body: unknown }) => Response | undefined;

/** Mock de fetch roteado por método + caminho. Rotas devolvem undefined para "não tratei". */
export function mockFetchRoutes(...routes: Route[]) {
  const fn = vi.fn<typeof fetch>().mockImplementation((input, init) => {
    const url = new URL(urlOf(input), "http://localhost");
    const method = (init?.method ?? "GET").toUpperCase();
    const body: unknown = typeof init?.body === "string" ? JSON.parse(init.body) : undefined;
    for (const r of routes) {
      const res = r({ method, url, body });
      if (res) return Promise.resolve(res);
    }
    return Promise.resolve(jsonResponse({ code: "NOT_MOCKED", message: `${method} ${url.pathname} não mockado` }, 404));
  });
  vi.stubGlobal("fetch", fn);
  return fn;
}

export const usersRoute: Route = ({ method, url }) =>
  method === "GET" && url.pathname === "/api/v1/users"
    ? jsonResponse({ items: [{ id: "u1", name: "Ana Souza", email: null, role: "OPERATOR", active: true, created_at: "", updated_at: "" }] })
    : undefined;
