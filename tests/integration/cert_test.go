//go:build integration

package integration

import (
	"strings"
	"testing"
)

// certResponse mirrors profile-ca's JSON response for cert issuance.
type certResponse struct {
	SystemName  string `json:"systemName"`
	Certificate string `json:"certificate"`
	PrivateKey  string `json:"privateKey"`
	Profile     string `json:"profile"`
	IssuedAt    string `json:"issuedAt"`
}

type issueRequest struct {
	SystemName string `json:"systemName"`
}

// TestCertIssuance tests certificate issuance via profile-ca.
func TestCertIssuance(t *testing.T) {
	t.Run("Onboarding", func(t *testing.T) {
		code, body := apiPost(t, profileCAURL+"/bootstrap/onboarding-cert",
			issueRequest{SystemName: cn("onboard")})
		if code != 201 {
			t.Fatalf("issue onboarding cert: got %d, body: %s", code, body)
		}
		var resp certResponse
		unmarshal(t, body, &resp)
		if resp.SystemName != cn("onboard") {
			t.Errorf("systemName: got %q, want %q", resp.SystemName, cn("onboard"))
		}
		if resp.Profile != "on" {
			t.Errorf("profile: got %q, want %q", resp.Profile, "on")
		}
		if !strings.Contains(resp.Certificate, "BEGIN CERTIFICATE") {
			t.Error("certificate missing PEM header")
		}
		if !strings.Contains(resp.PrivateKey, "BEGIN EC PRIVATE KEY") {
			t.Error("private key missing PEM header")
		}
		if resp.IssuedAt == "" {
			t.Error("issuedAt is empty")
		}
	})

	t.Run("Infra", func(t *testing.T) {
		code, body := apiPost(t, profileCAURL+"/ca/certificate/issue",
			issueRequest{SystemName: cn("infra")})
		if code != 201 {
			t.Fatalf("issue infra cert: got %d, body: %s", code, body)
		}
		var resp certResponse
		unmarshal(t, body, &resp)
		if resp.Profile != "sy" {
			t.Errorf("infra cert profile: got %q, want %q", resp.Profile, "sy")
		}
	})

	t.Run("EmptySystemName_400", func(t *testing.T) {
		code, _ := apiPost(t, profileCAURL+"/bootstrap/onboarding-cert",
			issueRequest{SystemName: ""})
		if code != 400 {
			t.Errorf("empty systemName: got %d, want 400", code)
		}
	})

	t.Run("CAInfo", func(t *testing.T) {
		code, body := apiGet(t, profileCAURL+"/ca/info")
		if code != 200 {
			t.Fatalf("CA info: got %d", code)
		}
		var info map[string]string
		unmarshal(t, body, &info)
		if info["commonName"] == "" {
			t.Error("CA info missing commonName")
		}
		if !strings.Contains(info["certificate"], "BEGIN CERTIFICATE") {
			t.Error("CA info missing certificate PEM")
		}
	})
}
