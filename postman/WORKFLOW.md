# Midaz API Workflow

This document outlines a complete linear workflow for testing all the main endpoints of the Midaz API. Each step builds on the previous ones, creating a comprehensive test sequence.

## Complete Linear Test Sequence

1. **Create Organization**

   - `POST /v1/organizations`
   - Creates a new organization in the system
   - **Output:** `organizationId`

2. **Get Organization**

   - `GET /v1/organizations/{organizationId}`
   - Retrieves the organization details
   - **Uses:** `organizationId` from step 1

3. **Update Organization**

   - `PATCH /v1/organizations/{organizationId}`
   - Updates organization details
   - **Uses:** `organizationId` from step 1

4. **List Organizations**

   - `GET /v1/organizations`
   - Lists all organizations

5. **Create Ledger**

   - `POST /v1/organizations/{organizationId}/ledgers`
   - Creates a new ledger within the organization
   - **Uses:** `organizationId` from step 1
   - **Output:** `ledgerId`

6. **Get Ledger**

   - `GET /v1/organizations/{organizationId}/ledgers/{ledgerId}`
   - Retrieves the ledger details
   - **Uses:** `organizationId` from step 1, `ledgerId` from step 5

7. **Update Ledger**

   - `PATCH /v1/organizations/{organizationId}/ledgers/{ledgerId}`
   - Updates ledger details
   - **Uses:** `organizationId` from step 1, `ledgerId` from step 5

8. **List Ledgers**

   - `GET /v1/organizations/{organizationId}/ledgers`
   - Lists all ledgers in the organization
   - **Uses:** `organizationId` from step 1

9. **Create Asset**

   - `POST /v1/organizations/{organizationId}/ledgers/{ledgerId}/assets`
   - Creates a new asset (e.g., USD) in the ledger
   - **Uses:** `organizationId` from step 1, `ledgerId` from step 5
   - **Output:** `assetId`

10. **Get Asset**

    - `GET /v1/organizations/{organizationId}/ledgers/{ledgerId}/assets/{assetId}`
    - Retrieves the asset details
    - **Uses:** `organizationId` from step 1, `ledgerId` from step 5, `assetId` from step 9

11. **Update Asset**

    - `PATCH /v1/organizations/{organizationId}/ledgers/{ledgerId}/assets/{assetId}`
    - Updates asset details
    - **Uses:** `organizationId` from step 1, `ledgerId` from step 5, `assetId` from step 9

12. **List Assets**

    - `GET /v1/organizations/{organizationId}/ledgers/{ledgerId}/assets`
    - Lists all assets in the ledger
    - **Uses:** `organizationId` from step 1, `ledgerId` from step 5

13. **Create Account**

    - `POST /v1/organizations/{organizationId}/ledgers/{ledgerId}/accounts`
    - Creates a new account in the ledger
    - **Uses:** `organizationId` from step 1, `ledgerId` from step 5
    - **Output:** `accountId`, `accountAlias`

14. **Get Account**

    - `GET /v1/organizations/{organizationId}/ledgers/{ledgerId}/accounts/{accountId}`
    - Retrieves the account details
    - **Uses:** `organizationId` from step 1, `ledgerId` from step 5, `accountId` from step 13

15. **Update Account**

    - `PATCH /v1/organizations/{organizationId}/ledgers/{ledgerId}/accounts/{accountId}`
    - Updates account details
    - **Uses:** `organizationId` from step 1, `ledgerId` from step 5, `accountId` from step 13

16. **List Accounts**

    - `GET /v1/organizations/{organizationId}/ledgers/{ledgerId}/accounts`
    - Lists all accounts in the ledger
    - **Uses:** `organizationId` from step 1, `ledgerId` from step 5

17. **Create Asset Rate**

    - `POST /v1/organizations/{organizationId}/ledgers/{ledgerId}/asset-rates`
    - Creates a new asset exchange rate
    - **Uses:** `organizationId` from step 1, `ledgerId` from step 5
    - **Output:** `assetRateId`

18. **Get Asset Rate**

    - `GET /v1/organizations/{organizationId}/ledgers/{ledgerId}/asset-rates/{assetRateId}`
    - Retrieves the asset rate details
    - **Uses:** `organizationId` from step 1, `ledgerId` from step 5, `assetRateId` from step 17

19. **Create Portfolio**

    - `POST /v1/organizations/{organizationId}/ledgers/{ledgerId}/portfolios`
    - Creates a new portfolio in the ledger
    - **Uses:** `organizationId` from step 1, `ledgerId` from step 5
    - **Output:** `portfolioId`

20. **Get Portfolio**

    - `GET /v1/organizations/{organizationId}/ledgers/{ledgerId}/portfolios/{portfolioId}`
    - Retrieves the portfolio details
    - **Uses:** `organizationId` from step 1, `ledgerId` from step 5, `portfolioId` from step 19

