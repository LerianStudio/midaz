#!/bin/bash

# Copyright (c) 2026 Lerian Studio. All rights reserved.
# Use of this source code is governed by the Elastic License 2.0
# that can be found in the LICENSE file.

set -euo pipefail

# OpenAPI documentation guardrail.
#
# (a) PARITY CHECK (always): asserts that the shared metadata blocks of the three
#     HTTP components' swagger.json are identical, so the published specs do not
#     drift in contact/license/terms/schemes/version/title.
# (b) DRIFT CHECK (CHECK_DOCS_REGEN=1 only): regenerates the docs and asserts the
#     committed artifacts still reproduce, so the source annotations and the
#     committed specs cannot silently diverge.
# (c) SECURITY COVERAGE (always, ledger only): asserts every ledger operation
#     carries a .security requirement, so the secure-by-default contract cannot
#     regress to a dangling securityDefinition (audit finding C1). Scoped to
#     ledger: tracer's /health, /readyz and /version are intentionally public,
#     and reporter already secures all its operations.

# Root directory of the repo (this script lives in postman/generator/)
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

GENERATOR_DIR="${ROOT_DIR}/postman/generator"

# Components whose swagger.json must agree on shared metadata.
PARITY_COMPONENTS=("ledger" "tracer" "reporter")

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

print_header() {
    echo ""
    echo -e "${BLUE}=================================================${NC}"
    echo -e "${BLUE}  🔍 $1${NC}"
    echo -e "${BLUE}=================================================${NC}"
    echo ""
}

fail() {
    echo -e "    ${RED}❌ $1${NC}"
    exit 1
}

ok() {
    echo -e "    ${GREEN}✅ $1${NC}"
}

require_jq() {
    if ! command -v jq >/dev/null 2>&1; then
        fail "jq is required but was not found on PATH."
    fi
}

# Read a swagger.json field as canonical JSON (sorted keys) for byte comparison.
read_field() {
    local component="$1"
    local jq_filter="$2"
    local file="${ROOT_DIR}/components/${component}/api/swagger.json"

    if [ ! -f "${file}" ]; then
        fail "Missing swagger.json for component '${component}' at ${file}. Run 'make generate-docs' first."
    fi

    jq -cS "${jq_filter}" "${file}"
}

# Read a swagger.json scalar field as a raw (unquoted) string for regex matching.
read_field_raw() {
    local component="$1"
    local jq_filter="$2"
    local file="${ROOT_DIR}/components/${component}/api/swagger.json"

    if [ ! -f "${file}" ]; then
        fail "Missing swagger.json for component '${component}' at ${file}. Run 'make generate-docs' first."
    fi

    jq -r "${jq_filter}" "${file}"
}

# Assert a field is byte-identical across all parity components.
assert_field_parity() {
    local field_label="$1"
    local jq_filter="$2"

    local reference_component="${PARITY_COMPONENTS[0]}"
    local reference_value
    reference_value="$(read_field "${reference_component}" "${jq_filter}")"

    local component value
    for component in "${PARITY_COMPONENTS[@]:1}"; do
        value="$(read_field "${component}" "${jq_filter}")"
        if [ "${value}" != "${reference_value}" ]; then
            echo -e "    ${RED}❌ Field '${field_label}' diverged between '${reference_component}' and '${component}':${NC}"
            echo -e "       ${reference_component}: ${reference_value}"
            echo -e "       ${component}: ${value}"
            exit 1
        fi
    done

    ok "Field '${field_label}' is identical across: ${PARITY_COMPONENTS[*]}"
}

# Assert a field matches a regex in every parity component.
assert_field_matches() {
    local field_label="$1"
    local jq_filter="$2"
    local regex="$3"

    local component value
    for component in "${PARITY_COMPONENTS[@]}"; do
        value="$(read_field_raw "${component}" "${jq_filter}")"
        if ! [[ "${value}" =~ ${regex} ]]; then
            fail "Field '${field_label}' in component '${component}' is '${value}', expected to match /${regex}/."
        fi
    done

    ok "Field '${field_label}' matches /${regex}/ across: ${PARITY_COMPONENTS[*]}"
}

parity_check() {
    print_header "Parity check (swagger.json shared metadata)"

    assert_field_parity "info.contact" '.info.contact'
    assert_field_parity "info.license" '.info.license'
    assert_field_parity "info.termsOfService" '.info.termsOfService'
    assert_field_parity "schemes" '.schemes'
    assert_field_matches "info.version" '.info.version' '^4\.0\.0$'
    assert_field_matches "info.title" '.info.title' '^Midaz'
}

# Component whose every operation must declare a .security requirement.
SECURITY_COVERAGE_COMPONENT="ledger"

# Assert every operation in the ledger spec carries a non-empty .security block.
security_coverage_check() {
    print_header "Security coverage check (${SECURITY_COVERAGE_COMPONENT}: every operation secured)"

    local file="${ROOT_DIR}/components/${SECURITY_COVERAGE_COMPONENT}/api/swagger.json"
    if [ ! -f "${file}" ]; then
        fail "Missing swagger.json for component '${SECURITY_COVERAGE_COMPONENT}' at ${file}. Run 'make generate-docs' first."
    fi

    # Operations are the HTTP-verb keys under each path; an operation is unsecured
    # when its .security array is absent or empty.
    local op_filter='.paths | to_entries[] | .key as $path | .value | to_entries[]
        | select(.key | test("^(get|post|put|patch|delete|head|options)$"))
        | { path: $path, method: .key, security: (.value.security // []) }'

    local total secured
    total="$(jq "[ ${op_filter} ] | length" "${file}")"
    secured="$(jq "[ ${op_filter} | select(.security | length > 0) ] | length" "${file}")"

    if [ "${secured}" != "${total}" ]; then
        echo -e "    ${RED}❌ ${SECURITY_COVERAGE_COMPONENT} has unsecured operations (${secured}/${total} secured):${NC}"
        jq -r "${op_filter} | select(.security | length == 0) | \"       \(.method | ascii_upcase) \(.path)\"" "${file}"
        echo -e "    ${RED}Every ledger operation must declare '@Security BearerAuth' (audit finding C1).${NC}"
        exit 1
    fi

    ok "All ${total} ${SECURITY_COVERAGE_COMPONENT} operations declare a .security requirement."
}

drift_check() {
    print_header "Drift check (regenerate and diff)"

    echo -e "    ${BLUE}⏳ Regenerating docs via generate-docs.sh...${NC}"
    if ! "${GENERATOR_DIR}/generate-docs.sh"; then
        fail "generate-docs.sh failed; cannot verify regeneration reproduces committed artifacts."
    fi

    if ! git -C "${ROOT_DIR}" diff --exit-code -- 'components/*/api' postman/specs; then
        echo ""
        echo -e "    ${RED}❌ Regeneration changed committed docs artifacts. Changed paths:${NC}"
        git -C "${ROOT_DIR}" diff --name-only -- 'components/*/api' postman/specs | sed 's/^/       /'
        echo -e "    ${RED}Run 'make generate-docs' and commit the result.${NC}"
        exit 1
    fi

    ok "Regeneration reproduces committed docs artifacts (no drift)."
}

main() {
    require_jq
    parity_check
    security_coverage_check

    if [ "${CHECK_DOCS_REGEN:-}" = "1" ]; then
        drift_check
    else
        echo ""
        echo -e "    ${BLUE}ℹ️  Drift check skipped (set CHECK_DOCS_REGEN=1 to enable).${NC}"
    fi

    echo ""
    echo -e "${GREEN}🎉 Documentation guardrail checks passed!${NC}"
    echo ""
}

main "$@"
