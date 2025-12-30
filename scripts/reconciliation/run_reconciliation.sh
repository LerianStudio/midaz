#!/bin/bash
# ============================================================================
# MIDAZ RECONCILIATION ORCHESTRATOR
# ============================================================================
# Purpose: Run all reconciliation scripts and generate telemetry output
# Usage: ./run_reconciliation.sh [--docker] [--output-dir /path/to/output]
#
# Options:
#   --docker      Use docker exec to connect to databases (default)
#   --local       Use local psql/mongosh clients
#   --output-dir  Directory for output files (default: ./output)
#   --json-only   Only output JSON telemetry (for worker consumption)
#   --quiet       Suppress verbose output
#
# Environment Variables:
#   PG_HOST, PG_PORT, PG_USER, PG_PASSWORD
#   MONGO_HOST, MONGO_PORT
# ============================================================================

set -euo pipefail
umask 077

# Require jq for JSON assembly
command -v jq &> /dev/null || { echo "Error: jq is required"; exit 1; }

# Configuration
USE_DOCKER=true
OUTPUT_DIR="./reports"
JSON_ONLY=false
QUIET=false

# Helper functions (defined early for use in argument parsing)
error() {
    echo "[ERROR] $1" >&2
}

log() {
    if [ "$QUIET" = false ]; then
        echo "[$(date '+%Y-%m-%d %H:%M:%S')] $1"
    fi
}

# Database connection (required environment variables)
PG_HOST="${PG_HOST:-localhost}"
PG_PORT="${PG_PORT:-5701}"
PG_USER="${PG_USER:?Error: PG_USER environment variable required}"
PG_PASSWORD="${PG_PASSWORD:?Error: PG_PASSWORD environment variable required}"
MONGO_HOST="${MONGO_HOST:?Error: MONGO_HOST environment variable required}"
MONGO_PORT="${MONGO_PORT:?Error: MONGO_PORT environment variable required}"

show_help() {
    cat << 'HELPEOF'
MIDAZ RECONCILIATION ORCHESTRATOR

Usage: ./run_reconciliation.sh [OPTIONS]

Options:
  --docker      Use docker exec to connect to databases (default)
  --local       Use local psql/mongosh clients
  --output-dir  Directory for output files (default: ./reports)
  --json-only   Only output JSON telemetry (for worker consumption)
  --quiet       Suppress verbose output
  --help        Show this help message

Required Environment Variables:
  PG_USER       PostgreSQL username
  PG_PASSWORD   PostgreSQL password
  MONGO_HOST    MongoDB host
  MONGO_PORT    MongoDB port

Optional Environment Variables:
  PG_HOST       PostgreSQL host (default: localhost)
  PG_PORT       PostgreSQL port (default: 5701)
HELPEOF
    exit 0
}

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --docker)
            USE_DOCKER=true
            shift
            ;;
        --local)
            USE_DOCKER=false
            shift
            ;;
        --output-dir)
            OUTPUT_DIR="$2"
            # Validate output directory for path traversal
            [[ "$OUTPUT_DIR" == *".."* ]] && { error "Invalid output directory: path traversal not allowed"; exit 1; }
            shift 2
            ;;
        --json-only)
            JSON_ONLY=true
            shift
            ;;
        --quiet)
            QUIET=true
            shift
            ;;
        --help)
            show_help
            ;;
        *)
            echo "Unknown option: $1"
            echo "Use --help for usage information"
            exit 1
            ;;
    esac
done

# Create output directory
mkdir -p "$OUTPUT_DIR"

TIMESTAMP=$(date +%Y%m%d_%H%M%S)
REPORT_FILE="$OUTPUT_DIR/reconciliation_${TIMESTAMP}.json"

# ============================================================================
# Database Connection Functions
# ============================================================================

