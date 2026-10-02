package orchestration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	pb "arrowhead/core/proto/authorize"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	// ErrMissingRequester is returned when requesterSystem.systemName is empty.
	ErrMissingRequester = errors.New("requesterSystem.systemName is required")
	// ErrMissingService is returned when requestedService.serviceDefinition is empty.
	ErrMissingService = errors.New("requestedService.serviceDefinition is required")
)

// AuthDecider abstracts the PDP access-control decision.
//
// Each field maps to a XACML attribute (see proto/authorize/authorize.proto):
//   - domainID  → XACML domain (AuthzForce UUID or external name; may be empty)
//   - subject   → XACML subject-id (consumer system name)
//   - service   → XACML resource-id (service definition)
//   - provider  → XACML urn:arrowhead:attribute:provider-id (provider system name; may be empty)
//   - action    → XACML action-id ("orchestrate" for orchestration, "consume" for enforcement)
//
// Implementations:
//   - *GRPCDecider   — calls authz-pdp over gRPC (authorize.proto)
//   - *CADecider     — calls AH5 ConsumerAuthorization over HTTP
type AuthDecider interface {
	Decide(domainID, subject, service, provider, action string) (bool, error)
}

// RegistryQuerier abstracts the ServiceRegistry query.
type RegistryQuerier interface {
	QuerySR(filter ServiceFilter) ([]ServiceInstance, error)
}

// XACMLOrchestrator performs AH5-evolved orchestration.
//
// Per-provider decision semantics (action = "orchestrate"):
//   - Permit      → include provider in result
//   - Deny        → exclude provider (fail-closed)
//   - error       → exclude provider (fail-closed; treat as Deny)
//   - enabled=false → bypass AuthDecider, return all SR results (passthrough mode)
type XACMLOrchestrator struct {
	sr       RegistryQuerier
	decider  AuthDecider
	domainID string
	enabled  bool
}

// NewXACMLOrchestrator creates a new XACMLOrchestrator.
//
//   - sr:       ServiceRegistry client
//   - decider:  AuthDecider implementation (GRPCDecider or CADecider)
//   - domainID: policy domain identifier passed to AuthDecider
//   - enabled:  when false, authorization check is skipped (ENABLE_AUTH=false)
func NewXACMLOrchestrator(sr RegistryQuerier, decider AuthDecider, domainID string, enabled bool) *XACMLOrchestrator {
	return &XACMLOrchestrator{sr: sr, decider: decider, domainID: domainID, enabled: enabled}
}

// Orchestrate processes an orchestration request:
//  1. Validate requesterSystem.systemName and requestedService.serviceDefinition.
//  2. Query ServiceRegistry for all providers of the requested service.
//  3. If enabled: for each provider call AuthDecider.Decide with
//     action="orchestrate", service and provider as separate fields.
//     Include only Permit providers; skip on Deny or error (fail-closed).
//  4. If disabled: return all providers without authorization check.
func (o *XACMLOrchestrator) Orchestrate(req OrchestrationRequest) (OrchestrationResponse, error) {
	if req.RequesterSystem.SystemName == "" {
		return OrchestrationResponse{}, ErrMissingRequester
	}
	if req.RequestedService.ServiceDefinition == "" {
		return OrchestrationResponse{}, ErrMissingService
	}

	instances, err := o.sr.QuerySR(req.RequestedService)
	if err != nil {
		return OrchestrationResponse{}, fmt.Errorf("service registry: %w", err)
	}

	if !o.enabled {
		return buildResponse(instances), nil
	}

	// Per-provider decision: action="orchestrate", service and provider as separate fields.
	// Fail-closed: error or Deny → exclude provider from results.
	results := make([]OrchestrationResult, 0, len(instances))
	for _, inst := range instances {
		permitted, decErr := o.decider.Decide(
			o.domainID,
			req.RequesterSystem.SystemName,
			inst.ServiceDefinition,
			inst.Provider.SystemName,
			"orchestrate",
		)
		if decErr != nil || !permitted {
			continue
		}
		results = append(results, OrchestrationResult{
			Provider: inst.Provider,
			Service: ServiceInfo{
				ServiceDefinition: inst.ServiceDefinition,
				ServiceUri:        inst.ServiceUri,
				Interfaces:        inst.Interfaces,
				Version:           inst.Version,
				Metadata:          inst.Metadata,
			},
			CloudIdentifier: "LOCAL",
		})
	}
	return OrchestrationResponse{Response: results}, nil
}

