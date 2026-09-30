package obligation_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details struct {
		Pending []struct {
			Code string `json:"code"`
			Name string `json:"name"`
		} `json:"pending"`
	} `json:"details"`
}

func TestCA011_DctfwebWarning(t *testing.T) {
	srv := server(t)
	// Componentes da DCTFWeb da 101 (Presumido): MIT, REINF Retidos, REINF Lucros e eSocial.
	for _, c := range []string{"MIT", "REINF_RETIDOS", "REINF_LUCROS"} {
		id, v := instanceID(t, srv, "101", c)
		srv.Patch("/api/v1/obligation-instances/"+id+"/status", map[string]any{"status_code": "TRANSMITTED", "version": v}).MustStatus(t, http.StatusOK)
	}
	// eSocial pendente → concluir DCTFWeb exibe alerta forte com o pré-requisito [CA-011].
	id, v := instanceID(t, srv, "101", "DCTFWEB")
	var e errorBody
	srv.Patch("/api/v1/obligation-instances/"+id+"/status", map[string]any{"status_code": "TRANSMITTED", "version": v}).
		MustStatus(t, http.StatusConflict).JSON(t, &e)
	if e.Code != "DEPENDENCY_WARNING" || e.Message != "DCTFWeb possui pré-requisitos pendentes: eSocial." ||
		len(e.Details.Pending) != 1 || e.Details.Pending[0].Code != "ESOCIAL" {
		t.Fatalf("alerta = %+v", e)
	}

	// Confirmação sem motivo é recusada.
	srv.Patch("/api/v1/obligation-instances/"+id+"/status", map[string]any{"status_code": "TRANSMITTED", "version": v, "confirm_exception": true, "exception_reason": "  "}).
		MustStatus(t, http.StatusBadRequest)

	// Com exceção e motivo, a conclusão é aceita e o motivo fica no histórico.
	reason := "eSocial transmitido pelo cliente em outro sistema"
	srv.Patch("/api/v1/obligation-instances/"+id+"/status", map[string]any{"status_code": "TRANSMITTED", "version": v, "confirm_exception": true, "exception_reason": reason}).
		MustStatus(t, http.StatusOK)
	var got string
	if err := srv.Pool.QueryRow(context.Background(), `SELECT override_reason FROM obligation_status_history
		WHERE obligation_instance_id = $1 AND field = 'STATUS'`, id).Scan(&got); err != nil || got != reason {
		t.Fatalf("motivo no histórico = %q (%v)", got, err)
	}
}

func TestCA011_NoWarningWhenComponentsFinal(t *testing.T) {
	srv := server(t)
	for _, c := range []string{"MIT", "REINF_RETIDOS", "REINF_LUCROS"} {
		id, v := instanceID(t, srv, "102", c)
		srv.Patch("/api/v1/obligation-instances/"+id+"/status", map[string]any{"status_code": "NO_MOVEMENT", "version": v}).MustStatus(t, http.StatusOK)
	}
	id, v := instanceID(t, srv, "102", "ESOCIAL")
	srv.Patch("/api/v1/obligation-instances/"+id+"/status", map[string]any{"status_code": "CLOSED", "version": v}).MustStatus(t, http.StatusOK)
	id, v = instanceID(t, srv, "102", "DCTFWEB")
	res := srv.Patch("/api/v1/obligation-instances/"+id+"/status", map[string]any{"status_code": "TRANSMITTED", "version": v})
	if res.Status != http.StatusOK || strings.Contains(string(res.Body), "DEPENDENCY_WARNING") {
		t.Fatalf("sem pendências não deveria alertar: %d %s", res.Status, res.Body)
	}
	// Status não final (Pendente) não dispara o alerta.
	id2, v2 := instanceID(t, srv, "103", "DCTFWEB")
	srv.Patch("/api/v1/obligation-instances/"+id2+"/status", map[string]any{"status_code": "PENDING", "version": v2}).MustStatus(t, http.StatusOK)
}
