import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ObligationInstance, StatusDefinition } from "@/services/obligations";
import { jsonResponse, urlOf } from "@/test/fetch";
import { renderWithProviders } from "@/test/render";
import { StatusCell, type StatusCellProps } from "./StatusCell";

afterEach(() => {
  vi.unstubAllGlobals();
});

const def = (code: string, label: string, sort: number, flags: Partial<StatusDefinition> = {}): StatusDefinition => ({
  code,
  label,
  sort_order: sort,
  counts_as_started: false,
  counts_as_operational_progress: false,
  is_final: false,
  is_blocking: false,
  excluded_from_denominator: false,
  ...flags,
});

const statuses: StatusDefinition[] = [
  def("NOT_STARTED", "A iniciar", 1),
  def("DOWNLOADED", "Baixadas", 2, { counts_as_started: true }),
  def("POSTED", "Lançadas", 3, { counts_as_started: true }),
  def("CLOSED", "Fechadas", 4, { counts_as_started: true, is_final: true }),
  def("MISSING_DOCUMENTS", "Documentos faltantes", 5, { counts_as_started: true, is_blocking: true }),
  def("NOT_APPLICABLE", "Não se aplica", 7, { is_final: true, excluded_from_denominator: true }),
];

const base: ObligationInstance = {
  id: "i1",
  company_id: "c1",
  competence: "2026-09",
  obligation_type: { id: "t1", code: "FISCAL_ICMS", name: "ICMS", primary_module: "FISCAL" },
  status_code: "DOWNLOADED",
  status_label: "Baixadas",
  applicable: true,
  counts_in_progress: true,
  is_final: false,
  is_started: true,
  is_blocking: false,
  is_overdue: false,
  due_date_original: "2026-10-20",
  due_date_effective: "2026-10-20",
  due_overridden: false,
  due_date_override_reason: null,
  transmission_date: null,
  protocol: null,
  has_history: false,
  version: 3,
  last_changed_at: null,
  last_changed_by: null,
};

const inst = (patch: Partial<ObligationInstance> = {}): ObligationInstance => ({ ...base, ...patch });

const row = { module: "FISCAL", progress: { operational: 0.5, completion: 0.2, done: 1, total: 5 }, overdue: 0, nearest_deadline: null };

type Handler = (req: { method: string; path: string; body: unknown }) => Promise<Response> | Response | undefined;

function stubFetch(handler: Handler) {
  const fn = vi.fn<typeof fetch>().mockImplementation(async (input, init) => {
    const url = new URL(urlOf(input), "http://localhost");
    const method = (init?.method ?? "GET").toUpperCase();
    const body: unknown = typeof init?.body === "string" ? JSON.parse(init.body) : undefined;
    const res = await handler({ method, path: url.pathname, body });
    return res ?? jsonResponse({ code: "NOT_MOCKED", message: `${method} ${url.pathname}` }, 404);
  });
  vi.stubGlobal("fetch", fn);
  return fn;
}

function renderCell(props: Partial<StatusCellProps> = {}) {
  return renderWithProviders(<StatusCell instance={base} statuses={statuses} label="ICMS — Alfa Comércio" {...props} />);
}

async function choose(user: ReturnType<typeof userEvent.setup>, trigger: string, option: string) {
  await user.click(screen.getByRole("button", { name: trigger }));
  await user.click(await screen.findByRole("menuitemradio", { name: option }));
}

