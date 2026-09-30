import { afterEach, describe, expect, it, vi } from "vitest";
import { api, ApiError, buildUrl } from "./api";

function mockFetch(response: Response) {
  const fn = vi.fn<typeof fetch>().mockResolvedValue(response);
  vi.stubGlobal("fetch", fn);
  return fn;
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("buildUrl", () => {
  it("substitui parâmetros de caminho e ignora query vazia", () => {
    expect(buildUrl("/companies/{id}", { id: "a b" }, { q: "", active: true, module: ["FISCAL", "PAYROLL"] })).toBe(
      "/api/v1/companies/a%20b?active=true&module=FISCAL&module=PAYROLL",
    );
  });

  it("falha quando falta parâmetro de caminho", () => {
    expect(() => buildUrl("/companies/{id}")).toThrow(/id/);
  });
});

describe("api", () => {
  it("retorna o corpo tipado em sucesso", async () => {
    const fetchFn = mockFetch(new Response(JSON.stringify({ status: "ok", database: "ok" }), { status: 200 }));
    const health = await api.get("/health");
    expect(health.status).toBe("ok");
    expect(fetchFn).toHaveBeenCalledWith("/api/v1/health", expect.objectContaining({ method: "GET" }));
  });

  it("converte o envelope de erro em ApiError", async () => {
    mockFetch(
      new Response(JSON.stringify({ code: "VERSION_CONFLICT", message: "Registro alterado.", details: { v: 2 } }), {
        status: 409,
      }),
    );
    const err: unknown = await api.get("/health").catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApiError);
    const apiErr = err as ApiError;
    expect(apiErr.status).toBe(409);
    expect(apiErr.code).toBe("VERSION_CONFLICT");
    expect(apiErr.message).toBe("Registro alterado.");
    expect(apiErr.details).toEqual({ v: 2 });
  });

  it("gera erro genérico quando o corpo não é JSON", async () => {
    mockFetch(new Response("<html>bad gateway</html>", { status: 502 }));
    await expect(api.get("/health")).rejects.toMatchObject({ status: 502, code: "HTTP_502" });
  });

  it("converte falha de rede em NETWORK_ERROR", async () => {
    vi.stubGlobal("fetch", vi.fn<typeof fetch>().mockRejectedValue(new TypeError("Failed to fetch")));
    await expect(api.get("/health")).rejects.toMatchObject({ status: 0, code: "NETWORK_ERROR" });
  });
});
