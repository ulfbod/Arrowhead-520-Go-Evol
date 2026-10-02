//go:build integration

package integration

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

// subsetComposeFile is the six-service ConsumerAuthorization-mode stack.
var subsetComposeFile = envOr("CONSUMERAUTH_COMPOSE_FILE", "../../deploy/docker-compose.consumerauth.yml")

// subsetPost POSTs a JSON body to a plain-HTTP port inside a subset container
// and returns the response body. The foundation's plain HTTP ports are only
// reachable inside the Docker network; the host sees the mTLS ports.
func subsetPost(t *testing.T, service, url, body string) string {
	t.Helper()
	cmd := `wget -qO- --header='Content-Type: application/json' --post-data='` + body + `' ` + url
	out, err := exec.Command("docker", "compose", "-f", subsetComposeFile,
		"exec", "-T", service, "sh", "-c", cmd).CombinedOutput()
	if err != nil {
		t.Fatalf("POST %s in %s: %v\n%s", url, service, err, out)
	}
	return strings.TrimSpace(string(out))
}

type pullResult struct {
	Provider struct {
		SystemName string `json:"systemName"`
		Address    string `json:"address"`
		Port       int    `json:"port"`
	} `json:"provider"`
	Service struct {
		ServiceDefinition string            `json:"serviceDefinition"`
		ServiceUri        string            `json:"serviceUri"`
		Interfaces        []string          `json:"interfaces"`
		Version           int               `json:"version"`
		Metadata          map[string]string `json:"metadata"`
	} `json:"service"`
	CloudIdentifier string `json:"cloudIdentifier"`
}

type pullResponse struct {
	Response []pullResult `json:"response"`
}

// ah5Name builds a name that is unique per run and valid under the AH5 naming
// rules enforced by the ServiceRegistry: system names are PascalCase
// (^[A-Z][A-Za-z0-9]{0,62}$), service definition names camelCase
// (^[a-z][A-Za-z0-9]{0,62}$). The harness prefix "t-<unix>-" has hyphens, so
// only its digits are used.
func ah5Name(base string) string {
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, testPrefix)
	return base + digits
}

// pull runs a pull orchestration and returns the results.
func pull(t *testing.T, consumer, svcDef string) []pullResult {
	t.Helper()
	code, body := apiPost(t, dynamicOrchURL+"/serviceorchestration/orchestration/pull", map[string]any{
		"requesterSystem":  map[string]string{"systemName": consumer},
		"requestedService": map[string]string{"serviceDefinition": svcDef},
	})
	if code != 200 {
		t.Fatalf("pull as %s: got %d, body: %s", consumer, code, body)
	}
	var resp pullResponse
	unmarshal(t, body, &resp)
	for _, r := range resp.Response {
		if r.Service.ServiceDefinition != svcDef {
			t.Errorf("pull returned service %q, want %q", r.Service.ServiceDefinition, svcDef)
		}
	}
	return resp.Response
}

// providerNames lists the provider system names of pull results.
func providerNames(results []pullResult) []string {
	names := make([]string, 0, len(results))
	for _, r := range results {
		names = append(names, r.Provider.SystemName)
	}
	return names
}

