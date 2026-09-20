//go:build integration

package integration

import (
	"strings"
	"testing"
)

// TestPreflight verifies every service is healthy and correctly configured
// before running any functional tests.
func TestPreflight(t *testing.T) {
	t.Run("ProfileCA_Health", func(t *testing.T) {
		code, body := apiGet(t, profileCAURL+"/health")
		if code != 200 {
			t.Fatalf("profile-ca health: got %d, body: %s", code, body)
		}
		var resp map[string]string
		unmarshal(t, body, &resp)
		if resp["status"] != "ok" {
			t.Errorf("profile-ca status: got %q, want %q", resp["status"], "ok")
		}
	})

	t.Run("PIP_Health", func(t *testing.T) {
		code, body := apiGet(t, profileCAURL+"/pip/health")
		if code != 200 {
			t.Fatalf("PIP health: got %d, body: %s", code, body)
		}
	})

	t.Run("PAP_Health", func(t *testing.T) {
		code, body := apiGet(t, papURL+"/health")
		if code != 200 {
			t.Fatalf("PAP health: got %d, body: %s", code, body)
		}
	})

	t.Run("AuthzForce_Health", func(t *testing.T) {
		code, body := apiGet(t, authzforceURL+"/health")
		if code != 200 {
			t.Fatalf("AuthzForce health: got %d, body: %s", code, body)
		}
	})

	t.Run("DynamicOrch_Health", func(t *testing.T) {
		code, _ := apiGet(t, dynamicOrchURL+"/health")
		if code != 200 {
			t.Fatalf("DynamicOrch health: got %d", code)
		}
	})

	t.Run("KafkaAuthz_Health", func(t *testing.T) {
		code, _ := apiGet(t, kafkaAuthzURL+"/health")
		if code != 200 {
			t.Fatalf("kafka-authz health: got %d", code)
		}
	})

	t.Run("PkiRestAuthz_Health", func(t *testing.T) {
		code, _, err := tryGet(pkiRestAuthzURL + "/health")
		if err != nil {
			t.Skipf("pki-rest-authz not reachable — may not be running: %v", err)
		}
		if code != 200 {
			t.Skipf("pki-rest-authz returned %d — may not be fully started", code)
		}
	})

	t.Run("Foundation_Health", func(t *testing.T) {
		// Foundation services enforce mTLS on external ports — verify via Docker healthcheck.
		for _, svc := range []struct{ name, container string }{
			{"ServiceRegistry", "serviceregistry"},
			{"Authentication", "authentication"},
			{"ConsumerAuth", "consumerauth"},
		} {
			t.Run(svc.name, func(t *testing.T) {
				out := dockerExec(t, svc.container, "wget -qO- http://localhost:8080/health 2>/dev/null || wget -qO- http://localhost:8081/health 2>/dev/null || wget -qO- http://localhost:8082/health 2>/dev/null")
				if !strings.Contains(out, "ok") {
					t.Errorf("%s health: got %q", svc.name, out)
				}
			})
		}
	})

	t.Run("Dashboard_Reachable", func(t *testing.T) {
		code, _ := apiGet(t, dashboardURL+"/")
		if code != 200 {
			t.Fatalf("Dashboard: got %d", code)
		}
	})

	t.Run("PIP_Has_Subjects", func(t *testing.T) {
		code, body := apiGet(t, profileCAURL+"/pip/status")
		if code != 200 {
			t.Fatalf("PIP status: got %d", code)
		}
		var status map[string]int
		unmarshal(t, body, &status)
		if status["subjects"] == 0 {
			t.Fatal("PIP reports 0 subjects — cert-provisioner may not have run")
		}
	})

	t.Run("PAP_Has_Policies", func(t *testing.T) {
		code, body := apiGet(t, papURL+"/policies")
		if code != 200 {
			t.Fatalf("PAP policies: got %d", code)
		}
		// PAP response format: {"count":N,"policies":[{"id":"...","subject":"...",...}]}
		var resp struct {
			Count    int `json:"count"`
			Policies []struct {
				ID string `json:"id"`
			} `json:"policies"`
		}
		unmarshal(t, body, &resp)
		if resp.Count == 0 {
			t.Fatal("PAP has no policies — setup container may not have run")
		}
	})
}
