import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { TooltipProvider } from "@/components/ui/tooltip";
import { CopyButton, CopyCnpjButton } from "./CopyButton";

function renderInProviders(ui: React.ReactElement) {
  return render(<TooltipProvider>{ui}</TooltipProvider>);
}

describe("CopyButton", () => {
  it("CA-003 copia CNPJ só com dígitos", async () => {
    const user = userEvent.setup();
    renderInProviders(<CopyCnpjButton cnpj="00.000.001/0001-01" />);

    await user.click(screen.getByRole("button", { name: "Copiar CNPJ" }));

    expect(await navigator.clipboard.readText()).toBe("00000001000101");
    expect(screen.getByRole("button", { name: "Copiado" })).toBeInTheDocument();
  });

  it("copia o código interno exatamente", async () => {
    const user = userEvent.setup();
    renderInProviders(<CopyButton value="101" label="Copiar código" />);
    await user.click(screen.getByRole("button", { name: "Copiar código" }));
    expect(await navigator.clipboard.readText()).toBe("101");
  });
});
