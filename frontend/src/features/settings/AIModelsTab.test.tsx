import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { AIModelAlias, AIModelList, AIModelValidation } from "@/services/aiModels";
import { jsonResponse } from "@/test/fetch";
import { mockFetchRoutes } from "@/test/fetchRouter";
import { renderWithProviders } from "@/test/render";
import { AIModelsTab } from "./AIModelsTab";

afterEach(() => {
  vi.unstubAllGlobals();
});

function alias(name: string, patch: Partial<AIModelAlias> = {}): AIModelAlias {
  return {
    alias: name,
    module: "conciliacao",
    purpose: `Propósito do ${name}`,
    requirements: { structured_output: true, vision: name === "triador" },
    provider: null,
    model: null,
    default_model: "openai/gpt-6-luna",
    effective_model: "openai/gpt-6-luna",
    source: "env",
    parameters: {},
    enabled: true,
    version: 1,
    updated_at: "2026-10-01T12:00:00Z",
    ...patch,
  };
}

const okValidation: AIModelValidation = {
  ok: true,
  model: "openai/gpt-6-sol",
  source: "settings",
  checks: [
    { code: "MODEL_KNOWN", ok: true, message: "O gateway conhece o modelo openai/gpt-6-sol." },
    { code: "CHAT_MODE", ok: true, message: "É um modelo de chat." },
    { code: "STRUCTURED_OUTPUT", ok: true, message: "Aceita saída estruturada (JSON Schema)." },
    { code: "VISION", ok: true, message: "Aceita imagens (visão)." },
  ],
  capabilities: { mode: "chat", structured_output: true, vision: true, max_output_tokens: 128000, parameters: ["temperature", "max_tokens"] },
};

function mock(opts: { list?: AIModelList; putStatus?: number; putBody?: unknown } = {}) {
  const list: AIModelList = opts.list ?? {
    gateway_configured: true,
    items: [
      alias("triador"),
      alias("classificador", { default_model: null, effective_model: null, source: "none" }),
      alias("consultor", { module: "fator_r", enabled: false }),
    ],
  };
  const puts: { path: string; body: unknown }[] = [];
  const validations: { path: string; body: unknown }[] = [];
  const fetchFn = mockFetchRoutes(({ method, url, body }) => {
    if (method === "GET" && url.pathname === "/api/v1/settings/ai/models") return jsonResponse(list);
    if (method === "POST" && url.pathname.endsWith("/validate")) {
      validations.push({ path: url.pathname, body });
      return jsonResponse(okValidation);
    }
    if (method === "PUT" && url.pathname.startsWith("/api/v1/settings/ai/models/")) {
      puts.push({ path: url.pathname, body });
      if (opts.putStatus) return jsonResponse(opts.putBody, opts.putStatus);
      const b = body as { provider: string | null; model: string | null; parameters: AIModelAlias["parameters"]; enabled: boolean };
      return jsonResponse(
        alias("triador", {
          provider: b.provider as AIModelAlias["provider"],
          model: b.model,
          effective_model: b.provider ? `${b.provider}/${b.model ?? ""}` : "openai/gpt-6-luna",
          source: b.provider ? "settings" : "env",
          parameters: b.parameters,
          enabled: b.enabled,
          version: 2,
        }),
      );
    }
    return undefined;
  });
  return { fetchFn, puts, validations };
}

