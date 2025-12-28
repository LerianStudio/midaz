// ============================================================================
// MIDAZ RECONCILIATION SCRIPT: Cross-Database Entity Validation
// ============================================================================
//
// STATUS: TEMPLATE / HELPER SCRIPT
//
// This script provides templates and helper functions for cross-database
// validation between PostgreSQL and MongoDB. It is NOT a fully automated
// reconciliation solution - it requires manual intervention and a Go worker
// implementation for production use.
//
// Purpose: Compare PostgreSQL entities with MongoDB metadata documents
//          Finds entities missing in either database
// Databases: onboarding (PostgreSQL + MongoDB), transaction (PostgreSQL + MongoDB)
// Frequency: Daily (when implemented in Go worker)
//
// IMPORTANT: This script requires entity IDs to be exported from PostgreSQL first
// See the bash wrapper script for full workflow
//
// Current functionality:
//   - Outputs MongoDB entity counts for manual comparison
//   - Provides SQL queries to run against PostgreSQL
//   - Documents expected differences between databases
//   - Provides Go code templates for worker implementation
//
// TODO: Implement full automation via Go reconciliation worker
//
// Usage: mongosh --port 5703 < 02_cross_db_validation.js
// ============================================================================

print("=".repeat(80));
print("MIDAZ CROSS-DATABASE RECONCILIATION");
print("Timestamp: " + new Date().toISOString());
print("=".repeat(80));

// ============================================================================
// Helper Functions (Templates for Go Worker Implementation)
// ============================================================================
//
// NOTE: The function below is a TEMPLATE showing the logic for cross-database
// validation. It is NOT called in this script because:
//   1. The use() command doesn't work correctly inside functions in mongosh
//   2. This script doesn't have access to PostgreSQL entity IDs
//
// TODO: Implement this logic in the Go reconciliation worker where:
//   - PostgreSQL can be queried directly via pgx
//   - MongoDB can be queried via the mongo-driver
//   - Results can be properly aggregated and reported
//
// The function is preserved here as reference documentation for the algorithm.
// ============================================================================

/**
 * TEMPLATE FUNCTION - NOT CALLED IN THIS SCRIPT
 *
 * Find PostgreSQL entities that don't have MongoDB metadata.
 * This demonstrates the comparison algorithm to implement in Go.
 *
 * @param {Object} dbConnection - Pre-selected MongoDB database connection
 * @param {string} collectionName - MongoDB collection name
 * @param {Array<string>} pgEntityIds - Array of entity IDs from PostgreSQL
 * @returns {Object} Comparison results with missing and extra entities
 *
 * Go implementation should:
 *   1. Query PostgreSQL: SELECT id FROM <entity> WHERE deleted_at IS NULL
 *   2. Query MongoDB: db.<collection>.distinct("entity_id", {deleted_at: null})
 *   3. Compare the two sets using this algorithm
 */
function findMissingInMongo_TEMPLATE(dbConnection, collectionName, pgEntityIds) {
  // NOTE: In Go, use mongo.Database directly instead of use()
  const coll = dbConnection.getCollection(collectionName);
  if (!coll) {
    print(`Collection ${collectionName} not found`);
    return { missing: [], extra: [] };
  }

  // Get all entity_ids from MongoDB
  const mongoIds = new Set(coll.distinct("entity_id", { deleted_at: null }));

  // Find IDs in PostgreSQL but not in MongoDB (missing metadata)
  const missingInMongo = pgEntityIds.filter((id) => !mongoIds.has(id));

  // Find IDs in MongoDB but not in PostgreSQL (orphan metadata)
  const pgIdSet = new Set(pgEntityIds);
  const extraInMongo = [...mongoIds].filter((id) => !pgIdSet.has(id));

  return {
    missing: missingInMongo,
    extra: extraInMongo,
    total_pg: pgEntityIds.length,
    total_mongo: mongoIds.size,
  };
}

// ============================================================================
// SECTION 1: SIMULATION MODE
// ============================================================================
//
// !! SIMULATION MODE !!
//
// This section runs in "simulation mode" because:
//   - PostgreSQL entity IDs are not available to this MongoDB script
//   - Full cross-database validation requires the Go worker implementation
//
// What this section does:
//   - Queries MongoDB counts for manual comparison
//   - Validates internal MongoDB consistency (references between collections)
//
// TODO: Replace simulation with real cross-DB validation in Go worker
// ============================================================================

print("\n--- CROSS-DB VALIDATION (SIMULATION MODE) ---\n");
print("NOTE: Running in simulation mode - PostgreSQL data not available");
print("      Use Go reconciliation worker for full cross-DB validation\n");

// TODO: In Go worker, fetch PostgreSQL entity IDs here:
//   rows, _ := pgPool.Query(ctx, "SELECT id FROM organization WHERE deleted_at IS NULL")
// For now, we sample from MongoDB and check internal consistency

use("onboarding");

const crossDbSummary = {
  check_type: "cross_db_validation",
  timestamp: new Date(),
  databases: {},
};

// Sample validation: Check MongoDB has consistent internal references
print("Validating internal MongoDB consistency...\n");

// Check organization references in ledger metadata
const ledgerOrgIds = db.ledger.distinct("entity_id", { deleted_at: null });
print(`Ledgers in MongoDB: ${ledgerOrgIds.length}`);

// Check account references
const accountIds = db.account.distinct("entity_id", { deleted_at: null });
print(`Accounts in MongoDB: ${accountIds.length}`);

// ============================================================================
// SECTION 2: ENTITY COUNT COMPARISON TEMPLATE
// ============================================================================
//
// This section outputs MongoDB entity counts that can be manually compared
// with PostgreSQL counts (see SQL queries in Section 3).
//
// TODO: Automate this comparison in Go worker by:
//   1. Running the SQL queries against PostgreSQL
//   2. Running the MongoDB count queries
//   3. Computing differences and generating alerts
// ============================================================================

