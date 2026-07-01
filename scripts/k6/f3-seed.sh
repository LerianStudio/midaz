#!/usr/bin/env bash
# F3-T20 seed: provisions the org/ledger/asset/accounts the k6 latency proof
# drives, and writes scripts/k6/f3-seed.json for the k6 script to read.
#
# It seeds TWO ledgers under one org:
#   - L_off:     tracer.mode=off              (k6 leg A baseline)
#   - L_enforce: tracer.mode=enforce, failPosture=open, timeoutMs=250
#                (k6 leg B — configured from birth so the 5-min settings cache
#                 TTL never lags the posture flip, R37)
#
# Each ledger gets its own A/B accounts (alias suffixed per ledger), default
# balances enabled, and A funded with a large inflow so the latency legs never
# hit an insufficient-funds reject (we measure the reserve seam, not denials).
set -euo pipefail

BASE="${LEDGER_BASE_URL:-http://localhost:3002}"
AUTH="${TEST_AUTH_HEADER:-Bearer test}"
OUT="${1:-scripts/k6/f3-seed.json}"
FUND="${FUND_AMOUNT:-100000000.00}"

hdr=(-H "Content-Type: application/json" -H "Authorization: ${AUTH}")

req() { # method path [body]
  local m="$1" p="$2" b="${3:-}"
  if [ -n "$b" ]; then
    curl -sS -X "$m" "${hdr[@]}" -H "X-Request-Id: $(uuidgen)" -d "$b" "${BASE}${p}"
  else
    curl -sS -X "$m" "${hdr[@]}" -H "X-Request-Id: $(uuidgen)" "${BASE}${p}"
  fi
}

jget() { python3 -c 'import sys,json;print(json.load(sys.stdin)["'"$1"'"])'; }

echo "[seed] creating organization..."
ORG=$(req POST /v1/organizations \
  '{"legalName":"F3T20 Org","legalDocument":"'"$(date +%s)$RANDOM"'","address":{"country":"US"}}' | jget id)
echo "[seed] org=$ORG"

seed_ledger() { # name mode failPosture -> echoes "<ledgerID> <aliasA> <aliasB>"
  local lname="$1" mode="$2" fp="$3"
  local LID aliasA aliasB defA defB
  LID=$(req POST "/v1/organizations/${ORG}/ledgers" '{"name":"'"$lname"'"}' | jget id)

  # Configure tracer settings BEFORE any transaction so the first settings read
  # is a cache miss that loads the intended posture from the DB.
  if [ "$mode" != "off" ]; then
    req PATCH "/v1/organizations/${ORG}/ledgers/${LID}/settings" \
      '{"tracer":{"mode":"'"$mode"'","failPosture":"'"$fp"'","timeoutMs":250}}' >/dev/null
  else
    req PATCH "/v1/organizations/${ORG}/ledgers/${LID}/settings" \
      '{"tracer":{"mode":"off"}}' >/dev/null
  fi

  req POST "/v1/organizations/${ORG}/ledgers/${LID}/assets" \
    '{"name":"US Dollar","type":"currency","code":"USD"}' >/dev/null

  aliasA="A-${lname}"
  aliasB="B-${lname}"
  req POST "/v1/organizations/${ORG}/ledgers/${LID}/accounts" \
    '{"name":"A","assetCode":"USD","type":"deposit","alias":"'"$aliasA"'"}' >/dev/null
  req POST "/v1/organizations/${ORG}/ledgers/${LID}/accounts" \
    '{"name":"B","assetCode":"USD","type":"deposit","alias":"'"$aliasB"'"}' >/dev/null

  # Wait for default balances and enable send/receive on both.
  for alias in "$aliasA" "$aliasB"; do
    local did="" tries=0
    while [ -z "$did" ] && [ $tries -lt 120 ]; do
      did=$(req GET "/v1/organizations/${ORG}/ledgers/${LID}/accounts/alias/${alias}/balances" \
        | python3 -c 'import sys,json
d=json.load(sys.stdin)
print(next((i["id"] for i in d.get("items",[]) if i.get("key")=="default"),""))' 2>/dev/null || true)
      [ -z "$did" ] && { sleep 0.3; tries=$((tries+1)); }
    done
    [ -z "$did" ] && { echo "[seed] FATAL: default balance for $alias never appeared" >&2; exit 1; }
    req PATCH "/v1/organizations/${ORG}/ledgers/${LID}/balances/${did}" \
      '{"allowSending":true,"allowReceiving":true}' >/dev/null
  done

  # Fund A so the latency legs never reject for funds.
  req POST "/v1/organizations/${ORG}/ledgers/${LID}/transactions/inflow" \
    '{"send":{"asset":"USD","value":"'"$FUND"'","distribute":{"to":[{"accountAlias":"'"$aliasA"'","amount":{"asset":"USD","value":"'"$FUND"'"}}]}}}' >/dev/null

  echo "$LID $aliasA $aliasB"
}

echo "[seed] ledger L_off (tracer.mode=off)..."
read -r LOFF AOFF BOFF < <(seed_ledger "off" off open)
echo "[seed] L_off=$LOFF"

echo "[seed] ledger L_enforce (tracer.mode=enforce, failPosture=open)..."
read -r LENF AENF BENF < <(seed_ledger "enforce" enforce open)
echo "[seed] L_enforce=$LENF"

cat > "$OUT" <<JSON
{
  "base": "${BASE}",
  "auth": "${AUTH}",
  "org": "${ORG}",
  "off":     { "ledger": "${LOFF}", "from": "${AOFF}", "to": "${BOFF}" },
  "enforce": { "ledger": "${LENF}", "from": "${AENF}", "to": "${BENF}" }
}
JSON
echo "[seed] wrote $OUT"
cat "$OUT"
