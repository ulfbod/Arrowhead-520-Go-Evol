//go:build integration

package integration

import (
	"strings"
	"testing"
	"time"
)

// TestGrpcModePull proves the positive orchestration path on the default
// stack (deploy/docker-compose.yml, AUTH_BACKEND=grpc): a provider registered
// through the AH5 service-discovery API is returned to a consumer that has a
// Permit policy for action "orchestrate", in the result shape of core/SPEC.md,
// and is not returned to a consumer without a policy.
//
// Skipped when the running orchestrator uses another backend (the
// consumerauth subset stack; see TestConsumerAuthMode).
func TestGrpcModePull(t *testing.T) {
	code, body := apiGet(t, dynamicOrchURL+"/status")
	if code != 200 {
		t.Fatalf("status: got %d, body: %s", code, body)
	}
	var st struct {
		AuthBackend string `json:"authBackend"`
		EnableAuth  bool   `json:"enableAuth"`
	}
	unmarshal(t, body, &st)
	if st.AuthBackend != "grpc" {
		t.Skipf("orchestrator authBackend is %q, not grpc; start deploy/docker-compose.yml", st.AuthBackend)
	}
	if !st.EnableAuth {
		t.Fatal("enableAuth is false; the proof needs authorization enabled")
	}

	svcDef := ah5Name("grpcModeSvc")
	provider := ah5Name("GrpcModeProvider")
	permitted := ah5Name("GrpcModeConsumer")
	other := ah5Name("GrpcModeOther")

	// Provider registers through the AH5 service-discovery API.
	out := dockerExec(t, "serviceregistry",
		`wget -qO- --header='Content-Type: application/json' --post-data='{"systemName":"`+provider+`","serviceDefinitionName":"`+svcDef+`","version":"2.1.0","metadata":{"zone":"B"},"interfaces":[{"templateName":"generic_http","protocol":"http","policy":"NONE","properties":{"accessAddresses":"10.0.0.1","accessPort":"9000","basePath":"/telemetry"}}]}' http://localhost:8080/serviceregistry/service-discovery/register`)
	if !strings.Contains(out, provider+"|"+svcDef) {
		t.Fatalf("SR service-discovery register: got %s", out)
	}

	t.Run("NoPolicy_PullIsEmpty", func(t *testing.T) {
		if got := pull(t, permitted, svcDef); len(got) != 0 {
			t.Fatalf("pull without a policy: got providers %v, want none", providerNames(got))
		}
	})

	// Permit policy for the consumer, through the PAP.
	code, body = apiPost(t, papURL+"/policies", map[string]string{
		"subject":  permitted,
		"resource": svcDef,
		"action":   "orchestrate",
		"effect":   "Permit",
	})
	if code != 200 && code != 201 {
		t.Fatalf("create policy: got %d, body: %s", code, body)
	}

	t.Run("Policy_PullReturnsProviderInSpecShape", func(t *testing.T) {
		// The PAP pushes the policy to the PDP asynchronously; allow a short wait.
		var got []pullResult
		deadline := time.Now().Add(15 * time.Second)
		for {
			got = pull(t, permitted, svcDef)
			if len(got) > 0 || time.Now().After(deadline) {
				break
			}
			time.Sleep(500 * time.Millisecond)
		}
		if len(got) != 1 || got[0].Provider.SystemName != provider {
			t.Fatalf("pull with a Permit policy: got providers %v, want [%s]", providerNames(got), provider)
		}
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
		if r.Service.Metadata["zone"] != "B" || r.CloudIdentifier != "LOCAL" {
			t.Errorf("metadata/cloud: got %v / %q, want zone=B / LOCAL", r.Service.Metadata, r.CloudIdentifier)
		}
	})

	t.Run("Policy_OtherConsumerStillEmpty", func(t *testing.T) {
		if got := pull(t, other, svcDef); len(got) != 0 {
			t.Fatalf("pull as a consumer without a policy: got providers %v, want none", providerNames(got))
		}
	})
}