run_psql() {
    local db=$1
    local query=$2

    if [ "$USE_DOCKER" = true ]; then
        docker exec midaz-postgres-primary psql -U "$PG_USER" -d "$db" -t -A -c "$query"
    else
        # Use .pgpass file to avoid password exposure in process list
        local PGPASSFILE
        PGPASSFILE=$(mktemp)
        echo "${PG_HOST}:${PG_PORT}:*:${PG_USER}:${PG_PASSWORD}" > "$PGPASSFILE"
        chmod 600 "$PGPASSFILE"
        PGPASSFILE="$PGPASSFILE" psql -h "$PG_HOST" -p "$PG_PORT" -U "$PG_USER" -d "$db" -t -A -c "$query"
        rm -f "$PGPASSFILE"
    fi
}

run_mongosh() {
    local db=$1
    local script=$2

    if [ "$USE_DOCKER" = true ]; then
        docker exec midaz-mongodb mongosh --quiet --port "$MONGO_PORT" "$db" --eval "$script"
    else
        mongosh --quiet --host "$MONGO_HOST" --port "$MONGO_PORT" "$db" --eval "$script"
    fi
}

# ============================================================================
# Reconciliation Checks
# ============================================================================

log "Starting Midaz Reconciliation..."
log "Output directory: $OUTPUT_DIR"

# ----------------------------------------------------------------------------
# Check 1: PostgreSQL Entity Counts
# ----------------------------------------------------------------------------
log "Running PostgreSQL entity counts..."

