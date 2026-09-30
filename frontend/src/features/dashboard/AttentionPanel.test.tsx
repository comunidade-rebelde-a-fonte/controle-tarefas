import { screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { Attention } from "@/services/dashboard";
import { renderWithProviders } from "@/test/render";
import { AttentionPanel } from "./AttentionPanel";

const empty = (key: Attention["categories"][number]["key"], label: string) => ({ key, label, count: 0, sample: [], drilldown: null });

const attention: Attention = {
  competence: "2026-09",
  categories: [
    {
      key: "OVERDUE",
      label: "Obrigações atrasadas",
      count: 7,
      sample: [
        {
          company: { id: "c1", internal_code: "101", legal_name: "Alfa Comércio" },
          title: "Fiscal — Entradas",
          detail: "A iniciar · venceu em 27/09/2026 (3 dias)",
          date: "2026-09-27",
          drilldown: { path: "/fiscal", query: { competence: "2026-09", q: "101", group: "normal" } },
        },
      ],
      drilldown: { path: "/obligations", query: { competence: "2026-09", state: "overdue" } },
    },
    empty("PENDING", "Pendências"),
    {
      key: "CERTIFICATE_DUE_SOON",
      label: "Certificados próximos do vencimento",
      count: 2,
      sample: [
        {
          company: { id: "c2", internal_code: "206", legal_name: "Fênix" },
          title: "Certificados digitais",
          detail: "Vence em 1 dia",
          date: "2026-10-01",
          drilldown: { path: "/companies/c2", query: { tab: "credentials" } },
        },
      ],
      drilldown: { path: "/credentials", query: { type: "DIGITAL_CERTIFICATE", filter: "due_60" } },
    },
  ],
};

describe("AttentionPanel", () => {
  it("oculta categorias vazias e liga amostra e 'ver todos' aos drill-downs", () => {
    renderWithProviders(<AttentionPanel attention={attention} />);
    const list = screen.getByRole("list", { name: "Categorias de atenção" });
    expect(within(list).queryByText("Pendências")).not.toBeInTheDocument();
    const overdue = within(list).getByRole("listitem", { name: "Obrigações atrasadas: 7" });
    expect(within(overdue).getByRole("link", { name: "Alfa Comércio" })).toHaveAttribute("href", "/fiscal?competence=2026-09&q=101&group=normal");
    expect(within(overdue).getByRole("link", { name: "Ver todos (7)" })).toHaveAttribute("href", "/obligations?competence=2026-09&state=overdue");
    const certs = within(list).getByRole("listitem", { name: "Certificados próximos do vencimento: 2" });
    expect(within(certs).getByRole("link", { name: "Ver todos (2)" })).toHaveAttribute("href", "/credentials?type=DIGITAL_CERTIFICATE&filter=due_60");
    expect(certs).toHaveTextContent("Vence em 1 dia · 01/10/2026");
  });

  it("sem ocorrências mostra a mensagem de tudo em dia", () => {
    renderWithProviders(<AttentionPanel attention={{ competence: "2026-09", categories: [empty("PENDING", "Pendências")] }} />);
    expect(screen.getByText("Nada exige atenção nesta competência.")).toBeInTheDocument();
  });
});
