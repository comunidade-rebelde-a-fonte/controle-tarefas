import { describe, expect, it } from "vitest";
import { selectAllState, toggleSelection } from "./selection";

const ids = ["a", "b", "c", "d", "e"];
const sorted = (s: Set<string>) => [...s].sort();

describe("toggleSelection", () => {
  it("alterna uma linha", () => {
    expect(sorted(toggleSelection(ids, new Set(), 1, null, false))).toEqual(["b"]);
    expect(sorted(toggleSelection(ids, new Set(["b"]), 1, null, false))).toEqual([]);
  });

  it("shift seleciona a faixa desde a âncora, em qualquer direção", () => {
    expect(sorted(toggleSelection(ids, new Set(["b"]), 3, 1, true))).toEqual(["b", "c", "d"]);
    expect(sorted(toggleSelection(ids, new Set(["d"]), 0, 3, true))).toEqual(["a", "b", "c", "d"]);
  });

  it("shift numa linha já selecionada desmarca a faixa", () => {
    expect(sorted(toggleSelection(ids, new Set(["a", "b", "c", "d"]), 3, 1, true))).toEqual(["a"]);
  });

  it("sem âncora ou sem shift, age só na linha", () => {
    expect(sorted(toggleSelection(ids, new Set(), 3, null, true))).toEqual(["d"]);
    expect(sorted(toggleSelection(ids, new Set(), 3, 0, false))).toEqual(["d"]);
  });

  it("âncora fora da lista (dados refiltrados) vira seleção simples", () => {
    expect(sorted(toggleSelection(["a", "b"], new Set(), 1, 7, true))).toEqual(["b"]);
  });

  it("não altera o conjunto recebido", () => {
    const selected = new Set(["a"]);
    toggleSelection(ids, selected, 2, 0, true);
    expect(sorted(selected)).toEqual(["a"]);
  });
});

describe("selectAllState", () => {
  it("indica todas, parcial ou nenhuma", () => {
    expect(selectAllState(ids, new Set())).toBe("none");
    expect(selectAllState(ids, new Set(["a"]))).toBe("some");
    expect(selectAllState(ids, new Set(ids))).toBe("all");
    expect(selectAllState(ids, new Set(["x"]))).toBe("none");
    expect(selectAllState([], new Set())).toBe("none");
  });
});
