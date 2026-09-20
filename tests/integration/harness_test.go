//go:build integration

// Package integration provides a Go test harness for the ADAPI stack.
//
// Run with:
//
//	cd tests/integration
//	go test -tags=integration -v -timeout 5m ./...
//
// Prerequisites: Docker stack must be running:
//
//	docker compose -f deploy/docker-compose.yml up --build -d
//
// Selective execution:
//
//	go test -tags=integration -run TestCertIssuance ./...
//	go test -tags=integration -run TestRevocation ./...
//	go test -tags=integration -run TestPIP ./...
//
// The harness uses a unique prefix per run to avoid state collisions across
// repeated executions. All test certs use the prefix "t-{unix_timestamp}-".
package integration

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"
)

// Service URLs — read from env vars with defaults matching docker-compose.yml.
var (
	profileCAURL    string
	papURL          string
	authzforceURL   string
	dynamicOrchURL  string
	kafkaAuthzURL   string
	topicAuthURL    string
	pkiRestAuthzURL string
	srURL           string
	authURL         string
	consumerAuthURL string
	rabbitmqMgmtURL string
	kafdropURL      string
	dashboardURL    string

	// testPrefix ensures test data is unique per run.
	testPrefix string

	// httpClient with reasonable timeouts and TLS skip for foundation services.
	client    *http.Client
	tlsClient *http.Client
)

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// TestMain sets up service URLs, creates the HTTP client, and validates
// connectivity before running any tests. If pre-flight fails, the entire
// suite is skipped with a clear message.
func TestMain(m *testing.M) {
	profileCAURL = envOr("PROFILE_CA_URL", "http://localhost:8787")
	papURL = envOr("PAP_URL", "http://localhost:9505")
	authzforceURL = envOr("AUTHZFORCE_URL", "http://localhost:8896")
	dynamicOrchURL = envOr("DYNAMICORCH_URL", "http://localhost:8083")
	kafkaAuthzURL = envOr("KAFKA_AUTHZ_URL", "http://localhost:9101")
	topicAuthURL = envOr("TOPIC_AUTH_URL", "http://localhost:9090")
	pkiRestAuthzURL = envOr("PKI_REST_AUTHZ_URL", "http://localhost:9209")
	srURL = envOr("SR_URL", "https://localhost:8490")
	authURL = envOr("AUTH_URL", "https://localhost:8491")
	consumerAuthURL = envOr("CONSUMER_AUTH_URL", "https://localhost:8492")
	rabbitmqMgmtURL = envOr("RABBITMQ_MGMT_URL", "http://localhost:15672")
	kafdropURL = envOr("KAFDROP_URL", "http://localhost:9000")
	dashboardURL = envOr("DASHBOARD_URL", "http://localhost:3000")

	testPrefix = fmt.Sprintf("t-%d-", time.Now().Unix())

	client = &http.Client{Timeout: 10 * time.Second}
	tlsClient = &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	os.Exit(m.Run())
}

// --- HTTP helpers ---

// apiGet performs a GET and returns status code and body. Fails the test on error.
func apiGet(t *testing.T, url string) (int, []byte) {
	t.Helper()
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, body
}

// tryGet performs a GET but returns (0, nil, error) on connection failure
// instead of calling t.Fatal. Use for optional services that may not be running.
func tryGet(url string) (int, []byte, error) {
	resp, err := client.Get(url)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, body, nil
}

// apiGetTLS performs a GET with TLS skip verification (for foundation services).
func apiGetTLS(t *testing.T, url string) (int, []byte) {
	t.Helper()
	resp, err := tlsClient.Get(url)
	if err != nil {
		t.Fatalf("GET (TLS) %s: %v", url, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, body
}

// apiPost performs a POST with JSON body and returns status code and body.
func apiPost(t *testing.T, url string, payload any) (int, []byte) {
	t.Helper()
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	resp, err := client.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, body
}

// apiPostTLS performs a POST with TLS skip verification.
func apiPostTLS(t *testing.T, url string, payload any) (int, []byte) {
	t.Helper()
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	resp, err := tlsClient.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		t.Fatalf("POST (TLS) %s: %v", url, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, body
}

// apiDelete performs a DELETE and returns status code and body.
func apiDelete(t *testing.T, url string) (int, []byte) {
	t.Helper()
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		t.Fatalf("new DELETE request: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("DELETE %s: %v", url, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, body
}

// unmarshal decodes JSON body into v, failing the test on error.
func unmarshal(t *testing.T, data []byte, v any) {
	t.Helper()
	if err := json.Unmarshal(data, v); err != nil {
		t.Fatalf("unmarshal JSON: %v\nbody: %s", err, string(data))
	}
}

// cn returns a test-prefixed common name to avoid collisions.
func cn(name string) string {
	return testPrefix + name
}
