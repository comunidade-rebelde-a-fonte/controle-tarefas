/**
 * Link para uma aba da ficha que mantém a competência da ficha (TASK-025, D-039); os filtros da
 * aba atual (classe, status) não passam para a outra.
 */
export function fichaHref(companyId: string, params: URLSearchParams, tab: string, extra: Record<string, string> = {}): string {
  const next = new URLSearchParams();
  next.set("tab", tab);
  const competence = params.get("competence");
  if (competence) next.set("competence", competence);
  for (const [k, v] of Object.entries(extra)) next.set(k, v);
  return `/companies/${companyId}?${next.toString()}`;
}
