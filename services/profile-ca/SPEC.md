# profile-ca — HTTP API Specification

Arrowhead 5.2 Local Cloud Certificate Authority with integrated PIP.

---

## Environment Variables

| Variable | Default | Required | Description |
|---|---|---|---|
| `PORT` | `8787` | No | Plain HTTP listen port |
| `TLS_PORT` | `8788` | No | mTLS HTTPS listen port |
| `CA_KEY_FILE` | `/data/ca.key` | No | Path to persist CA private key |

---

## Plain HTTP Endpoints (PORT)

### `GET /health`

**Response `200 OK`**
```json
{"status": "ok", "system": "profile-ca"}
```

### `GET /ca/info`

Returns CA certificate in PEM format.

**Response `200 OK`**
```json
{
  "commonName": "Arrowhead Local Cloud CA",
  "certificate": "-----BEGIN CERTIFICATE-----\n...\n-----END CERTIFICATE-----\n"
}
```

### `POST /bootstrap/onboarding-cert`

Issue an Onboarding certificate (OU=on). No authentication required.

**Request**
```json
{"systemName": "my-device"}
```

**Response `201 Created`**
```json
{
  "systemName": "my-device",
  "certificate": "-----BEGIN CERTIFICATE-----\n...",
  "privateKey": "-----BEGIN EC PRIVATE KEY-----\n...",
  "profile": "on",
  "issuedAt": "2026-06-25T00:00:00Z"
}
```

| Status | Condition |
|---|---|
| `400` | Empty systemName |

### `POST /ca/certificate/issue`

Issue a System certificate (OU=sy) without profile chain enforcement.
Used by cert-provisioner for infrastructure services.

**Request / Response:** Same format as `/bootstrap/onboarding-cert`. Profile is `"sy"`.

### `DELETE /ca/certificates/{cn}`

Revoke a certificate by Common Name.

**Response `204 No Content`**

| Status | Condition |
|---|---|
| `404` | CN not found or already revoked |

### `POST /ca/certificates/{cn}/reissue`

Un-revoke a previously revoked certificate.

**Response `204 No Content`**

| Status | Condition |
|---|---|
| `404` | CN not found or not currently revoked |

---

## PIP Endpoints (PORT, CA-as-PIP)

All PEP clients call these endpoints via `PIP_URL` environment variable.

### `GET /pip/health`

**Response `200 OK`**
```json
{"status": "ok", "system": "pip (CA-as-PIP)", "subjects": 12}
```

### `GET /pip/attributes/{cn}`

XACML-ready certificate validity attributes. Used by PEPs before XACML evaluation.

**Response `200 OK`**
```json
{"systemName": "my-device", "certLevel": "sy", "valid": true}
```

| Field | Type | Description |
|---|---|---|
| `systemName` | string | Certificate Common Name |
| `certLevel` | string | Profile tier: `on`, `de`, or `sy` |
| `valid` | boolean | `true` if not revoked AND not expired |

| Status | Condition |
|---|---|
| `404` | CN not found |

**Validity logic:** `valid = !revoked && now < expiresAt`

### `GET /pip/subjects`

All certificate records including revoked.

**Response `200 OK`**
```json
{
  "subjects": [
    {
      "cn": "my-device",
      "ou": "sy",
      "issuedAt": "2026-06-25T00:00:00Z",
      "expiresAt": "2027-06-25T00:00:00Z",
      "revoked": false,
      "valid": true
    }
  ],
  "count": 1
}
```

### `GET /pip/subjects/{cn}`

Single certificate record detail.

**Response `200 OK`** — same shape as one element of the `subjects` array above.

| Status | Condition |
|---|---|
| `404` | CN not found |

### `GET /pip/status`

Summary count of all certificate records (including revoked).

**Response `200 OK`**
```json
{"subjects": 12}
```

---

## mTLS Endpoints (TLS_PORT)

These endpoints require a valid client certificate issued by this CA.

### `POST /ca/device-cert`

Issue a Device certificate (OU=de). Requires Onboarding (OU=on) client cert.

**Request / Response:** Same format as `/bootstrap/onboarding-cert`. Profile is `"de"`.

| Status | Condition |
|---|---|
| `401` | No client certificate |
| `403` | Client cert is not OU=on |

### `POST /ca/system-cert`

Issue a System certificate (OU=sy). Requires Device (OU=de) client cert.

**Request / Response:** Same format as `/bootstrap/onboarding-cert`. Profile is `"sy"`.

| Status | Condition |
|---|---|
| `401` | No client certificate |
| `403` | Client cert is not OU=de |
