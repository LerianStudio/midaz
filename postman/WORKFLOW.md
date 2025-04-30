# Midaz API Workflow & Documentation Findings

This document outlines the typical API workflow for setting up entities and posting transactions, based on analysis of the Swagger annotations. It also highlights areas where the documentation (specifically examples) could be improved to better illustrate this flow.

## Identified Workflow

The following sequence represents a common path for creating the necessary entities, retrieving/updating them, and posting a simple transaction. Steps involving retrieval or updates assume a previous step (like Create) has provided the necessary ID.

1.  **Create Organization:**

    - `POST /v1/organizations`
    - **Input:** Organization details (legal name, document, etc.).
    - **Output:** Organization object including `organizationId`.

2.  **Get Organization by ID:**

    - `GET /v1/organizations/{organization_id}`
    - **Input:** Uses `organizationId` from Step 1 in the path.
    - **Output:** Organization object.

3.  **Update Organization:**

    - `PATCH /v1/organizations/{organization_id}`
    - **Input:** Uses `organizationId` from Step 1 in the path. Requires fields to update in the body.
    - **Output:** Updated Organization object.

4.  **List Organizations:**

    - `GET /v1/organizations`
    - **Input:** Optional pagination/filtering parameters.
    - **Output:** List of organization objects.

5.  **Create Ledger:**

    - `POST /v1/organizations/{organization_id}/ledgers`
    - **Input:** Uses `organizationId` from Step 1 in the path. Requires ledger details (name, etc.) in the body.
    - **Output:** Ledger object including `ledgerId`.

6.  **Get Ledger by ID:**

    - `GET /v1/organizations/{organization_id}/ledgers/{ledger_id}`
    - **Input:** Uses `organizationId` (Step 1) and `ledgerId` (Step 5) in the path.
    - **Output:** Ledger object.

7.  **Update Ledger:**

    - `PATCH /v1/organizations/{organization_id}/ledgers/{ledger_id}`
    - **Input:** Uses `organizationId` (Step 1) and `ledgerId` (Step 5) in the path. Requires fields to update in the body.
    - **Output:** Updated Ledger object.

8.  **List Ledgers (within an Org):**

    - `GET /v1/organizations/{organization_id}/ledgers`
    - **Input:** Uses `organizationId` (Step 1) in the path. Optional pagination/filtering.
    - **Output:** List of ledger objects for the specified organization.

9.  **Create Portfolio:**

    - `POST /v1/organizations/{organization_id}/ledgers/{ledger_id}/portfolios`
    - **Input:** Uses `organizationId` (Step 1) and `ledgerId` (Step 5) in the path. Requires portfolio details in the body.
    - **Output:** Portfolio object including `portfolioId`.

10. **Get Portfolio by ID:**

    - `GET /v1/organizations/{organization_id}/ledgers/{ledger_id}/portfolios/{portfolio_id}`
    - **Input:** Uses `organizationId` (Step 1), `ledgerId` (Step 5), and `portfolioId` in the path.
    - **Output:** Portfolio object.

11. **Update Portfolio:**

    - `PATCH /v1/organizations/{organization_id}/ledgers/{ledger_id}/portfolios/{portfolio_id}`
    - **Input:** Uses `organizationId` (Step 1), `ledgerId` (Step 5), and `portfolioId` in the path. Requires fields to update in the body.
    - **Output:** Updated Portfolio object.

12. **List Portfolios (within a Ledger):**

    - `GET /v1/organizations/{organization_id}/ledgers/{ledger_id}/portfolios`
    - **Input:** Uses `organizationId` (Step 1) and `ledgerId` (Step 5) in the path. Optional pagination/filtering.
    - **Output:** List of portfolio objects for the specified ledger.

13. **Create Segment:**

    - `POST /v1/organizations/{organization_id}/ledgers/{ledger_id}/segments`
    - **Input:** Uses `organizationId` (Step 1) and `ledgerId` (Step 5) in the path. Requires segment details in the body.
    - **Output:** Segment object including `segmentId`.

14. **Get Segment by ID:**

    - `GET /v1/organizations/{organization_id}/ledgers/{ledger_id}/segments/{segment_id}`
    - **Input:** Uses `organizationId` (Step 1), `ledgerId` (Step 5), and `segmentId` in the path.
    - **Output:** Segment object.

15. **Update Segment:**

    - `PATCH /v1/organizations/{organization_id}/ledgers/{ledger_id}/segments/{segment_id}`
    - **Input:** Uses `organizationId` (Step 1), `ledgerId` (Step 5), and `segmentId` in the path. Requires fields to update in the body.
    - **Output:** Updated Segment object.

16. **List Segments (within a Ledger):**

    - `GET /v1/organizations/{organization_id}/ledgers/{ledger_id}/segments`
    - **Input:** Uses `organizationId` (Step 1) and `ledgerId` (Step 5) in the path. Optional pagination/filtering.
    - **Output:** List of segment objects for the specified ledger.

17. **Create Asset:**

    - `POST /v1/organizations/{organization_id}/ledgers/{ledger_id}/assets`
    - **Input:** Uses `organizationId` (Step 1) and `ledgerId` (Step 5) in the path. Requires asset details (name, code like "USD", type) in the body.
    - **Output:** Asset object including `assetId` and `code`.

