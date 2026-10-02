package orchestration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// stubOrchestrator wraps a fixed response for handler tests.
type stubOrchestrator struct {
	resp OrchestrationResponse
	err  error
}

func (s *stubOrchestrator) Orchestrate(_ OrchestrationRequest) (OrchestrationResponse, error) {
	return s.resp, s.err
}

// ---- helpers ----------------------------------------------------------------

func postJSON(t *testing.T, h http.Handler, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	data, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func getReq(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

// ---- Existing pull-orchestration tests (paths updated to AH5) ---------------

const pullPath = "/serviceorchestration/orchestration/pull"

func TestHandler_ValidRequest_Returns200(t *testing.T) {
	stub := &stubOrchestrator{
		resp: OrchestrationResponse{
			Response: []OrchestrationResult{
				{
					Provider: System{SystemName: "prov", Address: "10.0.0.1", Port: 9000},
					Service:  ServiceInfo{ServiceDefinition: "telemetry", ServiceUri: "/t", Interfaces: []string{"HTTP-SECURE-JSON"}, Version: 1},
				},
			},
		},
	}
	h := NewHandler(stub, "")
	body := `{"requesterSystem":{"systemName":"c1","address":"1.2.3.4","port":8000},"requestedService":{"serviceDefinition":"telemetry"}}`
	req := httptest.NewRequest(http.MethodPost, pullPath, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status: got %d want 200", w.Code)
	}
	var resp OrchestrationResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Response) != 1 {
		t.Errorf("results: got %d want 1", len(resp.Response))
	}
}

func TestHandler_DenyReturnsEmptyResults(t *testing.T) {
	stub := &stubOrchestrator{
		resp: OrchestrationResponse{Response: []OrchestrationResult{}},
	}
	h := NewHandler(stub, "")
	body := `{"requesterSystem":{"systemName":"c1"},"requestedService":{"serviceDefinition":"telemetry"}}`
	req := httptest.NewRequest(http.MethodPost, pullPath, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status: got %d want 200", w.Code)
	}
	var resp OrchestrationResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Response) != 0 {
		t.Errorf("expected empty response on deny")
	}
}

func TestHandler_InvalidJSON_Returns400(t *testing.T) {
	stub := &stubOrchestrator{}
	h := NewHandler(stub, "")
	req := httptest.NewRequest(http.MethodPost, pullPath, strings.NewReader("{bad json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status: got %d want 400", w.Code)
	}
}

func TestHandler_MissingRequester_Returns400(t *testing.T) {
	stub := &stubOrchestrator{err: ErrMissingRequester}
	h := NewHandler(stub, "")
	body := `{"requesterSystem":{"systemName":""},"requestedService":{"serviceDefinition":"telemetry"}}`
	req := httptest.NewRequest(http.MethodPost, pullPath, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status: got %d want 400", w.Code)
	}
}

func TestHandler_MissingService_Returns400(t *testing.T) {
	stub := &stubOrchestrator{err: ErrMissingService}
	h := NewHandler(stub, "")
	body := `{"requesterSystem":{"systemName":"c1"},"requestedService":{"serviceDefinition":""}}`
	req := httptest.NewRequest(http.MethodPost, pullPath, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status: got %d want 400", w.Code)
	}
}

func TestHandler_WrongMethod_Returns405(t *testing.T) {
	stub := &stubOrchestrator{}
	h := NewHandler(stub, "")
	req := httptest.NewRequest(http.MethodGet, pullPath, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status: got %d want 405", w.Code)
	}
}

// ---- Health -----------------------------------------------------------------

func TestHealthEndpoints(t *testing.T) {
	h := NewHandler(&stubOrchestrator{}, "")
	for _, path := range []string{"/health", pullPath + "/health"} {
		w := getReq(t, h, path)
		if w.Code != http.StatusOK {
			t.Errorf("%s: got %d want 200", path, w.Code)
		}
	}
}

// ---- Status (backward compat) -----------------------------------------------