describe("StatusCell", () => {
  it("mostra o status com texto e lista as opções do conjunto, com a atual marcada", async () => {
    stubFetch(() => undefined);
    const user = userEvent.setup();
    renderCell();
    await user.click(screen.getByRole("button", { name: "ICMS — Alfa Comércio: Baixadas" }));
    const options = await screen.findAllByRole("menuitemradio");
    expect(options.map((o) => o.textContent)).toEqual(statuses.map((s) => s.label));
    expect(screen.getByRole("menuitemradio", { name: "Baixadas" })).toHaveAttribute("aria-checked", "true");
  });

  it("aplica o status de forma otimista e confirma com a resposta da API", async () => {
    let respond: (r: Response) => void = () => undefined;
    const fetchFn = stubFetch(({ method, path }) =>
      method === "PATCH" && path === "/api/v1/obligation-instances/i1/status" ? new Promise<Response>((r) => (respond = r)) : undefined,
    );
    const onChanged = vi.fn();
    const user = userEvent.setup();
    renderCell({ onChanged });

    await choose(user, "ICMS — Alfa Comércio: Baixadas", "Lançadas");
    // Otimista: já mostra o novo status antes da resposta.
    const trigger = screen.getByRole("button", { name: "ICMS — Alfa Comércio: Lançadas" });
    expect(trigger).toHaveAttribute("aria-busy", "true");
    expect(JSON.parse(fetchFn.mock.calls[0]?.[1]?.body as string)).toEqual({ status_code: "POSTED", version: 3 });

    const updated = inst({ status_code: "POSTED", status_label: "Lançadas", version: 4, has_history: true });
    respond(jsonResponse({ instance: updated, row }));
    await waitFor(() => expect(onChanged).toHaveBeenCalledWith({ instance: updated, row }));
    expect(screen.getByRole("button", { name: "ICMS — Alfa Comércio: Lançadas" })).not.toHaveAttribute("aria-busy");
    expect(screen.getByRole("img", { name: "Possui histórico de alterações" })).toBeInTheDocument();
  });

  it("desfaz a mudança (rollback) e avisa quando a API recusa", async () => {
    stubFetch(({ method }) =>
      method === "PATCH" ? jsonResponse({ code: "INVALID_STATUS", message: "Status inválido para esta obrigação." }, 422) : undefined,
    );
    const user = userEvent.setup();
    renderCell();

    await choose(user, "ICMS — Alfa Comércio: Baixadas", "Fechadas");
    expect(await screen.findByText("Não foi possível alterar o status")).toBeInTheDocument();
    expect(screen.getByText("Status inválido para esta obrigação.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "ICMS — Alfa Comércio: Baixadas" })).toBeInTheDocument();
  });

  it("desfaz a mudança quando a rede falha", async () => {
    stubFetch(({ method }) => (method === "PATCH" ? Promise.reject(new TypeError("Failed to fetch")) : undefined));
    const user = userEvent.setup();
    renderCell();
    await choose(user, "ICMS — Alfa Comércio: Baixadas", "Lançadas");
    expect(await screen.findByText("Não foi possível alterar o status")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "ICMS — Alfa Comércio: Baixadas" })).toBeInTheDocument();
  });

  it("em conflito de versão (409) recarrega a instância e avisa", async () => {
    const fetchFn = stubFetch(({ method, path }) => {
      if (method === "PATCH") return jsonResponse({ code: "VERSION_CONFLICT", message: "Registro alterado por outra operação." }, 409);
      if (method === "GET" && path === "/api/v1/obligation-instances/i1") {
        return jsonResponse(inst({ status_code: "CLOSED", status_label: "Fechadas", version: 5, has_history: true }));
      }
      return undefined;
    });
    const user = userEvent.setup();
    renderCell();

    await choose(user, "ICMS — Alfa Comércio: Baixadas", "Lançadas");
    expect(await screen.findByText("Obrigação alterada em outro lugar")).toBeInTheDocument();
    expect(await screen.findByRole("button", { name: "ICMS — Alfa Comércio: Fechadas" })).toBeInTheDocument();
    expect(fetchFn.mock.calls.some(([, init]) => (init?.method ?? "GET") === "GET")).toBe(true);
  });

  it("repassa dependência pendente (422/409) para o handler dos modais, sem aviso genérico", async () => {
    stubFetch(({ method }) =>
      method === "PATCH"
        ? jsonResponse({ code: "DEPENDENCY_BLOCKED", message: "Etapa anterior pendente.", details: { pending: ["ACCOUNTING_MOVEMENT_RECEIVED"] } }, 422)
        : undefined,
    );
    const onDependencyError = vi.fn();
    const user = userEvent.setup();
    renderCell({ onDependencyError });

    await choose(user, "ICMS — Alfa Comércio: Baixadas", "Fechadas");
    await waitFor(() => expect(onDependencyError).toHaveBeenCalledTimes(1));
    const event = onDependencyError.mock.calls[0]?.[0] as { error: { code: string; status: number }; statusCode: string };
    expect(event.statusCode).toBe("CLOSED");
    expect(event.error.code).toBe("DEPENDENCY_BLOCKED");
    expect(event.error.status).toBe(422);
    expect(screen.queryByText("Não foi possível alterar o status")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "ICMS — Alfa Comércio: Baixadas" })).toBeInTheDocument();
  });

  it("não aplicável fica desabilitada, com o motivo acessível", async () => {
    stubFetch(() => undefined);
    const user = userEvent.setup();
    renderCell({ instance: inst({ applicable: false, status_code: "NOT_APPLICABLE", status_label: "Não se aplica" }) });
    const button = screen.getByRole("button", { name: "ICMS — Alfa Comércio: Não se aplica" });
    expect(button).toHaveAttribute("aria-disabled", "true");
    expect(button).toHaveAccessibleDescription("Não se aplica a esta empresa nesta competência.");
    await user.click(button);
    expect(screen.queryByRole("menu")).not.toBeInTheDocument();
  });

  it("período fechado bloqueia a edição, com o motivo acessível", () => {
    stubFetch(() => undefined);
    renderCell({ locked: true });
    const button = screen.getByRole("button", { name: "ICMS — Alfa Comércio: Baixadas" });
    expect(button).toHaveAttribute("aria-disabled", "true");
    expect(button).toHaveAccessibleDescription("Módulo fechado nesta competência. Reabra o módulo para alterar.");
  });

  it("indicadores de atraso, prazo alterado e histórico têm aria-label (não dependem de cor)", () => {
    stubFetch(() => undefined);
    renderCell({
      instance: inst({
        is_overdue: true,
        due_date_effective: "2026-09-25",
        due_overridden: true,
        due_date_override_reason: "Prorrogação combinada com o cliente",
        has_history: true,
      }),
    });
    expect(screen.getByRole("img", { name: "Atrasada: venceu em 25/09/2026" })).toBeInTheDocument();
    expect(screen.getByRole("img", { name: "Prazo alterado manualmente: Prorrogação combinada com o cliente" })).toBeInTheDocument();
    expect(screen.getByRole("img", { name: "Possui histórico de alterações" })).toBeInTheDocument();
  });

  it("sem indicadores quando não há atraso, prazo alterado nem histórico", () => {
    stubFetch(() => undefined);
    renderCell();
    expect(screen.queryByRole("img")).not.toBeInTheDocument();
  });
  it("CA-010 etapa anterior pendente: desabilita as opções finais (exceto Sem movimento) e mostra a mensagem", async () => {
    const fetchFn = stubFetch(() => undefined);
    const user = userEvent.setup();
    const accounting = [
      def("NOT_STARTED", "A iniciar", 1),
      def("PENDING", "Pendente", 2, { counts_as_started: true, is_blocking: true }),
      def("NO_MOVEMENT", "Sem movimento", 3, { counts_as_started: true, is_final: true }),
      def("OK", "OK", 4, { counts_as_started: true, is_final: true }),
    ];
    const message = "Não é possível concluir Conciliação. A etapa Contábil digitado/processado ainda não foi concluída.";
    renderCell({ instance: inst({ status_code: "NOT_STARTED", status_label: "A iniciar" }), statuses: accounting, label: "Conciliação — Alfa", blockedMessage: message });
    await user.click(screen.getByRole("button", { name: "Conciliação — Alfa: A iniciar" }));
    expect(await screen.findByText(message)).toBeInTheDocument();
    expect(screen.getByRole("menuitemradio", { name: "OK" })).toHaveAttribute("aria-disabled", "true");
    expect(screen.getByRole("menuitemradio", { name: "Sem movimento" })).not.toHaveAttribute("aria-disabled");
    await user.click(screen.getByRole("menuitemradio", { name: "OK" }));
    expect(fetchFn).not.toHaveBeenCalled();
  });
});
