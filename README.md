# Arrowhead-520-Go-Evol

Arrowhead 5.2 evolved authorization stack with a gRPC PDP interface that sends
XACML requests, profile-based PKI, CA-as-PIP, and multi-transport policy
enforcement. The bundled policy decision point is an AuthzForce-compatible
stand-in that matches grant pairs; see "Known limitations" in ARCHITECTURE.md.

## Quick start

```bash
cp .env.example .env
docker compose -f deploy/docker-compose.yml up --build
open http://localhost:3000    # Dashboard
open http://localhost:15672   # RabbitMQ Management (admin/admin)
open http://localhost:9000    # Kafdrop (Kafka UI)
```

## Services

| Service | Port | Description |
|---|---|---|
| dynamicorch-xacml | 8083 | XACML-backed orchestration |
| authz-pdp | 9550 | gRPC Authorization PDP |
| profile-ca | 8787/8788 | Certificate Authority + PIP |
| ServiceRegistry | 8490 | AH5 service discovery |
| Authentication | 8491 | AH5 identity management |
| ConsumerAuth | 8492 | AH5 consumer authorization |
| PAP | 9505 | Policy Administration Point |
| AuthzForce (stand-in) | 8896 | AuthzForce-compatible PDP/PAP stand-in: permits iff a (subject, resource) grant exists; evaluates no XACML policy |
| kafka-authz | 9101 | Kafka message PEP |
| topic-auth-xacml | 9090 | RabbitMQ connection PEP |
| pki-rest-authz | 9208/9209 | REST mTLS proxy PEP |
| Dashboard | 3000 | Monitoring UI |
| RabbitMQ Mgmt | 15672 | Broker admin UI |
| Kafdrop | 9000 | Kafka topic browser |

## Development

```bash
# Fast check without Docker (levels 1-2)
go vet ./...
go test ./...

# Full integration test (requires running stack)
bash test-system.sh
```

## Documentation

| File | What it covers |
|---|---|
| `ARCHITECTURE.md` | System diagram and port allocation |
| `core/SPEC.md` | ADAPI stack API contract |
| `foundation/SPEC.md` | AH5 API contract |
