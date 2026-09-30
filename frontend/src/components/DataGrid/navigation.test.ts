import { describe, expect, it } from "vitest";
import { nextCellPosition } from "./navigation";

const bounds = { rowCount: 30, colCount: 5, pageSize: 10 };

describe("nextCellPosition", () => {
  it.each([
    ["ArrowUp", false, { row: 4, col: 2 }],
    ["ArrowDown", false, { row: 6, col: 2 }],
    ["ArrowLeft", false, { row: 5, col: 1 }],
    ["ArrowRight", false, { row: 5, col: 3 }],
    ["Home", false, { row: 5, col: 0 }],
    ["End", false, { row: 5, col: 4 }],
    ["Home", true, { row: 0, col: 0 }],
    ["End", true, { row: 29, col: 4 }],
    ["PageUp", false, { row: 0, col: 2 }],
    ["PageDown", false, { row: 15, col: 2 }],
  ])("%s (ctrl=%s)", (key, ctrl, want) => {
    expect(nextCellPosition({ row: 5, col: 2 }, key, ctrl, bounds)).toEqual(want);
  });

  it("não sai dos limites da grade", () => {
    expect(nextCellPosition({ row: 0, col: 0 }, "ArrowUp", false, bounds)).toEqual({ row: 0, col: 0 });
    expect(nextCellPosition({ row: 0, col: 0 }, "ArrowLeft", false, bounds)).toEqual({ row: 0, col: 0 });
    expect(nextCellPosition({ row: 29, col: 4 }, "ArrowDown", false, bounds)).toEqual({ row: 29, col: 4 });
    expect(nextCellPosition({ row: 29, col: 4 }, "ArrowRight", false, bounds)).toEqual({ row: 29, col: 4 });
    expect(nextCellPosition({ row: 25, col: 4 }, "PageDown", false, bounds)).toEqual({ row: 29, col: 4 });
  });

  it("ignora teclas que não são de navegação", () => {
    expect(nextCellPosition({ row: 1, col: 1 }, "Enter", false, bounds)).toBeNull();
    expect(nextCellPosition({ row: 1, col: 1 }, "a", false, bounds)).toBeNull();
  });
});
