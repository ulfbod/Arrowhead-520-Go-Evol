package orchestration

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// lookupBody is an AH5 service-discovery lookup response as the foundation
// ServiceRegistry writes it (foundation/SPEC.md), kept as literal JSON so the
// test does not share types with the client under test.
const lookupBody = `{
  "entries": [
    {
      "instanceId": "Prov1|telemetry|2.1.0",
      "provider": {"name": "Prov1", "addresses": [{"type": "IPV4", "address": "192.168.0.9"}], "createdAt": "", "updatedAt": ""},
      "serviceDefinitionName": "telemetry",
      "version": "2.1.0",
      "metadata": {"zone": "B"},
      "interfaces": [
        {"templateName": "generic_mqtt", "protocol": "tcp", "policy": "NONE"},
        {"templateName": "generic_http", "protocol": "http", "policy": "NONE",
         "properties": {"accessAddresses": "10.0.0.1, 10.0.0.2", "accessPort": "9000", "basePath": "/tel"}}
      ],
      "createdAt": "2026-09-30T00:00:00Z", "updatedAt": "2026-09-30T00:00:00Z"
    },
    {
      "instanceId": "Prov2|telemetry|1.0.0",
      "provider": {"name": "Prov2", "addresses": [{"type": "IPV4", "address": "192.168.0.7"}]},
      "serviceDefinitionName": "telemetry",
      "version": "1.0.0",
      "interfaces": [{"templateName": "generic_http", "protocol": "http", "policy": "NONE"}]
    }
  ],
  "count": 2
}`

// TestSRClient_QuerySR_WireShape pins the request the client sends: AH5
// lookup path, method and exact JSON keys.
func TestSRClient_QuerySR_WireShape(t *testing.T) {
	var (
		gotPath, gotMethod string
		captured           map[string]any
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		json.NewDecoder(r.Body).Decode(&captured)
		fmt.Fprint(w, `{"entries":[],"count":0}`)
	}))
	defer srv.Close()

	c := NewSRClient(srv.URL)
	if _, err := c.QuerySR(ServiceFilter{ServiceDefinition: "telemetry"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/serviceregistry/service-discovery/lookup" {
		t.Errorf("request: got %s %s", gotMethod, gotPath)
	}
	names, _ := captured["serviceDefinitionNames"].([]any)
	if len(captured) != 1 || len(names) != 1 || names[0] != "telemetry" {
		t.Errorf("body: got %v, want only serviceDefinitionNames=[telemetry]", captured)
	}

	// With an interface filter the template names are forwarded.
	if _, err := c.QuerySR(ServiceFilter{ServiceDefinition: "telemetry", Interfaces: []string{"generic_http"}}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tmpl, _ := captured["interfaceTemplateNames"].([]any)
	if len(captured) != 2 || len(tmpl) != 1 || tmpl[0] != "generic_http" {
		t.Errorf("body with interfaces: got %v", captured)
	}
}

// TestSRClient_QuerySR_Mapping verifies the SPEC.md mapping of an AH5 service
// instance to the orchestration fields.
func TestSRClient_QuerySR_Mapping(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, lookupBody)
	}))
	defer srv.Close()

	c := NewSRClient(srv.URL)
	instances, err := c.QuerySR(ServiceFilter{ServiceDefinition: "telemetry"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(instances) != 2 {
		t.Fatalf("expected 2 instances, got %d", len(instances))
	}

	// Entry 1: access details come from the first interface that has them.
	got := instances[0]
	if got.Provider != (System{SystemName: "Prov1", Address: "10.0.0.1", Port: 9000}) {
		t.Errorf("provider: got %+v", got.Provider)
	}
	if got.ServiceDefinition != "telemetry" || got.ServiceUri != "/tel" || got.Version != 2 {
		t.Errorf("service: got def=%q uri=%q version=%d", got.ServiceDefinition, got.ServiceUri, got.Version)
	}
	if len(got.Interfaces) != 2 || got.Interfaces[0] != "generic_mqtt" || got.Interfaces[1] != "generic_http" {
		t.Errorf("interfaces: got %v", got.Interfaces)
	}
	if got.Metadata["zone"] != "B" {
		t.Errorf("metadata: got %v", got.Metadata)
	}

	// Entry 2: no interface properties, so the system address is used; port and URI are empty.
	got = instances[1]
	if got.Provider != (System{SystemName: "Prov2", Address: "192.168.0.7", Port: 0}) {
		t.Errorf("provider fallback: got %+v", got.Provider)
	}
	if got.ServiceUri != "" || got.Version != 1 {
		t.Errorf("service fallback: got uri=%q version=%d", got.ServiceUri, got.Version)
	}
}

// TestSRClient_QuerySR_MetadataFilter verifies that requested metadata must be
// contained in the entry's metadata.
func TestSRClient_QuerySR_MetadataFilter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, lookupBody)
	}))
	defer srv.Close()

	c := NewSRClient(srv.URL)
	instances, err := c.QuerySR(ServiceFilter{ServiceDefinition: "telemetry", Metadata: map[string]string{"zone": "B"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(instances) != 1 || instances[0].Provider.SystemName != "Prov1" {
		t.Errorf("zone=B: got %+v", instances)
	}
	instances, _ = c.QuerySR(ServiceFilter{ServiceDefinition: "telemetry", Metadata: map[string]string{"zone": "A"}})
	if len(instances) != 0 {
		t.Errorf("zone=A: expected none, got %+v", instances)
	}
}

// TestSRClient_QuerySR_NetworkError verifies that a connection failure returns an error.
func TestSRClient_QuerySR_NetworkError(t *testing.T) {
	c := NewSRClient("http://127.0.0.1:1") // nothing listening
	_, err := c.QuerySR(ServiceFilter{ServiceDefinition: "x"})
	if err == nil {
		t.Fatal("expected error on network failure")
	}
}

// TestSRClient_QuerySR_BadJSON verifies that malformed JSON returns an error.
func TestSRClient_QuerySR_BadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "{not valid json")
	}))
	defer srv.Close()

	c := NewSRClient(srv.URL)
	_, err := c.QuerySR(ServiceFilter{ServiceDefinition: "x"})
	if err == nil {
		t.Fatal("expected error on bad JSON")
	}
}

