# Midaz API Workflow & Documentation Findings

This document outlines the typical API workflow for setting up entities and posting transactions, based on analysis of the Swagger annotations. It also highlights areas where the documentation (specifically examples) could be improved to better illustrate this flow.

## Identified Workflow

The following sequence represents a common path for creating the necessary entities, retrieving/updating them, and posting a simple transaction. Steps involving retrieval or updates assume a previous step (like Create) has provided the necessary ID.

1.  **Create Organization:**
    *   `POST /v1/organizations`
    *   **Input:** Organization details (legal name, document, etc.).
    *   **Output:** Organization object including `organizationId`.

2.  **Get Organization by ID:**
    *   `GET /v1/organizations/{organization_id}`
    *   **Input:** Uses `organizationId` from Step 1 in the path.
    *   **Output:** Organization object.

3.  **Update Organization:**
    *   `PATCH /v1/organizations/{organization_id}`
    *   **Input:** Uses `organizationId` from Step 1 in the path. Requires fields to update in the body.
    *   **Output:** Updated Organization object.

4.  **List Organizations:**
    *   `GET /v1/organizations`
    *   **Input:** Optional pagination/filtering parameters.
    *   **Output:** List of organization objects.

5.  **Create Ledger:**
    *   `POST /v1/organizations/{organization_id}/ledgers`
    *   **Input:** Uses `organizationId` from Step 1 in the path. Requires ledger details (name, etc.) in the body.
    *   **Output:** Ledger object including `ledgerId`.

6.  **Get Ledger by ID:**
    *   `GET /v1/organizations/{organization_id}/ledgers/{ledger_id}`
    *   **Input:** Uses `organizationId` (Step 1) and `ledgerId` (Step 5) in the path.
    *   **Output:** Ledger object.

7.  **Update Ledger:**
    *   `PATCH /v1/organizations/{organization_id}/ledgers/{ledger_id}`
    *   **Input:** Uses `organizationId` (Step 1) and `ledgerId` (Step 5) in the path. Requires fields to update in the body.
    *   **Output:** Updated Ledger object.

8.  **List Ledgers (within an Org):**
    *   `GET /v1/organizations/{organization_id}/ledgers`
    *   **Input:** Uses `organizationId` (Step 1) in the path. Optional pagination/filtering.
    *   **Output:** List of ledger objects for the specified organization.

9.  **Create Portfolio:**
    *   `POST /v1/organizations/{organization_id}/ledgers/{ledger_id}/portfolios`
    *   **Input:** Uses `organizationId` (Step 1) and `ledgerId` (Step 5) in the path. Requires portfolio details in the body.
    *   **Output:** Portfolio object including `portfolioId`.

10. **Get Portfolio by ID:**
    *   `GET /v1/organizations/{organization_id}/ledgers/{ledger_id}/portfolios/{portfolio_id}`
    *   **Input:** Uses `organizationId` (Step 1), `ledgerId` (Step 5), and `portfolioId` in the path.
    *   **Output:** Portfolio object.

11. **Update Portfolio:**
    *   `PATCH /v1/organizations/{organization_id}/ledgers/{ledger_id}/portfolios/{portfolio_id}`
    *   **Input:** Uses `organizationId` (Step 1), `ledgerId` (Step 5), and `portfolioId` in the path. Requires fields to update in the body.
    *   **Output:** Updated Portfolio object.

12. **List Portfolios (within a Ledger):**
    *   `GET /v1/organizations/{organization_id}/ledgers/{ledger_id}/portfolios`
    *   **Input:** Uses `organizationId` (Step 1) and `ledgerId` (Step 5) in the path. Optional pagination/filtering.
    *   **Output:** List of portfolio objects for the specified ledger.

13. **Create Segment:**
    *   `POST /v1/organizations/{organization_id}/ledgers/{ledger_id}/segments`
    *   **Input:** Uses `organizationId` (Step 1) and `ledgerId` (Step 5) in the path. Requires segment details in the body.
    *   **Output:** Segment object including `segmentId`.

14. **Get Segment by ID:**
    *   `GET /v1/organizations/{organization_id}/ledgers/{ledger_id}/segments/{segment_id}`
    *   **Input:** Uses `organizationId` (Step 1), `ledgerId` (Step 5), and `segmentId` in the path.
    *   **Output:** Segment object.

15. **Update Segment:**
    *   `PATCH /v1/organizations/{organization_id}/ledgers/{ledger_id}/segments/{segment_id}`
    *   **Input:** Uses `organizationId` (Step 1), `ledgerId` (Step 5), and `segmentId` in the path. Requires fields to update in the body.
    *   **Output:** Updated Segment object.

16. **List Segments (within a Ledger):**
    *   `GET /v1/organizations/{organization_id}/ledgers/{ledger_id}/segments`
    *   **Input:** Uses `organizationId` (Step 1) and `ledgerId` (Step 5) in the path. Optional pagination/filtering.
    *   **Output:** List of segment objects for the specified ledger.

