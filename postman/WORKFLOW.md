# Midaz API Workflow -- DO NOT MODIFY THIS FILE (this is used to generate documentation)

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

17. **Create Portfolio**

    - `POST /v1/organizations/{organizationId}/ledgers/{ledgerId}/portfolios`
    - Creates a new portfolio in the ledger
    - **Uses:** `organizationId` from step 1, `ledgerId` from step 5
    - **Output:** `portfolioId`

18. **Get Portfolio**

    - `GET /v1/organizations/{organizationId}/ledgers/{ledgerId}/portfolios/{portfolioId}`
    - Retrieves the portfolio details
    - **Uses:** `organizationId` from step 1, `ledgerId` from step 5, `portfolioId` from step 17

19. **Update Portfolio**

    - `PATCH /v1/organizations/{organizationId}/ledgers/{ledgerId}/portfolios/{portfolioId}`
    - Updates portfolio details
    - **Uses:** `organizationId` from step 1, `ledgerId` from step 5, `portfolioId` from step 17

20. **List Portfolios**

    - `GET /v1/organizations/{organizationId}/ledgers/{ledgerId}/portfolios`
    - Lists all portfolios in the ledger
    - **Uses:** `organizationId` from step 1, `ledgerId` from step 5

21. **Create Segment**

    - `POST /v1/organizations/{organizationId}/ledgers/{ledgerId}/segments`
    - Creates a new segment in the ledger
    - **Uses:** `organizationId` from step 1, `ledgerId` from step 5
    - **Output:** `segmentId`

22. **Get Segment**

    - `GET /v1/organizations/{organizationId}/ledgers/{ledgerId}/segments/{segmentId}`
    - Retrieves the segment details
    - **Uses:** `organizationId` from step 1, `ledgerId` from step 5, `segmentId` from step 21

23. **Update Segment**

    - `PATCH /v1/organizations/{organizationId}/ledgers/{ledgerId}/segments/{segmentId}`
    - Updates segment details
    - **Uses:** `organizationId` from step 1, `ledgerId` from step 5, `segmentId` from step 21

24. **List Segments**

    - `GET /v1/organizations/{organizationId}/ledgers/{ledgerId}/segments`
    - Lists all segments in the ledger
    - **Uses:** `organizationId` from step 1, `ledgerId` from step 5

25. **Count Organizations**

    - `HEAD /v1/organizations/metrics/count`
    - Gets the total count of organizations in the system
    - Returns count in response headers

26. **Count Ledgers**

    - `HEAD /v1/organizations/{organizationId}/ledgers/metrics/count`
    - Gets the total count of ledgers in the organization
    - **Uses:** `organizationId` from step 1

27. **Count Accounts**

    - `HEAD /v1/organizations/{organizationId}/ledgers/{ledgerId}/accounts/metrics/count`
    - Gets the total count of accounts in the ledger
    - **Uses:** `organizationId` from step 1, `ledgerId` from step 5

28. **Count Assets**

    - `HEAD /v1/organizations/{organizationId}/ledgers/{ledgerId}/assets/metrics/count`
    - Gets the total count of assets in the ledger
    - **Uses:** `organizationId` from step 1, `ledgerId` from step 5

29. **Count Portfolios**

    - `HEAD /v1/organizations/{organizationId}/ledgers/{ledgerId}/portfolios/metrics/count`
    - Gets the total count of portfolios in the ledger
    - **Uses:** `organizationId` from step 1, `ledgerId` from step 5

30. **Count Segments**

    - `HEAD /v1/organizations/{organizationId}/ledgers/{ledgerId}/segments/metrics/count`
    - Gets the total count of segments in the ledger
    - **Uses:** `organizationId` from step 1, `ledgerId` from step 5

31. **Get Account by Alias**

    - `GET /v1/organizations/{organizationId}/ledgers/{ledgerId}/accounts/alias/{alias}`
    - Retrieves an account using its alias
    - **Uses:** `organizationId` from step 1, `ledgerId` from step 5, `accountAlias` from step 13

32. **Get Account by External Code**

    - `GET /v1/organizations/{organizationId}/ledgers/{ledgerId}/accounts/external/{code}`
    - Retrieves an account using external code
    - **Uses:** `organizationId` from step 1, `ledgerId` from step 5

