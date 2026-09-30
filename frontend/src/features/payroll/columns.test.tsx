import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { GridRow } from "@/services/grids";
import { factorRColumn, payrollTypeColumn } from "./columns";

const row = (snapshot: Partial<GridRow["snapshot"]>): GridRow => ({
  company: { id: "c1", internal_code: "101", legal_name: "Alfa", trade_name: null, cnpj: "00000001000101" },
  snapshot: { tax_regime: "SIMPLES_NACIONAL", regime_group: "SIMPLES", payroll_type: "PAYROLL", factor_r: true, responsible: null, ...snapshot },
  period: { id: "p1", state: "OPEN", closed_at: null, closed_by: null, general_note: null, version: 1 },
  progress: { denominator: 0, operational_done: 0, final_done: 0, blocking: 0, operational_progress: 1, final_completion: 1, ready_to_close: true },
  overdue: 0,
  nearest_deadline: null,
  ready_to_close: false,
  dependencies: null,
  cells: {},
});

describe("colunas da Folha", () => {
  it("Fator R aparece no Simples e fica vazio nos demais regimes", () => {
    const { rerender } = render(<>{factorRColumn.cell(row({ factor_r: true }))}</>);
    expect(screen.getByText("Sim")).toBeInTheDocument();
    rerender(<>{factorRColumn.cell(row({ factor_r: false }))}</>);
    expect(screen.getByText("Não")).toBeInTheDocument();
    rerender(<>{factorRColumn.cell(row({ tax_regime: "LUCRO_PRESUMIDO", regime_group: "NORMAL", factor_r: null }))}</>);
    expect(screen.getByLabelText("Fator R: não se aplica (somente Simples Nacional)")).toHaveTextContent("—");
    expect(screen.queryByText("Sim")).not.toBeInTheDocument();
  });

  it("destaca empresas Sem folha com texto e ícone", () => {
    const { rerender } = render(<>{payrollTypeColumn.cell(row({ payroll_type: "NO_PAYROLL" }))}</>);
    expect(screen.getByText("Sem folha")).toBeInTheDocument();
    rerender(<>{payrollTypeColumn.cell(row({ payroll_type: "PRO_LABORE" }))}</>);
    expect(screen.getByText("Pró-labore")).toBeInTheDocument();
  });
});
