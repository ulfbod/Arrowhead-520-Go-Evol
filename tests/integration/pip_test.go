//go:build integration

package integration

import (
	"testing"
	"time"
)

// pipAttributesResponse mirrors profile-ca's PIP response.
type pipAttributesResponse struct {
	SystemName string `json:"systemName"`
	CertLevel  string `json:"certLevel"`
	Valid      bool   `json:"valid"`
}

type pipSubjectResponse struct {
	CN        string    `json:"cn"`
	OU        string    `json:"ou"`
	IssuedAt  time.Time `json:"issuedAt"`
	ExpiresAt time.Time `json:"expiresAt"`
	Revoked   bool      `json:"revoked"`
	Valid     bool      `json:"valid"`
}

type pipSubjectsListResponse struct {
	Subjects []pipSubjectResponse `json:"subjects"`
	Count    int                  `json:"count"`
}

type pipStatusResponse struct {
	Subjects int `json:"subjects"`
}

// TestPIP tests the CA-as-PIP endpoints after issuing a known cert.
func TestPIP(t *testing.T) {
	// Setup: issue a cert that all PIP subtests reference.
	name := cn("pip-probe")
	code, body := apiPost(t, profileCAURL+"/bootstrap/onboarding-cert",
		issueRequest{SystemName: name})
	if code != 201 {
		t.Fatalf("setup: issue cert: got %d, body: %s", code, body)
	}

	t.Run("Attributes_ValidCert", func(t *testing.T) {
		code, body := apiGet(t, profileCAURL+"/pip/attributes/"+name)
		if code != 200 {
			t.Fatalf("PIP attributes: got %d, body: %s", code, body)
		}
		var resp pipAttributesResponse
		unmarshal(t, body, &resp)
		if resp.SystemName != name {
			t.Errorf("systemName: got %q, want %q", resp.SystemName, name)
		}
		if resp.CertLevel != "on" {
			t.Errorf("certLevel: got %q, want %q", resp.CertLevel, "on")
		}
		if !resp.Valid {
			t.Error("valid: got false, want true")
		}
	})

	t.Run("Attributes_UnknownCN_404", func(t *testing.T) {
		code, _ := apiGet(t, profileCAURL+"/pip/attributes/"+cn("nonexistent"))
		if code != 404 {
			t.Errorf("unknown CN: got %d, want 404", code)
		}
	})

	t.Run("SubjectsList", func(t *testing.T) {
		code, body := apiGet(t, profileCAURL+"/pip/subjects")
		if code != 200 {
			t.Fatalf("PIP subjects: got %d, body: %s", code, body)
		}
		var resp pipSubjectsListResponse
		unmarshal(t, body, &resp)
		if resp.Count == 0 {
			t.Fatal("subjects count is 0")
		}
		// Our test cert should be in the list.
		found := false
		for _, s := range resp.Subjects {
			if s.CN == name {
				found = true
				if !s.Valid {
					t.Error("test cert should be valid in subjects list")
				}
			}
		}
		if !found {
			t.Errorf("test cert %q not found in subjects list", name)
		}
	})

	t.Run("SubjectDetail", func(t *testing.T) {
		code, body := apiGet(t, profileCAURL+"/pip/subjects/"+name)
		if code != 200 {
			t.Fatalf("PIP subject detail: got %d, body: %s", code, body)
		}
		var resp pipSubjectResponse
		unmarshal(t, body, &resp)
		if resp.CN != name {
			t.Errorf("cn: got %q, want %q", resp.CN, name)
		}
		if resp.OU != "on" {
			t.Errorf("ou: got %q, want %q", resp.OU, "on")
		}
		if resp.IssuedAt.IsZero() {
			t.Error("issuedAt is zero")
		}
		if resp.ExpiresAt.IsZero() {
			t.Error("expiresAt is zero")
		}
		if resp.Revoked {
			t.Error("should not be revoked")
		}
		if !resp.Valid {
			t.Error("should be valid")
		}
	})

	t.Run("SubjectDetail_UnknownCN_404", func(t *testing.T) {
		code, _ := apiGet(t, profileCAURL+"/pip/subjects/"+cn("nonexistent"))
		if code != 404 {
			t.Errorf("unknown CN detail: got %d, want 404", code)
		}
	})

	t.Run("Status", func(t *testing.T) {
		code, body := apiGet(t, profileCAURL+"/pip/status")
		if code != 200 {
			t.Fatalf("PIP status: got %d", code)
		}
		var resp pipStatusResponse
		unmarshal(t, body, &resp)
		if resp.Subjects == 0 {
			t.Error("subjects count is 0")
		}
	})
}
