// ============================================================================
// MIDAZ RECONCILIATION SCRIPT: PostgreSQL-MongoDB Metadata Synchronization
// ============================================================================
// Purpose: Verify that all PostgreSQL entities have corresponding MongoDB metadata
// Database: onboarding (MongoDB) + transaction (MongoDB)
// Frequency: Daily
// Expected: All entities should have metadata documents (even if empty)
//
// Usage: mongosh --port 5703 onboarding < 01_metadata_sync.js
// ============================================================================

print("=".repeat(80));
print("MIDAZ METADATA SYNC RECONCILIATION");
print("Timestamp: " + new Date().toISOString());
print("=".repeat(80));

// ============================================================================
// SECTION 1: ONBOARDING DATABASE
// ============================================================================

use("onboarding");

print("\n--- ONBOARDING METADATA ANALYSIS ---\n");

// Collection mapping: MongoDB collection name -> expected entity_name
const onboardingCollections = {
  organization: "Organization",
  ledger: "Ledger",
  asset: "Asset",
  account: "Account",
  portfolio: "Portfolio",
  segment: "Segment",
  accounttype: "AccountType",
  account_type: "AccountType", // Handle both naming conventions
};

const onboardingSummary = {
  check_type: "metadata_sync_onboarding",
  timestamp: new Date(),
  collections: {},
};

for (const [collName, entityName] of Object.entries(onboardingCollections)) {
  const coll = db.getCollection(collName);
  if (!coll) continue;

  const stats = {
    total_documents: coll.countDocuments({ deleted_at: null }),
    empty_metadata: coll.countDocuments({
      deleted_at: null,
      $or: [{ metadata: { $exists: false } }, { metadata: {} }, { metadata: null }],
    }),
    with_metadata: coll.countDocuments({
      deleted_at: null,
      $and: [
        { metadata: { $exists: true } },
        { metadata: { $ne: {} } },
        { metadata: { $ne: null } }
      ]
    }),
  };

  onboardingSummary.collections[collName] = stats;

  print(`Collection: ${collName}`);
  print(`  Total: ${stats.total_documents}`);
  print(`  With metadata: ${stats.with_metadata}`);
  print(`  Empty metadata: ${stats.empty_metadata}`);
  print("");
}

// ============================================================================
// SECTION 2: Check for duplicate entity_ids
// ============================================================================

print("\n--- DUPLICATE ENTITY_ID CHECK ---\n");

for (const [collName] of Object.entries(onboardingCollections)) {
  const coll = db.getCollection(collName);
  if (!coll) continue;

  const duplicates = coll.aggregate([
    { $match: { deleted_at: null } },
    { $group: { _id: "$entity_id", count: { $sum: 1 } } },
    { $match: { count: { $gt: 1 } } },
    { $sort: { count: -1 } },
    { $limit: 10 },
  ]).toArray();

  if (duplicates.length > 0) {
    print(`DUPLICATES in ${collName}:`);
    duplicates.forEach((d) => print(`  entity_id: ${d._id}, count: ${d.count}`));
    onboardingSummary.collections[collName].duplicate_entity_ids = duplicates.length;
  } else {
    onboardingSummary.collections[collName].duplicate_entity_ids = 0;
  }
}

// ============================================================================
// SECTION 3: Check for null/missing entity_ids
// ============================================================================

print("\n--- NULL/MISSING ENTITY_ID CHECK ---\n");

for (const [collName] of Object.entries(onboardingCollections)) {
  const coll = db.getCollection(collName);
  if (!coll) continue;

  const nullEntityIds = coll.countDocuments({
    deleted_at: null,
    $or: [{ entity_id: null }, { entity_id: { $exists: false } }, { entity_id: "" }],
  });

  if (nullEntityIds > 0) {
    print(`NULL entity_ids in ${collName}: ${nullEntityIds}`);
  }

  onboardingSummary.collections[collName].null_entity_ids = nullEntityIds;
}

// ============================================================================
// SECTION 4: TRANSACTION DATABASE
// ============================================================================