// SPEC.md "GET /status": {"status":"ok","authBackend":"grpc","enableAuth":true}.
func TestStatusHandler_SpecShape(t *testing.T) {
	for _, tc := range []struct {
		backend string
		enabled bool
	}{{"grpc", true}, {"consumerauth", false}} {
		mux := http.NewServeMux()
		RegisterRoutes(mux, &stubOrchestrator{}, tc.backend, tc.enabled, "")
		req := httptest.NewRequest(http.MethodGet, "/status", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("status: got %d want 200", w.Code)
		}
		var body map[string]interface{}
		if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		want := map[string]interface{}{"status": "ok", "authBackend": tc.backend, "enableAuth": tc.enabled}
		if len(body) != len(want) {
			t.Errorf("keys: got %v, want exactly %v", body, want)
		}
		for k, v := range want {
			if body[k] != v {
				t.Errorf("%s: got %v want %v", k, body[k], v)
			}
		}
	}
}

// SPEC.md "GET /health": {"status":"ok"}.
func TestHealthHandler_SpecShape(t *testing.T) {
	mux := http.NewServeMux()
	RegisterRoutes(mux, &stubOrchestrator{}, "grpc", true, "")
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200", w.Code)
	}
	if got := strings.TrimSpace(w.Body.String()); got != `{"status":"ok"}` {
		t.Errorf("body: got %s", got)
	}
}

// ---- Step 18: Lock management -----------------------------------------------

