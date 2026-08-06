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
# Env: DOCKERHUB_USERNAME, DOCKERHUB_TOKEN (Docker Hub PAT), optional DOCKERHUB_NAMESPACE
# and RELEASE_WORKFLOW.

set -euo pipefail

API="https://hub.docker.com/v2"
NAMESPACE="${DOCKERHUB_NAMESPACE:-lerianstudio}"
RELEASE_WORKFLOW="${RELEASE_WORKFLOW:-.github/workflows/release.yml}"
ATTEMPTS=3

: "${DOCKERHUB_USERNAME:?DOCKERHUB_USERNAME is required}"
: "${DOCKERHUB_TOKEN:?DOCKERHUB_TOKEN is required}"

BODY=$(mktemp)
trap 'rm -f "$BODY"' EXIT

# hub_call <method> <url> [json-payload]
# Writes the response body to $BODY, prints the HTTP status, and retries transport
# errors and 5xx so a flaky Docker Hub cannot fail a release on its own.
hub_call() {
  local method="$1" url="$2" payload="${3:-}" status="" attempt=1

  while [ "$attempt" -le "$ATTEMPTS" ]; do
    if [ -n "$payload" ]; then
      status=$(curl -sS -o "$BODY" -w '%{http_code}' -X "$method" \
        -H "Authorization: Bearer ${TOKEN}" \
        -H 'Content-Type: application/json' \
        -d "$payload" "$url" </dev/null || echo "000")
    else
      status=$(curl -sS -o "$BODY" -w '%{http_code}' -X "$method" \
        -H "Authorization: Bearer ${TOKEN}" "$url" </dev/null || echo "000")
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

TOKEN=$(jq -n --arg u "$DOCKERHUB_USERNAME" --arg p "$DOCKERHUB_TOKEN" '{username: $u, password: $p}' |
  curl -sS -X POST -H 'Content-Type: application/json' -d @- "${API}/users/login/" |
  jq -r '.token // empty')

if [ -z "$TOKEN" ]; then
  echo "error: Docker Hub login failed for ${DOCKERHUB_USERNAME}" >&2
  exit 1
fi

failed=0

while read -r image; do
  [ -n "$image" ] || continue

  repo="${NAMESPACE}/${image}"
  status=$(hub_call GET "${API}/repositories/${repo}/")

  case "$status" in
    200)
      if [ "$(jq -r '.is_private' "$BODY")" = "true" ]; then
        status=$(hub_call PATCH "${API}/repositories/${repo}/" '{"is_private": false}')
        if [ "$status" = "200" ]; then
          echo "${repo}: was private, now public"
        else
          echo "error: ${repo}: could not make public (HTTP ${status})" >&2
          failed=1
        fi
      else
        echo "${repo}: already public"
      fi
      ;;
    404)
      payload=$(jq -n --arg ns "$NAMESPACE" --arg name "$image" \
        '{namespace: $ns, name: $name, is_private: false}')
      status=$(hub_call POST "${API}/repositories/" "$payload")
      if [ "$status" = "201" ]; then
        echo "${repo}: created as public"
      else
        echo "error: ${repo}: could not create (HTTP ${status})" >&2
        failed=1
      fi
      ;;
    *)
      echo "error: ${repo}: unexpected response (HTTP ${status})" >&2
      failed=1
      ;;
  esac
done <<<"$images"

exit "$failed"
