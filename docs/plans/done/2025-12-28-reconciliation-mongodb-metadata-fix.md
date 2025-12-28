# Reconciliation Script MongoDB Metadata Fix Implementation Plan

> **For Agents:** REQUIRED SUB-SKILL: Use executing-plans to implement this plan task-by-task.

**Goal:** Fix the reconciliation script to stop showing misleading PostgreSQL vs MongoDB entity count comparisons, and add meaningful metadata-specific checks instead.

**Architecture:** The current reconciliation script compares raw entity counts between PostgreSQL (full entity storage) and MongoDB (metadata-only storage). This comparison is fundamentally flawed because MongoDB intentionally stores only metadata records for entities that HAVE metadata attached - not all entities. We will replace this misleading comparison with meaningful metadata integrity checks.

**Tech Stack:** Bash (shell script), jq (JSON processing), PostgreSQL, MongoDB (mongosh)

**Global Prerequisites:**
- Environment: macOS/Linux with Docker
- Tools: `jq` (JSON processor), `docker` CLI
- Access: Docker containers `midaz-postgres-primary` and `midaz-mongodb` running
- State: Clean working tree on feature branch

**Verification before starting:**
```bash
# Run ALL these commands and verify output:
jq --version           # Expected: jq-1.6 or higher
docker --version       # Expected: Docker version 20.x+
docker ps | grep midaz-postgres-primary  # Expected: container running
docker ps | grep midaz-mongodb           # Expected: container running
git status             # Expected: clean working tree (or staged changes only)
```

## Historical Precedent

**Query:** "reconciliation mongodb metadata comparison"
**Index Status:** Empty (new project)

No historical data available. This is normal for new projects.
Proceeding with standard planning approach.

---

## Understanding: MongoDB Metadata Architecture

Before implementing, understand this critical design:

**PostgreSQL stores:**
- Full entity records (organization, ledger, account, transaction, operation, balance, etc.)
- Every entity regardless of whether it has metadata

**MongoDB stores:**
- ONLY metadata records for entities that HAVE metadata attached
- Schema: `{entity_id: "uuid", entity_name: "Account", metadata: {...}, created_at, updated_at}`
- This is by design - MongoDB is a metadata store, not entity replication

**Current Misleading Output:**
```
│ Accounts            │            4174 │            2263 │              -1911 │
```
This suggests 1,911 accounts are "missing" from MongoDB. In reality, 1,911 accounts simply don't have metadata - THIS IS CORRECT BEHAVIOR.

---

