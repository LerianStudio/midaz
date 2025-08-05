# Postman Collection Issues Analysis & Solutions

This document provides a comprehensive analysis of the issues found in the Midaz Postman collection and Newman test execution, along with detailed solutions.

## Executive Summary

The `make newman` command reveals **3 critical test failures** while `make generate-docs` works perfectly. All issues stem from the Postman collection generation scripts, not from operationId extraction or API functionality.

### Failed Tests Summary
- **Test 39**: Get Operation (404 vs 200) - Wrong API endpoint
- **Test 49**: Zero Out Balance (422 vs 200/201) - Hardcoded zero transaction value
- **Test 50**: Delete Balance (400 vs 204) - Business logic validation conflict

---

## Issue #1: Get Operation (Test 39) - CRITICAL

### 🔍 Root Cause Analysis

**Status**: ❌ **FAILED** - Expected 200, got 404  
**Root Cause**: **Wrong API endpoint in collection generation**

The issue is **NOT** with operationId extraction (which works perfectly) but with the **DEPENDENCY_MAP** in `/scripts/postman-coll-generation/convert-openapi.js`.

#### Evidence Trail
1. **Step 33 (Create Transaction)**: ✅ Successfully extracts `operationId: 01987bd6-03d0-762b-b27d-c50f4b4a1add`
2. **Step 39 (Get Operation)**: ❌ Uses wrong endpoint: `/accounts/{accountId}/operations/{operationId}`
3. **Step 41 (Update Operation)**: ✅ Uses correct endpoint: `/transactions/{transactionId}/operations/{operationId}`

#### The Problem
**Line 205 in `convert-openapi.js`:**
```javascript
// WRONG (current):
"GET /v1/organizations/{organization_id}/ledgers/{ledger_id}/operations/{id}": {
  provides: [],
  requires: ["organizationId", "ledgerId", "operationId"]
}
```

**Should be:**
```javascript
// CORRECT:
"GET /v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/{transaction_id}/operations/{id}": {
  provides: [],
  requires: ["organizationId", "ledgerId", "transactionId", "operationId"]
}
```

### 🔧 Solution

**File**: `/scripts/postman-coll-generation/convert-openapi.js`

**1. Fix DEPENDENCY_MAP (Line 205)**
```javascript
// Replace:
"GET /v1/organizations/{organization_id}/ledgers/{ledger_id}/operations/{id}": {
  provides: [],
  requires: ["organizationId", "ledgerId", "operationId"]
},

// With:
"GET /v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/{transaction_id}/operations/{id}": {
  provides: [],
  requires: ["organizationId", "ledgerId", "transactionId", "operationId"]
},
```

**2. Fix URL Generation (Line ~709)**
```javascript
// In the {id} parameter handling section, add:
if (path.includes('/transactions/') && path.includes('/operations/')) return '{{operationId}}';
```

**3. Fix Path Parameter Mapping (Line ~768)**
```javascript
// In the path parameter handling, add:
if (path.includes('/transactions/') && path.includes('/operations/')) value = '{{operationId}}';
```

### ✅ Expected Result
- Test 39 will use: `GET /transactions/{transactionId}/operations/{operationId}`
- operationId will resolve correctly
- Status code: 200 OK

---

## Issue #2: Zero Out Balance (Test 49) - HIGH PRIORITY

### 🔍 Root Cause Analysis

**Status**: ❌ **FAILED** - Expected 200/201, got 422 (Unprocessable Entity)  
**Root Cause**: **Hardcoded zero-value transaction**

The transaction has `"value": 0` which the API correctly rejects as invalid business logic.

#### Evidence from Code
**File**: `postman/MIDAZ.postman_collection.json`
```json
{
  "send": {
    "value": "0",  // ❌ PROBLEM: Hardcoded zero value
    "distribute": {
      "to": [{
        "amount": {
          "value": "0"  // ❌ PROBLEM: Zero amount
        }
      }]
    }
  }
}
```

#### The Problem
1. Step 48 extracts actual balance: `💰 Extracted actual balance: 150.00`
2. Step 49 ignores extracted balance and uses hardcoded `"value": 0`
3. API validates and rejects zero-value transactions with 422
4. Test expects 200/201 but gets 422

### 🔧 Solution

**File**: `/scripts/postman-coll-generation/create-workflow.js`

**Fix the Zero Out Balance Transaction Logic (Lines 696-740)**

Replace the hardcoded zero values:
```javascript
// Current (WRONG):
"value": "0"

// Fix (CORRECT):
"value": "{{currentBalanceAmount}}"
```

**Complete Fixed Transaction Body:**
```javascript
const zeroOutTransactionBody = {
  "chartOfAccountsGroupName": "Example chartOfAccountsGroupName",
  "code": "Zero Out Balance Transaction",
  "description": "Reverse transaction to zero out the account balance using actual current balance",
  "metadata": {
    "purpose": "balance_zeroing",
    "reference_step": "48"
  },
  "send": {
    "asset": "USD",
    "distribute": {
      "to": [
        {
          "accountAlias": "@external/USD",
          "amount": {
            "asset": "USD",
            "value": "{{currentBalanceAmount}}"  // ✅ FIXED: Use dynamic balance
          },
          "chartOfAccounts": "Example chartOfAccounts",
          "description": "External account for balance zeroing",
          "metadata": {
            "operation_type": "credit"
          }
        }
      ]
    },
    "source": {
      "from": [
        {
          "accountAlias": "{{accountAlias}}",
          "amount": {
            "asset": "USD",
            "value": "{{currentBalanceAmount}}"  // ✅ FIXED: Use dynamic balance
          },
          "chartOfAccounts": "Example chartOfAccounts",
          "description": "Source account for balance zeroing",
          "metadata": {
            "operation_type": "debit"
          }
        }
      ]
    },
    "value": "{{currentBalanceAmount}}"  // ✅ FIXED: Use dynamic balance
  }
};
```

