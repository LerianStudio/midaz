#!/bin/bash

# Copyright (c) 2026 Lerian Studio. All rights reserved.
# Use of this source code is governed by the Elastic License 2.0
# that can be found in the LICENSE file.

set -euo pipefail

# OpenAPI documentation guardrail.
#
# (a) PARITY CHECK (always): asserts the two HTTP components' native Huma OAS 3.1
#     dumps (openapi.huma.yaml) agree on .info.version and carry the ^4.0.0$
#     release, so the published specs do not drift on the metadata the Huma dump
#     actually emits. (Huma emits only .info.title + .info.version; it does not
#     populate contact/license/termsOfService, and OAS 3.1 has no .schemes —
#     those swaggo-era parity fields are dropped honestly. See parity_check for
#     why .info.title is no longer asserted.)
# (b) DRIFT CHECK (CHECK_DOCS_REGEN=1 only): regenerates the docs and asserts the
#     committed artifacts still reproduce, so the source annotations and the
#     committed specs cannot silently diverge.
# (c) SECURITY COVERAGE (always, ledger only): asserts every ledger operation
#     carries a .security requirement, so the secure-by-default contract cannot
#     regress to a dangling securityDefinition (audit finding C1). Scoped to
#     ledger: tracer's /health, /readyz and /version are intentionally public.

# Root directory of the repo (this script lives in postman/generator/)
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

GENERATOR_DIR="${ROOT_DIR}/postman/generator"

# Components whose Huma OAS 3.1 dumps must agree on shared metadata.
PARITY_COMPONENTS=("ledger" "tracer")

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

# Emit a component's Huma OAS 3.1 dump as JSON on stdout. jq cannot read YAML,
# so we convert via the same bundled js-yaml the generator uses for its JSON twin.
huma_dump_json() {
    local component="$1"
    local file="${ROOT_DIR}/components/${component}/api/openapi.huma.yaml"

    if [ ! -f "${file}" ]; then
        fail "Missing openapi.huma.yaml for component '${component}' at ${file}. Run 'make generate-docs' first."
    fi

    NODE_PATH="${GENERATOR_DIR}/node_modules" node -e '
        const yaml = require("js-yaml");
        const fs = require("fs");
        process.stdout.write(JSON.stringify(yaml.load(fs.readFileSync(process.argv[1], "utf8"))));
    ' "${file}"
}

# Read a Huma dump field as canonical JSON (sorted keys) for byte comparison.
read_field() {
    local component="$1"
    local jq_filter="$2"

    huma_dump_json "${component}" | jq -cS "${jq_filter}"
}

# Read a Huma dump scalar field as a raw (unquoted) string for regex matching.
read_field_raw() {
    local component="$1"
    local jq_filter="$2"

    huma_dump_json "${component}" | jq -r "${jq_filter}"
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
    print_header "Parity check (Huma dump shared metadata)"

    # .info.version is the one metadata field both planes emit AND must agree on
    # (a joined spec with mismatched versions is nonsense). Byte-identical parity
    # plus the ^4.0.0$ shape covers both "they agree" and "they are the release".
    assert_field_parity "info.version" '.info.version'
    assert_field_matches "info.version" '.info.version' '^4\.0\.0$'

    # ponytail: dropped contact/license/termsOfService/schemes (swaggo-era) and
    # the ^Midaz title assertion. Huma emits only .info.title + .info.version;
    # contact/license/tos are absent and OAS 3.1 has no .schemes. Title is NOT
    # shared metadata — each plane names itself ("Midaz Ledger API" vs "Midaz
    # Tracer API") — and the ledger dump currently carries the golden-test
    # placeholder "contract-spec" (contract_spec_routes_test.go), so ^Midaz would
    # be a false assertion here. Re-add ^Midaz once the ledger dump's title is the
    # runtime "Midaz Ledger API" rather than the contract-spec fixture title.
}

# Component whose every operation must declare a .security requirement.
SECURITY_COVERAGE_COMPONENT="ledger"

# Assert every operation in the ledger spec carries a non-empty .security block.
security_coverage_check() {
    print_header "Security coverage check (${SECURITY_COVERAGE_COMPONENT}: every operation secured)"

    # jq cannot read YAML; work off the JSON projection of the Huma dump.
    local json
    json="$(huma_dump_json "${SECURITY_COVERAGE_COMPONENT}")"

    # Operations are the HTTP-verb keys under each path; an operation is unsecured
    # when its .security array is absent or empty.
    local op_filter='.paths | to_entries[] | .key as $path | .value | to_entries[]
        | select(.key | test("^(get|post|put|patch|delete|head|options)$"))
        | { path: $path, method: .key, security: (.value.security // []) }'

    local total secured
    total="$(jq "[ ${op_filter} ] | length" <<<"${json}")"
    secured="$(jq "[ ${op_filter} | select(.security | length > 0) ] | length" <<<"${json}")"

    if [ "${secured}" != "${total}" ]; then
        echo -e "    ${RED}❌ ${SECURITY_COVERAGE_COMPONENT} has unsecured operations (${secured}/${total} secured):${NC}"
        jq -r "${op_filter} | select(.security | length == 0) | \"       \(.method | ascii_upcase) \(.path)\"" <<<"${json}"
        echo -e "    ${RED}Every ledger operation must declare a .security requirement (audit finding C1).${NC}"
        exit 1
    fi

    ok "All ${total} ${SECURITY_COVERAGE_COMPONENT} operations declare a .security requirement."
}

# Lint the consolidated spec with @redocly/cli. The ruleset is `recommended`
# scoped by postman/generator/redocly.yaml, which relaxes ONLY rules whose
# findings are inherited from the per-component source specs or are structural
# artifacts of the join (each documented with a WHY in that file). A genuinely
# new merge-introduced problem (broken $ref, dropped security scheme, invalid
# structure) still fails this gate.
consolidated_lint_check() {
    print_header "Consolidated spec lint (redocly)"

    local redocly_bin="${GENERATOR_DIR}/node_modules/.bin/redocly"
    local consolidated_yaml="${ROOT_DIR}/postman/specs/midaz.openapi.yaml"
    local redocly_config="${GENERATOR_DIR}/redocly.yaml"

    # Gate on artifact + binary presence (mirrors how drift_check is opt-in):
    # when run standalone without a prior `make generate-docs`, the merged spec
    # or node_modules may be absent; skip-with-warning rather than hard-fail.
    if [ ! -f "${consolidated_yaml}" ]; then
        echo -e "    ${BLUE}ℹ️  Consolidated spec not found at ${consolidated_yaml}; skipping (run 'make generate-docs' first).${NC}"
        return 0
    fi

    if [ ! -x "${redocly_bin}" ]; then
        echo -e "    ${BLUE}ℹ️  @redocly/cli not installed at ${redocly_bin}; skipping (run 'make generate-docs' first).${NC}"
        return 0
    fi

    if (cd "${ROOT_DIR}" && "${redocly_bin}" lint postman/specs/midaz.openapi.yaml --config "${redocly_config}"); then
        ok "Consolidated spec passed redocly lint (recommended, scoped by redocly.yaml)."
    else
        fail "redocly lint failed on the consolidated spec; see findings above."
    fi
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
    consolidated_lint_check

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
