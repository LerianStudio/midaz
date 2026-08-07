#!/bin/bash

# Copyright (c) 2026 Lerian Studio. All rights reserved.
# Use of this source code is governed by the Elastic License 2.0
# that can be found in the LICENSE file.

set -euo pipefail

# Clean documentation generation script
# Regenerates the native Huma OAS 3.1 dumps and consolidates them for the hub

# Root directory of the repo (this script lives in postman/generator/)
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

# Generator tooling and published-spec directories
GENERATOR_DIR="${ROOT_DIR}/postman/generator"
SPECS_DIR="${ROOT_DIR}/postman/specs"

# Components to process (each must have a cmd/app/main.go entry point)
COMPONENTS=("ledger" "tracer")

# Temporary log dir
LOG_DIR="${ROOT_DIR}/tmp"
mkdir -p "${LOG_DIR}"

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Print a nice header
print_header() {
    echo ""
    echo -e "${BLUE}=================================================${NC}"
    echo -e "${BLUE}  📝 $1${NC}"
    echo -e "${BLUE}=================================================${NC}"
    echo ""
}

# Print step with status
print_step() {
    local step_name="$1"
    local status="$2"
    local time_taken="${3:-}"
    
    if [ "$status" = "SUCCESS" ]; then
        echo -e "    ${GREEN}✅ ${step_name}${time_taken:+ (${time_taken}s)}${NC}"
    elif [ "$status" = "FAILED" ]; then
        echo -e "    ${RED}❌ ${step_name} - FAILED${NC}"
    else
        echo -e "    ${YELLOW}⏳ ${step_name}...${NC}"
    fi
}

# Regenerate a component's native Huma OAS 3.1 dump (components/<c>/api/openapi.huma.yaml)
# by running its golden-dump test with -update. No swag, no Docker.
generate_openapi_spec() {
    local component="$1"
    local start_time=$(date +%s.%N)

    print_step "Generating ${component} OpenAPI spec (Huma dump)" "PROCESSING"

    local out_log="${LOG_DIR}/${component}_dump.out"
    local err_log="${LOG_DIR}/${component}_dump.err"

    if (go -C "${ROOT_DIR}" test -buildvcs=false -run TestOpenAPISpecDump \
            "./components/${component}/internal/adapters/http/in/" -update \
            > "${out_log}" 2> "${err_log}"); then
        local end_time=$(date +%s.%N)
        local elapsed=$(echo "scale=1; $end_time - $start_time" | bc 2>/dev/null || echo "0.0")
        print_step "Generated ${component} OpenAPI spec (Huma dump)" "SUCCESS" "${elapsed}"
        return 0
    else
        print_step "Generate ${component} OpenAPI spec (Huma dump)" "FAILED"
        echo -e "      ${RED}Error details:${NC}"
        head -5 "${err_log}" | sed 's/^/        /'
        return 1
    fi
}

# Copy the Huma dump into postman/specs/<component>/ for the hub
publish_specs() {
    local component="$1"
    local api_dir="${ROOT_DIR}/components/${component}/api"
    local dest_dir="${SPECS_DIR}/${component}"

    print_step "Publishing ${component} specs to postman/specs" "PROCESSING"

    if ! (mkdir -p "${dest_dir}" && cp "${api_dir}/openapi.huma.yaml" "${dest_dir}/"); then
        print_step "Publish ${component} specs to postman/specs" "FAILED"
        return 1
    fi

    print_step "Published ${component} specs to postman/specs" "SUCCESS"
    return 0
}

