// Package crossdb provides cross-database validation adapters for reconciliation.
package crossdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/LerianStudio/midaz/v3/components/reconciliation/internal/adapters/postgres"
	"github.com/LerianStudio/midaz/v3/components/reconciliation/internal/domain"
	"github.com/lib/pq"
)

// Static errors for cross-database validation.
var (
	ErrUnknownEntityType = errors.New("unknown entity type")
)

const (
	defaultCrossDBBatchSize = 200
	defaultCrossDBMaxScan   = 5000
	// defaultWarningThreshold is the number of missing entities before status becomes warning.
	defaultWarningThreshold = 10
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

	batchSize, maxScan := c.normalizeConfig(config)
	result := &domain.CrossDBCheckResult{}

	if err := c.checkAllEntityTypes(ctx, result, batchSize, maxScan, config.MaxResults); err != nil {
		return nil, err
	}

	c.determineStatus(result)

	return result, nil
}

// normalizeConfig returns normalized batch size and max scan values.
func (c *CrossDBChecker) normalizeConfig(config postgres.CheckerConfig) (int, int) {
	batchSize := config.CrossDBBatchSize
	if batchSize <= 0 {
		batchSize = defaultCrossDBBatchSize
	}

	maxScan := config.CrossDBMaxScan
	if maxScan <= 0 {
		maxScan = defaultCrossDBMaxScan
	}

	return batchSize, maxScan
}

// entityCheckSpec defines the query and type for an entity check.
type entityCheckSpec struct {
	query      string
	entityType string
}

// getEntityCheckSpecs returns the specifications for all entity checks.
func getEntityCheckSpecs() []entityCheckSpec {
	return []entityCheckSpec{
		{
			query: `
				SELECT DISTINCT account_id::text as id
				FROM (
					SELECT account_id FROM balance WHERE deleted_at IS NULL
					UNION
					SELECT account_id FROM operation WHERE deleted_at IS NULL
				) ids
				ORDER BY id
			`,
			entityType: "account",
		},
		{
			query: `
				SELECT DISTINCT ledger_id::text as id
				FROM (
					SELECT ledger_id FROM balance WHERE deleted_at IS NULL
					UNION
					SELECT ledger_id FROM operation WHERE deleted_at IS NULL
					UNION
					SELECT ledger_id FROM transaction WHERE deleted_at IS NULL
				) ids
				ORDER BY id
			`,
			entityType: "ledger",
		},
		{
			query: `
				SELECT DISTINCT organization_id::text as id
				FROM (
					SELECT organization_id FROM balance WHERE deleted_at IS NULL
					UNION
					SELECT organization_id FROM operation WHERE deleted_at IS NULL
					UNION
					SELECT organization_id FROM transaction WHERE deleted_at IS NULL
				) ids
				ORDER BY id
			`,
			entityType: "organization",
		},
	}
}

// checkAllEntityTypes runs checks for accounts, ledgers, and organizations.
func (c *CrossDBChecker) checkAllEntityTypes(ctx context.Context, result *domain.CrossDBCheckResult, batchSize, maxScan, maxResults int) error {
	specs := getEntityCheckSpecs()

	scannedCounts := make([]int, 0, len(specs))

	for _, spec := range specs {
		missing, samples, scanned, err := c.checkMissingIDs(ctx, spec.query, spec.entityType, batchSize, maxScan, maxResults)
		if err != nil {
			return err
		}

		c.applyEntityResult(result, spec.entityType, missing, samples)

		scannedCounts = append(scannedCounts, scanned)
	}

	for _, scanned := range scannedCounts {
		if scanned >= maxScan {
			result.ScanLimited = true
			break
		}
	}

	return nil
}

// applyEntityResult applies the check result for a specific entity type.
func (c *CrossDBChecker) applyEntityResult(result *domain.CrossDBCheckResult, entityType string, missing int, samples []domain.CrossDBMissing) {
	switch entityType {
	case "account":
		result.MissingAccounts = missing
	case "ledger":
		result.MissingLedgers = missing
	case "organization":
		result.MissingOrganizations = missing
	}

	result.Samples = append(result.Samples, samples...)
}

// determineStatus sets the status based on issue counts.
func (c *CrossDBChecker) determineStatus(result *domain.CrossDBCheckResult) {
	issueCount := result.MissingAccounts + result.MissingLedgers + result.MissingOrganizations
	result.Status = postgres.DetermineStatus(issueCount, postgres.StatusThresholds{
		WarningThreshold:          defaultWarningThreshold,
		WarningThresholdExclusive: true,
	})

	if result.MissingOrganizations > 0 {
		result.Status = domain.StatusCritical
	}
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
		ids, err := c.fetchBatchIDs(ctx, query, batchSize, offset)
		if err != nil {
			return 0, nil, scanned, err
		}

		if len(ids) == 0 {
			break
		}

		scanned += len(ids)
		offset += len(ids)

		newMissing, newSamples := c.findMissingIDs(ctx, entityType, ids, maxResults, len(samples))
		missingCount += newMissing

		samples = append(samples, newSamples...)
	}

	return missingCount, samples, scanned, nil
}

// fetchBatchIDs retrieves a batch of IDs from the transaction database.
func (c *CrossDBChecker) fetchBatchIDs(ctx context.Context, query string, batchSize, offset int) ([]string, error) {
	rows, err := c.transactionDB.QueryContext(ctx, query+" LIMIT $1 OFFSET $2", batchSize, offset)
	if err != nil {
		return nil, fmt.Errorf("crossdb query failed: %w", err)
	}
	defer rows.Close()

	var ids []string

	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("crossdb scan failed: %w", err)
		}

		if id != "" {
			ids = append(ids, id)
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("crossdb rows iteration error: %w", err)
	}

	return ids, nil
}

// findMissingIDs checks which IDs don't exist in the onboarding database.
func (c *CrossDBChecker) findMissingIDs(ctx context.Context, entityType string, ids []string, maxResults, currentSamples int) (int, []domain.CrossDBMissing) {
	exists, err := c.fetchExistingIDs(ctx, entityType, ids)
	if err != nil {
		return 0, nil
	}

	missingCount := 0

	var samples []domain.CrossDBMissing

	for _, id := range ids {
		if _, ok := exists[id]; ok {
			continue
		}

		missingCount++

		if maxResults > 0 && currentSamples+len(samples) < maxResults {
			samples = append(samples, domain.CrossDBMissing{
				Source:   "transaction_db",
				EntityID: id,
				RefType:  entityType,
				RefID:    id,
			})
		}
	}

	return missingCount, samples
}

func (c *CrossDBChecker) fetchExistingIDs(ctx context.Context, entityType string, ids []string) (map[string]struct{}, error) {
	if len(ids) == 0 {
		return map[string]struct{}{}, nil
	}

	query := c.getExistingIDsQuery(entityType)
	if query == "" {
		return nil, fmt.Errorf("%w: %s", ErrUnknownEntityType, entityType)
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

// getExistingIDsQuery returns the query for checking existing IDs by entity type.
func (c *CrossDBChecker) getExistingIDsQuery(entityType string) string {
	switch entityType {
	case "account":
		return `SELECT id::text FROM account WHERE id = ANY($1::uuid[]) AND deleted_at IS NULL`
	case "ledger":
		return `SELECT id::text FROM ledger WHERE id = ANY($1::uuid[]) AND deleted_at IS NULL`
	case "organization":
		return `SELECT id::text FROM organization WHERE id = ANY($1::uuid[]) AND deleted_at IS NULL`
	default:
		return ""
	}
}
