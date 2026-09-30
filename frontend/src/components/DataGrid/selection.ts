/**
 * Alterna a seleção da linha `index`. Com shift e uma âncora, aplica à faixa
 * âncora..index o novo estado da linha clicada (como em planilhas e clientes de e-mail).
 */
export function toggleSelection(
  ids: readonly string[],
  selected: ReadonlySet<string>,
  index: number,
  anchor: number | null,
  shift: boolean,
): Set<string> {
  const next = new Set(selected);
  const id = ids[index];
  if (id === undefined) return next;
  const select = !selected.has(id);
  const [from, to] = shift && anchor !== null && anchor < ids.length ? [Math.min(anchor, index), Math.max(anchor, index)] : [index, index];
  for (let i = from; i <= to; i++) {
    const rowId = ids[i];
    if (rowId === undefined) continue;
    if (select) next.add(rowId);
    else next.delete(rowId);
  }
  return next;
}

/** Estado do checkbox "selecionar todas": marcado, parcial ou vazio. */
export function selectAllState(ids: readonly string[], selected: ReadonlySet<string>): "all" | "some" | "none" {
  let count = 0;
  for (const id of ids) if (selected.has(id)) count++;
  if (count === 0) return "none";
  return count === ids.length ? "all" : "some";
}