describe("AIModelsTab", () => {
  it("lista os aliases sem nenhum campo de credencial (08 §9, REQ-047)", async () => {
    mock();
    renderWithProviders(<AIModelsTab />);
    const table = await screen.findByRole("table", { name: "Modelos de IA por agente" });
    const tri = within(table).getByTestId("alias-triador");
    expect(tri).toHaveTextContent("triador");
    expect(tri).toHaveTextContent("openai");
    expect(tri).toHaveTextContent("gpt-6-luna");
    expect(tri).toHaveTextContent("padrão do ambiente");
    expect(tri).toHaveTextContent("Ativo");
    expect(tri).toHaveTextContent("v1");
    expect(within(table).getByTestId("alias-classificador")).toHaveTextContent("Sem modelo");
    expect(within(table).getByTestId("alias-consultor")).toHaveTextContent("Desligado");
    expect(screen.getByRole("note")).toHaveTextContent("Credenciais são configuradas no ambiente e não ficam salvas nesta tela.");
    expect(screen.queryByTestId("sem-gateway")).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Editar triador" }));
    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).queryByLabelText(/chave|api.?key|senha|segredo|secret|password|credencia/i)).not.toBeInTheDocument();
    expect(dialog.querySelector('input[type="password"]')).toBeNull();
    // Os únicos campos são modelo e parâmetros permitidos (max_tokens é parâmetro, não credencial).
    const labels = within(dialog)
      .getAllByRole("textbox")
      .map((el) => el.getAttribute("id"))
      .map((id) => dialog.querySelector(`label[for="${id ?? ""}"]`)?.textContent);
    expect(labels).toEqual(["Temperature", "max_tokens"]);
    expect(dialog).toHaveTextContent("Este agente exige saída estruturada (JSON Schema) e visão.");
    expect(dialog).toHaveTextContent("Credenciais são configuradas no ambiente e não ficam salvas nesta tela.");
  });

  it("F-010: escolhe o modelo, valida sem gravar e salva com a versão lida", async () => {
    const { puts, validations } = mock();
    renderWithProviders(<AIModelsTab />);
    fireEvent.click(await screen.findByRole("button", { name: "Editar triador" }));
    const dialog = await screen.findByRole("dialog");

    fireEvent.click(within(dialog).getByLabelText("Escolher o modelo"));
    fireEvent.change(within(dialog).getByLabelText("Modelo"), { target: { value: " gpt-6-sol " } });
    fireEvent.change(within(dialog).getByLabelText("Temperature"), { target: { value: "0,2" } });
    fireEvent.click(within(dialog).getByRole("button", { name: "Validar configuração" }));

    const result = await within(dialog).findByTestId("resultado-validacao");
    expect(result).toHaveTextContent("Configuração válida: openai/gpt-6-sol");
    expect(result).toHaveTextContent("Aceita imagens (visão).");
    expect(result).toHaveTextContent("Parâmetros aceitos pelo modelo: temperature, max_tokens · saída máxima de 128.000 tokens.");
    expect(validations).toEqual([
      { path: "/api/v1/settings/ai/models/triador/validate", body: { provider: "openai", model: "gpt-6-sol", parameters: { temperature: 0.2 } } },
    ]);
    expect(puts).toHaveLength(0);

    fireEvent.click(within(dialog).getByRole("button", { name: "Salvar" }));
    await waitFor(() => expect(puts).toHaveLength(1));
    expect(puts[0]).toEqual({
      path: "/api/v1/settings/ai/models/triador",
      body: { provider: "openai", model: "gpt-6-sol", parameters: { temperature: 0.2 }, enabled: true, version: 1 },
    });
    expect(await screen.findByText("Configuração de triador salva (versão 2)")).toBeInTheDocument();
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    const row = screen.getByTestId("alias-triador");
    expect(row).toHaveTextContent("gpt-6-sol");
    expect(row).toHaveTextContent("escolhido aqui");
    expect(row).toHaveTextContent("v2");
    expect(row).toHaveTextContent("temperature 0,2");
  });

  it("volta ao padrão do ambiente e desliga o agente", async () => {
    const { puts } = mock({
      list: {
        gateway_configured: true,
        items: [alias("triador", { provider: "openai", model: "gpt-6-sol", effective_model: "openai/gpt-6-sol", source: "settings", version: 3 })],
      },
    });
    renderWithProviders(<AIModelsTab />);
    fireEvent.click(await screen.findByRole("button", { name: "Editar triador" }));
    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getByLabelText("Modelo")).toHaveValue("gpt-6-sol");

    fireEvent.click(within(dialog).getByLabelText(/Padrão do ambiente/));
    fireEvent.click(within(dialog).getByLabelText("Agente ligado"));
    expect(dialog).toHaveTextContent("Desligado, as execuções deste agente falham até religar.");
    fireEvent.click(within(dialog).getByRole("button", { name: "Salvar" }));
    await waitFor(() => expect(puts).toHaveLength(1));
    expect(puts[0]?.body).toEqual({ provider: null, model: null, parameters: {}, enabled: false, version: 3 });
  });

  it("422 AI_MODEL_INVALID: mostra as conferências reprovadas e mantém o diálogo", async () => {
    const { puts } = mock({
      putStatus: 422,
      putBody: {
        code: "AI_MODEL_INVALID",
        message: "A configuração não foi gravada. Não aceita imagens, e o agente triador lê páginas digitalizadas.",
        details: {
          model: "openai/gpt-6-texto",
          checks: [
            { code: "MODEL_KNOWN", ok: true, message: "O gateway conhece o modelo openai/gpt-6-texto." },
            { code: "VISION", ok: false, message: "Não aceita imagens, e o agente triador lê páginas digitalizadas." },
          ],
        },
      },
    });
    renderWithProviders(<AIModelsTab />);
    fireEvent.click(await screen.findByRole("button", { name: "Editar triador" }));
    const dialog = await screen.findByRole("dialog");
    fireEvent.click(within(dialog).getByLabelText("Escolher o modelo"));
    fireEvent.change(within(dialog).getByLabelText("Modelo"), { target: { value: "gpt-6-texto" } });
    fireEvent.click(within(dialog).getByRole("button", { name: "Validar configuração" }));
    await within(dialog).findByTestId("resultado-validacao");
    fireEvent.click(within(dialog).getByRole("button", { name: "Salvar" }));

    // Um painel só: o do salvar substitui o da validação anterior.
    await within(dialog).findByText("Não foi gravado: o modelo não atende ao agente");
    const panels = within(dialog).getAllByTestId("resultado-validacao");
    expect(panels).toHaveLength(1);
    const [panel] = panels;
    if (!panel) throw new Error("painel ausente");
    expect(panel).toHaveTextContent("Não foi gravado: o modelo não atende ao agente");
    expect(within(panel).getByLabelText("reprovado")).toBeInTheDocument();
    expect(panel).toHaveTextContent("Não aceita imagens, e o agente triador lê páginas digitalizadas.");
    expect(puts).toHaveLength(1);
    expect(screen.getByRole("dialog")).toBeInTheDocument();
  });

  it("valida localmente parâmetros mal formatados sem chamar a API", async () => {
    const { puts, validations } = mock();
    renderWithProviders(<AIModelsTab />);
    fireEvent.click(await screen.findByRole("button", { name: "Editar triador" }));
    const dialog = await screen.findByRole("dialog");
    fireEvent.change(within(dialog).getByLabelText("max_tokens"), { target: { value: "12,5" } });
    fireEvent.click(within(dialog).getByRole("button", { name: "Validar configuração" }));
    expect(await within(dialog).findByText("max_tokens inválido: use um número inteiro.")).toBeInTheDocument();
    expect(validations).toHaveLength(0);
    expect(puts).toHaveLength(0);
  });

  it("sem gateway configurado: avisa e não deixa validar", async () => {
    mock({ list: { gateway_configured: false, items: [alias("triador")] } });
    renderWithProviders(<AIModelsTab />);
    expect(await screen.findByTestId("sem-gateway")).toHaveTextContent("Gateway de IA não configurado (LITELLM_BASE_URL)");
    fireEvent.click(screen.getByRole("button", { name: "Editar triador" }));
    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getByRole("button", { name: "Validar configuração" })).toBeDisabled();
    expect(within(dialog).getByRole("button", { name: "Salvar" })).toBeEnabled();
  });
});