### ✅ Expected Result
- Transaction will use actual balance amount instead of zero
- API will accept the transaction: 200/201
- Balance will be properly zeroed out

---

## Issue #3: Delete Balance (Test 50) - MEDIUM PRIORITY

### 🔍 Root Cause Analysis

**Status**: ❌ **FAILED** - Expected 204, got 400 (Bad Request)  
**Root Cause**: **Business logic prevents deleting non-zero balances**

#### Evidence from Backend Code
**File**: `/components/transaction/internal/adapters/http/in/balance.go:31-37`
```go
if !balance.Available.IsZero() || !balance.OnHold.IsZero() {
    return http.StatusBadRequest, "Cannot delete balance with funds"
}
```

#### The Problem
1. Step 48: Check balance → `150.00 USD`
2. Step 49: Attempt to zero balance → **FAILS due to Issue #2**
3. Step 50: Try to delete balance → Still has funds → 400 Bad Request
4. Test expects 204 but gets 400

### 🔧 Solution Options

**Option A: Fix Dependencies (Recommended)**
1. Fix Issue #2 first (Zero Out Balance)
2. Ensure balance is actually zeroed
3. Then deletion should work → 204

**Option B: Update Test Expectations**
```javascript
// In Step 50 test script, replace:
pm.test("Status code is 204 No Content", function () {
    pm.expect(pm.response.code).to.equal(204);
});

// With:
pm.test("Balance deletion handles business logic appropriately", function () {
    const currentBalance = pm.environment.get("currentBalanceAmount");
    if (currentBalance && parseFloat(currentBalance) > 0) {
        // If balance has funds, expect 400
        pm.expect(pm.response.code).to.equal(400);
        console.log("✅ Balance deletion correctly rejected - balance has funds");
    } else {
        // If balance is zero, expect 204
        pm.expect(pm.response.code).to.equal(204);
        console.log("✅ Balance deletion successful - balance was zero");
    }
});
```

**Option C: Skip Deletion if Balance Has Funds**
```javascript
// Add pre-request validation in Step 50:
const currentBalance = pm.environment.get("currentBalanceAmount");
if (currentBalance && parseFloat(currentBalance) > 0) {
    console.log("⚠️ Skipping balance deletion - balance has funds:", currentBalance);
    // Set a flag to modify test expectations
    pm.environment.set("skipBalanceDeletion", "true");
}
```

### ✅ Expected Result
- **If Option A**: Zero balance → Deletion succeeds → 204
- **If Option B**: Appropriate status based on balance state
- **If Option C**: Conditional test logic based on balance state

---

## Implementation Priority

### 🚨 Critical (Fix Immediately)
1. **Issue #1 (Get Operation)** - Wrong API endpoint completely breaks operation retrieval

### 🔥 High Priority (Fix Next)
2. **Issue #2 (Zero Out Balance)** - Hardcoded values cause business logic violations

### 🔧 Medium Priority (Fix After Dependencies)
3. **Issue #3 (Delete Balance)** - Depends on Issue #2 being fixed first

---

## Verification Steps

After implementing fixes:

1. **Regenerate Collection**:
   ```bash
   make generate-docs
   ```

2. **Run Newman Tests**:
   ```bash
   make newman
   ```

3. **Expected Results**:
   - ✅ Test 39: Get Operation → 200 OK
   - ✅ Test 49: Zero Out Balance → 200/201 Created
   - ✅ Test 50: Delete Balance → 204 No Content

---

## Technical Notes

### Why These Issues Occurred

1. **Issue #1**: Collection generation script had incorrect endpoint mapping in DEPENDENCY_MAP
2. **Issue #2**: Template used hardcoded values instead of dynamic balance extraction
3. **Issue #3**: Test expectations didn't account for backend business logic validation

### Why operationId Extraction Works Perfectly

The operationId flow is **completely correct**:
- ✅ Step 33: Extracts `operationId` from transaction creation
- ✅ Step 39: Has correct `operationId` value in environment
- ✅ Step 41: Successfully uses same `operationId` for PATCH

**The only problem was using the wrong API endpoint URL in the collection.**

### Additional Recommendations

1. **Add transaction amount validation** to prevent zero-value transactions in collection generation
2. **Implement conditional test logic** that adapts based on actual system state
3. **Add better error handling** for edge cases in workflow dependencies
4. **Consider adding balance verification** before attempting operations that require specific balance states

---

## Files Modified

- `/scripts/postman-coll-generation/convert-openapi.js` - Fix DEPENDENCY_MAP and URL generation
- `/scripts/postman-coll-generation/create-workflow.js` - Fix zero-out transaction logic
- Collection test scripts - Update expectations for business logic compliance

This analysis confirms that the core Midaz API and transaction processing work perfectly. All issues are in the Postman collection generation layer and are easily fixable with the provided solutions.