print("\n--- ENTITY COUNTS FOR MANUAL COMPARISON ---\n");

const onboardingCounts = {
  organization: db.organization.countDocuments({ deleted_at: null }),
  ledger: db.ledger.countDocuments({ deleted_at: null }),
  asset: db.asset.countDocuments({ deleted_at: null }),
  account: db.account.countDocuments({ deleted_at: null }),
  portfolio: db.portfolio.countDocuments({ deleted_at: null }),
  segment: db.segment.countDocuments({ deleted_at: null }),
};

print("ONBOARDING MongoDB Counts:");
for (const [entity, count] of Object.entries(onboardingCounts)) {
  print(`  ${entity}: ${count}`);
}

crossDbSummary.databases.onboarding = {
  mongodb_counts: onboardingCounts,
  // postgresql_counts would be added by the worker after running SQL
};

use("transaction");

const transactionCounts = {
  transaction: db.transaction.countDocuments({ deleted_at: null }),
  operation: db.operation.countDocuments({ deleted_at: null }),
};

print("\nTRANSACTION MongoDB Counts:");
for (const [entity, count] of Object.entries(transactionCounts)) {
  print(`  ${entity}: ${count}`);
}

crossDbSummary.databases.transaction = {
  mongodb_counts: transactionCounts,
};

// ============================================================================
// SECTION 3: SQL QUERIES TO RUN FOR COMPARISON
// ============================================================================
//
// These SQL queries are provided for manual execution against PostgreSQL.
// Compare the results with MongoDB counts from Section 2.
//
// TODO: Integrate these queries into Go worker using pgx:
//   var count int
//   err := pgPool.QueryRow(ctx, "SELECT COUNT(*) FROM organization WHERE deleted_at IS NULL").Scan(&count)
// ============================================================================

print("\n--- SQL QUERIES FOR POSTGRESQL COMPARISON ---\n");

print("Run these on PostgreSQL 'onboarding' database:");
print("");
print(`
SELECT 'organization' as entity, COUNT(*) as count FROM organization WHERE deleted_at IS NULL
UNION ALL SELECT 'ledger', COUNT(*) FROM ledger WHERE deleted_at IS NULL
UNION ALL SELECT 'asset', COUNT(*) FROM asset WHERE deleted_at IS NULL
UNION ALL SELECT 'account', COUNT(*) FROM account WHERE deleted_at IS NULL
UNION ALL SELECT 'portfolio', COUNT(*) FROM portfolio WHERE deleted_at IS NULL
UNION ALL SELECT 'segment', COUNT(*) FROM segment WHERE deleted_at IS NULL;
`);

print("\nRun these on PostgreSQL 'transaction' database:");
print(`
SELECT 'transaction' as entity, COUNT(*) as count FROM transaction WHERE deleted_at IS NULL
UNION ALL SELECT 'operation', COUNT(*) FROM operation WHERE deleted_at IS NULL
UNION ALL SELECT 'balance', COUNT(*) FROM balance WHERE deleted_at IS NULL;
`);

// ============================================================================
// SECTION 4: EXPECTED DIFFERENCES
// ============================================================================

print("\n--- EXPECTED DIFFERENCES ---\n");

print("Some differences are expected:");
print("1. PostgreSQL may have more entities than MongoDB metadata");
print("   - Not all entities have user-provided metadata");
print("   - Metadata is optional and stored only when provided");
print("");
print("2. MongoDB should NOT have more entities than PostgreSQL");
print("   - This would indicate orphan metadata documents");
print("   - Could happen if PostgreSQL entities were hard-deleted");
print("");
print("3. 'balance' exists only in PostgreSQL (no MongoDB metadata)");
print("   - Balances are cache-synced from Redis, not user-managed");

// ============================================================================
// SECTION 5: GENERATE RECONCILIATION REPORT TEMPLATE
// ============================================================================

print("\n--- TELEMETRY OUTPUT ---\n");

printjson(crossDbSummary);

// ============================================================================
// SECTION 6: VALIDATION FUNCTIONS FOR WORKER USE
// ============================================================================
//
// This section provides Go code templates for implementing the reconciliation
// worker. Copy and adapt this code to the actual Go implementation.
//
// TODO: Create components/reconciliation/internal/crossdb/validator.go with
//       this logic implemented using pgx and mongo-driver
// ============================================================================

print("\n--- VALIDATION FUNCTIONS (Go code template) ---\n");

print(`
// JavaScript function to validate cross-DB consistency
// Usage: Call from Go worker using MongoDB driver

/*
func ValidateCrossDB(mongoClient, pgPool) (Report, error) {
    // 1. Get PostgreSQL counts
    pgCounts := getPgCounts(pgPool)

    // 2. Get MongoDB counts
    mongoCounts := getMongoCounts(mongoClient)

    // 3. Compare and generate report
    report := Report{
        Timestamp: time.Now(),
        Databases: make(map[string]DatabaseReport),
    }

    for entity, pgCount := range pgCounts {
        mongoCount := mongoCounts[entity]
        diff := pgCount - mongoCount

        report.Databases[entity] = DatabaseReport{
            PostgreSQL: pgCount,
            MongoDB: mongoCount,
            Difference: diff,
            Status: getStatus(diff),
        }
    }

    return report, nil
}
*/
`);

print("\n" + "=".repeat(80));
print("CROSS-DATABASE RECONCILIATION TEMPLATE COMPLETE");
print("");
print("Next steps:");
print("  1. Run the SQL queries above against PostgreSQL");
print("  2. Compare counts with MongoDB output");
print("  3. Implement Go worker for automated validation");
print("=".repeat(80));
