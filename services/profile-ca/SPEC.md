# profile-ca — HTTP API Specification

Arrowhead 5.2 Local Cloud Certificate Authority with integrated PIP.

---

## Environment Variables

| Variable | Default | Required | Description |
|---|---|---|---|
| `PORT` | `8787` | No | Plain HTTP listen port |
| `TLS_PORT` | `8788` | No | mTLS HTTPS listen port |
| `CA_KEY_FILE` | `/data/ca.key` | No | Path of the persisted CA private key. Unset or empty means the default; the service always persists. (An ephemeral, non-persisting CA exists only inside the unit tests.) |
| `CA_CERT_FILE` | `ca.crt` next to `CA_KEY_FILE` | No | Path of the persisted CA certificate (PEM) |
| `CA_STATE_FILE` | `records.json` next to `CA_KEY_FILE` | No | Path of the persisted certificate records and serial counter |

---

## Persistent State

The CA keeps three files, by default in the `/data` volume:

| File | Content | Mode |
|---|---|---|
| `ca.key` | CA private key, PEM `EC PRIVATE KEY` | `0600` |
| `ca.crt` | CA certificate, PEM `CERTIFICATE` | `0644` |
| `records.json` | Certificate records and the next serial number | `0600` |

`records.json` format (version 1):

```json
{
  "version": 1,
  "nextSerial": 7,
  "records": [
    {"cn": "sensor-1", "ou": "sy", "serial": 6,
     "issuedAt": "2026-10-03T10:00:00Z", "expiresAt": "2027-10-03T10:00:00Z",
     "revoked": false}
  ]
}
```

One record per Common Name; issuing again for a CN replaces its record.

**Writes.** Every issue, revoke and reissue writes the complete new state to a
temporary file in the same directory, fsyncs it, renames it over the state file and
fsyncs the directory, all while holding the CA lock. The certificate and private
key, or the `204`, are returned only after that write has completed. If the write
fails, the request fails with `500` and the in-memory state is left unchanged.
A request that is interrupted before it answers may be lost; a request that
answered success is not.

Known edge: if the directory fsync fails after the rename succeeded, the request
answers `500` although the new state file is already in place, so the disk is
briefly ahead of memory. The next successful write (which writes the in-memory
state) or a restart (which loads the file) brings them back in line.

**Start-up.**

- No key, certificate or state file: first start. A new key and CA certificate are
  generated and written; the record set is empty; the first issued serial is `3`.
- All present: the key, the certificate and the records are loaded. The CA
  certificate is reused byte for byte, so its fingerprint does not change across
  restarts, and certificates issued before a restart still verify. Serial numbers
  continue from the stored counter and are never reused.
- Key present, certificate and state both absent (state written by v0.1.1 or
  earlier): the CA certificate is created once from the existing key and written.
  Certificates issued before this upgrade were signed by the same key and still
  verify. This is the only case in which a CA certificate is created for an
  existing key.
- Leftover temporary files from an interrupted write (`.ca.key.tmp-*`,
  `.ca.crt.tmp-*`, `.records.json.tmp-*` in the state directories) are removed at
  start, before anything is loaded, with a log line per file; the last complete
  state file is used.
- Start-up fails (non-zero exit, a log line naming the file and the reason) if the
  key file exists but cannot be read or parsed, the certificate file exists but
  cannot be read or parsed or does not match the key, the state file exists but
  cannot be read or parsed, a certificate or state file exists without a key file,
  the state file exists without a certificate file, a leftover temporary file cannot
  be removed, or a newly generated key or certificate cannot be written. The CA never replaces
  an existing key, certificate or state file silently.

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
| `500` | The record could not be written; no certificate is returned |

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
| `500` | The new state could not be written; the certificate stays as it was |

### `POST /ca/certificates/{cn}/reissue`

Un-revoke a previously revoked certificate.

**Response `204 No Content`**

| Status | Condition |
|---|---|
| `404` | CN not found or not currently revoked |
| `500` | The new state could not be written; the certificate stays revoked |

---

## PIP Endpoints (PORT, CA-as-PIP)

All PEP clients call these endpoints via `PIP_URL` environment variable.

### `GET /pip/health`

**Response `200 OK`**
```json
{"status": "ok", "system": "pip (CA-as-PIP)", "subjects": 12}
```

### `GET /pip/attributes/{cn}`

XACML-ready certificate validity attributes. topic-auth-xacml refuses `valid: false`
itself; pki-rest-authz and kafka-authz pass the value to the PDP, and the bundled
PDP ignores it.

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

The TLS listener uses `tls.RequireAndVerifyClientCert` (`main.go`). A request
without a client certificate, or with one not issued by this CA, fails in the
TLS handshake: the client gets a TLS alert and no HTTP status at all. The
endpoints are not served on the plain HTTP port (`404` there).

### `POST /ca/device-cert`

Issue a Device certificate (OU=de). Requires Onboarding (OU=on) client cert.

**Request / Response:** Same format as `/bootstrap/onboarding-cert`. Profile is `"de"`.

| Status | Condition |
|---|---|
| `400` | Invalid JSON |
| `403` | Client cert is not OU=on |
| `500` | The record could not be written; no certificate is returned |

### `POST /ca/system-cert`

Issue a System certificate (OU=sy). Requires Device (OU=de) client cert.

**Request / Response:** Same format as `/bootstrap/onboarding-cert`. Profile is `"sy"`.

| Status | Condition |
|---|---|
| `400` | Invalid JSON |
| `403` | Client cert is not OU=de |
| `500` | The record could not be written; no certificate is returned |