21. **Update Portfolio**

    - `PATCH /v1/organizations/{organizationId}/ledgers/{ledgerId}/portfolios/{portfolioId}`
    - Updates portfolio details
    - **Uses:** `organizationId` from step 1, `ledgerId` from step 5, `portfolioId` from step 19

22. **List Portfolios**

    - `GET /v1/organizations/{organizationId}/ledgers/{ledgerId}/portfolios`
    - Lists all portfolios in the ledger
    - **Uses:** `organizationId` from step 1, `ledgerId` from step 5

23. **Create Segment**

    - `POST /v1/organizations/{organizationId}/ledgers/{ledgerId}/segments`
    - Creates a new segment in the ledger
    - **Uses:** `organizationId` from step 1, `ledgerId` from step 5
    - **Output:** `segmentId`

24. **Get Segment**

    - `GET /v1/organizations/{organizationId}/ledgers/{ledgerId}/segments/{segmentId}`
    - Retrieves the segment details
    - **Uses:** `organizationId` from step 1, `ledgerId` from step 5, `segmentId` from step 23

25. **Update Segment**

    - `PATCH /v1/organizations/{organizationId}/ledgers/{ledgerId}/segments/{segmentId}`
    - Updates segment details
    - **Uses:** `organizationId` from step 1, `ledgerId` from step 5, `segmentId` from step 23

26. **List Segments**

    - `GET /v1/organizations/{organizationId}/ledgers/{ledgerId}/segments`
    - Lists all segments in the ledger
    - **Uses:** `organizationId` from step 1, `ledgerId` from step 5

27. **Create Transaction**

    - `POST /v1/organizations/{organizationId}/ledgers/{ledgerId}/transactions/json`
    - Creates a new transaction in the ledger
    - **Uses:** `organizationId` from step 1, `ledgerId` from step 5, `accountAlias` from step 13
    - **Output:** `transactionId`, `balanceId`, `operationId`

28. **Get Transaction**

    - `GET /v1/organizations/{organizationId}/ledgers/{ledgerId}/transactions/{transactionId}`
    - Retrieves the transaction details
    - **Uses:** `organizationId` from step 1, `ledgerId` from step 5, `transactionId` from step 27

29. **Update Transaction**

    - `PATCH /v1/organizations/{organizationId}/ledgers/{ledgerId}/transactions/{transactionId}`
    - Updates transaction metadata
    - **Uses:** `organizationId` from step 1, `ledgerId` from step 5, `transactionId` from step 27

30. **List Transactions**

    - `GET /v1/organizations/{organizationId}/ledgers/{ledgerId}/transactions`
    - Lists all transactions in the ledger
    - **Uses:** `organizationId` from step 1, `ledgerId` from step 5

31. **Get Operation**

    - `GET /v1/organizations/{organizationId}/ledgers/{ledgerId}/operations/{operationId}`
    - Retrieves the operation details
    - **Uses:** `organizationId` from step 1, `ledgerId` from step 5, `operationId` from step 27

32. **List Operations by Account**

    - `GET /v1/organizations/{organizationId}/accounts/{accountId}/operations`
    - Lists all operations for an account
    - **Uses:** `organizationId` from step 1, `accountId` from step 13

33. **Update Operation Metadata**

    - `PATCH /v1/organizations/{organizationId}/accounts/{accountId}/operations/{operationId}`
    - Updates operation metadata
    - **Uses:** `organizationId` from step 1, `accountId` from step 13, `operationId` from step 27

34. **Get Balance**

    - `GET /v1/organizations/{organizationId}/ledgers/{ledgerId}/balances/{balanceId}`
    - Retrieves the balance details
    - **Uses:** `organizationId` from step 1, `ledgerId` from step 5, `balanceId` from step 27

35. **List Balances by Account**

    - `GET /v1/organizations/{organizationId}/accounts/{accountId}/balances`
    - Lists all balances for an account
    - **Uses:** `organizationId` from step 1, `accountId` from step 13

36. **Update Balance Metadata**

    - `PATCH /v1/organizations/{organizationId}/ledgers/{ledgerId}/balances/{balanceId}`
    - Updates balance metadata
    - **Uses:** `organizationId` from step 1, `ledgerId` from step 5, `balanceId` from step 27

37. **List All Balances**
    - `GET /v1/organizations/{organizationId}/balances`
    - Lists all balances across the organization
    - **Uses:** `organizationId` from step 1

## Notes

- This workflow provides a comprehensive test of all major API endpoints in a logical sequence.
- Each step builds on previous steps, using IDs and resources created earlier.
- The sequence follows the natural hierarchy: Organization → Ledger → Assets/Accounts → Transactions → Operations/Balances.
- This workflow can be automated in Postman by using environment variables to store and pass the IDs between requests.
