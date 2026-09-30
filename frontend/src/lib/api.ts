/**
 * Cliente HTTP tipado a partir do contrato OpenAPI (`src/types/api.d.ts`, gerado por `make gen`).
 *
 * Todas as chamadas à API passam por aqui. Erros no envelope `{code, message, details}`
 * viram `ApiError`, com a mensagem em português pronta para exibição.
 */
import type { components, paths } from "@/types/api";

export type Schemas = components["schemas"];
export type ErrorBody = Schemas["Error"];

export const API_BASE = "/api/v1";

type HttpMethod = "get" | "post" | "put" | "patch" | "delete";

/** Caminhos do contrato que declaram o método M. */
export type PathsFor<M extends HttpMethod> = {
  [P in keyof paths]: paths[P][M] extends undefined ? never : P;
}[keyof paths];

type Op<P extends keyof paths, M extends HttpMethod> = NonNullable<paths[P][M]>;

type JsonContent<T> = T extends { content: { "application/json": infer B } } ? B : never;

type SuccessCode = 200 | 201 | 202 | 204;

/** Corpo da resposta de sucesso da operação (204 → undefined). */
export type ResponseOf<O> = O extends { responses: infer R }
  ? {
      [K in keyof R]: K extends SuccessCode ? (K extends 204 ? undefined : JsonContent<R[K]>) : never;
    }[keyof R]
  : never;

type ParamsOf<O> = O extends { parameters: infer X } ? X : undefined;
type Field<T, K extends PropertyKey> = T extends Record<K, unknown> ? T[K] : T extends Partial<Record<K, infer V>> ? V | undefined : undefined;

type BodyOf<O> = O extends { requestBody: infer RB }
  ? JsonContent<RB>
  : O extends { requestBody?: infer RB }
    ? JsonContent<NonNullable<RB>> | undefined
    : undefined;

type Slot<K extends string, T> = [Exclude<T, undefined>] extends [never]
  ? { [P in K]?: undefined }
  : undefined extends T
    ? { [P in K]?: Exclude<T, undefined> }
    : { [P in K]: T };

/** Opções da requisição derivadas da operação: path, query e body. */
export type RequestOptions<O> = Slot<"path", Field<ParamsOf<O>, "path">> &
  Slot<"query", Field<ParamsOf<O>, "query">> &
  Slot<"body", BodyOf<O>> & { signal?: AbortSignal };

type OptionalArgs<T> = object extends T ? [options?: T] : [options: T];

/** Erro retornado pela API (ou falha de rede), com o envelope padrão. */
export class ApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly details: Record<string, unknown>;

  constructor(status: number, body: ErrorBody) {
    super(body.message);
    this.name = "ApiError";
    this.status = status;
    this.code = body.code;
    this.details = body.details ?? {};
  }
}

export function isApiError(err: unknown): err is ApiError {
  return err instanceof ApiError;
}

type QueryValue = string | number | boolean | null | undefined | readonly (string | number | boolean)[];

/** Monta a URL substituindo `{param}` e serializando a query (ignora vazios). */
export function buildUrl(path: string, pathParams?: Record<string, unknown>, query?: Record<string, QueryValue>): string {
  const resolved = path.replace(/\{(\w+)\}/g, (_, name: string) => {
    const value = pathParams?.[name];
    if (typeof value !== "string" && typeof value !== "number") throw new Error(`parâmetro de caminho ausente: ${name}`);
    return encodeURIComponent(String(value));
  });
  const qs = new URLSearchParams();
  for (const [key, value] of Object.entries(query ?? {})) {
    if (value === undefined || value === null || value === "") continue;
    if (Array.isArray(value)) {
      for (const v of value as readonly (string | number | boolean)[]) qs.append(key, String(v));
    } else {
      qs.append(key, String(value));
    }
  }
  const q = qs.toString();
  return `${API_BASE}${resolved}${q ? `?${q}` : ""}`;
}

async function toApiError(res: Response): Promise<ApiError> {
  try {
    const body = (await res.json()) as Partial<ErrorBody>;
    if (typeof body.code === "string" && typeof body.message === "string") {
      return new ApiError(res.status, { code: body.code, message: body.message, details: body.details ?? {} });
    }
  } catch {
    // corpo não é JSON: cai no erro genérico abaixo
  }
  return new ApiError(res.status, {
    code: `HTTP_${res.status}`,
    message: "Falha na comunicação com o servidor. Tente novamente.",
  });
}

interface RawOptions {
  path?: Record<string, unknown>;
  query?: Record<string, QueryValue>;
  body?: unknown;
  signal?: AbortSignal;
}

async function send(method: HttpMethod, path: string, options: RawOptions = {}): Promise<unknown> {
  const url = buildUrl(path, options.path, options.query);
  const headers: Record<string, string> = { Accept: "application/json" };
  const init: RequestInit = { method: method.toUpperCase(), headers };
  if (options.signal) init.signal = options.signal;
  if (options.body !== undefined) {
    headers["Content-Type"] = "application/json";
    init.body = JSON.stringify(options.body);
  }
  let res: Response;
  try {
    res = await fetch(url, init);
  } catch (err) {
    if (err instanceof DOMException && err.name === "AbortError") throw err;
    throw new ApiError(0, { code: "NETWORK_ERROR", message: "Não foi possível conectar ao servidor." });
  }
  if (!res.ok) throw await toApiError(res);
  if (res.status === 204) return undefined;
  const text = await res.text();
  return text ? (JSON.parse(text) as unknown) : undefined;
}

function method<M extends HttpMethod>(m: M) {
  return <P extends PathsFor<M>>(path: P, ...args: OptionalArgs<RequestOptions<Op<P, M>>>) =>
    send(m, path, args[0]) as Promise<ResponseOf<Op<P, M>>>;
}

/** Cliente tipado: `api.get("/companies/{id}", { path: { id } })`. */
export const api = {
  get: method("get"),
  post: method("post"),
  put: method("put"),
  patch: method("patch"),
  delete: method("delete"),
};

/** Envia multipart/form-data (uploads). */
export async function apiUpload<T>(
  httpMethod: "post" | "put",
  path: string,
  form: FormData,
  pathParams?: Record<string, unknown>,
): Promise<T> {
  let res: Response;
  try {
    res = await fetch(buildUrl(path, pathParams), { method: httpMethod.toUpperCase(), body: form, headers: { Accept: "application/json" } });
  } catch {
    throw new ApiError(0, { code: "NETWORK_ERROR", message: "Não foi possível conectar ao servidor." });
  }
  if (!res.ok) throw await toApiError(res);
  return (await res.json()) as T;
}
