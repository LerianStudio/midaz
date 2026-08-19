#!/usr/bin/env bash
# Copyright (c) 2026 Lerian Studio. All rights reserved.
# Use of this source code is governed by the Elastic License 2.0
# that can be found in the LICENSE file.

set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
ledger_env=${LEDGER_ENV_FILE:-$repo_root/components/ledger/.env}
tracer_env=${TRACER_ENV_FILE:-$repo_root/components/tracer/.env}

fail() {
  echo "required Tracer E2E auth configuration failed: $*" >&2
  exit 1
}

read_env_value() {
  local env_file=$1
  local key=$2
  local line value= count=0

  [[ -f $env_file ]] || fail "missing environment file"
  while IFS= read -r line || [[ -n $line ]]; do
    case $line in
      "$key="*)
        value=${line#*=}
        count=$((count + 1))
        ;;
    esac
  done < "$env_file"

  [[ $count -eq 1 ]] || fail "$key must occur exactly once"
  printf '%s' "$value"
}

replace_env_value() {
  local env_file=$1
  local key=$2
  local value=$3
  local line tmp count=0

  [[ -f $env_file ]] || fail "missing environment file"
  umask 077
  tmp=$(mktemp "${env_file}.tmp.XXXXXX")

  while IFS= read -r line || [[ -n $line ]]; do
    case $line in
      "$key="*)
        printf '%s=%s\n' "$key" "$value"
        count=$((count + 1))
        ;;
      *)
        printf '%s\n' "$line"
        ;;
    esac
  done < "$env_file" > "$tmp"

  if [[ $count -ne 1 ]]; then
    rm -f "$tmp"
    fail "$key must occur exactly once"
  fi

  mv "$tmp" "$env_file"
}

verify_auth_configuration() {
  local ledger_key tracer_key tracer_auth_only_validation tracer_auth_enabled transport

  transport=$(read_env_value "$ledger_env" TRACER_TRANSPORT)
  [[ $transport == rest ]] || fail "required E2E must use TRACER_TRANSPORT=rest"

  ledger_key=$(read_env_value "$ledger_env" TRACER_API_KEY)
  tracer_key=$(read_env_value "$tracer_env" API_KEY)
  [[ -n $ledger_key && -n $tracer_key ]] || fail "Ledger and Tracer API keys must be non-empty"
  [[ $ledger_key == "$tracer_key" ]] || fail "Ledger and Tracer API keys must match"
  [[ $ledger_key =~ ^[0-9a-f]{64}$ ]] || fail "the shared API key must contain 256 bits encoded as lowercase hex"

  tracer_auth_enabled=$(read_env_value "$tracer_env" API_KEY_ENABLED)
  [[ $tracer_auth_enabled == true ]] || fail "Tracer API-key authentication must be enabled"
  tracer_auth_only_validation=$(read_env_value "$tracer_env" API_KEY_ENABLED_ONLY_VALIDATION)
  [[ $tracer_auth_only_validation == false ]] || fail "Tracer API-key authentication must protect reservation routes"

  unset ledger_key tracer_key
}

case ${1:-} in
  "")
    command -v openssl >/dev/null 2>&1 || fail "openssl is required to generate the ephemeral key"
    shared_key=$(openssl rand -hex 32)
    [[ $shared_key =~ ^[0-9a-f]{64}$ ]] || fail "openssl returned an invalid key"

    replace_env_value "$ledger_env" TRACER_API_KEY "$shared_key"
    replace_env_value "$tracer_env" API_KEY "$shared_key"
    replace_env_value "$tracer_env" API_KEY_ENABLED true
    unset shared_key

    verify_auth_configuration
    echo "required Tracer E2E auth configured with one ephemeral shared key"
    ;;
  --verify)
    verify_auth_configuration
    echo "required Tracer E2E auth configuration verified"
    ;;
  --exec)
    shift
    [[ $# -gt 0 ]] || fail "--exec requires a command"
    verify_auth_configuration
    E2E_TRACER_API_KEY=$(read_env_value "$tracer_env" API_KEY)
    export E2E_TRACER_API_KEY
    exec "$@"
    ;;
  *)
    fail "usage: configure-required-tracer-e2e-auth.sh [--verify | --exec command ...]"
    ;;
esac
