# ADAPI Stack — API Specification

Two services: **authz-pdp** (gRPC) and **dynamicorch-xacml** (HTTP REST).

---

## authz-pdp — gRPC Authorization PDP

Implements `AuthorizationPDP.Decide` from `proto/authorize/authorize.proto`.

### Environment Variables

| Variable | Default | Description |
|---|---|---|
| `AUTHZFORCE_URL` | `http://authzforce:8080/authzforce-ce` | AuthzForce base URL |
| `AUTHZFORCE_DOMAIN` | `arrowhead-520` | XACML domain external ID |
| `PORT` | `9550` | gRPC listen port |

### gRPC Interface

```protobuf
service AuthorizationPDP {
  rpc Decide(DecisionRequest) returns (DecisionResponse);
}
```

**DecisionRequest:**

| Field | Type | XACML Attribute |
|---|---|---|
| `domain_id` | string | N/A (selects AuthzForce domain) |
| `subject` | string | `urn:oasis:names:tc:xacml:1.0:subject:subject-id` |
| `service` | string | `urn:oasis:names:tc:xacml:1.0:resource:resource-id` |
| `provider` | string | `urn:arrowhead:attribute:provider-id` (optional) |
| `action` | string | `urn:oasis:names:tc:xacml:1.0:action:action-id` |

**DecisionResponse:**

| Field | Type | Values |
|---|---|---|
| `decision` | Decision enum | `PERMIT`, `DENY`, `INDETERMINATE`, `NOT_APPLICABLE` |
| `status_code` | string | XACML status code URN |

**Semantics:**
- Missing `subject`, `service`, or `action` → `INDETERMINATE`
- AuthzForce error → `INDETERMINATE`
- Empty `provider` → omitted from XACML request (service-level decision)

**Inspection:**
```bash
grpcurl -plaintext localhost:9550 list
grpcurl -plaintext -d '{"subject":"test","service":"svc","action":"consume"}' \
  localhost:9550 arrowhead.authz.v1.AuthorizationPDP/Decide
```

---

## dynamicorch-xacml — HTTP REST Orchestration

### Environment Variables

| Variable | Default | Description |
|---|---|---|
| `SR_URL` | `http://localhost:8080` | ServiceRegistry base URL |
| `AUTHZ_PDP_ADDR` | `localhost:9550` | authz-pdp gRPC address |
| `CA_URL` | `http://localhost:8082` | ConsumerAuth base URL (fallback) |
| `AUTH_BACKEND` | `grpc` | `grpc` or `consumerauth` |
| `ENABLE_AUTH` | `true` | Enable authorization (`false` bypasses) |
| `PORT` | `8083` | HTTP listen port |
| `DOMAIN_ID` | `""` | Policy domain (passed to PDP) |
| `MGMT_AUTH_URL` | `""` | Auth URL for sysop mgmt endpoints |
| `PUSH_DELIVERY_TIMEOUT_SECONDS` | `5` | Push notification HTTP delivery timeout |

### Endpoints

#### `GET /health`

**Response `200 OK`**
```json
{"status":"ok"}
```

#### `GET /status`

**Response `200 OK`**
```json
{"status":"ok","authBackend":"grpc","enableAuth":true}
```

#### `POST /serviceorchestration/orchestration/pull`

Pull orchestration: find providers for a requested service.

**Request**
```json
{
  "requesterSystem": {"systemName": "consumer-a"},
  "requestedService": {"serviceDefinition": "telemetry"}
}
```

**Response `200 OK`**
```json
{
  "response": [
    {
      "provider": {"systemName": "provider-1", "address": "10.0.0.1", "port": 9000},
      "service": {"serviceDefinition": "telemetry"},
      "serviceUri": "/telemetry",
      "interfaces": ["HTTP-INSECURE-JSON"]
    }
  ]
}
```

**Error Responses**

| Status | Condition |
|---|---|
| `400` | Missing requesterSystem or requestedService |
| `404` | No providers found |

#### `POST /serviceorchestration/orchestration/subscribe`

Subscribe for push notifications.

**Request**
```json
{
  "requesterSystem": {"systemName": "consumer-a"},
  "requestedService": {"serviceDefinition": "telemetry"},
  "notifyInterface": "http://consumer-a:8080/notify"
}
```

**Response `201 Created`**
```json
{"id": 1}
```

#### `DELETE /serviceorchestration/orchestration/unsubscribe/{id}`

**Response `204 No Content`**

#### `POST /serviceorchestration/orchestration/mgmt/push/subscribe`

Management: create push subscription. Requires sysop Bearer token if `MGMT_AUTH_URL` is set.

#### `POST /serviceorchestration/orchestration/mgmt/push/trigger`

Management: trigger push notification delivery to a subscriber.

#### `POST /serviceorchestration/orchestration/mgmt/push/query`

Management: list push subscriptions.

#### `POST /serviceorchestration/orchestration/mgmt/lock/create`

Create an exclusive lock on a service instance.

**Request**
```json
{"owner": "consumer-a", "serviceInstanceId": 42, "ttlSeconds": 300}
```

**Response `201 Created`**
```json
{"id": 1, "owner": "consumer-a", "serviceInstanceId": 42}
```

#### `POST /serviceorchestration/orchestration/mgmt/lock/query`

Query active locks.

#### `DELETE /serviceorchestration/orchestration/mgmt/lock/remove/{owner}`

Remove all locks held by an owner.

**Response `204 No Content`**

#### `POST /serviceorchestration/orchestration/mgmt/history/query`

Query orchestration history.

**Request**
```json
{"pagination": {"pageSize": 20, "pageNumber": 0}}
```

#### `POST /serviceorchestration/orchestration/general/mgmt/logs`

Query in-memory log buffer.

**Request**
```json
{
  "pagination": {"pageSize": 20, "pageNumber": 0},
  "from": "2026-01-01T00:00:00Z",
  "severity": "INFO"
}
```

#### `GET /serviceorchestration/orchestration/general/mgmt/get-config`

Query runtime configuration.

**Query parameter:** `keys=PORT,AUTH_BACKEND`

**Response `200 OK`**
```json
{"PORT": "8083", "AUTH_BACKEND": "grpc"}
```
