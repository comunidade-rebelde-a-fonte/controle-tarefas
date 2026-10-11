import { screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { renderWithProviders } from "@/test/render";
import { ReconciliationNav } from "./ReconciliationLayout";

const COMPANY = "00000000-0000-4000-8000-0000000000c1";

describe("ReconciliationNav", () => {
  it("as seções da área, com a atual marcada", () => {
    renderWithProviders(<ReconciliationNav />, { route: "/reconciliation/ledger" });
    const nav = screen.getByRole("navigation", { name: "Seções da Conciliação" });
    expect(within(nav).getAllByRole("link").map((a) => a.textContent)).toEqual([
      "Fila de revisão",
      "Documentos",
      "Lançamentos",
      "Razão",
      "Plano de contas",
      "Regras",
      "Métricas",
      "Playbooks",
    ]);
    expect(within(nav).getByRole("link", { name: "Razão" })).toHaveAttribute("aria-current", "page");
    expect(within(nav).getByRole("link", { name: "Fila de revisão" })).not.toHaveAttribute("aria-current");
    expect(within(nav).getByRole("link", { name: "Regras" })).toHaveAttribute("href", "/reconciliation/rules");
  });

  it("a empresa escolhida acompanha a troca de seção; os demais filtros ficam", () => {
    renderWithProviders(<ReconciliationNav />, { route: `/reconciliation/ledger?company_id=${COMPANY}&from=2026-08&account=4.1.01` });
    const nav = screen.getByRole("navigation", { name: "Seções da Conciliação" });
    expect(within(nav).getByRole("link", { name: "Plano de contas" })).toHaveAttribute("href", `/reconciliation/chart?company_id=${COMPANY}`);
    expect(within(nav).getByRole("link", { name: "Fila de revisão" })).toHaveAttribute("href", `/reconciliation?company_id=${COMPANY}`);
  });
});