33. **Create Transaction**

    - `POST /v1/organizations/{organizationId}/ledgers/{ledgerId}/transactions/json`
    - Creates a new transaction in the ledger
    - **Uses:** `organizationId` from step 1, `ledgerId` from step 5, `accountAlias` from step 13
    - **Output:** `transactionId`, `balanceId`, `operationId`

34. **Create Transaction (Inflow)**

    - `POST /v1/organizations/{organizationId}/ledgers/{ledgerId}/transactions/inflow`
    - Creates an inflow transaction (money coming in)
    - **Uses:** `organizationId` from step 1, `ledgerId` from step 5, `accountAlias` from step 13
    - **Output:** `inflowTransactionId`

35. **Create Transaction (Outflow)**

    - `POST /v1/organizations/{organizationId}/ledgers/{ledgerId}/transactions/outflow`
    - Creates an outflow transaction (money going out)
    - **Uses:** `organizationId` from step 1, `ledgerId` from step 5, `accountAlias` from step 13
    - **Output:** `outflowTransactionId`

36. **Get Transaction**

    - `GET /v1/organizations/{organizationId}/ledgers/{ledgerId}/transactions/{transactionId}`
    - Retrieves the transaction details
    - **Uses:** `organizationId` from step 1, `ledgerId` from step 5, `transactionId` from step 33

37. **Update Transaction**

    - `PATCH /v1/organizations/{organizationId}/ledgers/{ledgerId}/transactions/{transactionId}`
    - Updates transaction metadata
    - **Uses:** `organizationId` from step 1, `ledgerId` from step 5, `transactionId` from step 33

38. **List Transactions**

    - `GET /v1/organizations/{organizationId}/ledgers/{ledgerId}/transactions`
    - Lists all transactions in the ledger
    - **Uses:** `organizationId` from step 1, `ledgerId` from step 5

39. **Get Operation**

    - `GET /v1/organizations/{organizationId}/ledgers/{ledgerId}/accounts/{accountId}/operations/{operationId}`
    - Retrieves the operation details
    - **Uses:** `organizationId` from step 1, `ledgerId` from step 5, `accountId` from step 13, `operationId` from step 33

40. **List Operations by Account**

    - `GET /v1/organizations/{organizationId}/ledgers/{ledgerId}/accounts/{accountId}/operations`
    - Lists all operations for a specific account
    - **Uses:** `organizationId` from step 1, `ledgerId` from step 5, `accountId` from step 13

41. **Update Operation Metadata**

    - `PATCH /v1/organizations/{organizationId}/ledgers/{ledgerId}/transactions/{transactionId}/operations/{operationId}`
    - Updates operation metadata
    - **Uses:** `organizationId` from step 1, `ledgerId` from step 5, `transactionId` from step 33, `operationId` from step 33

42. **Get Balance**

    - `GET /v1/organizations/{organizationId}/ledgers/{ledgerId}/balances/{balanceId}`
    - Retrieves the balance details
    - **Uses:** `organizationId` from step 1, `ledgerId` from step 5, `balanceId` from step 33

43. **List Balances by Account**

    - `GET /v1/organizations/{organizationId}/ledgers/{ledgerId}/accounts/{accountId}/balances`
    - Lists all balances for a specific account
    - **Uses:** `organizationId` from step 1, `ledgerId` from step 5, `accountId` from step 13

44. **List Balances by Account Alias**

    - `GET /v1/organizations/{organizationId}/ledgers/{ledgerId}/accounts/alias/{alias}/balances`
    - Lists all balances for an account identified by alias
    - **Uses:** `organizationId` from step 1, `ledgerId` from step 5, `accountAlias` from step 13

45. **List Balances by External Code**

    - `GET /v1/organizations/{organizationId}/ledgers/{ledgerId}/accounts/external/{code}/balances`
    - Lists all balances for an account identified by external code
    - **Uses:** `organizationId` from step 1, `ledgerId` from step 5

46. **Update Balance**

    - `PATCH /v1/organizations/{organizationId}/ledgers/{ledgerId}/balances/{balanceId}`
    - Updates balance metadata
    - **Uses:** `organizationId` from step 1, `ledgerId` from step 5, `balanceId` from step 33