17. **Create Asset:**
    *   `POST /v1/organizations/{organization_id}/ledgers/{ledger_id}/assets`
    *   **Input:** Uses `organizationId` (Step 1) and `ledgerId` (Step 5) in the path. Requires asset details (name, code like "USD", type) in the body.
    *   **Output:** Asset object including `assetId` and `code`.

18. **Get Asset by ID:**
    *   `GET /v1/organizations/{organization_id}/ledgers/{ledger_id}/assets/{asset_id}`
    *   **Input:** Uses `organizationId` (Step 1), `ledgerId` (Step 5), and `assetId` (Step 17) in the path.
    *   **Output:** Asset object.

19. **Update Asset:**
    *   `PATCH /v1/organizations/{organization_id}/ledgers/{ledger_id}/assets/{asset_id}`
    *   **Input:** Uses `organizationId` (Step 1), `ledgerId` (Step 5), and `assetId` (Step 17) in the path. Requires fields to update in the body.
    *   **Output:** Updated Asset object.

20. **List Assets (within a Ledger):**
    *   `GET /v1/organizations/{organization_id}/ledgers/{ledger_id}/assets`
    *   **Input:** Uses `organizationId` (Step 1) and `ledgerId` (Step 5) in the path. Optional pagination/filtering.
    *   **Output:** List of asset objects for the specified ledger.

21. **Create Source Account:**
    *   `POST /v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts`
    *   **Input:** Uses `organizationId` (Step 1) and `ledgerId` (Step 5) in the path. Requires account details (name, type, `assetCode` from Step 17 like "USD") in the body.
    *   **Output:** Account object including `sourceAccountId`.

22. **Create Destination Account:**
    *   `POST /v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts`
    *   **Input:** Uses `organizationId` (Step 1) and `ledgerId` (Step 5) in the path. Requires account details (name, type, `assetCode` from Step 17 like "USD") in the body.
    *   **Output:** Account object including `destinationAccountId`.

23. **Get Account by ID:**
    *   `GET /v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts/{account_id}`
    *   **Input:** Uses `organizationId` (Step 1), `ledgerId` (Step 5), and an `account_id` (e.g., from Step 21 or 22) in the path.
    *   **Output:** Account object.

24. **Update Account:**
    *   `PATCH /v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts/{account_id}`
    *   **Input:** Uses `organizationId` (Step 1), `ledgerId` (Step 5), and `account_id` in the path. Requires fields to update in the body.
    *   **Output:** Updated Account object.

25. **List Accounts (within a Ledger):**
    *   `GET /v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts`
    *   **Input:** Uses `organizationId` (Step 1) and `ledgerId` (Step 5) in the path. Optional pagination/filtering.
    *   **Output:** List of account objects for the specified ledger.

26. **Create Transaction:**
    *   `POST /v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/json`
    *   **Input:** Uses `organizationId` (Step 1) and `ledgerId` (Step 5) in the path. Requires transaction details in the body, referencing `sourceAccountId` (Step 21), `destinationAccountId` (Step 22), asset details (likely `assetCode`), and amount. (***Note: Exact body structure needs clarification***).
    *   **Output:** Transaction object including `transactionId`.

27. **Commit Transaction (Optional/Unclear):**
    *   `POST /v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/{transaction_id}/commit`
    *   **Input:** Uses `organizationId` (Step 1), `ledgerId` (Step 5), and `transactionId` (Step 26) in the path.
    *   **Output:** Unclear from annotations.

28. **Get Transaction by ID:**
    *   `GET /v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/{transaction_id}`
    *   **Input:** Uses `organizationId` (Step 1), `ledgerId` (Step 5), and `transactionId` (Step 26) in the path.
    *   **Output:** Transaction object.

29. **Update Transaction Metadata:**
    *   `PATCH /v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/{transaction_id}`
    *   **Input:** Uses `organizationId` (Step 1), `ledgerId` (Step 5), and `transactionId` (Step 26) in the path. Allows updating metadata in the body.
    *   **Output:** Updated Transaction object.

30. **List Transactions (within a Ledger):**
    *   `GET /v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions`
    *   **Input:** Uses `organizationId` (Step 1) and `ledgerId` (Step 5) in the path. Optional pagination/filtering (e.g., by date range, metadata).
    *   **Output:** List of transaction objects for the specified ledger.

### Transaction Related Endpoints

1.  **Create AssetRate:**
    *   `POST /v1/organizations/{organization_id}/asset-rates`
    *   Use this endpoint to define the rate for an asset (e.g., BTC/USD). This is often needed before creating transactions involving that asset if its value fluctuates.

2.  **Get AssetRate by External ID:**
    *   `GET /v1/organizations/{organization_id}/asset-rates/external/{external_id}`
    *   Retrieve a specific asset rate using its unique external identifier.