func buildResponse(instances []ServiceInstance) OrchestrationResponse {
	results := make([]OrchestrationResult, 0, len(instances))
	for _, inst := range instances {
		results = append(results, OrchestrationResult{
			Provider: inst.Provider,
			Service: ServiceInfo{
				ServiceDefinition: inst.ServiceDefinition,
				ServiceUri:        inst.ServiceUri,
				Interfaces:        inst.Interfaces,
				Version:           inst.Version,
				Metadata:          inst.Metadata,
			},
			CloudIdentifier: "LOCAL",
		})
	}
	return OrchestrationResponse{Response: results}
}

// --- GRPCDecider — calls authz-pdp over gRPC (authorize.proto) ---

// GRPCDecider implements AuthDecider by calling the authz-pdp gRPC service.
// It is the primary AuthDecider in experiment-12.
type GRPCDecider struct {
	client pb.AuthorizationPDPClient
}

// NewGRPCDecider dials the authz-pdp gRPC server at addr and returns a
// GRPCDecider. The caller owns the connection lifecycle.
func NewGRPCDecider(addr string) (*GRPCDecider, *grpc.ClientConn, error) {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, nil, fmt.Errorf("dial authz-pdp %s: %w", addr, err)
	}
	return &GRPCDecider{client: pb.NewAuthorizationPDPClient(conn)}, conn, nil
}

// Decide sends a DecisionRequest to authz-pdp. Returns true iff the response
// Decision is PERMIT. Any gRPC error or non-PERMIT decision → false (fail-closed).
func (g *GRPCDecider) Decide(domainID, subject, service, provider, action string) (bool, error) {
	resp, err := g.client.Decide(context.Background(), &pb.DecisionRequest{
		DomainId: domainID,
		Subject:  subject,
		Service:  service,
		Provider: provider,
		Action:   action,
	})
	if err != nil {
		return false, err
	}
	return resp.Decision == pb.Decision_PERMIT, nil
}

// --- CADecider — calls AH5 ConsumerAuthorization over HTTP ---

// CADecider implements AuthDecider by calling the AH5 ConsumerAuthorization
// service. It is the fallback when AUTH_BACKEND=consumerauth.
//
// Mapping: subject→consumer, provider→provider, service→target with
// targetType SERVICE_DEF.
// domainID and action are ignored (CA has no domain/action concept).
type CADecider struct {
	baseURL string
	http    *http.Client
}

// NewCADecider returns a CADecider backed by the ConsumerAuth service at baseURL.
func NewCADecider(baseURL string) *CADecider {
	return &CADecider{
		baseURL: baseURL,
		http:    &http.Client{Timeout: 5 * time.Second},
	}
}

// caVerifyPath is the AH5 ConsumerAuthorization verify endpoint.
const caVerifyPath = "/consumerauthorization/authorization/verify"

// caTargetServiceDef is the AH5 targetType for a service definition.
const caTargetServiceDef = "SERVICE_DEF"

// caVerifyRequest mirrors the ConsumerAuthorization verify body (AH5 model).
type caVerifyRequest struct {
	Consumer   string `json:"consumer"`
	Provider   string `json:"provider,omitempty"`
	Target     string `json:"target"`
	TargetType string `json:"targetType"`
}

