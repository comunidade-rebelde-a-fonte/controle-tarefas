/**
 * Valores decimais da API (strings exatas, `06 §12`) formatados em pt-BR sem passar por
 * float. O arredondamento é half-even, como a quantização do Fator R de origem
 * (`formatar_brl`, `formatar_percentual_br`). Somente apresentação: as contas são do backend.
 */

const PLAIN = /^(-?)(\d+)(?:\.(\d+))?$/;

interface Parts {
  negative: boolean;
  int: string;
  frac: string;
}

function parse(value: string): Parts | null {
  const m = PLAIN.exec(value.trim());
  if (!m) return null;
  return { negative: m[1] === "-", int: m[2] ?? "0", frac: m[3] ?? "" };
}

/** Soma 1 a uma string de dígitos ("0999" → "1000"). */
function increment(digits: string): string {
  const out = digits.split("");
  for (let i = out.length - 1; i >= 0; i--) {
    if (out[i] === "9") {
      out[i] = "0";
      continue;
    }
    out[i] = String(Number(out[i]) + 1);
    return out.join("");
  }
  return `1${out.join("")}`;
}

/** Arredonda half-even para `places` casas (ROUND_HALF_EVEN do decimal do Python). */
function round(p: Parts, places: number): Parts {
  const frac = p.frac.padEnd(places, "0");
  const kept = frac.slice(0, places);
  const rest = frac.slice(places);
  let up = false;
  if (rest !== "" && /[1-9]/.test(rest)) {
    const first = Number(rest[0]);
    const tail = /[1-9]/.test(rest.slice(1));
    const last = Number((p.int + kept).slice(-1));
    up = first > 5 || (first === 5 && (tail || last % 2 === 1));
  }
  let digits = p.int + kept;
  if (up) digits = increment(digits);
  const int = digits.slice(0, digits.length - places).replace(/^0+(?=\d)/, "") || "0";
  return { negative: p.negative, int, frac: digits.slice(digits.length - places) };
}

function group(int: string): string {
  return int.replace(/\B(?=(\d{3})+(?!\d))/g, ".");
}

function render(p: Parts, places: number): string {
  const r = round(p, places);
  const zero = /^0*$/.test(r.int + r.frac);
  const sign = r.negative && !zero ? "-" : "";
  return `${sign}${group(r.int)}${places > 0 ? `,${r.frac}` : ""}`;
}

/** Desloca a vírgula: multiplica por 10^n sem aritmética de float. */
function shift(p: Parts, n: number): Parts {
  const frac = p.frac.padEnd(n, "0");
  return { negative: p.negative, int: (p.int + frac.slice(0, n)).replace(/^0+(?=\d)/, ""), frac: frac.slice(n) };
}

/** "43740.00000000" → "R$ 43.740,00"; nulo → "—". */
export function formatMoney(value: string | null | undefined): string {
  if (value === null || value === undefined || value === "") return "—";
  const p = parse(value);
  if (!p) return value;
  const text = render(p, 2);
  return text.startsWith("-") ? `-R$ ${text.slice(1)}` : `R$ ${text}`;
}

/** Fração decimal → percentual: "0.2799" → "27,99%"; nulo → "—". */
export function formatRate(value: string | null | undefined, places = 2): string {
  if (value === null || value === undefined || value === "") return "—";
  const p = parse(value);
  if (!p) return value;
  return `${render(shift(p, 2), places)}%`;
}

/** Valor da API → texto de edição: "1234.50" → "1.234,50". */
export function moneyToInput(value: string | null | undefined): string {
  if (value === null || value === undefined || value === "") return "";
  const p = parse(value);
  return p ? render(p, 2) : value;
}

/**
 * Texto digitado → string decimal da API: "1.234,56", "1234,5", "1234.56" e "R$ 10" são
 * aceitos; vazio vale "0.00" (origem: valorBRParaDecimal). Devolve null se não for um valor
 * em reais válido (negativo, mais de 2 casas, letras).
 */
export function parseMoneyInput(input: string): string | null {
  const clean = input.trim().replace(/\s|R\$/g, "");
  if (clean === "") return "0.00";
  const normalized = clean.includes(",") ? clean.replace(/\./g, "").replace(",", ".") : clean;
  if (!/^\d+(\.\d{1,2})?$/.test(normalized)) return null;
  const [int = "0", cents = ""] = normalized.split(".");
  return `${int.replace(/^0+(?=\d)/, "")}.${cents.padEnd(2, "0")}`;
}

/** Valor da API é maior que zero? Sem float: basta haver dígito não nulo e não ser negativo. */
export function isPositive(value: string | null | undefined): boolean {
  if (!value) return false;
  const p = parse(value);
  return !!p && !p.negative && /[1-9]/.test(p.int + p.frac);
}
