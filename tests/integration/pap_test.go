//go:build integration

package integration

import "testing"

// TestPAP tests policy administration and verification.
func TestPAP(t *testing.T) {
	t.Run("ListPolicies", func(t *testing.T) {
		code, body := apiGet(t, papURL+"/policies")
		if code != 200 {
			t.Fatalf("list policies: got %d, body: %s", code, body)
		}
		var resp struct {
			Count    int `json:"count"`
			Policies []struct {
				ID      string `json:"id"`
				Subject string `json:"subject"`
			} `json:"policies"`
		}
		unmarshal(t, body, &resp)
		if resp.Count == 0 {
			t.Error("no policies found")
		}
	})

	t.Run("CreatePolicy", func(t *testing.T) {
		policy := map[string]string{
			"subject":  cn("pap-test-consumer"),
			"resource": "pap-test-svc",
			"action":   "consume",
			"effect":   "Permit",
		}
		code, body := apiPost(t, papURL+"/policies", policy)
		if code != 200 && code != 201 {
			t.Fatalf("create policy: got %d, body: %s", code, body)
		}
	})

	t.Run("VerifyCreatedPolicy", func(t *testing.T) {
		code, body := apiGet(t, papURL+"/policies")
		if code != 200 {
			t.Fatalf("list policies after create: got %d", code)
		}
		var resp struct {
			Count    int `json:"count"`
			Policies []struct {
				Subject string `json:"subject"`
			} `json:"policies"`
		}
		unmarshal(t, body, &resp)
		found := false
		for _, p := range resp.Policies {
			if p.Subject == cn("pap-test-consumer") {
				found = true
			}
		}
		if !found {
			t.Errorf("created policy for %q not found", cn("pap-test-consumer"))
		}
	})

	t.Run("Status", func(t *testing.T) {
		code, body := apiGet(t, papURL+"/status")
		if code != 200 {
			t.Fatalf("PAP status: got %d, body: %s", code, body)
		}
		if len(body) == 0 {
			t.Error("PAP status returned empty body")
		}
	})
}
