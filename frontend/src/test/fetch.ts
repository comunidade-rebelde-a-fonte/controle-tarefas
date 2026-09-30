import type { Mock } from "vitest";

/** URL de uma chamada ao fetch mockado. */
export function urlOf(input: RequestInfo | URL | undefined): string {
  if (input === undefined) return "";
  if (typeof input === "string") return input;
  if (input instanceof URL) return input.href;
  return input.url;
}

/** URLs de todas as chamadas feitas ao fetch mockado. */
export function requestedUrls(fetchFn: Mock<typeof fetch>): string[] {
  return fetchFn.mock.calls.map((c) => urlOf(c[0]));
}

/** Resposta JSON para mocks. */
export function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });
}

/** Corpo JSON da i-ésima chamada ao fetch mockado (undefined se não houver corpo). */
export function requestBody(fetchFn: Mock<typeof fetch>, i = 0): unknown {
  const body = fetchFn.mock.calls[i]?.[1]?.body;
  return typeof body === "string" ? (JSON.parse(body) as unknown) : undefined;
}
