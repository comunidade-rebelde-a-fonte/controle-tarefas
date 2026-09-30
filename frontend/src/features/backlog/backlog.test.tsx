import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ObligationList } from "@/components/ObligationList";
import type { Backlog } from "@/services/dashboard";
import { renderWithProviders } from "@/test/render";
import { backlogQueryFromParams } from "./query";

afterEach(() => {
  vi.unstubAllGlobals();
});

const data: Backlog = {
  total: 3,
  page: 1,
  page_size: 2,
  items: [
    {
      kind: "OBLIGATION", instance_id: "i1", competence: "2026-08",
      company: { id: "c1", internal_code: "101", legal_name: "Alfa Comércio", trade_name: null, cnpj: "00000001000101" },
      module: "FISCAL", obligation_type: { code: "FISCAL_ICMS", name: "ICMS" }, status_code: "NOT_STARTED", status_label: "A iniciar",
      due_date: "2026-09-20", days_overdue: 10, responsible: { id: "u1", name: "Ana Souza" },
      drilldown: { path: "/fiscal", query: { competence: "2026-08", q: "101" } },
    },
    {
      kind: "MODULE", instance_id: null, competence: "2026-07",
      company: { id: "c2", internal_code: "102", legal_name: "Beta Serviços", trade_name: null, cnpj: "00000002000102" },
      module: "ACCOUNTING", obligation_type: null, status_code: null, status_label: "Módulo aberto",
      due_date: null, days_overdue: 0, responsible: null,
      drilldown: { path: "/accounting", query: { competence: "2026-07", q: "102" } },
    },
  ],
};

describe("ObligationList", () => {
  it("mostra os campos do §25.2, o atraso e abre a grade do item", async () => {
    const onPage = vi.fn();
    renderWithProviders(<ObligationList aria-label="Backlog" data={data} isLoading={false} error={null} onPage={onPage} />);
    const rows = screen.getAllByRole("row").slice(1);
    expect(rows).toHaveLength(2);
    expect(rows[0]).toHaveTextContent("Alfa Comércio");
    expect(rows[0]).toHaveTextContent("00.000.001/0001-01");
    expect(rows[0]).toHaveTextContent("08/2026");
    expect(rows[0]).toHaveTextContent("Fiscal");
    expect(rows[0]).toHaveTextContent("ICMS");
    expect(rows[0]).toHaveTextContent("20/09/2026");
    expect(rows[0]).toHaveTextContent("10");
    expect(rows[0]).toHaveTextContent("Ana Souza");
    expect(rows[1]).toHaveTextContent("Módulo aberto");
    expect(screen.getByRole("link", { name: "Alfa Comércio" })).toHaveAttribute("href", "/fiscal?competence=2026-08&q=101");
    expect(screen.getByText("1–2 de 3")).toBeInTheDocument();
    await userEvent.setup().click(screen.getByRole("button", { name: "Próxima página" }));
    expect(onPage).toHaveBeenCalledWith(2);
  });
});

describe("backlogQueryFromParams", () => {
  it("leva os filtros válidos da URL para a API e ignora os inválidos", () => {
    const p = new URLSearchParams("state=overdue&module=FISCAL&status=PENDING&q=%20alfa%20&overdue_min=5&overdue_max=x&sort=due_date&page=2&regime_group=SIMPLES&tax_regime=MEI");
    expect(backlogQueryFromParams(p, { competence: "2026-09" })).toEqual({
      competence: "2026-09", state: "overdue", module: "FISCAL", status: "PENDING", q: "alfa", overdue_min: 5,
      sort: "due_date", page: 2, regime_group: "SIMPLES",
    });
    expect(backlogQueryFromParams(new URLSearchParams("state=x&module=CREDENTIALS"), { before: "2026-10" })).toEqual({ before: "2026-10" });
  });
});
