//go:build integration

package integration

import "testing"

// TestOrchestration tests DynamicOrch-XACML endpoints.
func TestOrchestration(t *testing.T) {
	t.Run("Status", func(t *testing.T) {
		code, body := apiGet(t, dynamicOrchURL+"/status")
		if code != 200 {
			t.Fatalf("orchestrator status: got %d, body: %s", code, body)
		}
		var resp map[string]any
		unmarshal(t, body, &resp)
		if _, ok := resp["status"]; !ok {
			t.Error("status field missing from response")
		}
	})

	t.Run("Health", func(t *testing.T) {
		code, _ := apiGet(t, dynamicOrchURL+"/health")
		if code != 200 {
			t.Fatalf("orchestrator health: got %d", code)
		}
	})

	t.Run("Pull_NoProviders", func(t *testing.T) {
		// With no providers registered for this service, orchestration should
		// return 200 (empty results) or 404 — both are valid.
		req := map[string]any{
			"requesterSystem":  map[string]string{"systemName": cn("orch-probe")},
			"requestedService": map[string]string{"serviceDefinition": cn("no-such-svc")},
		}
		code, _ := apiPost(t, dynamicOrchURL+"/serviceorchestration/orchestration/pull", req)
		if code != 200 && code != 404 {
			t.Errorf("pull with no providers: got %d, want 200 or 404", code)
		}
	})

	t.Run("MgmtLogs", func(t *testing.T) {
		req := map[string]any{
			"pagination": map[string]int{"pageSize": 5, "pageNumber": 0},
		}
		code, body := apiPost(t, dynamicOrchURL+"/serviceorchestration/orchestration/general/mgmt/logs", req)
		if code != 200 {
			t.Fatalf("mgmt/logs: got %d, body: %s", code, body)
		}
	})

	t.Run("MgmtGetConfig", func(t *testing.T) {
		code, _ := apiGet(t, dynamicOrchURL+"/serviceorchestration/orchestration/general/mgmt/get-config?keys=PORT,AUTH_BACKEND")
		if code != 200 {
			t.Fatalf("mgmt/get-config: got %d", code)
		}
	})
}
