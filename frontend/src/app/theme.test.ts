import { describe, expect, it } from "vitest";

/**
 * Guarda do tema InovaSala (TASK-029, D-043): as telas usam os tokens do index.css. As cores de
 * estado são `destructive`, `warning`, `success` e `info`; da paleta Tailwind direta ficam só as do
 * material (orange, yellow, neutral, zinc e white/black translúcidos). Cor da paleta clara antiga ou
 * hex solto no código quebra o teste.
 */
const SOURCES = import.meta.glob<string>(["../**/*.{ts,tsx}", "!../**/*.test.{ts,tsx}", "!../types/**", "!../test/**"], {
  query: "?raw",
  import: "default",
  eager: true,
});

const OFF_PALETTE =
  /(?<![\w-])(?:bg|text|border|ring|from|to|via|fill|stroke|outline|divide|decoration|placeholder)-(?:slate|gray|stone|red|amber|lime|green|emerald|teal|cyan|sky|blue|indigo|violet|purple|fuchsia|pink|rose)-\d{2,3}(?![\w-])/;
const SOLID_WHITE = /(?<![\w-])bg-white(?![\w/-])/;
const HEX = /["'[(\s]#[0-9a-fA-F]{6}\b/;

// Exceções com motivo: o botão secundário do material é branco, e a folha do original é papel.
const WHITE_ALLOWED = new Set(["components/ui/button-variants.ts", "features/reconciliation/OriginalViewer.tsx"]);
// O gráfico do histórico recebe as cores do material em constantes (o Recharts pede valores).
const HEX_ALLOWED = new Set(["features/dashboard/HistoryComparison.tsx"]);

describe("tema InovaSala", () => {
  it("lê as telas do frontend", () => {
    expect(Object.keys(SOURCES).length).toBeGreaterThan(100);
    expect(Object.keys(SOURCES)).toContain("./Layout.tsx");
  });

  it("as telas usam os tokens do tema, sem a paleta clara antiga", () => {
    const problems: string[] = [];
    for (const [key, text] of Object.entries(SOURCES)) {
      // As chaves são relativas a src/app: "./Layout.tsx" ou "../features/...".
      const rel = key.startsWith("./") ? `app/${key.slice(2)}` : key.replace(/^\.\.\//, "");
      text.split("\n").forEach((line, i) => {
        const where = `${rel}:${String(i + 1)}`;
        const off = OFF_PALETTE.exec(line);
        if (off) problems.push(`${where} ${off[0]}`);
        if (SOLID_WHITE.test(line) && !WHITE_ALLOWED.has(rel)) problems.push(`${where} bg-white`);
        if (HEX.test(line) && !HEX_ALLOWED.has(rel)) problems.push(`${where} cor em hex`);
      });
    }
    expect(problems).toEqual([]);
  });
});