use("transaction");

print("\n--- TRANSACTION METADATA ANALYSIS ---\n");

const transactionCollections = {
  transaction: "Transaction",
  operation: "Operation",
  operationroute: "OperationRoute",
  operation_route: "OperationRoute",
  transactionroute: "TransactionRoute",
  transaction_route: "TransactionRoute",
};

const transactionSummary = {
  check_type: "metadata_sync_transaction",
  timestamp: new Date(),
  collections: {},
};

for (const [collName, entityName] of Object.entries(transactionCollections)) {
  const coll = db.getCollection(collName);
  if (!coll) continue;

  const stats = {
    total_documents: coll.countDocuments({ deleted_at: null }),
    empty_metadata: coll.countDocuments({
      deleted_at: null,
      $or: [{ metadata: { $exists: false } }, { metadata: {} }, { metadata: null }],
    }),
    with_metadata: coll.countDocuments({
      deleted_at: null,
      $and: [
        { metadata: { $exists: true } },
        { metadata: { $ne: {} } },
        { metadata: { $ne: null } }
      ]
    }),
  };

  transactionSummary.collections[collName] = stats;

  print(`Collection: ${collName}`);
  print(`  Total: ${stats.total_documents}`);
  print(`  With metadata: ${stats.with_metadata}`);
  print(`  Empty metadata: ${stats.empty_metadata}`);
  print("");
}

// ============================================================================
// SECTION 5: Schema Consistency Check
// ============================================================================

print("\n--- SCHEMA CONSISTENCY CHECK ---\n");

// Check if all documents have required fields
const requiredFields = ["entity_id", "entity_name", "created_at", "updated_at"];

use("onboarding");

for (const [collName] of Object.entries(onboardingCollections)) {
  const coll = db.getCollection(collName);
  if (!coll) continue;

  const missingFields = {};

  for (const field of requiredFields) {
    const count = coll.countDocuments({
      deleted_at: null,
      [field]: { $exists: false },
    });
    if (count > 0) {
      missingFields[field] = count;
    }
  }

  if (Object.keys(missingFields).length > 0) {
    print(`Missing fields in ${collName}:`);
    for (const [field, count] of Object.entries(missingFields)) {
      print(`  ${field}: ${count} documents`);
    }
    onboardingSummary.collections[collName].missing_fields = missingFields;
  }
}

// ============================================================================
// SECTION 6: Soft Delete Consistency
// ============================================================================

print("\n--- SOFT DELETE ANALYSIS ---\n");

for (const [collName] of Object.entries(onboardingCollections)) {
  const coll = db.getCollection(collName);
  if (!coll) continue;

  const deletedCount = coll.countDocuments({
    deleted_at: { $ne: null },
  });

  const totalCount = coll.countDocuments();

  const percentage = totalCount > 0 ? ((deletedCount / totalCount) * 100).toFixed(2) : '0.00';
  print(`${collName}: ${deletedCount} deleted / ${totalCount} total (${percentage}%)`);
}

// ============================================================================
// SECTION 7: Telemetry Output (JSON)
// ============================================================================

print("\n--- TELEMETRY OUTPUT ---\n");

print("Onboarding Summary:");
printjson(onboardingSummary);

print("\nTransaction Summary:");
printjson(transactionSummary);

// ============================================================================
// SECTION 8: Export orphan entity_ids for cross-DB check
// ============================================================================

print("\n--- ENTITY IDS FOR CROSS-DB VALIDATION ---\n");
print("To validate against PostgreSQL, export these entity_ids:");
print("");
print("// Export onboarding entity_ids:");
print("db.account.distinct('entity_id', {deleted_at: null})");
print("db.organization.distinct('entity_id', {deleted_at: null})");
print("");
print("// Then compare with PostgreSQL:");
print("// SELECT id::text FROM account WHERE deleted_at IS NULL;");
print("// Look for IDs in PostgreSQL not in MongoDB = missing metadata");

print("\n" + "=".repeat(80));
print("RECONCILIATION COMPLETE");
print("=".repeat(80));
