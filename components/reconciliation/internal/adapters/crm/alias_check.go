package crm

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/LerianStudio/midaz/v3/components/reconciliation/internal/adapters/postgres"
	"github.com/LerianStudio/midaz/v3/components/reconciliation/internal/domain"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	defaultCRMSampleLimit = 2000
)

// AliasChecker validates CRM alias references to ledger/account.
type AliasChecker struct {
	onboardingDB *sql.DB
	crmMongo     *mongo.Database
}

// NewAliasChecker creates a new CRM alias checker.
func NewAliasChecker(onboardingDB *sql.DB, crmMongo *mongo.Database) *AliasChecker {
	return &AliasChecker{
		onboardingDB: onboardingDB,
		crmMongo:     crmMongo,
	}
}

// Name returns the unique name of this checker.
func (c *AliasChecker) Name() string {
	return postgres.CheckerNameCRMAlias
}

// Check validates CRM alias references.
func (c *AliasChecker) Check(ctx context.Context, config postgres.CheckerConfig) (postgres.CheckResult, error) {
	if c.onboardingDB == nil || c.crmMongo == nil {
		return &domain.CRMAliasCheckResult{Status: domain.StatusSkipped}, nil
	}

	maxScan := config.CrossDBMaxScan
	if maxScan <= 0 {
		maxScan = defaultCRMSampleLimit
	}

	result := &domain.CRMAliasCheckResult{}

	collections, err := c.crmMongo.ListCollectionNames(ctx, bson.M{"name": bson.M{"$regex": "^aliases_"}})
	if err != nil {
		return nil, fmt.Errorf("crm alias collections lookup failed: %w", err)
	}

	ledgerCache := map[string]bool{}
	accountCache := map[string]bool{}

	scanned := 0

	for _, collName := range collections {
		if scanned >= maxScan {
			result.ScanLimited = true
			break
		}

		coll := c.crmMongo.Collection(collName)
		findOptions := options.Find().SetProjection(bson.M{
			"_id":        1,
			"ledger_id":  1,
			"account_id": 1,
		}).SetLimit(int64(maxScan - scanned))

		cursor, err := coll.Find(ctx, bson.M{"deleted_at": bson.M{"$eq": nil}}, findOptions)
		if err != nil {
			return nil, fmt.Errorf("crm alias query failed: %w", err)
		}

		for cursor.Next(ctx) {
			if scanned >= maxScan {
				result.ScanLimited = true
				break
			}

			var doc struct {
				ID        any    `bson:"_id"`
				LedgerID  string `bson:"ledger_id"`
				AccountID string `bson:"account_id"`
			}

			if err := cursor.Decode(&doc); err != nil {
				cursor.Close(ctx)
				return nil, fmt.Errorf("crm alias decode failed: %w", err)
			}

			scanned++

			aliasID := fmt.Sprintf("%v", doc.ID)

			if doc.LedgerID != "" {
				exists, err := c.existsInOnboarding(ctx, "ledger", doc.LedgerID, ledgerCache)
				if err != nil {
					cursor.Close(ctx)
					return nil, err
				}
				if !exists {
					result.MissingLedgerIDs++
					c.appendSample(result, aliasID, doc.LedgerID, "", "missing_ledger", config.MaxResults)
				}
			}

			if doc.AccountID != "" {
				exists, err := c.existsInOnboarding(ctx, "account", doc.AccountID, accountCache)
				if err != nil {
					cursor.Close(ctx)
					return nil, err
				}
				if !exists {
					result.MissingAccountIDs++
					c.appendSample(result, aliasID, "", doc.AccountID, "missing_account", config.MaxResults)
				}
			}
		}

		if err := cursor.Err(); err != nil {
			cursor.Close(ctx)
			return nil, fmt.Errorf("crm alias cursor failed: %w", err)
		}

		cursor.Close(ctx)
	}

	issueCount := result.MissingLedgerIDs + result.MissingAccountIDs
	result.Status = postgres.DetermineStatus(issueCount, postgres.StatusThresholds{
		WarningThreshold:          1,
		WarningThresholdExclusive: true,
	})

	return result, nil
}

func (c *AliasChecker) existsInOnboarding(ctx context.Context, entity, id string, cache map[string]bool) (bool, error) {
	if cached, ok := cache[id]; ok {
		return cached, nil
	}

	table, err := onboardingTableForEntity(entity)
	if err != nil {
		// Fail fast: never let unchecked user input influence SQL identifiers.
		return false, fmt.Errorf("crm alias onboarding lookup blocked: %w", err)
	}

	// NOTE: table comes from a strict allowlist (not user input). Keep id parameterized.
	query := "SELECT 1 FROM " + table + " WHERE id = $1 AND deleted_at IS NULL"
	var tmp int
	err = c.onboardingDB.QueryRowContext(ctx, query, id).Scan(&tmp)
	if err != nil {
		if err == sql.ErrNoRows {
			cache[id] = false
			return false, nil
		}
		return false, fmt.Errorf("crm alias onboarding lookup failed: %w", err)
	}

	cache[id] = true
	return true, nil
}

func onboardingTableForEntity(entity string) (string, error) {
	switch entity {
	case "ledger":
		return "ledger", nil
	case "account":
		return "account", nil
	default:
		return "", fmt.Errorf("entity %q is not allowed", entity)
	}
}

func (c *AliasChecker) appendSample(result *domain.CRMAliasCheckResult, aliasID, ledgerID, accountID, issue string, maxResults int) {
	if maxResults <= 0 || len(result.Samples) >= maxResults {
		return
	}

	result.Samples = append(result.Samples, domain.CRMAliasMissing{
		AliasID:   strings.TrimSpace(aliasID),
		LedgerID:  ledgerID,
		AccountID: accountID,
		Issue:     issue,
	})
}