// TestSRClient_QuerySR_Non200 verifies that an error status is not read as an
// empty result (the legacy path now answers 404 on the AH5-only client).
func TestSRClient_QuerySR_Non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"errorCode":400,"errorMessage":"bad","entries":[]}`)
	}))
	defer srv.Close()

	c := NewSRClient(srv.URL)
	_, err := c.QuerySR(ServiceFilter{ServiceDefinition: "x"})
	if err == nil {
		t.Fatal("expected error on non-200 response")
	}
}

// TestSRClient_QuerySR_EmptyResponse verifies that an empty result list is handled.
func TestSRClient_QuerySR_EmptyResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"entries":[],"count":0}`)
	}))
	defer srv.Close()

	c := NewSRClient(srv.URL)
	instances, err := c.QuerySR(ServiceFilter{ServiceDefinition: "x"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(instances) != 0 {
		t.Errorf("expected empty, got %d", len(instances))
	}
}

// TestHandler_InternalError_Returns500 covers the non-validation error branch.
type errOrchestrator struct{}

func (e *errOrchestrator) Orchestrate(_ OrchestrationRequest) (OrchestrationResponse, error) {
	return OrchestrationResponse{}, errors.New("registry unreachable")
}

func TestHandler_InternalError_Returns500(t *testing.T) {
	h := NewHandler(&errOrchestrator{}, "")
	body := `{"requesterSystem":{"systemName":"c1"},"requestedService":{"serviceDefinition":"svc"}}`
	req := httptest.NewRequest(http.MethodPost, "/serviceorchestration/orchestration/pull", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status: got %d want 500", w.Code)
	}
}
