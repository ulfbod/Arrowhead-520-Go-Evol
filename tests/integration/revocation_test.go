//go:build integration

package integration

import "testing"

// TestRevocation tests the full revocation lifecycle:
// issue → verify valid → revoke → verify invalid → reissue → verify valid.
// This proves CA-as-PIP (D1): zero replication lag between CA state and PIP queries.
func TestRevocation(t *testing.T) {
	name := cn("revoke-lifecycle")

	t.Run("Issue", func(t *testing.T) {
		code, body := apiPost(t, profileCAURL+"/bootstrap/onboarding-cert",
			issueRequest{SystemName: name})
		if code != 201 {
			t.Fatalf("issue cert: got %d, body: %s", code, body)
		}
	})

	t.Run("ValidBeforeRevoke", func(t *testing.T) {
		code, body := apiGet(t, profileCAURL+"/pip/attributes/"+name)
		if code != 200 {
			t.Fatalf("PIP attributes: got %d", code)
		}
		var resp pipAttributesResponse
		unmarshal(t, body, &resp)
		if !resp.Valid {
			t.Fatal("cert should be valid before revocation")
		}
	})

	t.Run("Revoke", func(t *testing.T) {
		code, body := apiDelete(t, profileCAURL+"/ca/certificates/"+name)
		if code != 204 {
			t.Fatalf("revoke: got %d, body: %s", code, body)
		}
	})

	t.Run("InvalidAfterRevoke", func(t *testing.T) {
		// CA-as-PIP: zero lag — PIP must immediately reflect revocation.
		code, body := apiGet(t, profileCAURL+"/pip/attributes/"+name)
		if code != 200 {
			t.Fatalf("PIP attributes after revoke: got %d", code)
		}
		var resp pipAttributesResponse
		unmarshal(t, body, &resp)
		if resp.Valid {
			t.Fatal("cert should be invalid after revocation (zero-lag CA-as-PIP)")
		}
	})

	t.Run("DoubleRevoke_Error", func(t *testing.T) {
		code, _ := apiDelete(t, profileCAURL+"/ca/certificates/"+name)
		if code == 204 {
			t.Error("double revoke should return error, not 204")
		}
	})

	t.Run("RevokeUnknownCN_Error", func(t *testing.T) {
		code, _ := apiDelete(t, profileCAURL+"/ca/certificates/"+cn("nonexistent"))
		if code == 204 {
			t.Error("revoke unknown CN should return error, not 204")
		}
	})

	t.Run("Reissue", func(t *testing.T) {
		code, body := apiPost(t, profileCAURL+"/ca/certificates/"+name+"/reissue", nil)
		if code != 204 {
			t.Fatalf("reissue: got %d, body: %s", code, body)
		}
	})

	t.Run("ValidAfterReissue", func(t *testing.T) {
		code, body := apiGet(t, profileCAURL+"/pip/attributes/"+name)
		if code != 200 {
			t.Fatalf("PIP attributes after reissue: got %d", code)
		}
		var resp pipAttributesResponse
		unmarshal(t, body, &resp)
		if !resp.Valid {
			t.Fatal("cert should be valid after reissue")
		}
	})

	t.Run("ReissueNotRevoked_Error", func(t *testing.T) {
		// Cert is now valid — reissuing a non-revoked cert is an error.
		code, _ := apiPost(t, profileCAURL+"/ca/certificates/"+name+"/reissue", nil)
		if code == 204 {
			t.Error("reissue of non-revoked cert should return error, not 204")
		}
	})

	t.Run("RevokeAgain", func(t *testing.T) {
		// Full cycle: revoke again to prove the cert can be re-revoked after reissue.
		code, _ := apiDelete(t, profileCAURL+"/ca/certificates/"+name)
		if code != 204 {
			t.Fatalf("second revoke: got %d", code)
		}

		code2, body2 := apiGet(t, profileCAURL+"/pip/attributes/"+name)
		if code2 != 200 {
			t.Fatalf("PIP after second revoke: got %d", code2)
		}
		var resp pipAttributesResponse
		unmarshal(t, body2, &resp)
		if resp.Valid {
			t.Fatal("cert should be invalid after second revocation")
		}
	})

	t.Run("RevokedInSubjectsList", func(t *testing.T) {
		// Revoked certs must still appear in the subjects list.
		code, body := apiGet(t, profileCAURL+"/pip/subjects")
		if code != 200 {
			t.Fatalf("PIP subjects: got %d", code)
		}
		var resp pipSubjectsListResponse
		unmarshal(t, body, &resp)
		found := false
		for _, s := range resp.Subjects {
			if s.CN == name {
				found = true
				if s.Valid {
					t.Error("revoked cert should show valid=false in subjects list")
				}
				if !s.Revoked {
					t.Error("revoked cert should show revoked=true in subjects list")
				}
			}
		}
		if !found {
			t.Errorf("revoked cert %q not found in subjects list", name)
		}
	})
}