// TestConsumerAuthMode proves that dynamicorch-xacml with
// AUTH_BACKEND=consumerauth filters providers by the rules held in the AH5
// ConsumerAuthorization system, with no authz-pdp in the stack.
//
// Providers register through the AH5 service-discovery API; a provider present
// only in the legacy /serviceregistry/register store is not orchestrated even
// with a rule.
//
// Requires the subset stack:
//
//	docker compose -f deploy/docker-compose.consumerauth.yml up --build -d
//
// Skipped when the running orchestrator uses another backend (full stack).
func TestConsumerAuthMode(t *testing.T) {
	code, body := apiGet(t, dynamicOrchURL+"/status")
	if code != 200 {
		t.Fatalf("status: got %d, body: %s", code, body)
	}
	var st struct {
		Status      string `json:"status"`
		AuthBackend string `json:"authBackend"`
		EnableAuth  bool   `json:"enableAuth"`
	}
	unmarshal(t, body, &st)
	if st.AuthBackend != "consumerauth" {
		t.Skipf("orchestrator authBackend is %q, not consumerauth; start deploy/docker-compose.consumerauth.yml", st.AuthBackend)
	}
	if !st.EnableAuth {
		t.Fatal("enableAuth is false; the proof needs authorization enabled")
	}

	svcDef := ah5Name("caModeSvc")
	consumer := ah5Name("CaModeConsumer")
	other := ah5Name("CaModeOther")
	granted := ah5Name("CaModeProvGranted")
	ungranted := ah5Name("CaModeProvUngranted")
	legacyOnly := ah5Name("CaModeProvLegacy")

	// Two providers of the same service, registered through the AH5
	// service-discovery API (the store the orchestrator reads).
	for _, p := range []string{granted, ungranted} {
		out := subsetPost(t, "serviceregistry", "http://localhost:8080/serviceregistry/service-discovery/register",
			`{"systemName":"`+p+`","serviceDefinitionName":"`+svcDef+`","version":"2.1.0","interfaces":[{"templateName":"generic_http","protocol":"http","policy":"NONE","properties":{"accessAddresses":"10.0.0.1","accessPort":"9000","basePath":"/telemetry"}}]}`)
		if !strings.Contains(out, p+"|"+svcDef) {
			t.Fatalf("SR service-discovery register %s: got %s", p, out)
		}
	}
	// A third provider registered only through the legacy endpoint.
	out := subsetPost(t, "serviceregistry", "http://localhost:8080/serviceregistry/register",
		`{"serviceDefinition":"`+svcDef+`","providerSystem":{"systemName":"`+legacyOnly+`","address":"10.0.0.3","port":9000},"serviceUri":"/telemetry","interfaces":["HTTP-INSECURE-JSON"]}`)
	if !strings.Contains(out, legacyOnly) {
		t.Fatalf("SR legacy register: got %s", out)
	}

	t.Run("NoRule_PullIsEmpty", func(t *testing.T) {
		if got := pull(t, consumer, svcDef); len(got) != 0 {
			t.Fatalf("pull without a rule: got providers %v, want none", providerNames(got))
		}
	})

	// ConsumerAuthorization rules: consumer may use svcDef at the granted
	// provider and at the legacy-only provider; nothing for the ungranted one.
	for _, p := range []string{granted, legacyOnly} {
		out := subsetPost(t, "consumerauth", "http://localhost:8082/consumerauthorization/authorization/grant",
			`{"provider":"`+p+`","targetType":"SERVICE_DEF","target":"`+svcDef+`","defaultPolicy":{"policyType":"WHITELIST","policyList":["`+consumer+`"]}}`)
		var policy struct {
			InstanceID string `json:"instanceId"`
		}
		if err := json.Unmarshal([]byte(out), &policy); err != nil || policy.InstanceID == "" {
			t.Fatalf("grant for %s: no instanceId in response: %s", p, out)
		}
	}

	t.Run("Rule_PullReturnsOnlyGrantedAH5Provider", func(t *testing.T) {
		got := pull(t, consumer, svcDef)
		if len(got) != 1 || got[0].Provider.SystemName != granted {
			t.Fatalf("pull with a rule: got providers %v, want [%s]", providerNames(got), granted)
		}
		// Access details mapped from the AH5 interface properties (core/SPEC.md).
		r := got[0]
		if r.Provider.Address != "10.0.0.1" || r.Provider.Port != 9000 {
			t.Errorf("provider access: got %s:%d, want 10.0.0.1:9000", r.Provider.Address, r.Provider.Port)
		}
		if r.Service.ServiceUri != "/telemetry" || r.Service.Version != 2 {
			t.Errorf("service: got uri=%q version=%d, want /telemetry and 2", r.Service.ServiceUri, r.Service.Version)
		}
		if len(r.Service.Interfaces) != 1 || r.Service.Interfaces[0] != "generic_http" {
			t.Errorf("interfaces: got %v, want [generic_http]", r.Service.Interfaces)
		}
	})

	t.Run("Rule_OtherConsumerStillEmpty", func(t *testing.T) {
		if got := pull(t, other, svcDef); len(got) != 0 {
			t.Fatalf("pull as a consumer not on the whitelist: got providers %v, want none", providerNames(got))
		}
	})
}
