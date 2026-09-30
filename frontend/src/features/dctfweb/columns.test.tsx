import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { GridRow } from "@/services/grids";
import { dependenciesColumn } from "./columns";

const row = (dependencies: GridRow["dependencies"]): GridRow => ({
  company: { id: "c1", internal_code: "101", legal_name: "Alfa", trade_name: null, cnpj: "00000001000101" },
  snapshot: { tax_regime: "LUCRO_PRESUMIDO", regime_group: "NORMAL", payroll_type: "PAYROLL", factor_r: null, responsible: null },
  period: { id: "p1", state: "OPEN", closed_at: null, closed_by: null, general_note: null, version: 1 },
  progress: { denominator: 0, operational_done: 0, final_done: 0, blocking: 0, operational_progress: 1, final_completion: 1, ready_to_close: false },
  overdue: 0,
  nearest_deadline: null,
  ready_to_close: false,
  dependencies,
  cells: {},
});

describe("coluna Dependências", () => {
  it("mostra Apta ou Pendente com os componentes", () => {
    const { rerender } = render(<>{dependenciesColumn.cell(row({ ready: true, pending: [] }))}</>);
    expect(screen.getByText("Apta")).toBeInTheDocument();
    rerender(
      <>
        {dependenciesColumn.cell(
          row({
            ready: false,
            pending: [
              { obligation_type_id: "t1", code: "ESOCIAL", name: "eSocial", status_code: "NOT_STARTED" },
              { obligation_type_id: "t2", code: "REINF_RETIDOS", name: "REINF Retidos", status_code: "PENDING" },
            ],
          }),
        )}
      </>,
    );
    expect(screen.getByText("Pendente: eSocial, REINF Retidos")).toBeInTheDocument();
  });
});
