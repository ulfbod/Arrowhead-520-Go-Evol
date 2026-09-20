#!/usr/bin/env bash
# test-system.sh — Tiered regression test suite for Arrowhead-520-evol.
#
# Usage:
#   bash test-system.sh              # Full regression (levels 1-4, requires Docker stack)
#   bash test-system.sh --smoke      # Levels 1-2 only (no Docker, fast)
#   bash test-system.sh --run REGEX  # Full regression, but filter integration tests by name
#
# Tiered execution (cheapest first — never skip a level):
#   Level 1: go vet across all modules
#   Level 2: go test across all modules (unit tests, no Docker)
#   Level 3-4: Go integration test harness (tests/integration/, -tags=integration)
#              Covers: preflight, cert issuance, PIP, revocation, PAP, orchestration,
#              foundation services, external tools
#
# Prerequisites for levels 3-4:
#   docker compose -f deploy/docker-compose.yml up --build -d
#   (wait for all services healthy)
#
# Selective execution of integration tests (level 3-4 only):
#   cd tests/integration
#   go test -tags=integration -v -run TestRevocation ./...
#   go test -tags=integration -v -run TestPIP ./...
#   go test -tags=integration -v -run TestPreflight ./...
#
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

GREEN='\033[0;32m'; RED='\033[0;31m'; NC='\033[0m'
PASS=0; FAIL=0
pass() { PASS=$((PASS+1)); printf "${GREEN}  PASS${NC}  %s\n" "$1"; }
fail() { FAIL=$((FAIL+1)); printf "${RED}  FAIL${NC}  %s\n" "$1"; }

SMOKE_ONLY=false
RUN_FILTER=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --smoke) SMOKE_ONLY=true; shift ;;
    --run)   RUN_FILTER="$2"; shift 2 ;;
    *)       echo "Unknown option: $1"; exit 1 ;;
  esac
done

# All Go modules in the repo (excluding tests/integration which uses build tags).
MODULES=(
  core
  foundation
  services/profile-ca
  services/pap
  services/cert-provisioner
  services/kafka-authz
  services/topic-auth-xacml
  services/pki-rest-authz
  shared/authzforce
  shared/authzforce-server
  shared/kafka-authz
  shared/rest-authz
  shared/topic-auth-xacml
  shared/policy-sync
)

# ═══════════════════════════════════════════════════════════════════════════════
# Level 1: go vet
# ═══════════════════════════════════════════════════════════════════════════════
echo "=== Level 1: go vet ==="
VET_FAIL=0
for mod in "${MODULES[@]}"; do
  if [ -f "$mod/go.mod" ]; then
    if ! (cd "$mod" && go vet ./... 2>&1); then
      echo "  FAIL: go vet in $mod"
      VET_FAIL=1
    fi
  fi
done
[[ $VET_FAIL -eq 0 ]] && pass "go vet (all modules)" || { fail "go vet"; exit 2; }

# ═══════════════════════════════════════════════════════════════════════════════
# Level 2: Unit tests
# ═══════════════════════════════════════════════════════════════════════════════
echo ""
echo "=== Level 2: Unit tests ==="
TEST_FAIL=0
for mod in "${MODULES[@]}"; do
  if [ -f "$mod/go.mod" ]; then
    if ! (cd "$mod" && go test -count=1 ./... 2>&1); then
      echo "  FAIL: go test in $mod"
      TEST_FAIL=1
    fi
  fi
done
[[ $TEST_FAIL -eq 0 ]] && pass "go test (all modules)" || { fail "go test"; exit 2; }

if $SMOKE_ONLY; then
  echo ""
  echo "=== Smoke mode: levels 1-2 complete, skipping Docker integration ==="
  echo ""
  echo "─────────────────────────────────────"
  printf "${GREEN}  ALL PASS  %d checks${NC}\n" "$PASS"
  echo "─────────────────────────────────────"
  exit 0
fi

# ═══════════════════════════════════════════════════════════════════════════════
# Levels 3-4: Go integration test harness
#
# Uses tests/integration/ with -tags=integration. This provides:
#   - Proper test isolation (unique prefix per run)
#   - Structured output (go test -v shows subtests)
#   - Selective execution (-run TestRevocation)
#   - Typed JSON unmarshaling (no grep-based parsing)
#   - Per-test timeouts (10s default via http.Client)
#   - TestMain for setup validation
#
# Test files and what they cover:
#   preflight_test.go     — health checks, domain invariant, subject count, policies
#   cert_test.go          — certificate issuance (onboarding, infra, errors)
#   pip_test.go           — PIP queries (attributes, subjects, detail, status, 404)
#   revocation_test.go    — full lifecycle (issue→valid→revoke→invalid→reissue→valid)
#   pap_test.go           — policy CRUD
#   orchestration_test.go — DynamicOrch status, pull, mgmt endpoints
#   foundation_test.go    — SR register+query, Auth, ConsumerAuth (TLS)
#   external_test.go      — RabbitMQ Management, Kafdrop reachable
# ═══════════════════════════════════════════════════════════════════════════════
echo ""
echo "=== Levels 3-4: Integration tests (Go harness) ==="

RUN_ARG=""
if [[ -n "$RUN_FILTER" ]]; then
  RUN_ARG="-run $RUN_FILTER"
  echo "  Filter: $RUN_FILTER"
fi

if (cd tests/integration && go test -tags=integration -v -timeout 5m $RUN_ARG ./... 2>&1); then
  pass "integration tests"
else
  fail "integration tests"
fi

# ═══════════════════════════════════════════════════════════════════════════════
# Summary
# ═══════════════════════════════════════════════════════════════════════════════
echo ""
echo "─────────────────────────────────────"
if [[ $FAIL -eq 0 ]]; then
  printf "${GREEN}  ALL PASS  %d checks${NC}\n" "$PASS"
  echo "─────────────────────────────────────"
  exit 0
else
  printf "${RED}  %d FAILED / %d passed${NC}\n" "$FAIL" "$PASS"
  echo "─────────────────────────────────────"
  exit 1
fi
