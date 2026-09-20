//go:build integration

package integration

import (
	"os/exec"
	"strings"
	"testing"
)

// dockerExec runs a command inside a compose service container and returns stdout.
// This is used for foundation services that require mTLS — their plain HTTP
// health port is only accessible from inside the Docker network.
func dockerExec(t *testing.T, service, cmd string) string {
	t.Helper()
	out, err := exec.Command("docker", "compose", "-f", "../../deploy/docker-compose.yml",
		"exec", "-T", service, "sh", "-c", cmd).CombinedOutput()
	if err != nil {
		t.Fatalf("docker exec %s: %v\n%s", service, err, out)
	}
	return string(out)
}

// TestFoundation tests the AH5 foundation services.
// Foundation services enforce mTLS on their TLS ports, so we test via
// docker exec to reach the plain HTTP health endpoint inside the container.
func TestFoundation(t *testing.T) {
	t.Run("ServiceRegistry_Health", func(t *testing.T) {
		out := dockerExec(t, "serviceregistry", "wget -qO- http://localhost:8080/health")
		if !strings.Contains(out, "serviceregistry") {
			t.Errorf("SR health: got %q", out)
		}
	})

	t.Run("Authentication_Health", func(t *testing.T) {
		out := dockerExec(t, "authentication", "wget -qO- http://localhost:8081/health")
		if !strings.Contains(out, "authentication") {
			t.Errorf("Auth health: got %q", out)
		}
	})

	t.Run("ConsumerAuth_Health", func(t *testing.T) {
		out := dockerExec(t, "consumerauth", "wget -qO- http://localhost:8082/health")
		if !strings.Contains(out, "consumerauthorization") {
			t.Errorf("ConsumerAuth health: got %q", out)
		}
	})

	t.Run("ServiceRegistry_Register", func(t *testing.T) {
		svcDef := cn("foundation-test-svc")
		payload := `{"serviceDefinition":"` + svcDef + `","providerSystem":{"systemName":"` + cn("fnd-provider") + `","address":"localhost","port":9999},"serviceUri":"/test","interfaces":["HTTP-INSECURE-JSON"]}`
		cmd := `wget -qO- --post-data='` + payload + `' --header='Content-Type: application/json' http://localhost:8080/serviceregistry/register`
		out := dockerExec(t, "serviceregistry", cmd)
		if !strings.Contains(out, svcDef) {
			t.Errorf("SR register: response missing service definition, got: %s", out)
		}
	})

	t.Run("ServiceRegistry_Query", func(t *testing.T) {
		svcDef := cn("foundation-test-svc")
		payload := `{"serviceDefinition":"` + svcDef + `"}`
		cmd := `wget -qO- --post-data='` + payload + `' --header='Content-Type: application/json' http://localhost:8080/serviceregistry/query`
		out := dockerExec(t, "serviceregistry", cmd)
		if !strings.Contains(out, svcDef) {
			t.Errorf("SR query: result missing %q, got: %s", svcDef, out)
		}
	})
}
