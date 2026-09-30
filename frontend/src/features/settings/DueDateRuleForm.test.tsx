import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { DueDateRuleForm } from "./DueDateRuleForm";

const rule = {
  id: "r1",
  name: "Provisória — dia 20 do mês seguinte",
  month_offset: 1,
  base_day: 20,
  day_type: "CALENDAR" as const,
  non_business_day_adjustment: "PREVIOUS" as const,
  calendar_id: null,
  notes: "A VALIDAR (Q1)",
  version: 1,
};

describe("DueDateRuleForm", () => {
  it("valida o dia-base entre 1 e 31 e o deslocamento entre 0 e 12", async () => {
    const onSubmit = vi.fn<(values: unknown) => Promise<void>>(() => Promise.resolve());
    const user = userEvent.setup();
    render(<DueDateRuleForm onSubmit={onSubmit} onCancel={() => undefined} submitting={false} />);

    await user.type(screen.getByLabelText(/Nome/), "Nova regra");
    await user.clear(screen.getByLabelText(/Dia-base/));
    await user.type(screen.getByLabelText(/Dia-base/), "32");
    await user.clear(screen.getByLabelText(/Meses após/));
    await user.type(screen.getByLabelText(/Meses após/), "13");
    await user.click(screen.getByRole("button", { name: "Salvar regra" }));

    expect(await screen.findByText("Entre 1 e 31.")).toBeInTheDocument();
    expect(screen.getByText("Máximo 12.")).toBeInTheDocument();
    expect(onSubmit).not.toHaveBeenCalled();
  });

  it("edita a regra e envia os valores convertidos", async () => {
    const onSubmit = vi.fn<(values: unknown) => Promise<void>>(() => Promise.resolve());
    const user = userEvent.setup();
    render(<DueDateRuleForm rule={rule} onSubmit={onSubmit} onCancel={() => undefined} submitting={false} />);

    expect(screen.getByText("Dia 20 do mês seguinte; se não for dia útil, antecipa.")).toBeInTheDocument();
    await user.clear(screen.getByLabelText(/Dia-base/));
    await user.type(screen.getByLabelText(/Dia-base/), "5");
    await user.selectOptions(screen.getByLabelText(/Tipo de dia/), "BUSINESS");
    expect(screen.getByText("5º dia útil do mês seguinte.")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Salvar regra" }));
    await waitFor(() => {
      expect(onSubmit).toHaveBeenCalledWith(
        expect.objectContaining({ base_day: 5, month_offset: 1, day_type: "BUSINESS", name: "Provisória — dia 20 do mês seguinte" }),
      );
    });
  });
});