47. **List All Balances**

    - `GET /v1/organizations/{organizationId}/ledgers/{ledgerId}/balances`
    - Lists all balances in the ledger
    - **Uses:** `organizationId` from step 1, `ledgerId` from step 5

48. **Check Account Balance Before Zeroing**

    - `GET /v1/organizations/{organizationId}/ledgers/{ledgerId}/accounts/alias/{alias}/balances`
    - Gets the current balance of the account before zeroing it out
    - **Uses:** `organizationId` from step 1, `ledgerId` from step 5, `accountAlias` from step 13
    - **Output:** `currentBalanceAmount`, `currentBalanceScale`

49. **Zero Out Balance**

    - `POST /v1/organizations/{organizationId}/ledgers/{ledgerId}/transactions/json`
    - Creates a reverse transaction to zero out the account balance using the actual current balance
    - **Uses:** `organizationId` from step 1, `ledgerId` from step 5, `accountAlias` from step 13, `currentBalanceAmount` from step 48, `currentBalanceScale` from step 48
    - **Description:** Creates a transaction that transfers the current balance amount from account to external/USD, completely zeroing the balance

50. **Delete Balance**

    - `DELETE /v1/organizations/{organizationId}/ledgers/{ledgerId}/balances/{balanceId}`
    - Deletes a balance
    - **Uses:** `organizationId` from step 1, `ledgerId` from step 5, `balanceId` from step 33

51. **Delete Segment**

    - `DELETE /v1/organizations/{organizationId}/ledgers/{ledgerId}/segments/{segmentId}`
    - Deletes a segment
    - **Uses:** `organizationId` from step 1, `ledgerId` from step 5, `segmentId` from step 21

52. **Delete Portfolio**

    - `DELETE /v1/organizations/{organizationId}/ledgers/{ledgerId}/portfolios/{portfolioId}`
    - Deletes a portfolio
    - **Uses:** `organizationId` from step 1, `ledgerId` from step 5, `portfolioId` from step 17

53. **Delete Account**

    - `DELETE /v1/organizations/{organizationId}/ledgers/{ledgerId}/accounts/{accountId}`
    - Deletes an account
    - **Uses:** `organizationId` from step 1, `ledgerId` from step 5, `accountId` from step 13

54. **Delete Asset**

    - `DELETE /v1/organizations/{organizationId}/ledgers/{ledgerId}/assets/{assetId}`
    - Deletes an asset
    - **Uses:** `organizationId` from step 1, `ledgerId` from step 5, `assetId` from step 9

55. **Delete Ledger**

    - `DELETE /v1/organizations/{organizationId}/ledgers/{ledgerId}`
    - Deletes a ledger
    - **Uses:** `organizationId` from step 1, `ledgerId` from step 5

56. **Delete Organization**

    - `DELETE /v1/organizations/{organizationId}`
    - Deletes an organization
    - **Uses:** `organizationId` from step 1

## Notes

- This workflow provides a comprehensive test of **ALL API endpoints** across both onboarding and transaction services.
- **Complete Coverage**: Now includes 56 steps covering implemented API endpoints plus business flow variations.
- **Enhanced Sections Added**:
  - **Metrics Endpoints** (Steps 25-30): All count/metrics operations for monitoring
  - **Alternative Access** (Steps 31-32): Account lookup by alias and external code  
  - **Transaction Variants** (Steps 34-35): Inflow and outflow transaction types
  - **Extended Balance Operations** (Steps 44-45): Balance queries by alias/external code
- **Dependency Chain Maintained**: Each step builds on previous steps, using IDs and resources created earlier.
- **Logical Sequence**: Organization → Ledger → Assets/Accounts → Metrics → Alternative Access → Transactions → Transaction Variants → Operations → Balances → Cleanup.
- **Referential Integrity**: The cleanup sequence follows the reverse order to maintain referential integrity.
- **CI/CD Ready**: This comprehensive workflow ensures complete API regression testing in automated pipelines.
- **Environment Variables**: Can be automated in Postman by using environment variables to store and pass the IDs between requests.
