/**
 * Competência mensal: armazenada como `AAAA-MM`, exibida como `MM/AAAA` [PRD §5.3].
 * Somente formatação e navegação; a competência corrente vem da API.
 */

const KEY = /^(\d{4})-(0[1-9]|1[0-2])$/;

export interface Competence {
  year: number;
  month: number; // 1..12
}

export function parseCompetence(key: string): Competence | null {
  const m = KEY.exec(key);
  if (!m) return null;
  return { year: Number(m[1]), month: Number(m[2]) };
}

export function isCompetenceKey(key: string | null | undefined): key is string {
  return !!key && KEY.test(key);
}

export function competenceKey(c: Competence): string {
  return `${String(c.year).padStart(4, "0")}-${String(c.month).padStart(2, "0")}`;
}

/** `AAAA-MM` → `MM/AAAA` (valores inválidos voltam inalterados). */
export function displayCompetence(key: string | null | undefined): string {
  if (!key) return "";
  const c = parseCompetence(key);
  if (!c) return key;
  return `${String(c.month).padStart(2, "0")}/${String(c.year)}`;
}

/** Desloca `AAAA-MM` em n meses (virada de ano incluída). */
export function addMonths(key: string, n: number): string {
  const c = parseCompetence(key);
  if (!c) throw new Error(`competência inválida: ${key}`);
  const index = c.year * 12 + (c.month - 1) + n;
  return competenceKey({ year: Math.floor(index / 12), month: (index % 12) + 1 });
}

export const nextCompetence = (key: string): string => addMonths(key, 1);
export const prevCompetence = (key: string): string => addMonths(key, -1);

/** Compara duas competências `AAAA-MM` (ordem lexicográfica = cronológica). */
export function compareCompetence(a: string, b: string): number {
  return a < b ? -1 : a > b ? 1 : 0;
}
