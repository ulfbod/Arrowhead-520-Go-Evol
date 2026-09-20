#!/usr/bin/env bash
# test-lib.sh — shared assertion helpers for test-system.sh.
# Usage: source "$(dirname "$0")/test-lib.sh"

PASS=0; FAIL=0
GREEN='\033[0;32m'; RED='\033[0;31m'; YELLOW='\033[0;33m'; NC='\033[0m'

pass()  { PASS=$((PASS+1)); printf "${GREEN}  PASS${NC}  %s\n" "$1"; }
fail()  { FAIL=$((FAIL+1)); printf "${RED}  FAIL${NC}  %s\n" "$1"; }
red()   { printf "${RED}%s${NC}\n" "$1"; }

check_eq() {
  local label="$1" got="$2" want="$3"
  if [[ "$got" == "$want" ]]; then pass "$label"
  else fail "$label"; echo "       got:  $got"; echo "       want: $want"; fi
}

http_code() { curl -s -o /dev/null -w "%{http_code}" "$@"; }
http_body() { curl -s "$@"; }

smoke_fail() { red "  SMOKE FAIL  $1"; exit 2; }

smoke_http() {
  local label="$1" url="$2" want="${3:-200}"
  local code; code=$(http_code "$url")
  [[ "$code" == "$want" ]] || smoke_fail "$label: expected $want, got $code from $url"
  printf "${GREEN}  SMOKE OK${NC}  %s\n" "$label"
}

assert_http()     { local label="$1" want="$2"; shift 2; check_eq "$label" "$(http_code "$@")" "$want"; }
assert_contains() { local l="$1" b="$2" w="$3"; [[ "$b" == *"$w"* ]] && pass "$l" || { fail "$l"; echo "       want substring: $w"; }; }
assert_not_contains() { local l="$1" b="$2" w="$3"; [[ "$b" != *"$w"* ]] && pass "$l" || { fail "$l"; echo "       unexpected: $w"; }; }
assert_json_field()  { local l="$1" b="$2" f="$3"; [[ "$b" == *"\"$f\""* ]] && pass "$l" || { fail "$l"; echo "       missing field: $f"; }; }
assert_json_value() {
  local label="$1" body="$2" key="$3" want="$4"
  local got; got=$(echo "$body" | grep -o "\"$key\":[^,}]*" | head -1 | sed 's/.*: *//;s/[",]//g')
  check_eq "$label" "$got" "$want"
}

summary() {
  echo ""
  echo "─────────────────────────────────────"
  [[ $FAIL -eq 0 ]] \
    && printf "${GREEN}  ALL PASS  %d tests${NC}\n" "$PASS" \
    || printf "${RED}  %d FAILED / %d passed${NC}\n" "$FAIL" "$PASS"
  echo "─────────────────────────────────────"
  [[ $FAIL -eq 0 ]]
}
