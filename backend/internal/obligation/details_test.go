package obligation_test

import (
	"net/http"
	"testing"

	"controle/internal/testutil"
)

type historyList struct {
	Items []struct {
		Field          string  `json:"field"`
		OldValue       *string `json:"old_value"`
		NewValue       *string `json:"new_value"`
		NewLabel       *string `json:"new_label"`
		ChangeSource   string  `json:"change_source"`
		OverrideReason *string `json:"override_reason"`
		User           struct {
			Name string `json:"name"`
		} `json:"user"`
	} `json:"items"`
}

type detailsResult struct {
	Instance struct {
		StatusCode            string  `json:"status_code"`
		Applicable            bool    `json:"applicable"`
		DueDateOriginal       *string `json:"due_date_original"`
		DueDateEffective      *string `json:"due_date_effective"`
		DueOverridden         bool    `json:"due_overridden"`
		DueDateOverrideReason *string `json:"due_date_override_reason"`
		TransmissionDate      *string `json:"transmission_date"`
		Protocol              *string `json:"protocol"`
		Version               int     `json:"version"`
	} `json:"instance"`
	Row struct {
		Progress struct {
			Denominator int `json:"denominator"`
		} `json:"progress"`
	} `json:"row"`
}

func patchDetails(srv *testutil.Server, id string, body map[string]any) testutil.Response {
	return srv.Patch("/api/v1/obligation-instances/"+id+"/details", body)
}

func TestDetails_DueDateOverrideRequiresReasonAndIsInHistory(t *testing.T) {
	srv := server(t)
	id, v := instanceID(t, srv, "103", "SPED_FISCAL")

	if code := patchDetails(srv, id, map[string]any{"version": v, "due_date_effective": "2026-11-25"}).
		MustStatus(t, http.StatusUnprocessableEntity).ErrorCode(t); code != "REASON_REQUIRED" {
		t.Fatalf("override sem motivo: code = %s", code)
	}

	var res detailsResult
	patchDetails(srv, id, map[string]any{"version": v, "due_date_effective": "2026-11-25", "due_date_override_reason": "Prorrogação combinada com o cliente"}).
		MustStatus(t, http.StatusOK).JSON(t, &res)
	if *res.Instance.DueDateEffective != "2026-11-25" || *res.Instance.DueDateOriginal != "2026-11-19" || !res.Instance.DueOverridden {
		t.Fatalf("vencimento = %+v", res.Instance)
	}

	srv.Patch("/api/v1/obligation-instances/"+id+"/status", map[string]any{"status_code": "PENDING", "version": res.Instance.Version}).MustStatus(t, http.StatusOK)

	var h historyList
	srv.Get("/api/v1/obligation-instances/"+id+"/history").MustStatus(t, http.StatusOK).JSON(t, &h)
	if len(h.Items) != 2 {
		t.Fatalf("histórico = %d linhas, want 2", len(h.Items))
	}
	// Mais recente primeiro: status, depois vencimento (original 19/11: dia 20 é feriado nacional).
	if h.Items[0].Field != "STATUS" || *h.Items[0].NewValue != "PENDING" || *h.Items[0].NewLabel != "Pendente" {
		t.Fatalf("1ª linha = %+v", h.Items[0])
	}
	due := h.Items[1]
	if due.Field != "DUE_DATE" || *due.OldValue != "2026-11-19" || *due.NewValue != "2026-11-25" ||
		due.OverrideReason == nil || *due.OverrideReason != "Prorrogação combinada com o cliente" || due.User.Name != "Administrador" || due.ChangeSource != "INLINE" {
		t.Fatalf("2ª linha = %+v", due)
	}
}

func TestDetails_TransmissionAndProtocolOnlyWhenAllowed(t *testing.T) {
	srv := server(t)
	fid, fv := instanceID(t, srv, "101", "FISCAL_ENTRADAS")
	if code := patchDetails(srv, fid, map[string]any{"version": fv, "protocol": "123"}).MustStatus(t, http.StatusUnprocessableEntity).ErrorCode(t); code != "FIELD_NOT_ALLOWED" {
		t.Fatalf("code = %s", code)
	}
	id, v := instanceID(t, srv, "101", "MIT")
	var res detailsResult
	patchDetails(srv, id, map[string]any{"version": v, "transmission_date": "2026-11-12", "protocol": " REC-2026-0001 "}).MustStatus(t, http.StatusOK).JSON(t, &res)
	if *res.Instance.TransmissionDate != "2026-11-12" || *res.Instance.Protocol != "REC-2026-0001" {
		t.Fatalf("detalhes = %+v", res.Instance)
	}
	patchDetails(srv, id, map[string]any{"version": res.Instance.Version, "protocol": nil}).MustStatus(t, http.StatusOK).JSON(t, &res)
	if res.Instance.Protocol != nil {
		t.Fatal("protocol null deveria limpar o campo")
	}
	var h historyList
	srv.Get("/api/v1/obligation-instances/"+id+"/history").MustStatus(t, http.StatusOK).JSON(t, &h)
	if len(h.Items) != 3 {
		t.Fatalf("histórico = %d, want 3 (transmissão, protocolo, protocolo limpo)", len(h.Items))
	}
	patchDetails(srv, id, map[string]any{"version": v, "protocol": "X"}).MustStatus(t, http.StatusConflict)
}

func TestDetails_ApplicabilityChangeKeepsVisibleAndLeavesDenominator(t *testing.T) {
	srv := server(t)
	id, v := instanceID(t, srv, "101", "FISCAL_IPI")
	var res detailsResult
	patchDetails(srv, id, map[string]any{"version": v, "applicable": false}).MustStatus(t, http.StatusOK).JSON(t, &res)
	if res.Instance.Applicable || res.Instance.StatusCode != "NOT_APPLICABLE" || res.Row.Progress.Denominator != 11 {
		t.Fatalf("após não aplicável: %+v / denominador %d", res.Instance, res.Row.Progress.Denominator)
	}
	var h historyList
	srv.Get("/api/v1/obligation-instances/"+id+"/history").MustStatus(t, http.StatusOK).JSON(t, &h)
	fields := map[string]bool{}
	for _, it := range h.Items {
		fields[it.Field] = true
	}
	if !fields["APPLICABLE"] || !fields["STATUS"] {
		t.Fatalf("histórico deveria registrar aplicabilidade e status: %+v", h.Items)
	}
	patchDetails(srv, id, map[string]any{"version": res.Instance.Version, "applicable": true}).MustStatus(t, http.StatusOK).JSON(t, &res)
	if !res.Instance.Applicable || res.Instance.StatusCode != "NOT_STARTED" || res.Row.Progress.Denominator != 12 {
		t.Fatalf("após voltar a aplicável: %+v", res.Instance)
	}
	srv.Get("/api/v1/obligation-instances/7b0f8d7e-0000-4000-8000-000000000000/history").MustStatus(t, http.StatusNotFound)
}
