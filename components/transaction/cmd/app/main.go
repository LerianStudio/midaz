package main

import (
	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/bootstrap"
)

// @title						Midaz Transaction API
// @version					v1.48.0
// @description				Financial transaction processing API for creating, tracking, and managing ledger transactions.
// @description
// @description				## Overview
// @description				The Transaction API provides endpoints for:
// @description				- **Transactions**: Create and manage double-entry ledger transactions in DSL or JSON format
// @description				- **Operations**: Individual debit/credit entries within transactions
// @description				- **Balances**: Real-time and historical account balance tracking
// @description				- **Asset Rates**: Exchange rates and conversion rates between assets
// @description				- **Transaction Routes**: Configurable routing rules for transaction processing
// @description				- **Operation Routes**: Rules for mapping operations to specific accounts
// @description
// @description				## Transaction Lifecycle
// @description				1. **Create**: Submit transaction via DSL or JSON format
// @description				2. **Pending**: Transaction created but not yet committed
// @description				3. **Commit**: Finalize transaction to update balances
// @description				4. **Cancel**: Abort pending transaction (before commit)
// @description				5. **Revert**: Create reversing transaction for committed transaction
// @description
// @description				## Authentication
// @description				All endpoints require Bearer token authentication. Include your token in the Authorization header:
// @description				```
// @description				Authorization: Bearer <your-token-here>
// @description				```
// @description
// @description				## Transaction Formats
// @description
// @description				### DSL Format (Recommended)
// @description				Human-readable domain-specific language for expressing complex transactions:
// @description				```
// @description				send $100 (
// @description				  from accounts.checking
// @description				  to accounts.savings
// @description				)
// @description				```
// @description
// @description				### JSON Format
// @description				Direct specification of debits and credits:
// @description				```json
// @description				{
// @description				  "operations": [
// @description				    {"type": "debit", "accountId": "...", "amount": "100"},
// @description				    {"type": "credit", "accountId": "...", "amount": "100"}
// @description				  ]
// @description				}
// @description				```
// @description
// @description				## Pagination
// @description				List endpoints support pagination via query parameters:
// @description				- `limit`: Maximum records per page (default: 10, max: 100)
// @description				- `cursor`: Cursor token for fetching next/previous page
// @description				- Responses include pagination metadata with cursors and item count
// @description
// @description				## Rate Limiting
// @description				Rate limiting is enforced at the infrastructure level (API Gateway/Load Balancer).
// @description				Default limits: 500 transactions per minute per organization to ensure system stability.
// @description				Contact your administrator for custom rate limit configuration.
// @description
// @description				## Consistency Guarantees
// @description				- All transactions are ACID-compliant
// @description				- Balance updates are atomic with transaction commits
// @description				- Failed commits automatically rollback all changes
// @description
// @description				## Error Handling
// @description				Transaction validation errors include specific details about which operations failed and why.
// @description				Common transaction errors:
// @description				- Insufficient balance
// @description				- Invalid account state
// @description				- Unbalanced debits and credits
// @description				- Asset mismatch between operations
// @termsOfService				http://swagger.io/terms/
// @contact.name				Discord Community
// @contact.url				https://discord.gg/DnhqKwkGv3
// @license.name				Apache 2.0
// @license.url				http://www.apache.org/licenses/LICENSE-2.0.html
// @host						localhost:3001
// @BasePath					/
// @securityDefinitions.apikey	BearerAuth
// @in							header
// @name						Authorization
// @description				Bearer token authentication. Format: Bearer {token}
func main() {
	libCommons.InitLocalEnvConfig()
	bootstrap.InitServers().Run()
}
