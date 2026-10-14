import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { PgdasDocument, PgdasUpload } from "@/services/factorRInbox";
import { DOC_ID, JOB_ID, pgdasDocument } from "@/test/factorR";
import { jsonResponse, requestedUrls, urlOf } from "@/test/fetch";
import { mockFetchRoutes } from "@/test/fetchRouter";
import { renderWithProviders } from "@/test/render";
import { InboxPage } from "./InboxPage";

afterEach(() => {
  vi.unstubAllGlobals();
});

const ROUTE = "/factor-r/inbox";

const documents: PgdasDocument[] = [
  pgdasDocument({ id: "d-review" }),
  pgdasDocument({
    id: "d-running",
    status: "received",
    motivo: null,
    campos: { cnpj: null, pa: null, rbt12: null, rpa: null, fs12: null, fator_r: null, das: null, anexo: null },
    confianca: null,
    job: { id: JOB_ID, status: "running", attempts: 1, max_attempts: 3, last_error_code: null },
    sugestao_cadastro: null,
  }),
  pgdasDocument({
    id: "d-dead",
    status: "received",
    motivo: null,
    job: { id: "j-dead", status: "dead", attempts: 1, max_attempts: 3, last_error_code: "PGDAS_PA_INVALIDO" },
    sugestao_cadastro: null,
  }),
  pgdasDocument({ id: "d-linked", status: "linked", motivo: null, company_id: "c1", nome_original: "vinculado.pdf", sugestao_cadastro: null }),
];

function mock(opts: { upload?: () => Response; document?: PgdasDocument } = {}) {
  return mockFetchRoutes(({ method, url }) => {
    if (method === "GET" && url.pathname === "/api/v1/factor-r/pgdas") {
      const status = url.searchParams.get("status");
      const items = status ? documents.filter((d) => d.status === status) : documents;
      return jsonResponse({ items, total: items.length, contagem_por_status: { needs_review: 1, received: 2, linked: 1 } });
    }
    if (method === "GET" && url.pathname === `/api/v1/factor-r/pgdas/${DOC_ID}`) return jsonResponse(opts.document ?? pgdasDocument());
    if (method === "POST" && url.pathname === "/api/v1/factor-r/pgdas") {
      return opts.upload?.() ?? jsonResponse({ document_id: DOC_ID, job_id: JOB_ID, status: "received", duplicate: false } satisfies PgdasUpload, 202);
    }
    if (method === "GET" && url.pathname.startsWith("/api/v1/jobs/")) {
      return jsonResponse({ id: JOB_ID, status: "running", attempts: 1, max_attempts: 3, worker_online: true, last_error_code: null });
    }
    return undefined;
  });
}

describe("InboxPage", () => {
  it("08 §6: estados claros (revisão, processando, falha, vinculado) e contagem por status", async () => {
    const fetchFn = mock();
    renderWithProviders(<InboxPage />, { route: ROUTE });
    const table = await screen.findByRole("table", { name: "Extratos do inbox" });
    const rows = within(table).getAllByRole("row").slice(1);
    expect(rows).toHaveLength(4);
    expect(rows[0]).toHaveTextContent("Em revisão");
    expect(rows[0]).toHaveTextContent("CNPJ fora do cadastro de empresas");
    expect(rows[0]).toHaveTextContent("45.723.174/0001-10");
    expect(rows[0]).toHaveTextContent("09/2026");
    expect(rows[1]).toHaveTextContent("Processando");
    expect(rows[2]).toHaveTextContent("Falha no processamento");
    expect(rows[3]).toHaveTextContent("Vinculado");
    expect(within(rows[0] as HTMLElement).getByRole("link")).toHaveAttribute("href", "/factor-r/inbox/d-review");
    expect(screen.getByRole("button", { name: "Todos (4)" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByRole("note")).toHaveTextContent("O PGDAS-D da Receita Federal prevalece.");

    await userEvent.click(screen.getByRole("button", { name: "Em revisão (1)" }));
    expect(screen.getByRole("button", { name: "Em revisão (1)" })).toHaveAttribute("aria-pressed", "true");
    await waitFor(() => expect(requestedUrls(fetchFn).some((u) => u.includes("/factor-r/pgdas?status=needs_review"))).toBe(true));
  });

  it("FR-08: envio acompanha o documento e oferece o cadastro pelo extrato", async () => {
    const fetchFn = mock();
    const user = userEvent.setup();
    renderWithProviders(<InboxPage />, { route: ROUTE });
    await user.upload(screen.getByLabelText("Extratos do PGDAS-D (PDF ou TXT)"), new File(["extrato"], "extrato.txt", { type: "text/plain" }));

    const sent = await screen.findByRole("list", { name: "Envios desta sessão" });
    expect(await within(sent).findByText("Em revisão")).toBeInTheDocument();
    expect(within(sent).getByText(/precisa de revisão: cnpj_nao_encontrado/)).toBeInTheDocument();
    expect(within(sent).getByRole("button", { name: "Cadastrar e vincular" })).toBeInTheDocument();
    expect(within(sent).getByRole("link", { name: "ver documento" })).toHaveAttribute("href", `/factor-r/inbox/${DOC_ID}`);

    const call = fetchFn.mock.calls.find((c) => (c[1]?.method ?? "GET") === "POST" && urlOf(c[0]).endsWith("/api/v1/factor-r/pgdas"));
    const body = call?.[1]?.body;
    expect(body).toBeInstanceOf(FormData);
    expect((body as FormData).get("file")).toBeInstanceOf(File);
  });

  it("documento ainda na fila: mostra o andamento do job", async () => {
    mock({ document: pgdasDocument({ status: "received", motivo: null, sugestao_cadastro: null, texto_agente: null, job: { id: JOB_ID, status: "running", attempts: 1, max_attempts: 3, last_error_code: null } }) });
    renderWithProviders(<InboxPage />, { route: ROUTE });
    await userEvent.upload(screen.getByLabelText("Extratos do PGDAS-D (PDF ou TXT)"), new File(["x"], "extrato.txt", { type: "text/plain" }));
    expect(await screen.findByTestId("job-andamento")).toHaveTextContent("Lendo o extrato");
  });

  it("recusas do envio: formato não aceito e arquivo repetido", async () => {
    let n = 0;
    mock({
      upload: () =>
        ++n === 1
          ? jsonResponse({ code: "UNSUPPORTED_FILE", message: "O conteúdo não é PDF nem TXT." }, 415)
          : jsonResponse({ document_id: DOC_ID, job_id: JOB_ID, status: "needs_review", duplicate: true } satisfies PgdasUpload),
    });
    const user = userEvent.setup({ applyAccept: false });
    renderWithProviders(<InboxPage />, { route: ROUTE });
    const input = screen.getByLabelText("Extratos do PGDAS-D (PDF ou TXT)");
    await user.upload(input, new File(["MZ"], "programa.exe", { type: "application/octet-stream" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("programa.exe: Formato não aceito. Envie o extrato em PDF ou TXT.");
    await user.upload(input, new File(["extrato"], "extrato.txt", { type: "text/plain" }));
    expect(await screen.findByText("Este arquivo já tinha sido enviado.")).toBeInTheDocument();
  });
});
