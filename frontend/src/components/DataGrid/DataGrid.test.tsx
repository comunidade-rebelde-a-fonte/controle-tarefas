import { act, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterAll, beforeAll, describe, expect, it, vi } from "vitest";
import { DataGrid, type DataGridColumn, EMPTY_GRID_MESSAGE } from "./index";

// O jsdom não faz layout: a área rolável mediria 0 px e a virtualização não renderizaria
// linhas. Simula uma viewport de 1280 × 600 e o scrollTo que atualiza scrollTop.
const originalScrollTo = Object.getOwnPropertyDescriptor(HTMLElement.prototype, "scrollTo");
beforeAll(() => {
  vi.spyOn(HTMLElement.prototype, "offsetHeight", "get").mockReturnValue(600);
  vi.spyOn(HTMLElement.prototype, "offsetWidth", "get").mockReturnValue(1280);
  vi.spyOn(HTMLElement.prototype, "clientHeight", "get").mockReturnValue(600);
  vi.spyOn(HTMLElement.prototype, "scrollHeight", "get").mockReturnValue(100_000);
  Object.defineProperty(HTMLElement.prototype, "scrollTo", {
    configurable: true,
    value(this: HTMLElement, opts: ScrollToOptions) {
      // scrollTop do jsdom é sempre 0; guarda o valor na própria instância.
      if (opts.top !== undefined) Object.defineProperty(this, "scrollTop", { configurable: true, writable: true, value: opts.top });
      this.dispatchEvent(new Event("scroll"));
    },
  });
});
afterAll(() => {
  vi.restoreAllMocks();
  if (originalScrollTo) Object.defineProperty(HTMLElement.prototype, "scrollTo", originalScrollTo);
  else Reflect.deleteProperty(HTMLElement.prototype, "scrollTo");
});

interface Row {
  id: string;
  name: string;
  icms: string;
  ipi: string;
}

const rows: Row[] = [
  { id: "a", name: "Alfa Comércio", icms: "A iniciar", ipi: "Baixadas" },
  { id: "b", name: "Beta Serviços", icms: "Lançadas", ipi: "Não se aplica" },
  { id: "c", name: "Gama Indústria", icms: "Fechadas", ipi: "Sem movimento" },
  { id: "d", name: "Delta Ltda", icms: "A iniciar", ipi: "A iniciar" },
];

const columns: DataGridColumn<Row>[] = [
  { id: "company", header: "Empresa", cell: (r) => r.name },
  { id: "icms", header: "ICMS", cell: (r) => r.icms },
  { id: "ipi", header: "IPI", cell: (r) => r.ipi },
];

function cell(container: HTMLElement, row: number, col: number): HTMLElement {
  const el = container.querySelector<HTMLElement>(`[data-cell="${row}:${col}"]`);
  if (!el) throw new Error(`célula ${row}:${col} não renderizada`);
  return el;
}

function SelectableGrid({ onCellActivate }: { onCellActivate?: (row: Row, col: string) => void }) {
  const [selected, setSelected] = useState<Set<string>>(new Set());
  return (
    <DataGrid
      aria-label="Grade Fiscal"
      rows={rows}
      columns={columns}
      getRowId={(r) => r.id}
      onCellActivate={onCellActivate}
      selection={{ selected, onChange: setSelected, rowLabel: (r) => `Selecionar ${r.name}` }}
    />
  );
}