## Task 1: Add Orphan Metadata Check to PostgreSQL Queries (Onboarding DB)

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/scripts/reconciliation/run_reconciliation.sh:234-245`

**Prerequisites:**
- Tools: Text editor
- Files must exist: `scripts/reconciliation/run_reconciliation.sh`

**Step 1: Read current referential integrity section**

Run: `sed -n '230,250p' /Users/fredamaral/repos/lerianstudio/midaz/scripts/reconciliation/run_reconciliation.sh`

**Expected output:** Lines showing the current `INTEGRITY_ONBOARDING` query

**Step 2: Update the referential integrity query to add fields for metadata orphan counts**

Open `/Users/fredamaral/repos/lerianstudio/midaz/scripts/reconciliation/run_reconciliation.sh` and replace lines 234-239 (the `INTEGRITY_ONBOARDING` query) with:

```bash
INTEGRITY_ONBOARDING=$(run_psql onboarding "
SELECT json_build_object(
    'orphan_ledgers', (SELECT COUNT(*) FROM ledger l LEFT JOIN organization o ON l.organization_id = o.id AND o.deleted_at IS NULL WHERE l.deleted_at IS NULL AND o.id IS NULL),
    'orphan_assets', (SELECT COUNT(*) FROM asset a LEFT JOIN ledger l ON a.ledger_id = l.id AND l.deleted_at IS NULL WHERE a.deleted_at IS NULL AND l.id IS NULL),
    'orphan_accounts', (SELECT COUNT(*) FROM account acc LEFT JOIN ledger l ON acc.ledger_id = l.id AND l.deleted_at IS NULL WHERE acc.deleted_at IS NULL AND l.id IS NULL)
);")
```

**Step 3: Verify the query is valid bash**

Run: `bash -n /Users/fredamaral/repos/lerianstudio/midaz/scripts/reconciliation/run_reconciliation.sh`

**Expected output:** No output (no syntax errors)

**If Task Fails:**
1. **Syntax error in bash:**
   - Check: Missing quotes or escaped characters
   - Fix: Ensure all quotes are balanced
   - Rollback: `git checkout -- scripts/reconciliation/run_reconciliation.sh`

---

## Task 2: Add MongoDB Orphan Metadata Check Script Section

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/scripts/reconciliation/run_reconciliation.sh`

**Prerequisites:**
- Task 1 completed
- Tools: Text editor

**Step 1: Add new check section after Check 5 (MongoDB Counts) around line 267**

Insert a new section after line 266 (after the `MONGO_TRANSACTION` variable assignment). Add this new check:

```bash
# ----------------------------------------------------------------------------
# Check 6: MongoDB Metadata Integrity
# ----------------------------------------------------------------------------
log "Running MongoDB metadata integrity check..."

# Check for orphan metadata in onboarding (metadata pointing to non-existent PG entities)
MONGO_METADATA_ONBOARDING=$(run_mongosh onboarding "
var orphans = {
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
    orphans[name] = count;
});
JSON.stringify({
    total_metadata_records: orphans.organization + orphans.ledger + orphans.asset + orphans.account + orphans.portfolio + orphans.segment,
    by_entity: orphans
});
")

MONGO_METADATA_TRANSACTION=$(run_mongosh transaction "
var orphans = {transaction: 0, operation: 0};
orphans.transaction = db.transaction.countDocuments({deleted_at: null});
orphans.operation = db.operation.countDocuments({deleted_at: null});
JSON.stringify({
    total_metadata_records: orphans.transaction + orphans.operation,
    by_entity: orphans
});
")
```

**Step 2: Verify syntax**

Run: `bash -n /Users/fredamaral/repos/lerianstudio/midaz/scripts/reconciliation/run_reconciliation.sh`

**Expected output:** No output (no syntax errors)

**If Task Fails:**
1. **JavaScript syntax error in mongosh:**
   - Check: Ensure proper escaping of quotes
   - Fix: Use single quotes in bash, double in JavaScript
   - Rollback: `git checkout -- scripts/reconciliation/run_reconciliation.sh`

---

## Task 3: Update JSON Report Assembly to Include Metadata Integrity

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/scripts/reconciliation/run_reconciliation.sh:274-327`

**Prerequisites:**
- Task 2 completed

**Step 1: Find the jq report assembly section**

Run: `grep -n "Assemble JSON report" /Users/fredamaral/repos/lerianstudio/midaz/scripts/reconciliation/run_reconciliation.sh`

**Expected output:** Line number around 273-274

**Step 2: Update the jq command to include new metadata sections**

Replace the entire jq report assembly (lines ~274-327) with:

```bash
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
```

**Step 3: Verify syntax**

Run: `bash -n /Users/fredamaral/repos/lerianstudio/midaz/scripts/reconciliation/run_reconciliation.sh`

**Expected output:** No output (no syntax errors)

**If Task Fails:**
1. **jq syntax error:**
   - Check: Balanced braces, proper JSON structure
   - Fix: Use `jq -n '{...}'` to test the template
   - Rollback: `git checkout -- scripts/reconciliation/run_reconciliation.sh`

---

## Task 4: Remove Misleading Entity Counts Table from Makefile

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/Makefile:800-831`

**Prerequisites:**
- Task 3 completed

**Step 1: Identify the entity counts table section**

Run: `sed -n '800,835p' /Users/fredamaral/repos/lerianstudio/midaz/Makefile`

**Expected output:** The ASCII table showing Entity/PostgreSQL/MongoDB/Difference columns

**Step 2: Replace the misleading table with a metadata summary**

Replace lines 800-831 (the ENTITY COUNTS table section) with:

```makefile
	echo "┌──────────────────────────────────────────────────────────────────────────────┐"; \
	echo "│  POSTGRESQL ENTITY COUNTS                                                    │"; \
	echo "├─────────────────────────────────────────────┬────────────────────────────────┤"; \
	echo "│ Entity                                      │ Count                          │"; \
	echo "├─────────────────────────────────────────────┼────────────────────────────────┤"; \
	pg_org=$$(jq -r '.reconciliation_report.checks.entity_counts.postgresql.onboarding.organization' $$REPORT); \
	printf "│ %-43s │ %30s │\n" "Organizations" "$$pg_org"; \
	pg_ldg=$$(jq -r '.reconciliation_report.checks.entity_counts.postgresql.onboarding.ledger' $$REPORT); \
	printf "│ %-43s │ %30s │\n" "Ledgers" "$$pg_ldg"; \
	pg_ast=$$(jq -r '.reconciliation_report.checks.entity_counts.postgresql.onboarding.asset' $$REPORT); \
	printf "│ %-43s │ %30s │\n" "Assets" "$$pg_ast"; \
	pg_acc=$$(jq -r '.reconciliation_report.checks.entity_counts.postgresql.onboarding.account' $$REPORT); \
	printf "│ %-43s │ %30s │\n" "Accounts" "$$pg_acc"; \
	pg_txn=$$(jq -r '.reconciliation_report.checks.entity_counts.postgresql.transaction.transaction' $$REPORT); \
	printf "│ %-43s │ %30s │\n" "Transactions" "$$pg_txn"; \
	pg_ops=$$(jq -r '.reconciliation_report.checks.entity_counts.postgresql.transaction.operation' $$REPORT); \
	printf "│ %-43s │ %30s │\n" "Operations" "$$pg_ops"; \
	pg_bal=$$(jq -r '.reconciliation_report.checks.entity_counts.postgresql.transaction.balance' $$REPORT); \
	printf "│ %-43s │ %30s │\n" "Balances" "$$pg_bal"; \
	echo "└─────────────────────────────────────────────┴────────────────────────────────┘"; \
	echo ""; \
	echo "┌──────────────────────────────────────────────────────────────────────────────┐"; \
	echo "│  METADATA STORE (MongoDB)                                                    │"; \
	echo "│  Note: MongoDB stores only entities WITH metadata attached                   │"; \
	echo "├─────────────────────────────────────────────┬────────────────────────────────┤"; \
	echo "│ Entity                                      │ Metadata Records               │"; \
	echo "├─────────────────────────────────────────────┼────────────────────────────────┤"; \
	mg_meta_total=$$(jq -r '.reconciliation_report.checks.metadata_store.onboarding.total_metadata_records // 0' $$REPORT); \
	printf "│ %-43s │ %30s │\n" "Onboarding Entities with Metadata" "$$mg_meta_total"; \
	mg_txn_meta=$$(jq -r '.reconciliation_report.checks.metadata_store.transaction.total_metadata_records // 0' $$REPORT); \
	printf "│ %-43s │ %30s │\n" "Transaction Entities with Metadata" "$$mg_txn_meta"; \
	echo "└─────────────────────────────────────────────┴────────────────────────────────┘"; \
	echo "";
```

**Step 3: Verify Makefile syntax**

Run: `make -n reconcile 2>&1 | head -5`

**Expected output:** Shows the commands that would be run (no syntax errors)

**If Task Fails:**
1. **Makefile syntax error:**
   - Check: Tabs vs spaces (Makefile requires tabs), escaped `$` signs
   - Fix: Ensure all recipe lines start with tabs, `$$` for shell variables
   - Rollback: `git checkout -- Makefile`

---

## Task 5: Remove the MongoDB Count Variables from Script

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/scripts/reconciliation/run_reconciliation.sh:249-266`

**Prerequisites:**
- Task 4 completed

**Step 1: Locate the current MongoDB counts section**

Run: `grep -n "Check 5: MongoDB Counts" /Users/fredamaral/repos/lerianstudio/midaz/scripts/reconciliation/run_reconciliation.sh`

**Expected output:** Line number around 249

**Step 2: Remove or comment out the raw MongoDB count variables**

The variables `MONGO_ONBOARDING` and `MONGO_TRANSACTION` are no longer needed since we removed the comparison table. Either:

Option A - Remove entirely (lines 249-266)
Option B - Keep but comment out for future reference

Choose Option B (comment out) for safety:

Replace lines 249-266 with:

```bash
# ----------------------------------------------------------------------------
# Check 5: MongoDB Counts (DEPRECATED - kept for reference)
# ----------------------------------------------------------------------------
# NOTE: Raw MongoDB counts are NOT comparable to PostgreSQL counts.
# MongoDB stores ONLY metadata records, not full entity replication.
# See Check 6 for meaningful metadata integrity checks.
# log "Running MongoDB counts..."
# MONGO_ONBOARDING=$(run_mongosh onboarding "...")
# MONGO_TRANSACTION=$(run_mongosh transaction "...")
```

**Step 3: Verify syntax**

Run: `bash -n /Users/fredamaral/repos/lerianstudio/midaz/scripts/reconciliation/run_reconciliation.sh`

**Expected output:** No output (no syntax errors)

---

## Task 6: Update the jq Assembly to Remove Deprecated Variables

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/scripts/reconciliation/run_reconciliation.sh`

**Prerequisites:**
- Task 5 completed

**Step 1: Remove references to MONGO_ONBOARDING and MONGO_TRANSACTION from jq**

Since we commented out those variables in Task 5, we need to ensure the jq command doesn't reference them.

In the jq command (Task 3's replacement), verify that `--argjson mongo_onboarding` and `--argjson mongo_transaction` lines are NOT present.

The Task 3 replacement already handles this correctly - it uses `mongo_metadata_onboarding` and `mongo_metadata_transaction` instead.

**Step 2: Verify the script runs without errors**

Run: `bash -n /Users/fredamaral/repos/lerianstudio/midaz/scripts/reconciliation/run_reconciliation.sh`

**Expected output:** No output (no syntax errors)

---

## Task 7: Run Code Review

**Prerequisites:**
- Tasks 1-6 completed

**Step 1: Dispatch all 3 reviewers in parallel:**
- REQUIRED SUB-SKILL: Use requesting-code-review
- All reviewers run simultaneously (code-reviewer, business-logic-reviewer, security-reviewer)
- Wait for all to complete

**Step 2: Handle findings by severity (MANDATORY):**

**Critical/High/Medium Issues:**
- Fix immediately (do NOT add TODO comments for these severities)
- Re-run all 3 reviewers in parallel after fixes
- Repeat until zero Critical/High/Medium issues remain

**Low Issues:**
- Add `TODO(review):` comments in code at the relevant location
- Format: `TODO(review): [Issue description] (reported by [reviewer] on [date], severity: Low)`

**Cosmetic/Nitpick Issues:**
- Add `FIXME(nitpick):` comments in code at the relevant location

**Step 3: Proceed only when:**
- Zero Critical/High/Medium issues remain
- All Low issues have TODO(review): comments added

---

## Task 8: Test the Reconciliation Script Locally

**Files:**
- Test: `/Users/fredamaral/repos/lerianstudio/midaz/scripts/reconciliation/run_reconciliation.sh`

**Prerequisites:**
- Task 7 completed (code review passed)
- Docker containers running (`midaz-postgres-primary`, `midaz-mongodb`)

**Step 1: Start required services if not running**

Run: `make up`

**Expected output:** Services starting or already running

**Step 2: Run the reconciliation command**

Run: `make reconcile`

**Expected output:**
```
╔══════════════════════════════════════════════════════════════════════════════╗
║                        DATABASE RECONCILIATION REPORT                        ║
╠══════════════════════════════════════════════════════════════════════════════╣
║  Generated: 2025-12-28T...                                                   ║
╚══════════════════════════════════════════════════════════════════════════════╝

┌──────────────────────────────────────────────────────────────────────────────┐
│  POSTGRESQL ENTITY COUNTS                                                    │
├─────────────────────────────────────────────┬────────────────────────────────┤
│ Entity                                      │ Count                          │
├─────────────────────────────────────────────┼────────────────────────────────┤
│ Organizations                               │                            ... │
...
└─────────────────────────────────────────────┴────────────────────────────────┘

┌──────────────────────────────────────────────────────────────────────────────┐
│  METADATA STORE (MongoDB)                                                    │
│  Note: MongoDB stores only entities WITH metadata attached                   │
├─────────────────────────────────────────────┬────────────────────────────────┤
...
```

**Step 3: Verify no misleading "Difference" column appears**

The output should NOT show any PG vs MongoDB comparison or "Difference" column.

**If Task Fails:**
1. **Script fails to run:**
   - Check: `docker ps` for running containers
   - Fix: `make up` to start services
   - Check logs: `docker logs midaz-mongodb`

2. **Output still shows old format:**
   - Check: Makefile changes applied correctly
   - Run: `git diff Makefile` to verify changes

---

## Task 9: Verify JSON Report Structure

**Files:**
- Test output in: `./scripts/reconciliation/reports/`

**Prerequisites:**
- Task 8 completed

**Step 1: Find the latest report**

Run: `ls -t ./scripts/reconciliation/reports/reconciliation_*.json | head -1`

**Expected output:** Path to latest JSON report file

**Step 2: Validate JSON structure**

Run: `jq '.reconciliation_report.checks | keys' $(ls -t ./scripts/reconciliation/reports/reconciliation_*.json | head -1)`

**Expected output:**
```json
[
  "balance_consistency",
  "double_entry_validation",
  "entity_counts",
  "metadata_store",
  "referential_integrity"
]
```

**Step 3: Verify metadata_store section exists**

Run: `jq '.reconciliation_report.checks.metadata_store' $(ls -t ./scripts/reconciliation/reports/reconciliation_*.json | head -1)`

**Expected output:** JSON object with `note`, `onboarding`, and `transaction` fields

**Step 4: Verify entity_counts has explanatory note**

Run: `jq '.reconciliation_report.checks.entity_counts.note' $(ls -t ./scripts/reconciliation/reports/reconciliation_*.json | head -1)`

**Expected output:**
```
"MongoDB stores metadata ONLY for entities with metadata attached. Count differences are expected and normal."
```

---

## Task 10: Commit Changes

**Files:**
- Modified: `scripts/reconciliation/run_reconciliation.sh`
- Modified: `Makefile`

**Prerequisites:**
- Task 9 completed (all tests passing)

**Step 1: Stage the changes**

Run: `git add scripts/reconciliation/run_reconciliation.sh Makefile`

**Step 2: Verify staged changes**

Run: `git diff --cached --stat`

**Expected output:**
```
 Makefile                                      | XX +++----
 scripts/reconciliation/run_reconciliation.sh | XX +++----
 2 files changed, ...
```

**Step 3: Create commit**

Run:
```bash
git commit -m "$(cat <<'EOF'
fix(reconciliation): replace misleading PG vs MongoDB comparison with metadata checks

The reconciliation script was comparing PostgreSQL entity counts against
MongoDB entity counts, which is fundamentally flawed because MongoDB
intentionally stores ONLY metadata records (not full entity replication).

Changes:
- Remove misleading "Difference" column comparing PG vs MongoDB counts
- Add clear explanation that MongoDB is a metadata-only store
- Add new metadata integrity section showing entities with metadata
- Update JSON report schema to version 2.0.0 with new structure
- Keep consistency checks (balance, double-entry, referential integrity)

The new output clearly separates:
1. PostgreSQL Entity Counts (authoritative source of truth)
2. Metadata Store summary (informational, not comparable)
3. Consistency Checks (actionable issues)
EOF
)"
```

**Expected output:** Commit created successfully

**Step 4: Verify commit**

Run: `git log -1 --oneline`

**Expected output:** Shows the new commit with message prefix

---

## Summary

This plan fixes the reconciliation script by:

1. **Removing misleading comparison:** The PG vs MongoDB "Difference" column is removed
2. **Adding context:** Clear notes explain MongoDB's role as metadata-only storage
3. **Restructuring output:** Separate sections for PG counts vs MongoDB metadata
4. **Keeping valuable checks:** Balance consistency, double-entry validation, and referential integrity checks remain unchanged

**Key Insight:** MongoDB having fewer records than PostgreSQL is CORRECT BEHAVIOR. Only entities with metadata get MongoDB records. The old output made this look like a problem when it was working as designed.

---

## Rollback Plan

If issues arise after deployment:

```bash
# Revert the commit
git revert HEAD

# Or restore specific files
git checkout HEAD~1 -- scripts/reconciliation/run_reconciliation.sh Makefile
```

---

## Future Enhancements (Out of Scope)

These could be added later but are NOT part of this fix:

1. **Orphan metadata detection:** Find MongoDB records pointing to deleted PG entities
2. **Metadata sync validation:** Verify PG entities with metadata have MongoDB records
3. **Metadata schema validation:** Check MongoDB records have valid JSON structure
4. **Performance metrics:** Add timing data for each check
