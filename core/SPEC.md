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
- authz-pdp returns `PERMIT` when the PDP answers Permit and `DENY` otherwise;
  `NOT_APPLICABLE` is never returned. The PDP bundled in `deploy/docker-compose.yml`
  (`shared/authzforce-server`) decides on (subject, resource) only and ignores
  `action`, `provider`, cert-level and cert-valid.

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
| `MGMT_AUTH_URL` | `""` | Auth URL for sysop mgmt endpoints (see Management access below) |
| `PUSH_DELIVERY_TIMEOUT_SECONDS` | `5` | Push notification HTTP delivery timeout |

### ServiceRegistry lookup

Candidate providers come from the AH5 ServiceRegistry service-discovery API:
`POST <SR_URL>/serviceregistry/service-discovery/lookup`. Only services
registered through `POST /serviceregistry/service-discovery/register` are
orchestrated; the legacy `/serviceregistry/register` store is not read.

**Lookup request** — `interfaceTemplateNames` is sent only when
`requestedService.interfaces` is non-empty:

```json
{
  "serviceDefinitionNames": ["<requestedService.serviceDefinition>"],
  "interfaceTemplateNames": ["<requestedService.interfaces...>"]
}
```

If `requestedService.metadata` is given, an entry is kept only when its
`metadata` contains every requested key with an equal value.

**Mapping of each lookup entry to an orchestration result:**

| Result field | Source in the AH5 service instance |
|---|---|
| `provider.systemName` | `provider.name` |
| `provider.address` | property `accessAddresses` of the first interface that has it (first value if comma-separated); otherwise `provider.addresses[0].address`; otherwise `""` |
| `provider.port` | property `accessPort` of the first interface that has it, as an integer; otherwise `0` |
| `serviceDefinition` | `serviceDefinitionName` |
| `serviceUri` | property `basePath` of the first interface that has it; otherwise `""` |
| `interfaces` | `interfaces[].templateName` |
| `version` | leading integer of `version` (`"2.1.0"` → `2`); `0` if there is none |
| `metadata` | `metadata` |

A transport error, a non-`200` status or an undecodable body from the
ServiceRegistry fails the request with `500`.

### Management access

When `MGMT_AUTH_URL` is set, every `/serviceorchestration/orchestration/mgmt/*`
endpoint requires `Authorization: Bearer <token>`, checked with
`GET <MGMT_AUTH_URL>/authentication/identity/verify/<token>`:

| Case | Status |
|---|---|
| No Bearer token | `401` AUTH_EXCEPTION |
| Authentication unreachable or non-`200` | `401` AUTH_EXCEPTION |
| `"verified": false` (unknown or expired token) | `401` AUTH_EXCEPTION |
| Verified, `"sysop": false` | `403` FORBIDDEN |
| Verified, `"sysop": true` | allowed |

Errors use the AH5 envelope with `origin` `dynamicorch-xacml`. When
`MGMT_AUTH_URL` is empty, management endpoints are open. The `general/mgmt`
endpoints (logs, get-config) are not guarded.

### Authorization backends

`AUTH_BACKEND` selects how each candidate provider is authorized when
`ENABLE_AUTH=true`. In both backends an error, a non-`200` status or an
undecodable response excludes the provider (fail-closed).

| `AUTH_BACKEND` | Call per candidate provider |
|---|---|
| `grpc` | `AuthorizationPDP.Decide` on `AUTHZ_PDP_ADDR` with `action="orchestrate"` |
| `consumerauth` | `POST <CA_URL>/consumerauthorization/authorization/verify` (AH5 ConsumerAuthorization) |

**`consumerauth` request** — the AH5 verify body; `targetType` is always
`SERVICE_DEF`, `scope` is not sent, and `DOMAIN_ID` and the action are not used:

```json
{
  "consumer":   "<requesterSystem.systemName>",
  "provider":   "<candidate provider systemName>",
  "target":     "<serviceDefinition>",
  "targetType": "SERVICE_DEF"
}
```

**`consumerauth` response `200 OK`** — a plain JSON Boolean, `true` or `false`
(not a wrapped object). `true` keeps the provider, `false` excludes it.