ONBOARDING_COUNTS=$(run_psql onboarding "
SELECT json_build_object(
    'organization', (SELECT COUNT(*) FROM organization WHERE deleted_at IS NULL),
    'ledger', (SELECT COUNT(*) FROM ledger WHERE deleted_at IS NULL),
    'asset', (SELECT COUNT(*) FROM asset WHERE deleted_at IS NULL),
    'account', (SELECT COUNT(*) FROM account WHERE deleted_at IS NULL),
    'portfolio', (SELECT COUNT(*) FROM portfolio WHERE deleted_at IS NULL),
    'segment', (SELECT COUNT(*) FROM segment WHERE deleted_at IS NULL)
);")

TRANSACTION_COUNTS=$(run_psql transaction "
SELECT json_build_object(
    'transaction', (SELECT COUNT(*) FROM transaction WHERE deleted_at IS NULL),
    'operation', (SELECT COUNT(*) FROM operation WHERE deleted_at IS NULL),
    'balance', (SELECT COUNT(*) FROM balance WHERE deleted_at IS NULL),
    'asset_rate', (SELECT COUNT(*) FROM asset_rate)
);")

# ----------------------------------------------------------------------------
# Check 2: Balance Consistency
# ----------------------------------------------------------------------------
log "Running balance consistency check..."

BALANCE_CHECK=$(run_psql transaction "
WITH balance_calc AS (
    SELECT
        b.available as current_balance,
        -- Calculate expected balance from operations:
        -- CREDITS add to available
        -- DEBITS subtract from available
        -- ON_HOLD subtracts from available (for APPROVED or PENDING, as CANCELED holds are fully reversed)
        COALESCE(SUM(CASE WHEN o.type = 'CREDIT' AND o.balance_affected = true THEN o.amount ELSE 0 END), 0) -
        COALESCE(SUM(CASE WHEN o.type = 'DEBIT' AND o.balance_affected = true THEN o.amount ELSE 0 END), 0) -
        COALESCE(SUM(CASE WHEN o.type = 'ON_HOLD' AND o.balance_affected = true AND t.status IN ('APPROVED', 'PENDING') THEN o.amount ELSE 0 END), 0) as expected
    FROM balance b
    LEFT JOIN operation o ON b.account_id = o.account_id AND b.asset_code = o.asset_code AND b.key = o.balance_key AND o.deleted_at IS NULL
    LEFT JOIN transaction t ON o.transaction_id = t.id
    WHERE b.deleted_at IS NULL
    GROUP BY b.id, b.available
)
SELECT json_build_object(
    'total_balances', COUNT(*),
    'discrepancies', SUM(CASE WHEN ABS(current_balance - expected) > 0 THEN 1 ELSE 0 END),
    'total_discrepancy_amount', COALESCE(SUM(ABS(current_balance - expected)), 0)
)
FROM balance_calc;")

# ----------------------------------------------------------------------------
# Check 3: Double-Entry Validation
# ----------------------------------------------------------------------------
log "Running double-entry validation..."

DOUBLE_ENTRY_CHECK=$(run_psql transaction "
WITH txn_balance AS (
    SELECT
        t.id,
        COALESCE(SUM(CASE WHEN o.type = 'CREDIT' THEN o.amount ELSE 0 END), 0) as credits,
        COALESCE(SUM(CASE WHEN o.type = 'DEBIT' THEN o.amount ELSE 0 END), 0) as debits,
        COUNT(o.id) as ops
    FROM transaction t
    LEFT JOIN operation o ON t.id = o.transaction_id AND o.deleted_at IS NULL
    WHERE t.deleted_at IS NULL
    GROUP BY t.id
)
SELECT json_build_object(
    'total_transactions', COUNT(*),
    'unbalanced', SUM(CASE WHEN credits != debits THEN 1 ELSE 0 END),
    'orphan_transactions', SUM(CASE WHEN ops = 0 THEN 1 ELSE 0 END)
)
FROM txn_balance;")

# ----------------------------------------------------------------------------
# Check 4: Referential Integrity
# ----------------------------------------------------------------------------
log "Running referential integrity check..."

INTEGRITY_ONBOARDING=$(run_psql onboarding "
SELECT json_build_object(
    'orphan_ledgers', (SELECT COUNT(*) FROM ledger l LEFT JOIN organization o ON l.organization_id = o.id AND o.deleted_at IS NULL WHERE l.deleted_at IS NULL AND o.id IS NULL),
    'orphan_assets', (SELECT COUNT(*) FROM asset a LEFT JOIN ledger l ON a.ledger_id = l.id AND l.deleted_at IS NULL WHERE a.deleted_at IS NULL AND l.id IS NULL),
    'orphan_accounts', (SELECT COUNT(*) FROM account acc LEFT JOIN ledger l ON acc.ledger_id = l.id AND l.deleted_at IS NULL WHERE acc.deleted_at IS NULL AND l.id IS NULL)
);")

INTEGRITY_TRANSACTION=$(run_psql transaction "
SELECT json_build_object(
    'orphan_operations', (SELECT COUNT(*) FROM operation o LEFT JOIN transaction t ON o.transaction_id = t.id WHERE o.deleted_at IS NULL AND t.id IS NULL),
    'operations_without_balance', (SELECT COUNT(*) FROM operation o LEFT JOIN balance b ON o.balance_id = b.id WHERE o.deleted_at IS NULL AND o.balance_id IS NOT NULL AND b.id IS NULL)
);")

# ----------------------------------------------------------------------------
# Check 5: MongoDB Counts (DEPRECATED - kept for reference)
# ----------------------------------------------------------------------------
# NOTE: Raw MongoDB counts are NOT comparable to PostgreSQL counts.
# MongoDB stores ONLY metadata records, not full entity replication.
# See Check 6 for meaningful metadata integrity checks.
# log "Running MongoDB counts..."
# MONGO_ONBOARDING=$(run_mongosh onboarding "...")
# MONGO_TRANSACTION=$(run_mongosh transaction "...")

# ----------------------------------------------------------------------------
# Check 6: MongoDB Metadata Integrity
# ----------------------------------------------------------------------------
log "Running MongoDB metadata integrity check..."

# Check for metadata in onboarding (metadata records for entities with metadata attached)
MONGO_METADATA_ONBOARDING=$(run_mongosh onboarding "
var counts = {
    organization: 0,
    ledger: 0,
    asset: 0,
    account: 0,
    portfolio: 0,
    segment: 0
};
var entityNames = ['organization', 'ledger', 'asset', 'account', 'portfolio', 'segment'];
entityNames.forEach(function(name) {
    var coll = db.getCollection(name);
    var count = coll.countDocuments({deleted_at: null});
    counts[name] = count;
});
JSON.stringify({
    total_metadata_records: counts.organization + counts.ledger + counts.asset + counts.account + counts.portfolio + counts.segment,
    by_entity: counts
});
")

MONGO_METADATA_TRANSACTION=$(run_mongosh transaction "
var counts = {transaction: 0, operation: 0};
counts.transaction = db.transaction.countDocuments({deleted_at: null});
counts.operation = db.operation.countDocuments({deleted_at: null});
JSON.stringify({
    total_metadata_records: counts.transaction + counts.operation,
    by_entity: counts
});
")

# ----------------------------------------------------------------------------
# Assemble Final Report
# ----------------------------------------------------------------------------
log "Assembling final report..."

# Assemble JSON report using jq (required dependency)
jq -n \
    --arg ts "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    --argjson pg_onboarding "$ONBOARDING_COUNTS" \
    --argjson pg_transaction "$TRANSACTION_COUNTS" \
    --argjson balance "$BALANCE_CHECK" \
    --argjson double_entry "$DOUBLE_ENTRY_CHECK" \
    --argjson integrity_onboarding "$INTEGRITY_ONBOARDING" \
    --argjson integrity_transaction "$INTEGRITY_TRANSACTION" \
    --argjson mongo_metadata_onboarding "$MONGO_METADATA_ONBOARDING" \
    --argjson mongo_metadata_transaction "$MONGO_METADATA_TRANSACTION" \
    '{
        reconciliation_report: {
            timestamp: $ts,
            version: "2.0.0",
            checks: {
                entity_counts: {
                    postgresql: {
                        onboarding: $pg_onboarding,
                        transaction: $pg_transaction
                    },
                    note: "MongoDB stores metadata ONLY for entities with metadata attached. Count differences are expected and normal."
                },
                metadata_store: {
                    note: "MongoDB is a metadata-only store. Records exist only for entities that have metadata attached.",
                    onboarding: $mongo_metadata_onboarding,
                    transaction: $mongo_metadata_transaction
                },
                balance_consistency: $balance,
                double_entry_validation: $double_entry,
                referential_integrity: {
                    onboarding: $integrity_onboarding,
                    transaction: $integrity_transaction
                }
            },
            summary: {
                status: (
                    if ($balance.discrepancies // 0) == 0 and
                       ($double_entry.unbalanced // 0) == 0 and
                       ($integrity_onboarding.orphan_ledgers // 0) == 0 and
                       ($integrity_transaction.orphan_operations // 0) == 0
                    then "HEALTHY"
                    else "ISSUES_DETECTED"
                    end
                ),
                total_issues: (
                    ($balance.discrepancies // 0) +
                    ($double_entry.unbalanced // 0) +
                    ($double_entry.orphan_transactions // 0) +
                    ($integrity_onboarding.orphan_ledgers // 0) +
                    ($integrity_onboarding.orphan_assets // 0) +
                    ($integrity_onboarding.orphan_accounts // 0) +
                    ($integrity_transaction.orphan_operations // 0)
                )
            }
        }
    }' > "$REPORT_FILE"

# ----------------------------------------------------------------------------
# Output
# ----------------------------------------------------------------------------

if [ "$JSON_ONLY" = true ]; then
    cat "$REPORT_FILE"
else
    log "Reconciliation complete!"
    log "Report saved to: $REPORT_FILE"
    echo ""
    echo "=== RECONCILIATION SUMMARY ==="
    jq '.reconciliation_report.summary' "$REPORT_FILE"
fi

exit 0
