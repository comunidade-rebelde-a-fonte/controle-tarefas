/**
 * CNPJ: a API armazena 14 dígitos; a interface exibe formatado e copia só dígitos [RN-014].
 * A validação dos dígitos verificadores é feita na API.
 */

/** Remove tudo que não for dígito. */
export function sanitizeCnpj(value: string): string {
  return value.replace(/\D/g, "");
}

/** `00000001000101` → `00.000.001/0001-01` (valores que não têm 14 dígitos voltam inalterados). */
export function formatCnpj(value: string | null | undefined): string {
  if (!value) return "";
  const d = sanitizeCnpj(value);
  if (d.length !== 14) return value;
  return `${d.slice(0, 2)}.${d.slice(2, 5)}.${d.slice(5, 8)}/${d.slice(8, 12)}-${d.slice(12, 14)}`;
}

/** Máscara progressiva para campos de digitação. */
export function maskCnpjInput(value: string): string {
  const d = sanitizeCnpj(value).slice(0, 14);
  const parts = [d.slice(0, 2), d.slice(2, 5), d.slice(5, 8), d.slice(8, 12), d.slice(12, 14)];
  let out = parts[0] ?? "";
  if (d.length > 2) out += `.${parts[1] ?? ""}`;
  if (d.length > 5) out += `.${parts[2] ?? ""}`;
  if (d.length > 8) out += `/${parts[3] ?? ""}`;
  if (d.length > 12) out += `-${parts[4] ?? ""}`;
  return out;
}

/** Termo de busca que parece CNPJ (dígitos com pontuação opcional). */
export function looksLikeCnpj(term: string): boolean {
  return /^[\d./\-\s]+$/.test(term.trim()) && sanitizeCnpj(term).length >= 3;
}
