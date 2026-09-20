# core/proto — ADAPI gRPC Interface

This directory contains the canonical gRPC interface specification for the
ADAPI authorization stack. The single interface — `authorize.proto` — is the
source of truth, with pre-generated Go bindings and a human README.

Conventions:
- Inline XACML-aligned comments on every field and RPC
- gRPC server reflection enabled on every server implementation
- `make gen` (Docker-based) to regenerate Go bindings
- No external proto dependencies (no `google/protobuf/` imports)

---

## Interface

| Interface | Server | Clients | Port |
|---|---|---|---|
| [`authorize/`](authorize/) | `authz-pdp` | `dynamicorch-xacml` | :9550 |

---

## authorize.proto

PEP → PDP authorization decision interface.

```
DynamicOrch-XACML  ──gRPC──▶  authz-pdp  ──HTTP/XML──▶  AuthzForce
```