3.  **Get All AssetRates by Asset Code:**
    *   `GET /v1/organizations/{organization_id}/asset-rates/asset/{asset_code}`
    *   List all recorded rates for a given asset code (e.g., all historical BTC rates).

4.  **Create Transaction:**
    *   `POST /v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions`
    *   Submit a new transaction. This will typically generate corresponding operations and potentially update balances.

5.  **Get All Balances:**
    *   `GET /v1/organizations/{organization_id}/balances`
    *   List all balances across all accounts within the organization.

6.  **Get All Balances by Account ID:**
    *   `GET /v1/organizations/{organization_id}/accounts/{account_id}/balances`
    *   List all balances associated with a specific account.

7.  **Get Balance by ID:**
    *   `GET /v1/organizations/{organization_id}/ledgers/{ledger_id}/balances/{balance_id}`
    *   Retrieve a specific balance record by its ID.

8.  **Update Balance Metadata:**
    *   `PATCH /v1/organizations/{organization_id}/ledgers/{ledger_id}/balances/{balance_id}`
    *   Update the metadata associated with a specific balance.

9.  **Delete Balance by ID:**
    *   `DELETE /v1/organizations/{organization_id}/ledgers/{ledger_id}/balances/{balance_id}`
    *   *Note: Use with caution. Deleting balances directly might impact financial reporting integrity. This is usually reserved for specific cleanup or correction scenarios.* 
    *   Remove a specific balance record.

10. **Get All Operations by Account:**
    *   `GET /v1/organizations/{organization_id}/accounts/{account_id}/operations`
    *   List all operations (debits/credits) associated with a specific account.

11. **Get Operation by ID:**
    *   `GET /v1/organizations/{organization_id}/accounts/{account_id}/operations/{operation_id}`
    *   Retrieve details of a specific operation by its ID.

12. **Update Operation Metadata:**
    *   `PATCH /v1/organizations/{organization_id}/accounts/{account_id}/operations/{operation_id}`
    *   Update the metadata associated with a specific operation.

13. **Get All Transactions:**
    *   `GET /v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions`
    *   List all transactions within a ledger.

## Findings & Areas for Improvement

The primary area for improvement lies in providing concrete examples within the Swagger annotations to facilitate understanding and usage, especially for chained workflows:

1.  **Lack of `@Example` Annotations:**
    *   **Issue:** Many critical `POST`, `PATCH`, and `GET` endpoints lack `@Example` tags for request bodies (where applicable) or success responses.
    *   **Impact:**
        *   Developers cannot easily copy/paste example request payloads (especially for `POST`/`PATCH`).
        *   The expected structure of response objects (including the specific IDs needed for subsequent calls) is not explicitly shown.
        *   The *flow* of using an ID from one response in the next request (e.g., using `organizationId` in the Create Ledger path) is not demonstrated visually within the generated documentation (like Swagger UI or Postman).
    *   **Recommendation:** Add `@Example` annotations for both request bodies (`@Param ... body ... example({ "field": "value" })`) and success responses (`@Success ... {object} ... { "id": "uuid-goes-here", ... }`) for all `POST`, `PATCH`, and relevant `GET` endpoints. Examples should use clear placeholder values (e.g., `"YOUR_ORG_ID"`, `"YOUR_LEDGER_ID"`) to illustrate the chaining.

2.  **Ambiguous `CreateTransactionJSON` Body:**
    *   **Issue:** The `@Param transaction body transaction.CreateTransactionSwaggerModel` annotation doesn't clarify the *structure* needed within the JSON body. How should source/destination accounts be specified (ID, alias)? What fields are mandatory (e.g., asset reference, amount)?
    *   **Impact:** Makes it difficult for users to correctly format the transaction creation request.
    *   **Recommendation:** Provide a detailed `@Example` for the `CreateTransactionJSON` request body, showing exactly how to reference accounts, assets, and specify the amount and postings.

3.  **Unclear `CommitTransaction` Role & Response:**
    *   **Issue:** The necessity and outcome of the `POST .../commit` endpoint after using the `POST .../transactions/json` endpoint are unclear. The success response `@Success 201 {object} interface{}` is too vague.
    *   **Impact:** Users don't know if the commit step is required for JSON-based transactions or what to expect upon success.
    *   **Recommendation:** Clarify the relationship between `CreateTransactionJSON` and `CommitTransaction` in the `@Description` of both endpoints. Specify a concrete success response object (even if just `{ "status": "committed" }`) instead of `interface{}` and provide an example. If `CommitTransaction` is *not* needed after `CreateTransactionJSON`, update the descriptions accordingly.

By addressing these points, primarily through the addition of clear, flow-oriented examples in the Swagger annotations, the usability and understandability of the Midaz API for developers will be significantly improved.
