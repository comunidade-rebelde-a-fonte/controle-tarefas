/**
 * Formatadores de exibição (pt-BR). Somente apresentação: nenhuma regra de negócio.
 */

const DATE_ONLY = /^(\d{4})-(\d{2})-(\d{2})$/;
const COMPETENCE_KEY = /^(\d{4})-(\d{2})$/;

/** Data civil da API (`AAAA-MM-DD`) → `DD/MM/AAAA`. Não aplica fuso (data sem hora). */
export function formatDate(value: string | null | undefined): string {
  if (!value) return "";
  const m = DATE_ONLY.exec(value);
  if (!m) return value;
  return `${m[3] ?? ""}/${m[2] ?? ""}/${m[1] ?? ""}`;
}

const dateTimeFormatter = new Intl.DateTimeFormat("pt-BR", {
  day: "2-digit",
  month: "2-digit",
  year: "numeric",
  hour: "2-digit",
  minute: "2-digit",
});

/** Instante ISO 8601 (UTC) → `DD/MM/AAAA HH:mm` no fuso do navegador. */
export function formatDateTime(value: string | null | undefined): string {
  if (!value) return "";
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return value;
  return dateTimeFormatter.format(d);
}

/** Competência armazenada (`AAAA-MM`) → exibição `MM/AAAA`. */
export function formatCompetence(key: string | null | undefined): string {
  if (!key) return "";
  const m = COMPETENCE_KEY.exec(key);
  if (!m) return key;
  return `${m[2] ?? ""}/${m[1] ?? ""}`;
}

const percentFormatter = new Intl.NumberFormat("pt-BR", { maximumFractionDigits: 0 });

/** Fração 0..1 → `NN%`. */
export function formatPercent(fraction: number | null | undefined): string {
  if (fraction === null || fraction === undefined || Number.isNaN(fraction)) return "—";
  return `${percentFormatter.format(fraction * 100)}%`;
}