describe("DataGrid", () => {
  it("expõe a estrutura ARIA grid com cabeçalhos de coluna e de linha", () => {
    render(<DataGrid aria-label="Grade Fiscal" rows={rows} columns={columns} getRowId={(r) => r.id} />);
    const grid = screen.getByRole("grid", { name: "Grade Fiscal" });
    expect(grid).toHaveAttribute("aria-rowcount", "5");
    expect(grid).toHaveAttribute("aria-colcount", "3");
    expect(within(grid).getAllByRole("columnheader").map((h) => h.textContent)).toEqual(["Empresa", "ICMS", "IPI"]);
    expect(within(grid).getAllByRole("rowheader").map((h) => h.textContent)).toEqual(rows.map((r) => r.name));
  });

  it("fixa o cabeçalho e a primeira coluna na rolagem", () => {
    const { container } = render(<DataGrid aria-label="Grade" rows={rows} columns={columns} getRowId={(r) => r.id} />);
    const [first, second] = screen.getAllByRole("columnheader");
    expect(first).toHaveClass("sticky", "top-0");
    expect(first).toHaveStyle({ left: "0px" });
    expect(second).toHaveClass("sticky", "top-0");
    expect(cell(container, 2, 0)).toHaveClass("sticky");
    expect(cell(container, 2, 1)).not.toHaveClass("sticky");
  });

  it("navega por teclado entre as células (setas, Home/End, Ctrl+Home/End)", async () => {
    const user = userEvent.setup();
    const { container } = render(<DataGrid aria-label="Grade" rows={rows} columns={columns} getRowId={(r) => r.id} />);

    await user.tab();
    expect(cell(container, 0, 0)).toHaveFocus();

    await user.keyboard("{ArrowRight}");
    expect(cell(container, 0, 1)).toHaveFocus();
    await user.keyboard("{ArrowDown}{ArrowDown}");
    expect(cell(container, 2, 1)).toHaveFocus();
    await user.keyboard("{ArrowUp}");
    expect(cell(container, 1, 1)).toHaveFocus();

    await user.keyboard("{End}");
    expect(cell(container, 1, 2)).toHaveFocus();
    await user.keyboard("{ArrowRight}");
    expect(cell(container, 1, 2)).toHaveFocus(); // não sai da grade
    await user.keyboard("{Home}");
    expect(cell(container, 1, 0)).toHaveFocus();
    await user.keyboard("{Control>}{End}{/Control}");
    expect(cell(container, 3, 2)).toHaveFocus();
    await user.keyboard("{Control>}{Home}{/Control}");
    expect(cell(container, 0, 0)).toHaveFocus();

    // Roving tabindex: só a célula ativa entra na ordem do Tab.
    expect(container.querySelectorAll('[data-cell][tabindex="0"]')).toHaveLength(1);
  });

  it("Enter e Espaço ativam o editor da célula", async () => {
    const user = userEvent.setup();
    const onCellActivate = vi.fn();
    const { container } = render(
      <DataGrid aria-label="Grade" rows={rows} columns={columns} getRowId={(r) => r.id} onCellActivate={onCellActivate} />,
    );
    await user.tab();
    await user.keyboard("{ArrowDown}{ArrowRight}{Enter}");
    expect(onCellActivate).toHaveBeenLastCalledWith(rows[1], "icms");
    await user.keyboard("{ArrowRight} ");
    expect(onCellActivate).toHaveBeenLastCalledWith(rows[1], "ipi");
    expect(onCellActivate).toHaveBeenCalledTimes(2);
    expect(cell(container, 1, 2)).toHaveFocus();
  });

  it("clicar numa célula a torna ativa para o teclado", async () => {
    const user = userEvent.setup();
    const { container } = render(<DataGrid aria-label="Grade" rows={rows} columns={columns} getRowId={(r) => r.id} />);
    await user.click(cell(container, 2, 2));
    await user.keyboard("{ArrowLeft}");
    expect(cell(container, 2, 1)).toHaveFocus();
  });

  it("teclas dentro de um controle da célula não movem a grade", async () => {
    const user = userEvent.setup();
    const onOpen = vi.fn<(id: string) => void>();
    const onCellActivate = vi.fn();
    const withButton: DataGridColumn<Row>[] = [
      columns[0] as DataGridColumn<Row>,
      {
        id: "icms",
        header: "ICMS",
        cell: (r) => (
          <button type="button" onClick={() => onOpen(r.id)}>
            {r.icms}
          </button>
        ),
      },
    ];
    render(<DataGrid aria-label="Grade" rows={rows} columns={withButton} getRowId={(r) => r.id} onCellActivate={onCellActivate} />);
    const button = screen.getAllByRole("button", { name: "A iniciar" })[0] as HTMLElement;
    button.focus();
    await user.keyboard("{ArrowDown}{Enter}");
    expect(button).toHaveFocus();
    expect(onOpen).toHaveBeenCalledWith("a");
    expect(onCellActivate).not.toHaveBeenCalled();
  });

  it("seleciona linhas por checkbox, com shift-click em faixa e selecionar todas", async () => {
    const user = userEvent.setup();
    render(<SelectableGrid />);
    const grid = screen.getByRole("grid");
    expect(grid).toHaveAttribute("aria-multiselectable", "true");
    const all = screen.getByRole("checkbox", { name: "Selecionar todas as linhas" });
    const rowOf = (name: string) => screen.getByRole("rowheader", { name }).closest("tr") as HTMLElement;

    await user.click(screen.getByRole("checkbox", { name: "Selecionar Alfa Comércio" }));
    expect(rowOf("Alfa Comércio")).toHaveAttribute("aria-selected", "true");
    expect(all).toBePartiallyChecked();

    await user.keyboard("{Shift>}");
    await user.click(screen.getByRole("checkbox", { name: "Selecionar Gama Indústria" }));
    await user.keyboard("{/Shift}");
    for (const name of ["Alfa Comércio", "Beta Serviços", "Gama Indústria"]) {
      expect(rowOf(name)).toHaveAttribute("aria-selected", "true");
    }
    expect(rowOf("Delta Ltda")).toHaveAttribute("aria-selected", "false");

    await user.click(all);
    expect(all).toBeChecked();
    expect(screen.getAllByRole("checkbox", { checked: true })).toHaveLength(rows.length + 1);

    await user.click(all);
    expect(screen.queryAllByRole("checkbox", { checked: true })).toHaveLength(0);
  });

  it("Espaço na coluna de seleção alterna a linha e Shift+Espaço seleciona a faixa", async () => {
    const user = userEvent.setup();
    const onCellActivate = vi.fn();
    const { container } = render(<SelectableGrid onCellActivate={onCellActivate} />);
    // O checkbox "selecionar todas" é um controle próprio no cabeçalho; o Tab seguinte entra na grade.
    await user.tab();
    expect(screen.getByRole("checkbox", { name: "Selecionar todas as linhas" })).toHaveFocus();
    await user.tab();
    expect(cell(container, 0, 0)).toHaveFocus();
    await user.keyboard(" ");
    expect(screen.getByRole("checkbox", { name: "Selecionar Alfa Comércio" })).toBeChecked();
    await user.keyboard("{ArrowDown}{ArrowDown}{ArrowDown}{Shift>} {/Shift}");
    expect(screen.getAllByRole("checkbox", { checked: true })).toHaveLength(rows.length + 1);
    expect(onCellActivate).not.toHaveBeenCalled();
  });

  it("mostra o estado vazio mantendo os cabeçalhos", () => {
    render(<DataGrid aria-label="Grade" rows={[]} columns={columns} getRowId={(r) => r.id} />);
    expect(screen.getByText(EMPTY_GRID_MESSAGE)).toBeInTheDocument();
    expect(screen.getAllByRole("columnheader")).toHaveLength(3);
  });

  it("renderiza 300 empresas × 20 colunas virtualizando as linhas", () => {
    const many = Array.from({ length: 300 }, (_, i) => ({ id: `c${i}`, name: `Empresa ${i + 1}` }));
    const cols: DataGridColumn<{ id: string; name: string }>[] = [
      { id: "company", header: "Empresa", cell: (r) => r.name },
      ...Array.from({ length: 19 }, (_, j) => ({
        id: `ob${j}`,
        header: `Obrigação ${j + 1}`,
        cell: (r: { id: string }) => `${r.id}-${j}`,
      })),
    ];
    render(<DataGrid aria-label="Grade grande" rows={many} columns={cols} getRowId={(r) => r.id} />);

    const grid = screen.getByRole("grid", { name: "Grade grande" });
    expect(grid).toHaveAttribute("aria-rowcount", "301");
    expect(grid).toHaveAttribute("aria-colcount", "20");
    expect(within(grid).getAllByRole("columnheader")).toHaveLength(20);
    const rendered = grid.querySelectorAll("tbody tr[aria-rowindex]");
    expect(rendered.length).toBeGreaterThan(0);
    expect(rendered.length).toBeLessThan(50); // virtualização ativa: não monta as 300 linhas
    expect(screen.getByRole("rowheader", { name: "Empresa 1" })).toBeInTheDocument();
    expect(screen.queryByRole("rowheader", { name: "Empresa 300" })).not.toBeInTheDocument();
  });

  it("continua alcançável pelo Tab quando a célula ativa sai da janela virtual", async () => {
    const user = userEvent.setup();
    const many = Array.from({ length: 300 }, (_, i) => ({ id: `c${i}`, name: `Empresa ${i + 1}` }));
    const cols: DataGridColumn<{ id: string; name: string }>[] = [{ id: "company", header: "Empresa", cell: (r) => r.name }];
    const { container } = render(<DataGrid aria-label="Grade grande" rows={many} columns={cols} getRowId={(r) => r.id} />);
    const scroller = screen.getByRole("grid").parentElement as HTMLElement;
    act(() => scroller.scrollTo({ top: 150 * 36 }));
    await screen.findByRole("rowheader", { name: "Empresa 151" });
    expect(screen.queryByRole("rowheader", { name: "Empresa 1" })).not.toBeInTheDocument();

    const tabbable = container.querySelectorAll<HTMLElement>('[data-cell][tabindex="0"]');
    expect(tabbable).toHaveLength(1);
    await user.tab();
    expect(tabbable[0]).toHaveFocus();
    await user.keyboard("{ArrowDown}");
    const [row] = (document.activeElement?.getAttribute("data-cell") ?? "").split(":");
    expect(Number(row)).toBeGreaterThan(100);
  });

  it("o teclado leva a linhas fora da janela virtual e as foca", async () => {
    const user = userEvent.setup();
    const many = Array.from({ length: 300 }, (_, i) => ({ id: `c${i}`, name: `Empresa ${i + 1}` }));
    const cols: DataGridColumn<{ id: string; name: string }>[] = [
      { id: "company", header: "Empresa", cell: (r) => r.name },
      { id: "icms", header: "ICMS", cell: (r) => `ICMS ${r.name}` },
    ];
    const onCellActivate = vi.fn();
    const { container } = render(
      <DataGrid aria-label="Grade grande" rows={many} columns={cols} getRowId={(r) => r.id} onCellActivate={onCellActivate} />,
    );
    await user.tab();
    await user.keyboard("{Control>}{End}{/Control}");
    expect(await screen.findByRole("rowheader", { name: "Empresa 300" })).toBeInTheDocument();
    expect(cell(container, 299, 1)).toHaveFocus();
    await user.keyboard("{Enter}");
    expect(onCellActivate).toHaveBeenCalledWith(many[299], "icms");
    expect(screen.queryByRole("rowheader", { name: "Empresa 1" })).not.toBeInTheDocument();
  });
});
