#!/bin/sh
# seed-policies.sh -- Seeds basic XACML policies into PAP for the Arrowhead 5.2.0
# local cloud deployment.
#
# This script is run by the setup container after PAP and profile-ca are healthy.
# It creates generic orchestration and service-level enforcement policies.

set -e

PAP_URL="${PAP_URL:-http://pap:9505}"
AUTH_URL="${AUTH_URL:-http://authentication:8081}"

echo "[setup] Seeding XACML policies via PAP at ${PAP_URL}..."

# Helper: POST a policy and verify it was created
post_policy() {
  resp=$(curl -s -X POST "${PAP_URL}/policies" -H 'Content-Type: application/json' -d "$1")
  if echo "$resp" | grep -q '"id"'; then
    echo "[setup]   OK: $1"
  else
    echo "[setup]   WARN: unexpected response for $1: $resp"
  fi
}

# -- Orchestration policies (action=orchestrate) --
# Allow test-probe to orchestrate any service (for system tests)
post_policy '{"subject":"test-probe","resource":"telemetry","action":"orchestrate","effect":"Permit"}'
post_policy '{"subject":"test-probe","resource":"telemetry-rest","action":"orchestrate","effect":"Permit"}'

# -- Service-level enforcement policies --
# Allow test-probe to consume and invoke services
post_policy '{"subject":"test-probe","resource":"telemetry","action":"consume","effect":"Permit"}'
post_policy '{"subject":"test-probe","resource":"telemetry-rest","action":"consume","effect":"Permit"}'
post_policy '{"subject":"test-probe","resource":"telemetry-rest","action":"invoke","effect":"Permit"}'

echo "[setup] Policy seeding complete."
