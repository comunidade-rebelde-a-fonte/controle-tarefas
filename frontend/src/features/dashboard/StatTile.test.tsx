import { screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { renderWithProviders } from "@/test/render";
import { StatTile } from "./StatTile";

describe("StatTile", () => {
  it("é um link para o drill-down com o número no nome acessível", () => {
    renderWithProviders(
      <StatTile label="Atrasadas" value="14" metric={{ value: 14, drilldown: { path: "/obligations", query: { competence: "2026-09", state: "overdue" } } }} tone="danger" />,
    );
    expect(screen.getByRole("link", { name: "Atrasadas: 14" })).toHaveAttribute("href", "/obligations?competence=2026-09&state=overdue");
  });

  it("sem drill-down fica como grupo de leitura", () => {
    renderWithProviders(<StatTile label="Conclusão final" value="62%" />);
    expect(screen.queryByRole("link")).not.toBeInTheDocument();
    expect(screen.getByRole("group", { name: "Conclusão final: 62%" })).toBeInTheDocument();
  });
});
