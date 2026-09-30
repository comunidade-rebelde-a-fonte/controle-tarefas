import { fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { type CredentialControl, type CredentialRow, credentialKeys } from "@/services/credentials";
import { jsonResponse } from "@/test/fetch";
import { mockFetchRoutes } from "@/test/fetchRouter";
import { renderWithProviders } from "@/test/render";
import { CredentialsTab } from "./CredentialsTab";

afterEach(() => {
  vi.unstubAllGlobals();
});

const ALFA = "11111111-1111-4111-8111-111111111111";

const control = (patch: Partial<CredentialControl>): CredentialControl => ({
  type: "RFB_POWER_OF_ATTORNEY",
  expires_at: "2026-10-05",
  renewal_status: "CURRENT",
  renewal_label: "Vigente",
  situation: { code: "DUE_SOON", label: "Vence em até 7 dias", threshold_days: 7, days_left: 4 },
  notes: null,
  file: null,
  version: 2,
  updated_at: null,
  ...patch,
});

const row: CredentialRow = {
  company: { id: ALFA, internal_code: "101", legal_name: "Empresa Fictícia Alfa Comércio Ltda", trade_name: null, cnpj: "00000001000101", tax_regime: "LUCRO_PRESUMIDO" },
  rfb_power_of_attorney: control({}),
  digital_certificate: control({ type: "DIGITAL_CERTIFICATE", expires_at: null, version: 0, situation: { code: "NO_DATE", label: "Sem data cadastrada", threshold_days: null, days_left: null } }),
};

describe("CredentialsTab", () => {
  it("edita a validade na ficha e invalida a tela geral", async () => {
    const puts: unknown[] = [];
    mockFetchRoutes(({ method, url, body }) => {
      if (method === "GET" && url.pathname === `/api/v1/companies/${ALFA}/credential-controls`) return jsonResponse(row);
      if (method === "PUT" && url.pathname === `/api/v1/companies/${ALFA}/credential-controls/RFB_POWER_OF_ATTORNEY`) {
        puts.push(body);
        return jsonResponse(control({ expires_at: "2026-12-31", version: 3 }));
      }
      if (method === "GET" && url.pathname === "/api/v1/credential-controls") return jsonResponse({ items: [row], total: 1, thresholds: [7] });
      return undefined;
    });
    const { queryClient } = renderWithProviders(<CredentialsTab companyId={ALFA} />);
    const invalidate = vi.spyOn(queryClient, "invalidateQueries");

    expect(await screen.findByRole("heading", { name: "Procuração RFB" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Certificado digital" })).toBeInTheDocument();
    const input = screen.getByLabelText("Vencimento — Procuração RFB — Empresa Fictícia Alfa Comércio Ltda");
    expect(input).toHaveAttribute("tabindex", "0");

    fireEvent.change(input, { target: { value: "2026-12-31" } });
    fireEvent.blur(input);

    await waitFor(() => expect(puts).toEqual([{ version: 2, expires_at: "2026-12-31", renewal_status: "CURRENT", notes: null }]));
    // O prefixo ["credentials"] cobre a lista da tela geral (credentialKeys.list).
    await waitFor(() => expect(invalidate).toHaveBeenCalledWith({ queryKey: credentialKeys.all }));
    expect(credentialKeys.list("").slice(0, 1)).toEqual([...credentialKeys.all]);
    expect(screen.getByLabelText("Etapa de renovação — Certificado digital — Empresa Fictícia Alfa Comércio Ltda")).toHaveAttribute("tabindex", "0");
  });
});