In `consumerauth` mode the service needs only `SR_URL` and `CA_URL`; it does not
contact `AUTHZ_PDP_ADDR`.

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
      "provider": {"systemName": "Provider1", "address": "10.0.0.1", "port": 9000},
      "service": {
        "serviceDefinition": "telemetry",
        "serviceUri": "/telemetry",
        "interfaces": ["generic_http"],
        "version": 2,
        "metadata": {"zone": "B"}
      },
      "cloudIdentifier": "LOCAL"
    }
  ]
}
```

`service.metadata` is omitted when the service instance has none. When no
provider is registered for the service, or none is authorized for the requester,
the response is `200 OK` with an empty list: `{"response":[]}`.

This nested result shape differs from the AH5 `OrchestrationResult` (flat
`providerName`, `results` list) served by the AH5 DynamicOrchestration system.

**Error Responses**

| Status | Condition |
|---|---|
| `400` | Invalid JSON, or missing `requesterSystem.systemName` or `requestedService.serviceDefinition` |
| `500` | ServiceRegistry unreachable or answering with an error |

#### `POST /serviceorchestration/orchestration/subscribe`

Subscribe for push notifications. One subscription per
`ownerSystemName` + `targetSystemName`; subscribing again with the same pair
overwrites `orchestrationRequest`, `notifyInterface` and `expiredAt` and keeps
the `id`. `expiredAt` is stored and returned but not enforced.

**Request**
```json
{
  "ownerSystemName":  "ConsumerApp",
  "targetSystemName": "ConsumerApp",
  "orchestrationRequest": {
    "requesterSystem":  {"systemName": "ConsumerApp"},
    "requestedService": {"serviceDefinition": "telemetry"}
  },
  "notifyInterface": {"notifyUri": "http://consumer-app:8080/notify"},
  "expiredAt": "2027-01-01T00:00:00Z"
}
```

`notifyInterface` is an object. The delivery URL is taken from `notifyUri`, else
`uri`, else built as `http://<address>[:<port>]<path>` from those keys.

**Response `201 Created`** (new) or **`200 OK`** (overwrite) — the stored subscription:
```json
{
  "id": "3f1c2a9e-8d4b-4c1a-9e2f-6b7a8c9d0e1f",
  "ownerSystemName":  "ConsumerApp",
  "targetSystemName": "ConsumerApp",
  "orchestrationRequest": {
    "requesterSystem":  {"systemName": "ConsumerApp"},
    "requestedService": {"serviceDefinition": "telemetry"}
  },
  "notifyInterface": {"notifyUri": "http://consumer-app:8080/notify"},
  "expiredAt": "2027-01-01T00:00:00Z",
  "createdAt": "2026-09-30T12:00:00Z"
}
```

`notifyInterface` and `expiredAt` are omitted when not given. `400` on invalid JSON.

#### `DELETE /serviceorchestration/orchestration/unsubscribe/{id}`

**Response `200 OK`** (removed) or **`204 No Content`** (no such id)

#### `POST /serviceorchestration/orchestration/mgmt/push/subscribe`

Management: create push subscription. Same body and response as `subscribe`.
Requires sysop Bearer token if `MGMT_AUTH_URL` is set.

#### `POST /serviceorchestration/orchestration/mgmt/push/trigger`

Management: trigger push notification delivery to a subscriber.

**Request:** `{"subscriptionId": "<id>"}` — **Response `200 OK`:**
`{"status": "triggered"}`; `404` for an unknown id.

Delivery is real and asynchronous: the handler records a `PUSH` history entry
with status `PENDING`, answers, then POSTs to the subscription's notify URL
with the timeout `PUSH_DELIVERY_TIMEOUT_SECONDS`. A `2xx` answer sets the entry to
`DELIVERED`; a transport error, a non-`2xx` answer or a missing notify URL sets
`FAILED`. There is no retry.

The notification carries the subscription identity only, **no provider list**;
the subscriber runs its own pull to get providers:

```json
{"subscriptionId": "<id>", "ownerSystemName": "ConsumerApp", "targetSystemName": "ConsumerApp"}
```

#### `POST /serviceorchestration/orchestration/mgmt/push/query`

Management: list push subscriptions. **Response `200 OK`:**
`{"subscriptions": [ /* subscription */ ], "count": 1}`

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

Query orchestration history. The request body is ignored; every entry is
returned (no filter, no pagination). Requires a sysop Bearer token if
`MGMT_AUTH_URL` is set.

**Response `200 OK`**
```json
{
  "entries": [
    {
      "id": "…",
      "status": "DELIVERED",
      "type": "PUSH",
      "requesterSystem": "ConsumerApp",
      "serviceDefinition": "telemetry",
      "createdAt": "2026-09-30T12:00:00Z",
      "finishedAt": "2026-09-30T12:00:01Z"
    }
  ],
  "count": 1
}
```

`status` is `DONE` or `ERROR` for `PULL` entries and `PENDING`, `DELIVERED` or
`FAILED` for `PUSH` entries. `requesterSystem`, `serviceDefinition`, `message`
and `finishedAt` are omitted when empty.

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

**Query parameter:** `keys=PORT,AUTH_BACKEND` — a comma-separated list of names
from the Environment Variables table above. Values are returned as strings with
defaults applied; unknown keys are omitted.

**Response `200 OK`**
```json
{"PORT": "8083", "AUTH_BACKEND": "grpc"}
```
