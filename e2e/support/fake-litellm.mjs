// Gateway LiteLLM simulado para o E2E. Atende GET /utils/model_info (TASK-008), com os mesmos
// corpos e status do LiteLLM v1.104.0, e POST /v1/chat/completions (TASK-014b): o LLM
// determinístico dos agentes do Fator R no E2E (FR-20; origem: LLM_PROVIDER=fake). O contrato do
// gateway real é coberto por `make test-litellm`. Nenhuma chamada a provedor e nenhuma chave
// real: a chave abaixo é um valor de teste do E2E.
import { createServer } from "node:http";

const port = Number(process.env.FAKE_LITELLM_PORT ?? "8093");
const key = process.env.FAKE_LITELLM_KEY ?? "";

const full = {
  mode: "chat",
  supports_response_schema: true,
  supports_vision: true,
  max_output_tokens: 128000,
  supported_openai_params: ["temperature", "max_tokens", "max_completion_tokens", "response_format"],
};

// Tabela de modelos: os padrões do .env.example (D-015), outro provedor e um modelo sem visão.
const models = {
  "openai/gpt-6-luna": full,
  "openai/gpt-6-sol": full,
  "openai/gpt-6-astra": full,
  "anthropic/claude-sonnet-5": full,
  "openai/gpt-6-texto": { ...full, supports_vision: false, max_output_tokens: 32000 },
};

// Texto redigido pelo "modelo": sem número, data, valor por extenso nem recomendação, para passar
// pelo validador do texto (ValidateText) e chegar à tela como texto do LLM.
const RENDERED = "Resposta redigida pelo modelo de teste a partir da decisão calculada pelo sistema.";

/** Intenção por palavras-chave do pedido, no formato estrito do plan (classificar_intencao). */
function classify(messages) {
  const user = [...messages].reverse().find((m) => m.role === "user");
  const text = String(user?.content ?? "").toLowerCase();
  let intencao = "status_empresa";
  if (/prioriz|fila|carteira/.test(text)) intencao = "priorizar";
  else if (/simul|corrig|reforç|reforc/.test(text)) intencao = "simular";
  else if (/explic|por que|porque/.test(text)) intencao = "explicar";
  return JSON.stringify({ intencao, company_ref: null, pa: null, meta: null });
}

let calls = 0;

function readBody(req) {
  return new Promise((resolve) => {
    const chunks = [];
    req.on("data", (c) => chunks.push(c));
    req.on("end", () => {
      try {
        resolve(JSON.parse(Buffer.concat(chunks).toString("utf8")));
      } catch {
        resolve(null);
      }
    });
  });
}

createServer(async (req, res) => {
  const url = new URL(req.url ?? "/", `http://127.0.0.1:${port}`);
  const send = (status, body, headers = {}) => {
    res.writeHead(status, { "Content-Type": "application/json", ...headers });
    res.end(JSON.stringify(body));
  };
  if (url.pathname === "/health/liveliness") return send(200, "I'm alive!");
  const known = url.pathname === "/utils/model_info" || (req.method === "POST" && url.pathname === "/v1/chat/completions");
  if (!known) return send(404, { detail: "Not Found" });
  if (req.headers.authorization !== `Bearer ${key}`) {
    return send(401, { error: { message: "Authentication Error, No api key passed in.", type: "auth_error" } });
  }
  if (url.pathname === "/v1/chat/completions") {
    const body = await readBody(req);
    if (!body || !models[body.model]) return send(400, { error: { message: `Invalid model name passed in model=${body?.model}`, type: "invalid_request_error" } });
    calls += 1;
    const content = body.response_format?.type === "json_schema" ? classify(body.messages ?? []) : RENDERED;
    return send(
      200,
      {
        id: `chatcmpl-e2e-${calls}`,
        object: "chat.completion",
        created: 1,
        model: body.model.split("/")[1],
        choices: [{ index: 0, message: { role: "assistant", content }, finish_reason: "stop" }],
        usage: { prompt_tokens: 120, completion_tokens: 24, total_tokens: 144 },
      },
      { "x-litellm-response-cost": "0.000042", "x-litellm-call-id": `e2e-call-${calls}` },
    );
  }
  const model = url.searchParams.get("model") ?? "";
  const info = models[model];
  if (!info) return send(404, { detail: { error: `model=${model}, custom_llm_provider=None is not in the model cost map` } });
  const [provider, name] = model.split("/");
  return send(200, { model, custom_llm_provider: provider, model_info: { key: name, ...info } });
}).listen(port, "127.0.0.1");
