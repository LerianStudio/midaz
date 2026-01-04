package crossdb

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/LerianStudio/midaz/v3/components/reconciliation/internal/adapters/postgres"
	"github.com/LerianStudio/midaz/v3/components/reconciliation/internal/domain"
	"github.com/lib/pq"
)

const (
	defaultCrossDBBatchSize = 200
	defaultCrossDBMaxScan   = 5000
)

// CrossDBChecker validates transaction references against onboarding records.
type CrossDBChecker struct {
	onboardingDB  *sql.DB
	transactionDB *sql.DB
}

// NewCrossDBChecker creates a new CrossDB checker.
func NewCrossDBChecker(onboardingDB, transactionDB *sql.DB) *CrossDBChecker {
	return &CrossDBChecker{
		onboardingDB:  onboardingDB,
		transactionDB: transactionDB,
	}
}

// Name returns the unique name of this checker.
func (c *CrossDBChecker) Name() string {
	return postgres.CheckerNameCrossDB
}

// Check validates that transaction references exist in onboarding DB.
func (c *CrossDBChecker) Check(ctx context.Context, config postgres.CheckerConfig) (postgres.CheckResult, error) {
	if c.onboardingDB == nil || c.transactionDB == nil {
		return &domain.CrossDBCheckResult{Status: domain.StatusSkipped}, nil
	}

	batchSize := config.CrossDBBatchSize
	if batchSize <= 0 {
		batchSize = defaultCrossDBBatchSize
	}

	maxScan := config.CrossDBMaxScan
	if maxScan <= 0 {
		maxScan = defaultCrossDBMaxScan
	}

	result := &domain.CrossDBCheckResult{}

	missingAccounts, accountSamples, scannedAccounts, err := c.checkMissingIDs(ctx,
		`
		SELECT DISTINCT account_id::text as id
		FROM (
			SELECT account_id FROM balance WHERE deleted_at IS NULL
			UNION
			SELECT account_id FROM operation WHERE deleted_at IS NULL
		) ids
		ORDER BY id
		`, "account", batchSize, maxScan, config.MaxResults,
	)
	if err != nil {
		return nil, err
	}

	result.MissingAccounts = missingAccounts

	missingLedgers, ledgerSamples, scannedLedgers, err := c.checkMissingIDs(ctx,
		`
		SELECT DISTINCT ledger_id::text as id
		FROM (
			SELECT ledger_id FROM balance WHERE deleted_at IS NULL
			UNION
			SELECT ledger_id FROM operation WHERE deleted_at IS NULL
			UNION
			SELECT ledger_id FROM transaction WHERE deleted_at IS NULL
		) ids
		ORDER BY id
		`, "ledger", batchSize, maxScan, config.MaxResults,
	)
	if err != nil {
		return nil, err
	}

	result.MissingLedgers = missingLedgers

	missingOrgs, orgSamples, scannedOrgs, err := c.checkMissingIDs(ctx,
		`
		SELECT DISTINCT organization_id::text as id
		FROM (
			SELECT organization_id FROM balance WHERE deleted_at IS NULL
			UNION
			SELECT organization_id FROM operation WHERE deleted_at IS NULL
			UNION
			SELECT organization_id FROM transaction WHERE deleted_at IS NULL
		) ids
		ORDER BY id
		`, "organization", batchSize, maxScan, config.MaxResults,
	)
	if err != nil {
		return nil, err
	}

	result.MissingOrganizations = missingOrgs

	result.Samples = append(result.Samples, accountSamples...)
	result.Samples = append(result.Samples, ledgerSamples...)
	result.Samples = append(result.Samples, orgSamples...)
	result.ScanLimited = scannedAccounts >= maxScan || scannedLedgers >= maxScan || scannedOrgs >= maxScan

	issueCount := result.MissingAccounts + result.MissingLedgers + result.MissingOrganizations
	result.Status = postgres.DetermineStatus(issueCount, postgres.StatusThresholds{
		WarningThreshold:          10,
		WarningThresholdExclusive: true,
	})

	if result.MissingOrganizations > 0 {
		result.Status = domain.StatusCritical
	}

	return result, nil
}

func (c *CrossDBChecker) checkMissingIDs(
	ctx context.Context,
	query, entityType string,
	batchSize, maxScan, maxResults int,
) (int, []domain.CrossDBMissing, int, error) {
	offset := 0
	missingCount := 0
	scanned := 0
	samples := make([]domain.CrossDBMissing, 0, maxResults)

	for scanned < maxScan {
		rows, err := c.transactionDB.QueryContext(ctx, query+" LIMIT $1 OFFSET $2", batchSize, offset)
		if err != nil {
			return 0, nil, scanned, fmt.Errorf("crossdb query failed: %w", err)
		}

		var ids []string

		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return 0, nil, scanned, fmt.Errorf("crossdb scan failed: %w", err)
			}

			if id != "" {
				ids = append(ids, id)
			}
		}

		if err := rows.Err(); err != nil {
			rows.Close()
			return 0, nil, scanned, fmt.Errorf("crossdb rows iteration error: %w", err)
		}

		rows.Close()

		if len(ids) == 0 {
			break
		}

		scanned += len(ids)
		offset += len(ids)

		exists, err := c.fetchExistingIDs(ctx, entityType, ids)
		if err != nil {
			return 0, nil, scanned, err
		}

		for _, id := range ids {
			if _, ok := exists[id]; ok {
				continue
			}

			missingCount++

			if maxResults > 0 && len(samples) < maxResults {
				samples = append(samples, domain.CrossDBMissing{
					Source:   "transaction_db",
					EntityID: id,
					RefType:  entityType,
					RefID:    id,
				})
			}
		}
	}

	return missingCount, samples, scanned, nil
}

func (c *CrossDBChecker) fetchExistingIDs(ctx context.Context, entityType string, ids []string) (map[string]struct{}, error) {
	if len(ids) == 0 {
		return map[string]struct{}{}, nil
	}

	var query string

	switch entityType {
	case "account":
		query = `SELECT id::text FROM account WHERE id = ANY($1::uuid[]) AND deleted_at IS NULL`
	case "ledger":
		query = `SELECT id::text FROM ledger WHERE id = ANY($1::uuid[]) AND deleted_at IS NULL`
	case "organization":
		query = `SELECT id::text FROM organization WHERE id = ANY($1::uuid[]) AND deleted_at IS NULL`
	default:
		return nil, fmt.Errorf("unknown entity type: %s", entityType)
	}

	rows, err := c.onboardingDB.QueryContext(ctx, query, pq.Array(ids))
	if err != nil {
		return nil, fmt.Errorf("crossdb onboarding query failed: %w", err)
	}
	defer rows.Close()

	exists := make(map[string]struct{}, len(ids))

	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("crossdb onboarding scan failed: %w", err)
		}

		exists[id] = struct{}{}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("crossdb onboarding iteration failed: %w", err)
	}

	return exists, nil
}
