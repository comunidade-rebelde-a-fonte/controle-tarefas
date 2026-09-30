/** Posição de uma célula do corpo da grade (índices a partir de 0). */
export interface CellPosition {
  row: number;
  col: number;
}

export interface GridBounds {
  rowCount: number;
  colCount: number;
  /** Linhas puladas por PageUp/PageDown. */
  pageSize: number;
}

/**
 * Próxima célula ativa para uma tecla de navigação no padrão ARIA grid:
 * setas, Home/End (linha), Ctrl+Home/Ctrl+End (grade) e PageUp/PageDown.
 * Devolve null quando a tecla não é de navegação.
 */
export function nextCellPosition(
  pos: CellPosition,
  key: string,
  ctrl: boolean,
  { rowCount, colCount, pageSize }: GridBounds,
): CellPosition | null {
  const lastRow = rowCount - 1;
  const lastCol = colCount - 1;
  const clampRow = (r: number) => Math.max(0, Math.min(lastRow, r));
  const clampCol = (c: number) => Math.max(0, Math.min(lastCol, c));
  switch (key) {
    case "ArrowUp":
      return { row: clampRow(pos.row - 1), col: pos.col };
    case "ArrowDown":
      return { row: clampRow(pos.row + 1), col: pos.col };
    case "ArrowLeft":
      return { row: pos.row, col: clampCol(pos.col - 1) };
    case "ArrowRight":
      return { row: pos.row, col: clampCol(pos.col + 1) };
    case "Home":
      return ctrl ? { row: 0, col: 0 } : { row: pos.row, col: 0 };
    case "End":
      return ctrl ? { row: lastRow, col: lastCol } : { row: pos.row, col: lastCol };
    case "PageUp":
      return { row: clampRow(pos.row - pageSize), col: pos.col };
    case "PageDown":
      return { row: clampRow(pos.row + pageSize), col: pos.col };
    default:
      return null;
  }
}