# Merge the three per-component openapi.yaml specs into one consolidated spec
# (postman/specs/midaz.openapi.{yaml,json}) via @redocly/cli join. Ledger is
# listed first so it acts as the "main" and takes precedence on shared metadata.
consolidate_openapi() {
    print_step "Consolidating OpenAPI specs" "PROCESSING"

    local out_log="${LOG_DIR}/consolidate.out"
    local err_log="${LOG_DIR}/consolidate.err"
    local start_time=$(date +%s.%N)

    local redocly_bin="${GENERATOR_DIR}/node_modules/.bin/redocly"
    local consolidated_yaml="${SPECS_DIR}/midaz.openapi.yaml"
    local consolidated_json="${SPECS_DIR}/midaz.openapi.json"

    if [ ! -x "${redocly_bin}" ]; then
        print_step "Consolidate OpenAPI specs" "FAILED"
        echo -e "      ${RED}Error details:${NC}"
        echo "        @redocly/cli not found at ${redocly_bin}; run install_npm_dependencies first."
        return 1
    fi

    # 1. Assert all component specs declare the same openapi: version.
    local ref_version="" version=""
    for component in "${COMPONENTS[@]}"; do
        local spec="${ROOT_DIR}/components/${component}/api/openapi.huma.yaml"
        if [ ! -f "${spec}" ]; then
            print_step "Consolidate OpenAPI specs" "FAILED"
            echo -e "      ${RED}Error details:${NC}"
            echo "        Missing component spec: ${spec}"
            return 1
        fi
        version="$(awk '/^openapi:/ {print $2; exit}' "${spec}" | tr -d '"'"'"\\r)"
        if [ -z "${ref_version}" ]; then
            ref_version="${version}"
        elif [ "${version}" != "${ref_version}" ]; then
            print_step "Consolidate OpenAPI specs" "FAILED"
            echo -e "      ${RED}Error details:${NC}"
            echo "        openapi version mismatch: ${component} is '${version}', expected '${ref_version}'."
            echo "        All component specs must share one openapi version before join."
            return 1
        fi
    done

    # 2a. Derive the tracer join input from its committed Huma dump. The dump is
    #     served under /v1 with server-relative paths (servers ["/v1"]) and path keys
    #     WITHOUT the /v1 prefix (e.g. "/audit-events"). The ledger dump, by contrast,
    #     carries "/v1/" + "/v2/" directly on its keys and declares servers ["/"].
    #     redocly join derives the joined document's root servers from the FIRST input
    #     only; if any later input's servers differ from the first's (including a bare
    #     "/v1" vs "/"), redocly compensates by stamping a servers array onto EVERY
    #     path item of EVERY input. This transform makes the tracer input SYMMETRIC to
    #     the ledger dump so no such override is emitted, WITHOUT mutating the committed
    #     tracer dump (js-yaml parses a fresh copy; the source file is never written):
    #       - prefix "/v1" onto every path key so each key is self-describing and
    #         globally unique against the ledger keys;
    #       - declare top-level servers "/" so it matches the ledger dump's servers.
    #     Net: both inputs reach the join with self-describing keys and servers ["/"],
    #     so the joined root servers is ["/"] and NOT ONE path item carries a servers
    #     override. No per-path-item servers are written here — that asymmetry is
    #     exactly what produced the overrides.
    local tracer_dump="${ROOT_DIR}/components/tracer/api/openapi.huma.yaml"
    local tracer_join_input="${LOG_DIR}/tracer_join_input.yaml"

    # LOG_DIR survives between runs, so discard whatever an earlier run left here.
    # Joining below keys off THIS run having derived the input, never off the file
    # merely being present — otherwise renaming the dump would silently republish a
    # stale key set from a leftover output file.
    rm -f "${tracer_join_input}"

    # The tracer surface is a REQUIRED member of the hub: fail rather than degrade to
    # a ledger-only document if its dump is missing.
    if [ ! -f "${tracer_dump}" ]; then
        print_step "Consolidate OpenAPI specs" "FAILED"
        echo -e "      ${RED}Error details:${NC}"
        echo "        Missing required tracer dump: ${tracer_dump}"
        return 1
    fi

    if ! (cd "${ROOT_DIR}" && NODE_PATH="${GENERATOR_DIR}/node_modules" node -e '
        const yaml = require("js-yaml");
        const fs = require("fs");
        const doc = yaml.load(fs.readFileSync(process.argv[1], "utf8"));
        const prefixed = {};
        for (const key of Object.keys(doc.paths || {})) {
            prefixed["/v1" + key] = doc.paths[key];
        }
        doc.paths = prefixed;
        doc.servers = [{ url: "/" }];
        fs.writeFileSync(process.argv[2], yaml.dump(doc));
    ' "${tracer_dump}" "${tracer_join_input}" >> "${out_log}" 2>> "${err_log}"); then
        print_step "Consolidate OpenAPI specs" "FAILED"
        echo -e "      ${RED}Error details:${NC}"
        head -5 "${err_log}" | sed 's/^/        /'
        return 1
    fi

    # 2b. Join (ledger first => takes precedence on shared metadata and supplies the
    #     joined root servers). The derived tracer input follows. Run the
    #     locally-installed binary directly so the component paths stay relative to
    #     ROOT_DIR.
    local join_inputs=(components/ledger/api/openapi.huma.yaml "${tracer_join_input}")

    if ! (cd "${ROOT_DIR}" && "${redocly_bin}" join \
            "${join_inputs[@]}" \
            --prefix-tags-with-info-prop title \
            -o postman/specs/midaz.openapi.yaml > "${out_log}" 2> "${err_log}"); then
        print_step "Consolidate OpenAPI specs" "FAILED"
        echo -e "      ${RED}Error details:${NC}"
        head -5 "${err_log}" | sed 's/^/        /'
        return 1
    fi

    # 3. Produce a deterministic JSON twin from the YAML via the bundled js-yaml.
    if ! (cd "${ROOT_DIR}" && NODE_PATH="${GENERATOR_DIR}/node_modules" node -e '
        const yaml = require("js-yaml");
        const fs = require("fs");
        const doc = yaml.load(fs.readFileSync("postman/specs/midaz.openapi.yaml", "utf8"));
        fs.writeFileSync("postman/specs/midaz.openapi.json", JSON.stringify(doc, null, 2) + "\n");
    ' >> "${out_log}" 2>> "${err_log}"); then
        print_step "Consolidate OpenAPI specs" "FAILED"
        echo -e "      ${RED}Error details:${NC}"
        head -5 "${err_log}" | sed 's/^/        /'
        return 1
    fi

    # 4. Post-join validation against the JSON twin.

    # 4a. Structural HTTP-neutrality guard. Both join inputs arrive with
    #     self-describing keys and servers ["/"], so the joined document must expose a
    #     single root server "/" and NOT ONE path item may carry a servers override. A
    #     redocly upgrade that reverts to per-path-item stamping (the pre-symmetry
    #     behaviour) is invisible to the effective URL of any operation but changes the
    #     document shape; only this assertion catches that silent return.
    if ! jq -e '.servers == [{"url":"/"}]' "${consolidated_json}" > /dev/null; then
        print_step "Consolidate OpenAPI specs" "FAILED"
        echo -e "      ${RED}Error details:${NC}"
        echo "        Consolidated root servers is not exactly [{\"url\":\"/\"}]."
        echo "        A non-first input's servers likely diverged from the ledger dump's."
        return 1
    fi
    if ! jq -e '[.paths[] | select(has("servers"))] | length == 0' "${consolidated_json}" > /dev/null; then
        print_step "Consolidate OpenAPI specs" "FAILED"
        echo -e "      ${RED}Error details:${NC}"
        echo "        One or more path items carry a servers override; expected none."
        echo "        The join stamped per-path-item servers, meaning an input was asymmetric."
        return 1
    fi

    # 4b. Membership guard (derived, not a brittle literal). A join that silently loses
    #     a member — a dropped join_inputs entry, or a derivation that drops path keys —
    #     still produces a valid, smaller document every other check passes. A hardcoded
    #     count instead fails on any legitimate route change while misdirecting with a
    #     "dropped member" message. So derive the expected surface from the two required
    #     members' SOURCE dumps and assert each member's contribution to the hub is
    #     whole:
    #       - expected counts come from the committed dumps (ledger dump + the ORIGINAL
    #         tracer dump), NOT the derived tracer input — deriving from the intermediate
    #         would move in lockstep with a buggy derivation and hide a partial loss;
    #       - the hub partitions cleanly because the tracer keys are /v1-prefixed and
    #         disjoint from ledger's, so hub keys also present in the ledger dump are
    #         ledger's contribution and everything else is the tracer input's.
    #     A dropped or partially-lost member then fails the per-member "contributed X of
    #     Y" check, while a normal route addition simply moves the derived totals.
    if ! (cd "${ROOT_DIR}" && NODE_PATH="${GENERATOR_DIR}/node_modules" node -e '
        const yaml = require("js-yaml");
        const fs = require("fs");
        const pathKeys = f => Object.keys((yaml.load(fs.readFileSync(f, "utf8")) || {}).paths || {});
        const hub = JSON.parse(fs.readFileSync("postman/specs/midaz.openapi.json", "utf8"));
        const hubKeys = Object.keys(hub.paths || {});

        const ledgerKeys = new Set(pathKeys(process.argv[1]));
        const expectedLedger = ledgerKeys.size;
        const expectedTracer = pathKeys(process.argv[2]).length;

        const ledgerActual = hubKeys.filter(k => ledgerKeys.has(k)).length;
        const tracerActual = hubKeys.length - ledgerActual;

        const problems = [];
        if (ledgerActual !== expectedLedger) {
            problems.push("ledger contributed " + ledgerActual + " of " + expectedLedger + " path keys");
        }
        if (tracerActual !== expectedTracer) {
            problems.push("tracer contributed " + tracerActual + " of " + expectedTracer + " path keys");
        }
        if (problems.length > 0) {
            console.error("Consolidated hub is missing a join input contribution: " + problems.join("; ") + ". A join input was likely dropped or partially lost.");
            process.exit(1);
        }

        const expectedTotal = expectedLedger + expectedTracer;
        if (hubKeys.length !== expectedTotal) {
            console.error("Consolidated path-key count mismatch: hub has " + hubKeys.length + ", join inputs sum to " + expectedTotal + ".");
            process.exit(1);
        }
    ' components/ledger/api/openapi.huma.yaml "${tracer_dump}" 2>"${err_log}"); then
        print_step "Consolidate OpenAPI specs" "FAILED"
        echo -e "      ${RED}Error details:${NC}"
        head -5 "${err_log}" | sed 's/^/        /'
        return 1
    fi

    # 4c. Security post-validation against the JSON twin. redocly join's security
    #    merge is undocumented and root security may be dropped (known issue), so
    #    this guard catches a regression where a scheme goes missing or an
    #    operation references a scheme that is not defined.
    local missing
    missing="$(jq -r '
        ["BearerAuth","ApiKeyAuth"]
        - (.components.securitySchemes // {} | keys)
        | join(", ")
    ' "${consolidated_json}")"
    if [ -n "${missing}" ]; then
        print_step "Consolidate OpenAPI specs" "FAILED"
        echo -e "      ${RED}Error details:${NC}"
        echo "        Consolidated spec is missing required securityScheme(s): ${missing}."
        echo "        Expected both BearerAuth and ApiKeyAuth (both declared by the tracer dump)."
        return 1
    fi

    local orphans
    orphans="$(jq -r '
        (.components.securitySchemes // {} | keys) as $defined
        | [ .paths | to_entries[] | .value | to_entries[]
            | select(.key | test("^(get|post|put|patch|delete|head|options)$"))
            | (.value.security // [])[] | keys[] ]
        | unique
        | map(select(. as $s | ($defined | index($s)) | not))
        | join(", ")
    ' "${consolidated_json}")"
    if [ -n "${orphans}" ]; then
        print_step "Consolidate OpenAPI specs" "FAILED"
        echo -e "      ${RED}Error details:${NC}"
        echo "        Consolidated spec has operations referencing undefined securityScheme(s): ${orphans}."
        return 1
    fi

    local end_time=$(date +%s.%N)
    local elapsed=$(echo "scale=1; $end_time - $start_time" | bc 2>/dev/null || echo "0.0")
    print_step "Consolidated OpenAPI specs (openapi ${ref_version})" "SUCCESS" "${elapsed}"
    return 0
}

# Install Node.js dependencies for Postman generation
install_npm_dependencies() {
    print_step "Installing Node.js dependencies" "PROCESSING"

    local npm_out="${LOG_DIR}/npm.out"
    local npm_err="${LOG_DIR}/npm.err"
    local start_time=$(date +%s.%N)
    local postman_dir="${GENERATOR_DIR}"
    
    # Check if node_modules exists and package.json hasn't changed
    if [ -d "${postman_dir}/node_modules" ] && [ "${postman_dir}/node_modules" -nt "${postman_dir}/package.json" ]; then
        print_step "Node.js dependencies already up to date" "SUCCESS" "0.0"
        return 0
    fi
    
    if [ -f "${postman_dir}/package-lock.json" ]; then
        if (cd "${postman_dir}" && npm ci --silent > "${npm_out}" 2> "${npm_err}"); then
            local end_time=$(date +%s.%N)
            local elapsed=$(echo "scale=1; $end_time - $start_time" | bc 2>/dev/null || echo "0.0")
            print_step "Installed Node.js dependencies" "SUCCESS" "${elapsed}"
            return 0
        fi
    elif (cd "${postman_dir}" && npm install --silent > "${npm_out}" 2> "${npm_err}"); then
        local end_time=$(date +%s.%N)
        local elapsed=$(echo "scale=1; $end_time - $start_time" | bc 2>/dev/null || echo "0.0")
        print_step "Installed Node.js dependencies" "SUCCESS" "${elapsed}"
        return 0
    fi

    print_step "Install Node.js dependencies" "FAILED"
    echo -e "      ${RED}Error details:${NC}"
    head -5 "${npm_err}" | sed 's/^/        /'
    return 1
}

# Convert to Postman collection
convert_to_postman() {
    print_step "Converting to Postman collection" "PROCESSING"
    
    local sync_out="${LOG_DIR}/sync.out"
    local sync_err="${LOG_DIR}/sync.err"
    local start_time=$(date +%s.%N)
    
    if "${GENERATOR_DIR}/sync-postman.sh" > "${sync_out}" 2> "${sync_err}"; then
        local end_time=$(date +%s.%N)
        local elapsed=$(echo "scale=1; $end_time - $start_time" | bc 2>/dev/null || echo "0.0")
        print_step "Converted to Postman collection" "SUCCESS" "${elapsed}"
        return 0
    else
        print_step "Convert to Postman collection" "FAILED"
        echo -e "      ${RED}Error details:${NC}"
        head -5 "${sync_err}" | sed 's/^/        /'
        return 1
    fi
}

# Verify outputs
verify_outputs() {
    print_step "Verifying generated files" "PROCESSING"
    
    local collection_file="${ROOT_DIR}/postman/MIDAZ.postman_collection.json"
    local environment_file="${ROOT_DIR}/postman/MIDAZ.postman_environment.json"
    
    if [ -f "${collection_file}" ] && [ -f "${environment_file}" ]; then
        # Check if collection has content
        local request_count=$(jq '.item | length' "${collection_file}" 2>/dev/null || echo "0")
        local env_vars_count=$(jq '.values | length' "${environment_file}" 2>/dev/null || echo "0")
        
        print_step "Generated collection with ${request_count} folders and ${env_vars_count} environment variables" "SUCCESS"
        return 0
    else
        print_step "Verify generated files" "FAILED"
        return 1
    fi
}

# Main execution
main() {
    print_header "Generating OpenAPI API Documentation"

    # Track overall success
    local overall_success=true

    # Regenerate each component's native Huma OAS 3.1 dump via its golden test.
    if [ "$overall_success" = true ]; then
        for component in "${COMPONENTS[@]}"; do
            if ! generate_openapi_spec "$component"; then
                overall_success=false
                break
            fi
        done
    fi

    # Publish spec artifacts into postman/specs/<component>/ (copies; the api/
    # directory stays the dump home that Go and the contract test import)
    if [ "$overall_success" = true ]; then
        for component in "${COMPONENTS[@]}"; do
            if ! publish_specs "$component"; then
                overall_success=false
                break
            fi
        done
    fi

    # If OpenAPI generation succeeded, install dependencies, consolidate the
    # per-component specs into one, then convert to Postman.
    if [ "$overall_success" = true ]; then
        if ! install_npm_dependencies; then
            overall_success=false
        elif ! consolidate_openapi; then
            overall_success=false
        elif ! convert_to_postman; then
            overall_success=false
        fi
    fi
    
    # Verify outputs
    if [ "$overall_success" = true ]; then
        if ! verify_outputs; then
            overall_success=false
        fi
    fi
    
    # Final status
    echo ""
    if [ "$overall_success" = true ]; then
        echo -e "${GREEN}🎉 Documentation generation completed successfully!${NC}"
        echo -e "   📄 Collection: postman/MIDAZ.postman_collection.json"
        echo -e "   🌍 Environment: postman/MIDAZ.postman_environment.json"
        echo -e "   📚 Consolidated spec: postman/specs/midaz.openapi.yaml"
    else
        echo -e "${RED}❌ Documentation generation failed.${NC}"
        echo -e "   📋 Check logs in: ${LOG_DIR}/"
        exit 1
    fi
    
    # Clean up temporary logs on success
    if [ "$overall_success" = true ]; then
        rm -rf "${LOG_DIR}"
    fi
    
    echo ""
}

# Run main function
main "$@"
