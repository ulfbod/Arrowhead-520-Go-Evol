package httputil_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"arrowhead/foundation/internal/httputil"
)

func TestErrorTypeForStatus(t *testing.T) {
	tests := []struct {
		status int
		want   string
	}{
		{http.StatusBadRequest, "INVALID_PARAMETER"},
		{http.StatusUnauthorized, "AUTH_EXCEPTION"},
		{http.StatusForbidden, "FORBIDDEN"},
		{http.StatusNotFound, "DATA_NOT_FOUND"},
		{http.StatusLocked, "LOCKED"},
		{http.StatusNotImplemented, "NOT_IMPLEMENTED"},
		{http.StatusInternalServerError, "ARROWHEAD_EXCEPTION"},
		{http.StatusConflict, "ARROWHEAD_EXCEPTION"},
		{http.StatusMethodNotAllowed, "ARROWHEAD_EXCEPTION"},
	}
	for _, tc := range tests {
		got := httputil.ErrorTypeForStatus(tc.status)
		if got != tc.want {
			t.Errorf("ErrorTypeForStatus(%d) = %q, want %q", tc.status, got, tc.want)
		}
	}
}

func TestWriteError(t *testing.T) {
	w := httptest.NewRecorder()
	httputil.WriteError(w, http.StatusBadRequest, "test error", "myservice")

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var body map[string]any
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["errorMessage"] != "test error" {
		t.Errorf("errorMessage = %v", body["errorMessage"])
	}
	if int(body["errorCode"].(float64)) != http.StatusBadRequest {
		t.Errorf("errorCode = %v", body["errorCode"])
	}
	if body["exceptionType"] != "INVALID_PARAMETER" {
		t.Errorf("exceptionType = %v", body["exceptionType"])
	}
	if body["origin"] != "myservice" {
		t.Errorf("origin = %v", body["origin"])
	}
}

func TestWriteError_404(t *testing.T) {
	w := httptest.NewRecorder()
	httputil.WriteError(w, http.StatusNotFound, "not found", "svc")
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
	var body map[string]any
	json.NewDecoder(w.Body).Decode(&body) //nolint:errcheck
	if body["exceptionType"] != "DATA_NOT_FOUND" {
		t.Errorf("exceptionType = %v", body["exceptionType"])
	}
}

// ── Token guards (SPEC.md: REGISTER_AUTH_URL, MGMT_AUTH_URL) ──────────────────

// fakeAuthentication stands in for GET /authentication/identity/verify/<token>.
// "sysop-token" and "user-token" are verified (sysop true/false) for systems
// SysopSystem and UserSystem; "broken-token" answers 500; "garbage-token"
// answers a non-JSON body; any other token gets 200 {"verified":false,"sysop":false},
// as the real Authentication system answers an unknown or expired token.
func fakeAuthentication(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimPrefix(r.URL.Path, "/authentication/identity/verify/")
		w.Header().Set("Content-Type", "application/json")
		switch token {
		case "sysop-token":
			fmt.Fprint(w, `{"verified":true,"systemName":"SysopSystem","sysop":true}`)
		case "user-token":
			fmt.Fprint(w, `{"verified":true,"systemName":"UserSystem","sysop":false}`)
		case "broken-token":
			w.WriteHeader(http.StatusInternalServerError)
		case "garbage-token":
			fmt.Fprint(w, `not json`)
		default:
			fmt.Fprint(w, `{"verified":false,"sysop":false}`)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func requestWithToken(token string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/x", nil)
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	return r
}

func TestVerifyTokenIdentity_Cases(t *testing.T) {
	auth := fakeAuthentication(t)
	cases := []struct {
		name, authURL, token, claimed string
		wantOK                        bool
		wantStatus                    int
	}{
		{"open mode", "", "", "UserSystem", true, 0},
		{"no token", auth.URL, "", "UserSystem", false, http.StatusUnauthorized},
		{"auth unreachable", "http://127.0.0.1:1", "user-token", "UserSystem", false, http.StatusUnauthorized},
		{"auth non-200", auth.URL, "broken-token", "UserSystem", false, http.StatusUnauthorized},
		{"undecodable answer", auth.URL, "garbage-token", "UserSystem", false, http.StatusUnauthorized},
		{"verified false", auth.URL, "bogus", "UserSystem", false, http.StatusUnauthorized},
		{"verified false, empty claimed name", auth.URL, "bogus", "", false, http.StatusUnauthorized},
		{"another system", auth.URL, "user-token", "OtherSystem", false, http.StatusForbidden},
		{"same system", auth.URL, "user-token", "UserSystem", true, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ok, status := httputil.VerifyTokenIdentity(requestWithToken(tc.token), tc.authURL, tc.claimed)
			if ok != tc.wantOK || status != tc.wantStatus {
				t.Errorf("got (%v, %d), want (%v, %d)", ok, status, tc.wantOK, tc.wantStatus)
			}
		})
	}
}

func TestRequireManagementAuth_Cases(t *testing.T) {
	auth := fakeAuthentication(t)
	cases := []struct {
		name, authURL, token string
		wantOK               bool
		wantStatus           int
		wantType             string
	}{
		{"open mode", "", "", true, http.StatusOK, ""},
		{"no token", auth.URL, "", false, http.StatusUnauthorized, "AUTH_EXCEPTION"},
		{"auth unreachable", "http://127.0.0.1:1", "sysop-token", false, http.StatusUnauthorized, "AUTH_EXCEPTION"},
		{"auth non-200", auth.URL, "broken-token", false, http.StatusUnauthorized, "AUTH_EXCEPTION"},
		{"undecodable answer", auth.URL, "garbage-token", false, http.StatusUnauthorized, "AUTH_EXCEPTION"},
		{"verified false", auth.URL, "bogus", false, http.StatusUnauthorized, "AUTH_EXCEPTION"},
		{"verified, not sysop", auth.URL, "user-token", false, http.StatusForbidden, "FORBIDDEN"},
		{"verified sysop", auth.URL, "sysop-token", true, http.StatusOK, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			ok := httputil.RequireManagementAuth(w, requestWithToken(tc.token), tc.authURL, "testsystem")
			if ok != tc.wantOK || w.Code != tc.wantStatus {
				t.Fatalf("got (%v, %d), want (%v, %d); body %s", ok, w.Code, tc.wantOK, tc.wantStatus, w.Body.String())
			}
			if tc.wantType == "" {
				return
			}
			var body map[string]any
			if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if body["exceptionType"] != tc.wantType || body["origin"] != "testsystem" {
				t.Errorf("envelope: got %v", body)
			}
		})
	}
}