18. **Get Asset by ID:**

    - `GET /v1/organizations/{organization_id}/ledgers/{ledger_id}/assets/{asset_id}`
    - **Input:** Uses `organizationId` (Step 1), `ledgerId` (Step 5), and `assetId` (Step 17) in the path.
    - **Output:** Asset object.

19. **Update Asset:**

    - `PATCH /v1/organizations/{organization_id}/ledgers/{ledger_id}/assets/{asset_id}`
    - **Input:** Uses `organizationId` (Step 1), `ledgerId` (Step 5), and `assetId` (Step 17) in the path. Requires fields to update in the body.
    - **Output:** Updated Asset object.

20. **List Assets (within a Ledger):**

    - `GET /v1/organizations/{organization_id}/ledgers/{ledger_id}/assets`
    - **Input:** Uses `organizationId` (Step 1) and `ledgerId` (Step 5) in the path. Optional pagination/filtering.
    - **Output:** List of asset objects for the specified ledger.

21. **Create Account:**

    - `POST /v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts`
    - **Input:** Uses `organizationId` (Step 1) and `ledgerId` (Step 5) in the path. Requires account details (name, type, `assetCode` from Step 17 like "USD") in the body.
    - **Output:** Account object including `accountId` and `accountAlias`.

22. **Get Account by ID:**

    - `GET /v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts/{account_id}`
    - **Input:** Uses `organizationId` (Step 1), `ledgerId` (Step 5), and an `account_id` (from Step 21) in the path.
    - **Output:** Account object.

23. **Update Account:**

    - `PATCH /v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts/{account_id}`
    - **Input:** Uses `organizationId` (Step 1), `ledgerId` (Step 5), and `account_id` in the path. Requires fields to update in the body.
    - **Output:** Updated Account object.

24. **List Accounts (within a Ledger):**

    - `GET /v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts`
    - **Input:** Uses `organizationId` (Step 1) and `ledgerId` (Step 5) in the path. Optional pagination/filtering.
    - **Output:** List of account objects for the specified ledger.

25. **Create Transaction:**

    - `POST /v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/json`
    - **Input:** Uses `organizationId` (Step 1) and `ledgerId` (Step 5) in the path. Requires transaction details in the body, referencing account aliases, asset details (likely `assetCode`), and amount.
    - **Output:** Transaction object including `transactionId`, `balanceId`, and `operationId`.

26. **Get Transaction by ID:**

    - `GET /v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/{transaction_id}`
    - **Input:** Uses `organizationId` (Step 1), `ledgerId` (Step 5), and `transactionId` (Step 25) in the path.
    - **Output:** Transaction object.

27. **Update Transaction Metadata:**

    - `PATCH /v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/{transaction_id}`
    - **Input:** Uses `organizationId` (Step 1), `ledgerId` (Step 5), and `transactionId` (Step 25) in the path. Allows updating metadata in the body.
    - **Output:** Updated Transaction object.

28. **List Transactions (within a Ledger):**
    - `GET /v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions`
    - **Input:** Uses `organizationId` (Step 1) and `ledgerId` (Step 5) in the path. Optional pagination/filtering (e.g., by date range, metadata).
    - **Output:** List of transaction objects for the specified ledger.

### Transaction Related Endpoints

1.  **Create Transaction:**

    - `POST /v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/json`
    - Submit a new transaction. This will typically generate corresponding operations and potentially update balances.

2.  **Get All Balances:**

    - `GET /v1/organizations/{organization_id}/balances`
    - List all balances across all accounts within the organization.

3.  **Get All Balances by Account ID:**

    - `GET /v1/organizations/{organization_id}/accounts/{account_id}/balances`
    - List all balances associated with a specific account.

4.  **Get Balance by ID:**

    - `GET /v1/organizations/{organization_id}/ledgers/{ledger_id}/balances/{balance_id}`
    - Retrieve a specific balance record by its ID.

5.  **Update Balance Metadata:**

    - `PATCH /v1/organizations/{organization_id}/ledgers/{ledger_id}/balances/{balance_id}`
    - Update the metadata associated with a specific balance.

6.  **Delete Balance by ID:**

    - `DELETE /v1/organizations/{organization_id}/ledgers/{ledger_id}/balances/{balance_id}`
    - _Note: Use with caution. Deleting balances directly might impact financial reporting integrity. This is usually reserved for specific cleanup or correction scenarios._
    - Remove a specific balance record.

7.  **Get All Operations by Account:**

    - `GET /v1/organizations/{organization_id}/accounts/{account_id}/operations`
    - List all operations (debits/credits) associated with a specific account.

8.  **Get Operation by ID:**

    - `GET /v1/organizations/{organization_id}/accounts/{account_id}/operations/{operation_id}`
    - Retrieve details of a specific operation by its ID.

9.  **Update Operation Metadata:**

    - `PATCH /v1/organizations/{organization_id}/accounts/{account_id}/operations/{operation_id}`
    - Update the metadata associated with a specific operation.

10. **Get All Transactions:**
    - `GET /v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions`
    - List all transactions within a ledger.