// Decide calls ConsumerAuthorization verify(consumer, provider, target).
// The response body is a plain JSON Boolean, not a wrapped object.
// domainID and action are ignored — CA is a flat boolean grant store.
func (c *CADecider) Decide(_, subject, service, provider, _ string) (bool, error) {
	body := caVerifyRequest{
		Consumer:   subject,
		Provider:   provider,
		Target:     service,
		TargetType: caTargetServiceDef,
	}
	data, _ := json.Marshal(body)
	resp, err := c.http.Post(c.baseURL+caVerifyPath, "application/json", bytes.NewReader(data))
	if err != nil {
		return false, fmt.Errorf("consumerauth verify: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("consumerauth verify returned %d", resp.StatusCode)
	}
	var authorized bool
	if err := json.NewDecoder(resp.Body).Decode(&authorized); err != nil {
		return false, fmt.Errorf("consumerauth decode: %w", err)
	}
	return authorized, nil
}

// --- ServiceRegistry HTTP client ---

// srClient implements RegistryQuerier against a live ServiceRegistry.
type srClient struct {
	baseURL string
	http    *http.Client
}

// NewSRClient returns a RegistryQuerier backed by a real ServiceRegistry.
func NewSRClient(baseURL string) RegistryQuerier {
	return &srClient{
		baseURL: baseURL,
		http:    &http.Client{Timeout: 5 * time.Second},
	}
}

// srLookupPath is the AH5 ServiceRegistry service-discovery lookup endpoint.
const srLookupPath = "/serviceregistry/service-discovery/lookup"

// AH5 interface property names that carry a provider's access details.
const (
	propAccessAddresses = "accessAddresses"
	propAccessPort      = "accessPort"
	propBasePath        = "basePath"
)

// srLookupRequest is the body for POST /serviceregistry/service-discovery/lookup.
type srLookupRequest struct {
	ServiceDefinitionNames []string `json:"serviceDefinitionNames"`
	InterfaceTemplateNames []string `json:"interfaceTemplateNames,omitempty"`
}

type srAddress struct {
	Type    string `json:"type"`
	Address string `json:"address"`
}

type srSystem struct {
	Name      string      `json:"name"`
	Addresses []srAddress `json:"addresses,omitempty"`
}

type srInterface struct {
	TemplateName string            `json:"templateName"`
	Properties   map[string]string `json:"properties,omitempty"`
}

// srServiceInstance mirrors the AH5 service instance in a lookup response.
type srServiceInstance struct {
	Provider              *srSystem         `json:"provider,omitempty"`
	ServiceDefinitionName string            `json:"serviceDefinitionName"`
	Version               string            `json:"version,omitempty"`
	Metadata              map[string]string `json:"metadata,omitempty"`
	Interfaces            []srInterface     `json:"interfaces,omitempty"`
}

type srLookupResponse struct {
	Entries []srServiceInstance `json:"entries"`
	Count   int                 `json:"count"`
}

// QuerySR looks up providers of filter.ServiceDefinition in the AH5
// service-discovery store and maps each entry as specified in SPEC.md
// ("ServiceRegistry lookup"). The metadata filter is applied here.
func (c *srClient) QuerySR(filter ServiceFilter) ([]ServiceInstance, error) {
	body := srLookupRequest{
		ServiceDefinitionNames: []string{filter.ServiceDefinition},
		InterfaceTemplateNames: filter.Interfaces,
	}
	data, _ := json.Marshal(body)
	resp, err := c.http.Post(c.baseURL+srLookupPath, "application/json", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("service-discovery lookup returned %d", resp.StatusCode)
	}
	var result srLookupResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	instances := make([]ServiceInstance, 0, len(result.Entries))
	for _, e := range result.Entries {
		if !metadataContains(e.Metadata, filter.Metadata) {
			continue
		}
		inst := ServiceInstance{
			ServiceDefinition: e.ServiceDefinitionName,
			ServiceUri:        firstProperty(e.Interfaces, propBasePath),
			Interfaces:        make([]string, 0, len(e.Interfaces)),
			Version:           leadingInt(e.Version),
			Metadata:          e.Metadata,
		}
		for _, ifc := range e.Interfaces {
			inst.Interfaces = append(inst.Interfaces, ifc.TemplateName)
		}
		if e.Provider != nil {
			inst.Provider.SystemName = e.Provider.Name
			if len(e.Provider.Addresses) > 0 {
				inst.Provider.Address = e.Provider.Addresses[0].Address
			}
		}
		if addrs := firstProperty(e.Interfaces, propAccessAddresses); addrs != "" {
			first, _, _ := strings.Cut(addrs, ",")
			inst.Provider.Address = strings.TrimSpace(first)
		}
		inst.Provider.Port, _ = strconv.Atoi(strings.TrimSpace(firstProperty(e.Interfaces, propAccessPort)))
		instances = append(instances, inst)
	}
	return instances, nil
}

// firstProperty returns the named property of the first interface that has it.
func firstProperty(ifaces []srInterface, name string) string {
	for _, ifc := range ifaces {
		if v, ok := ifc.Properties[name]; ok {
			return v
		}
	}
	return ""
}

// metadataContains reports whether have holds every key of want with an equal value.
func metadataContains(have, want map[string]string) bool {
	for k, v := range want {
		if hv, ok := have[k]; !ok || hv != v {
			return false
		}
	}
	return true
}

// leadingInt returns the leading decimal integer of s ("2.1.0" → 2), or 0.
func leadingInt(s string) int {
	end := 0
	for end < len(s) && s[end] >= '0' && s[end] <= '9' {
		end++
	}
	n, _ := strconv.Atoi(s[:end])
	return n
}
