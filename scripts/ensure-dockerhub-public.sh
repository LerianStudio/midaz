#!/bin/bash

# Copyright (c) 2026 Lerian Studio. All rights reserved.
# Use of this source code is governed by the Elastic License 2.0
# that can be found in the LICENSE file.

# Ensure every image the release pipeline publishes exists on Docker Hub and is public.
#
# Docker Hub creates a repository on first push using the organization's default
# visibility, which is private. Nothing in the release pipeline flips it, so each new
# image ships unpullable until someone changes it by hand: midaz-tracer,
# midaz-tracer-migrations and midaz-ledger-migrations all answer denied/unauthorized
# today, which breaks any anonymous `helm install` of the midaz chart.
#
# This script is idempotent: it pre-creates missing repositories as public and flips
# existing private ones. Run it before the images are pushed so a first release never
# lands private.
#
# Changing visibility is an administrative operation: DOCKERHUB_TOKEN must carry
# repo:admin (an organization access token, or an owner's credentials). A push token is
# enough to read visibility but answers 403 on the write, which this script reports as
# such instead of leaving the image quietly private.
#
# Env: DOCKERHUB_USERNAME, DOCKERHUB_TOKEN, optional DOCKERHUB_NAMESPACE and
# RELEASE_WORKFLOW.

set -euo pipefail

API="https://hub.docker.com/v2"
NAMESPACE="${DOCKERHUB_NAMESPACE:-lerianstudio}"
RELEASE_WORKFLOW="${RELEASE_WORKFLOW:-.github/workflows/release.yml}"
ATTEMPTS=3

: "${DOCKERHUB_USERNAME:?DOCKERHUB_USERNAME is required}"
: "${DOCKERHUB_TOKEN:?DOCKERHUB_TOKEN is required}"

BODY=$(mktemp)
PAYLOAD_FILE=$(mktemp)
trap 'rm -f "$BODY" "$PAYLOAD_FILE"' EXIT

# hub_call <method> <url> [json-payload]
# Writes the response body to $BODY, prints the HTTP status, and retries transport
# errors and 5xx so a flaky Docker Hub cannot fail a release on its own. The payload
# goes through a file so secrets (the login call) never appear on curl's argv, and
# curl's exit code is captured separately: on a transport error curl already prints
# "000" for %{http_code}, so appending a fallback would produce "000000" and dodge
# the retry branch.
hub_call() {
  local method="$1" url="$2" payload="${3:-}" status="" attempt=1
  local args=(-sS -o "$BODY" -w '%{http_code}' --connect-timeout 10 --max-time 60 -X "$method")

  if [ -n "${TOKEN:-}" ]; then
    args+=(-H "Authorization: Bearer ${TOKEN}")
  fi
  if [ -n "$payload" ]; then
    printf '%s' "$payload" >"$PAYLOAD_FILE"
    args+=(-H 'Content-Type: application/json' -d "@${PAYLOAD_FILE}")
  fi

  while [ "$attempt" -le "$ATTEMPTS" ]; do
    if ! status=$(curl "${args[@]}" "$url" </dev/null); then
      status="000"
    fi

    case "$status" in
      000|5??) attempt=$((attempt + 1)); sleep $((attempt * 2)) ;;
      *) break ;;
    esac
  done

  echo "$status"
}

# The GitOps mapping is the release pipeline's own registry of published images (one
# "<image>.tag" key per image), so deriving the list from it cannot drift from what the
# pipeline actually pushes. It is a single-line, single-quoted JSON scalar; bail out
# rather than guess if that ever stops holding.
mappings=$(sed -n "s/^[[:space:]]*gitops_yaml_key_mappings:[[:space:]]*'\(.*\)'[[:space:]]*$/\1/p" "$RELEASE_WORKFLOW")
images=$(printf '%s' "$mappings" | jq -er 'keys[] | sub("\\.tag$"; "")' 2>/dev/null | sort -u || true)

if [ -z "$images" ]; then
  echo "error: could not read gitops_yaml_key_mappings from ${RELEASE_WORKFLOW}" >&2
  exit 1
fi

# Login goes through hub_call so it gets the same retry and timeout policy as every
# other request: a single 5xx or a stalled connection at login must not fail a release.
login_payload=$(jq -n --arg u "$DOCKERHUB_USERNAME" --arg p "$DOCKERHUB_TOKEN" '{username: $u, password: $p}')
status=$(hub_call POST "${API}/users/login/" "$login_payload")
TOKEN=$(jq -r '.token // empty' "$BODY" 2>/dev/null || true)

if [ "$status" != "200" ] || [ -z "$TOKEN" ]; then
  echo "error: Docker Hub login failed for ${DOCKERHUB_USERNAME} (HTTP ${status})" >&2
  exit 1
fi

failed=0
unresolved=""

# fail_repo <repo> <message>
# Records a repository this run could not leave public, so the job summary names the
# work left to do instead of burying it in the step log.
fail_repo() {
  echo "error: $1: $2" >&2
  unresolved="${unresolved}- \`$1\`: $2"$'\n'
  failed=1
}

while read -r image; do
  [ -n "$image" ] || continue

  repo="${NAMESPACE}/${image}"
  status=$(hub_call GET "${API}/repositories/${repo}/")

  case "$status" in
    200)
      # Only trust an explicit boolean. A missing field or a malformed body means the
      # visibility was never confirmed, and this script's job is verification, so that
      # must fail rather than pass as "already public". (Not `.is_private // "unknown"`:
      # jq's // treats false as empty, which would flag every public repo as unknown.)
      is_private=$(jq -r '.is_private | if type == "boolean" then tostring else "unknown" end' "$BODY" 2>/dev/null || echo "unknown")
      case "$is_private" in
        true)
          status=$(hub_call PATCH "${API}/repositories/${repo}/" '{"is_private": false}')
          if [ "$status" = "200" ]; then
            echo "${repo}: was private, now public"
          elif [ "$status" = "403" ]; then
            # Reading visibility only needs a push token; changing it does not. This is
            # the credential telling us so, not a transient failure.
            fail_repo "$repo" "still private: the Docker Hub token lacks repo:admin (HTTP 403)"
          else
            fail_repo "$repo" "could not make public (HTTP ${status})"
          fi
          ;;
        false)
          echo "${repo}: already public"
          ;;
        *)
          fail_repo "$repo" "response did not report is_private"
          ;;
      esac
      ;;
    404)
      payload=$(jq -n --arg ns "$NAMESPACE" --arg name "$image" \
        '{namespace: $ns, name: $name, is_private: false}')
      status=$(hub_call POST "${API}/repositories/" "$payload")
      if [ "$status" = "201" ]; then
        echo "${repo}: created as public"
      elif [ "$status" = "403" ]; then
        fail_repo "$repo" "does not exist and the Docker Hub token lacks repo:admin to create it (HTTP 403)"
      else
        fail_repo "$repo" "could not create (HTTP ${status})"
      fi
      ;;
    *)
      fail_repo "$repo" "unexpected response (HTTP ${status})"
      ;;
  esac
done <<<"$images"

if [ -n "$unresolved" ] && [ -n "${GITHUB_STEP_SUMMARY:-}" ]; then
  {
    echo "### Images still not public"
    echo
    printf '%s' "$unresolved"
    echo
    echo "Anonymous \`docker pull\` and \`helm install\` of the midaz chart fail while this holds."
  } >>"$GITHUB_STEP_SUMMARY"
fi

exit "$failed"