func TestLockCreate(t *testing.T) {
	h := NewHandler(&stubOrchestrator{}, "")
	w := postJSON(t, h, "/serviceorchestration/orchestration/mgmt/lock/create", map[string]any{
		"orchestrationJobId": "job-1",
		"serviceInstanceId":  "svc-1",
		"owner":              "sys-a",
		"temporary":          true,
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var lock Lock
	json.NewDecoder(w.Body).Decode(&lock)
	if lock.ID == 0 {
		t.Error("expected non-zero lock ID")
	}
}

func TestLockQuery(t *testing.T) {
	h := NewHandler(&stubOrchestrator{}, "")
	postJSON(t, h, "/serviceorchestration/orchestration/mgmt/lock/create", map[string]any{
		"owner": "sys-a",
	})
	w := postJSON(t, h, "/serviceorchestration/orchestration/mgmt/lock/query", map[string]any{})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp LockQueryResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Count < 1 {
		t.Error("expected at least 1 lock")
	}
}

func TestLockRemoveByOwner(t *testing.T) {
	h := NewHandler(&stubOrchestrator{}, "")
	postJSON(t, h, "/serviceorchestration/orchestration/mgmt/lock/create", map[string]any{
		"owner": "owner-x",
	})
	req := httptest.NewRequest(http.MethodDelete,
		"/serviceorchestration/orchestration/mgmt/lock/remove/owner-x", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}
	// Query should now be empty.
	w2 := postJSON(t, h, "/serviceorchestration/orchestration/mgmt/lock/query", map[string]any{})
	var resp LockQueryResponse
	json.NewDecoder(w2.Body).Decode(&resp)
	if resp.Count != 0 {
		t.Errorf("expected 0 locks after remove, got %d", resp.Count)
	}
}

func TestHistoryRecordedOnPull(t *testing.T) {
	h := NewHandler(&stubOrchestrator{
		resp: OrchestrationResponse{Response: []OrchestrationResult{}},
	}, "")
	body := `{"requesterSystem":{"systemName":"c1"},"requestedService":{"serviceDefinition":"svc1"}}`
	req := httptest.NewRequest(http.MethodPost, pullPath, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	httptest.NewRecorder() // discard response
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	w2 := postJSON(t, h, "/serviceorchestration/orchestration/mgmt/history/query", map[string]any{})
	var resp HistoryQueryResponse
	json.NewDecoder(w2.Body).Decode(&resp)
	if resp.Count < 1 {
		t.Error("expected history entry after pull orchestration")
	}
}

// ---- Step 19: Subscribe / unsubscribe ---------------------------------------

var validSubscribeBody = map[string]any{
	"ownerSystemName":  "consumer-app",
	"targetSystemName": "consumer-app",
	"orchestrationRequest": map[string]any{
		"requesterSystem":  map[string]any{"systemName": "consumer-app"},
		"requestedService": map[string]any{"serviceDefinition": "svc"},
	},
}

func TestSubscribeReturns201(t *testing.T) {
	h := NewHandler(&stubOrchestrator{}, "")
	w := postJSON(t, h, "/serviceorchestration/orchestration/subscribe", validSubscribeBody)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var sub struct{ ID string `json:"id"` }
	json.NewDecoder(w.Body).Decode(&sub)
	if len(sub.ID) != 36 {
		t.Errorf("id = %q, not a UUID", sub.ID)
	}
}

func TestSubscribeDuplicateReturns200(t *testing.T) {
	h := NewHandler(&stubOrchestrator{}, "")
	postJSON(t, h, "/serviceorchestration/orchestration/subscribe", validSubscribeBody)
	w := postJSON(t, h, "/serviceorchestration/orchestration/subscribe", validSubscribeBody)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 on duplicate, got %d", w.Code)
	}
}

func TestUnsubscribeFound200(t *testing.T) {
	h := NewHandler(&stubOrchestrator{}, "")
	sw := postJSON(t, h, "/serviceorchestration/orchestration/subscribe", validSubscribeBody)
	var sub struct{ ID string `json:"id"` }
	json.NewDecoder(sw.Body).Decode(&sub)

	req := httptest.NewRequest(http.MethodDelete,
		"/serviceorchestration/orchestration/unsubscribe/"+sub.ID, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestUnsubscribeNotFound204(t *testing.T) {
	h := NewHandler(&stubOrchestrator{}, "")
	req := httptest.NewRequest(http.MethodDelete,
		"/serviceorchestration/orchestration/unsubscribe/no-such-id", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}
}

func TestPushMgmtSubscribeAndQuery(t *testing.T) {
	h := NewHandler(&stubOrchestrator{}, "")
	postJSON(t, h, "/serviceorchestration/orchestration/mgmt/push/subscribe", validSubscribeBody)
	w := postJSON(t, h, "/serviceorchestration/orchestration/mgmt/push/query", map[string]any{})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp SubscriptionQueryResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Count < 1 {
		t.Error("expected at least 1 subscription")
	}
}

func TestTriggerCreatesPendingHistory(t *testing.T) {
	// The subscriber holds the delivery open until the history has been read,
	// so the entry is still PENDING. (Without a notifyInterface the delivery
	// goroutine marks FAILED at once and races the query.)
	release := make(chan struct{})
	subscriber := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
		w.WriteHeader(http.StatusNoContent)
	}))
	defer subscriber.Close()
	defer close(release)

	h := NewHandler(&stubOrchestrator{}, "")
	body := map[string]any{
		"ownerSystemName":      "consumer-app",
		"targetSystemName":     "consumer-app",
		"orchestrationRequest": validSubscribeBody["orchestrationRequest"],
		"notifyInterface":      map[string]any{"notifyUri": subscriber.URL + "/notify"},
	}
	sw := postJSON(t, h, "/serviceorchestration/orchestration/subscribe", body)
	var sub struct{ ID string `json:"id"` }
	json.NewDecoder(sw.Body).Decode(&sub)

	tw := postJSON(t, h, "/serviceorchestration/orchestration/mgmt/push/trigger",
		map[string]any{"subscriptionId": sub.ID})
	if tw.Code != http.StatusOK {
		t.Fatalf("trigger: expected 200, got %d: %s", tw.Code, tw.Body.String())
	}

	hw := postJSON(t, h, "/serviceorchestration/orchestration/mgmt/history/query", map[string]any{})
	var hist HistoryQueryResponse
	json.NewDecoder(hw.Body).Decode(&hist)
	found := false
	for _, e := range hist.Entries {
		if e.Status == "PENDING" && e.Type == "PUSH" {
			found = true
		}
	}
	if !found {
		t.Errorf("no PENDING PUSH history entry found: %+v", hist.Entries)
	}
}

func TestTriggerNotFoundReturns404(t *testing.T) {
	h := NewHandler(&stubOrchestrator{}, "")
	w := postJSON(t, h, "/serviceorchestration/orchestration/mgmt/push/trigger",
		map[string]any{"subscriptionId": "no-such-id"})
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// ---- Subscribe response and push delivery (SPEC.md "subscribe", "push/trigger") ----

// TestSubscribeReturnsFullSubscription pins the subscribe response: the whole
// stored subscription, not only its id.
func TestSubscribeReturnsFullSubscription(t *testing.T) {
	h := NewHandler(&stubOrchestrator{}, "")
	body := map[string]any{
		"ownerSystemName":  "ConsumerApp",
		"targetSystemName": "ConsumerApp",
		"orchestrationRequest": map[string]any{
			"requesterSystem":  map[string]any{"systemName": "ConsumerApp"},
			"requestedService": map[string]any{"serviceDefinition": "telemetry"},
		},
		"notifyInterface": map[string]any{"notifyUri": "http://consumer:8080/notify"},
		"expiredAt":       "2027-01-01T00:00:00Z",
	}
	w := postJSON(t, h, "/serviceorchestration/orchestration/subscribe", body)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var got map[string]any
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, k := range []string{"id", "ownerSystemName", "targetSystemName", "orchestrationRequest", "notifyInterface", "expiredAt", "createdAt"} {
		if _, ok := got[k]; !ok {
			t.Errorf("response lacks %q: %v", k, got)
		}
	}
	if len(got) != 7 {
		t.Errorf("keys: got %d (%v), want 7", len(got), got)
	}
	if got["ownerSystemName"] != "ConsumerApp" || got["expiredAt"] != "2027-01-01T00:00:00Z" {
		t.Errorf("echoed fields: got %v", got)
	}
}

// historyStatus polls mgmt/history/query until a PUSH entry leaves PENDING.
func historyStatus(t *testing.T, h http.Handler) string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		hw := postJSON(t, h, "/serviceorchestration/orchestration/mgmt/history/query", map[string]any{})
		var hist HistoryQueryResponse
		json.NewDecoder(hw.Body).Decode(&hist)
		for _, e := range hist.Entries {
			if e.Type == "PUSH" && e.Status != "PENDING" {
				return e.Status
			}
		}
		if time.Now().After(deadline) {
			return "PENDING"
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// TestTriggerDeliversRealPost verifies that a trigger POSTs to notifyInterface,
// that the notification carries only the subscription identity (no provider
// list), and that the history entry becomes DELIVERED.
func TestTriggerDeliversRealPost(t *testing.T) {
	received := make(chan map[string]any, 1)
	subscriber := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var n map[string]any
		json.NewDecoder(r.Body).Decode(&n)
		if r.Method == http.MethodPost && r.URL.Path == "/notify" {
			received <- n
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer subscriber.Close()

	h := NewHandler(&stubOrchestrator{}, "")
	body := map[string]any{
		"ownerSystemName":      "ConsumerApp",
		"targetSystemName":     "ConsumerApp",
		"orchestrationRequest": validSubscribeBody["orchestrationRequest"],
		"notifyInterface":      map[string]any{"notifyUri": subscriber.URL + "/notify"},
	}
	sw := postJSON(t, h, "/serviceorchestration/orchestration/subscribe", body)
	var sub struct{ ID string `json:"id"` }
	json.NewDecoder(sw.Body).Decode(&sub)

	tw := postJSON(t, h, "/serviceorchestration/orchestration/mgmt/push/trigger", map[string]any{"subscriptionId": sub.ID})
	if tw.Code != http.StatusOK || !strings.Contains(tw.Body.String(), `"triggered"`) {
		t.Fatalf("trigger: got %d %s", tw.Code, tw.Body.String())
	}

	select {
	case n := <-received:
		want := map[string]any{"subscriptionId": sub.ID, "ownerSystemName": "ConsumerApp", "targetSystemName": "ConsumerApp"}
		if len(n) != len(want) {
			t.Errorf("notification keys: got %v, want exactly %v", n, want)
		}
		for k, v := range want {
			if n[k] != v {
				t.Errorf("notification[%q]: got %v want %v", k, n[k], v)
			}
		}
	case <-time.After(5 * time.Second):
		t.Fatal("subscriber received no POST")
	}
	if got := historyStatus(t, h); got != "DELIVERED" {
		t.Errorf("history status: got %s want DELIVERED", got)
	}
}

// TestTriggerNon2xxMarksFailed verifies that a subscriber error marks FAILED.
func TestTriggerNon2xxMarksFailed(t *testing.T) {
	subscriber := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer subscriber.Close()

	h := NewHandler(&stubOrchestrator{}, "")
	body := map[string]any{
		"ownerSystemName":      "ConsumerApp",
		"targetSystemName":     "ConsumerApp",
		"orchestrationRequest": validSubscribeBody["orchestrationRequest"],
		"notifyInterface":      map[string]any{"notifyUri": subscriber.URL + "/notify"},
	}
	sw := postJSON(t, h, "/serviceorchestration/orchestration/subscribe", body)
	var sub struct{ ID string `json:"id"` }
	json.NewDecoder(sw.Body).Decode(&sub)
	postJSON(t, h, "/serviceorchestration/orchestration/mgmt/push/trigger", map[string]any{"subscriptionId": sub.ID})

	if got := historyStatus(t, h); got != "FAILED" {
		t.Errorf("history status: got %s want FAILED", got)
	}
}

// ---- Management access (SPEC.md "Management access") ------------------------

// TestMgmtAuth_Cases drives requireMgmtAuth through mgmt/history/query with an
// Authentication stub. An unknown token gets 200 {"verified":false,"sysop":false},
// as from the real Authentication system.
func TestMgmtAuth_Cases(t *testing.T) {
	auth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimPrefix(r.URL.Path, "/authentication/identity/verify/")
		w.Header().Set("Content-Type", "application/json")
		switch token {
		case "sysop-token":
			w.Write([]byte(`{"verified":true,"systemName":"SysopSystem","sysop":true}`)) //nolint:errcheck
		case "user-token":
			w.Write([]byte(`{"verified":true,"systemName":"UserSystem","sysop":false}`)) //nolint:errcheck
		case "broken-token":
			w.WriteHeader(http.StatusInternalServerError)
		case "garbage-token":
			w.Write([]byte(`not json`)) //nolint:errcheck
		default:
			w.Write([]byte(`{"verified":false,"sysop":false}`)) //nolint:errcheck
		}
	}))
	defer auth.Close()

	cases := []struct {
		name, authURL, token string
		wantStatus           int
		wantType             string
	}{
		{"open mode", "", "", http.StatusOK, ""},
		{"no token", auth.URL, "", http.StatusUnauthorized, "AUTH_EXCEPTION"},
		{"auth unreachable", "http://127.0.0.1:1", "sysop-token", http.StatusUnauthorized, "AUTH_EXCEPTION"},
		{"auth non-200", auth.URL, "broken-token", http.StatusUnauthorized, "AUTH_EXCEPTION"},
		{"undecodable answer", auth.URL, "garbage-token", http.StatusUnauthorized, "AUTH_EXCEPTION"},
		{"verified false", auth.URL, "bogus", http.StatusUnauthorized, "AUTH_EXCEPTION"},
		{"verified, not sysop", auth.URL, "user-token", http.StatusForbidden, "FORBIDDEN"},
		{"verified sysop", auth.URL, "sysop-token", http.StatusOK, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := NewHandler(&stubOrchestrator{}, tc.authURL)
			req := httptest.NewRequest(http.MethodPost, "/serviceorchestration/orchestration/mgmt/history/query", strings.NewReader("{}"))
			if tc.token != "" {
				req.Header.Set("Authorization", "Bearer "+tc.token)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)
			if w.Code != tc.wantStatus {
				t.Fatalf("status: got %d want %d; body %s", w.Code, tc.wantStatus, w.Body.String())
			}
			if tc.wantType == "" {
				return
			}
			var body map[string]any
			if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if body["exceptionType"] != tc.wantType || body["origin"] != "dynamicorch-xacml" {
				t.Errorf("envelope: got %v", body)
			}
		})
	}
}
