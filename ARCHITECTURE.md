# ARCHITECTURE.md — Arrowhead-520-evol

## System Overview

This system implements the Arrowhead 5.2 evolved authorization stack (ADAPI):
a gRPC-based XACML PDP with certificate-based PKI, CA-as-PIP, and policy
enforcement across three transport types (Kafka, RabbitMQ AMQP, REST/mTLS).

The stack builds on the AH5 foundation systems (ServiceRegistry, Authentication,
ConsumerAuthorization) and extends them with XACML/ABAC authorization, profile-based
PKI certificate management, and multi-transport enforcement.

## Service Topology

```
┌─────────────────────────────────────────────────────────────────────┐
│                        ADAPI Core Stack                             │
│                                                                     │
│  ┌──────────────────┐    gRPC     ┌──────────────────┐              │
│  │ dynamicorch-xacml├───────────►│    authz-pdp      │              │
│  │     :8083         │            │     :9550         │              │
│  └────────┬─────────┘            └────────┬──────────┘              │
│           │ HTTP                           │ HTTP                    │
│           ▼                               ▼                         │
│  ┌──────────────────┐            ┌──────────────────┐              │
│  │ ServiceRegistry   │            │   AuthzForce      │              │
│  │     :8080         │            │     :8096         │              │
│  └──────────────────┘            └────────▲──────────┘              │
│                                           │ HTTP                    │
│                                  ┌────────┴──────────┐              │
│                                  │      PAP           │              │
│                                  │     :9505          │              │
│                                  └───────────────────┘              │
└─────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────┐
│                     PKI & Certificate Layer                         │
│                                                                     │
│  ┌──────────────────┐            ┌──────────────────┐              │
│  │   profile-ca      │   certs   │ cert-provisioner  │              │
│  │  :8787 (HTTP)     │◄──────────│   (runs once)     │              │
│  │  :8788 (mTLS)     │            └──────────────────┘              │
│  └────────┬──────────┘                                              │
│           │ /pip/attributes/{cn}                                    │
│           ▼                                                         │
│  ┌──────────────────────────────────────────────────┐              │
│  │              PEP Services (all query PIP)         │              │
│  │  ┌────────────┐  ┌──────────────┐  ┌──────────┐ │              │
│  │  │kafka-authz │  │topic-auth-   │  │pki-rest- │ │              │
│  │  │  :9101     │  │xacml  :9090  │  │authz     │ │              │
│  │  └─────┬──────┘  └──────┬───────┘  │:9208/9209│ │              │
│  │        │                │           └────┬─────┘ │              │
│  └────────┼────────────────┼────────────────┼───────┘              │
│           ▼                ▼                ▼                       │
│  ┌──────────────┐  ┌──────────────┐                                │
│  │    Kafka      │  │  RabbitMQ    │                                │
│  │    :9092      │  │  :5671       │                                │
│  │              │  │  Mgmt :15672 │                                │
│  └──────────────┘  └──────────────┘                                │
└─────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────┐
│                        Monitoring                                   │
│  ┌──────────────┐  ┌──────────────┐                                │
│  │  Dashboard    │  │   Kafdrop    │                                │
│  │   :3000       │  │   :9000      │                                │
│  └──────────────┘  └──────────────┘                                │
└─────────────────────────────────────────────────────────────────────┘
```

## Port Allocation

| Service | Internal Port | Host Port | Area |
|---|---|---|---|
| ServiceRegistry (TLS) | 8490 | 8490 | foundation |
| Authentication (TLS) | 8491 | 8491 | foundation |
| ConsumerAuth (TLS) | 8492 | 8492 | foundation |
| dynamicorch-xacml | 8083 | 8083 | core |
| authz-pdp (gRPC) | 9550 | 9550 | core |
| profile-ca (HTTP+PIP) | 8787 | 8787 | services |
| profile-ca (mTLS) | 8788 | 8788 | services |
| PAP | 9505 | 9505 | services |
| kafka-authz | 9101 | 9101 | services |
| topic-auth-xacml | 9090 | 9090 | services |
| pki-rest-authz (mTLS) | 9208 | 9208 | services |
| pki-rest-authz (HTTP) | 9209 | 9209 | services |
| AuthzForce | 8080 | 8096 | shared |
| RabbitMQ (AMQPS) | 5671 | 5671 | infra |
| RabbitMQ Management | 15672 | 15672 | infra |
| Kafka (TLS) | 9092 | 9092 | infra |
| Kafdrop | 9000 | 9000 | infra |
| Dashboard | 80 | 3000 | dashboard |

## Directory Tree

```
Arrowhead-520-evol/
├── core/                  ← ADAPI stack: authz-pdp + dynamicorch-xacml
├── foundation/            ← AH5 systems: ServiceRegistry, Authentication, ConsumerAuth
├── services/              ← Promoted services: profile-ca, pap, PEPs, cert-provisioner
├── shared/                ← Libraries: authzforce, authzforce-server, PEP bases, policy-sync
├── dashboard/             ← React+Vite monitoring frontend
├── deploy/                ← Docker Compose, Dockerfiles, broker configs
└── (root docs)            ← README.md, ARCHITECTURE.md, etc.
```

## Technology Choices

| Concern | Choice | Reason |
|---|---|---|
| Backend language | Go | Performance, single binary, strong typing |
| PDP protocol | gRPC (authorize.proto) | Low latency, type safety, reflection |
| Policy engine | AuthzForce CE (XACML 3.0) | Standards-based ABAC, mature |
| PKI | Profile-based X.509 (lo/on/de/sy) | Arrowhead 5.2 certificate hierarchy |
| PIP delivery | HTTP from CA (D1) | Zero replication lag, instant revocation |
| Message brokers | Kafka + RabbitMQ | Multi-transport enforcement validation |
| Frontend | React + Vite | Component reuse, fast dev cycle |
| Deployment | Docker Compose | Single-host, reproducible |
| Database | SQLite (in-memory or file) | Lightweight, no external DB service |

## Design Decisions

The key design decisions are summarized below.

| ID | Decision | Summary |
|---|---|---|
| D1 | CA-as-PIP | PIP merged into profile-ca for zero-lag cert queries |
| D2 | Single gRPC interface | Only authorize.proto; no certlifecycle |
| D3 | CADecider retained | AH5.2 compliance via ConsumerAuth fallback |
| D4 | Connection-time pre-gate | PEPs check cert validity before XACML |
| D5 | Foundation module | AH5 core as separate Go module |
