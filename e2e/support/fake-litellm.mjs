// Gateway LiteLLM simulado para o E2E (TASK-008). Atende só GET /utils/model_info, com os mesmos
// corpos e status do LiteLLM v1.104.0; o contrato do gateway real é coberto por `make test-litellm`.
// Nenhuma chamada a provedor e nenhuma chave real: a chave abaixo é um valor de teste do E2E.
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

createServer((req, res) => {
  const url = new URL(req.url ?? "/", `http://127.0.0.1:${port}`);
  const send = (status, body) => {
    res.writeHead(status, { "Content-Type": "application/json" });
    res.end(JSON.stringify(body));
  };
  if (url.pathname === "/health/liveliness") return send(200, "I'm alive!");
  if (url.pathname !== "/utils/model_info") return send(404, { detail: "Not Found" });
  if (req.headers.authorization !== `Bearer ${key}`) {
    return send(401, { error: { message: "Authentication Error, No api key passed in.", type: "auth_error" } });
  }
  const model = url.searchParams.get("model") ?? "";
  const info = models[model];
  if (!info) return send(404, { detail: { error: `model=${model}, custom_llm_provider=None is not in the model cost map` } });
  const [provider, name] = model.split("/");
  return send(200, { model, custom_llm_provider: provider, model_info: { key: name, ...info } });
}).listen(port, "127.0.0.1");
