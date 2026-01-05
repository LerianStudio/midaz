// Package crm provides CRM alias validation adapters for reconciliation.
package crm

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/LerianStudio/midaz/v3/components/reconciliation/internal/adapters/postgres"
	"github.com/LerianStudio/midaz/v3/components/reconciliation/internal/domain"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Static errors for alias validation.
var (
	ErrEntityNotAllowed = errors.New("entity not allowed")
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

	checkCtx := &aliasCheckContext{
		ledgerCache:  map[string]bool{},
		accountCache: map[string]bool{},
		scanned:      0,
		maxScan:      maxScan,
		maxResults:   config.MaxResults,
	}

	for _, collName := range collections {
		if checkCtx.scanned >= maxScan {
			result.ScanLimited = true
			break
		}

		if err := c.processCollection(ctx, collName, checkCtx, result); err != nil {
			return nil, err
		}
	}

	issueCount := result.MissingLedgerIDs + result.MissingAccountIDs
	result.Status = postgres.DetermineStatus(issueCount, postgres.StatusThresholds{
		WarningThreshold:          1,
		WarningThresholdExclusive: true,
	})

	return result, nil
}

type aliasCheckContext struct {
	ledgerCache  map[string]bool
	accountCache map[string]bool
	scanned      int
	maxScan      int
	maxResults   int
}

func (c *AliasChecker) processCollection(ctx context.Context, collName string, checkCtx *aliasCheckContext, result *domain.CRMAliasCheckResult) error {
	coll := c.crmMongo.Collection(collName)
	findOptions := options.Find().SetProjection(bson.M{
		"_id":        1,
		"ledger_id":  1,
		"account_id": 1,
	}).SetLimit(int64(checkCtx.maxScan - checkCtx.scanned))

	cursor, err := coll.Find(ctx, bson.M{"deleted_at": bson.M{"$eq": nil}}, findOptions)
	if err != nil {
		return fmt.Errorf("crm alias query failed: %w", err)
	}
	defer cursor.Close(ctx)

	for cursor.Next(ctx) {
		if checkCtx.scanned >= checkCtx.maxScan {
			result.ScanLimited = true
			break
		}

		if err := c.processAliasDocument(ctx, cursor, checkCtx, result); err != nil {
			return err
		}
	}

	if err := cursor.Err(); err != nil {
		return fmt.Errorf("crm alias cursor failed: %w", err)
	}

	return nil
}

type aliasDocument struct {
	ID        any    `bson:"_id"`
	LedgerID  string `bson:"ledger_id"`
	AccountID string `bson:"account_id"`
}

func (c *AliasChecker) processAliasDocument(ctx context.Context, cursor *mongo.Cursor, checkCtx *aliasCheckContext, result *domain.CRMAliasCheckResult) error {
	var doc aliasDocument
	if err := cursor.Decode(&doc); err != nil {
		return fmt.Errorf("crm alias decode failed: %w", err)
	}

	checkCtx.scanned++
	aliasID := fmt.Sprintf("%v", doc.ID)

	if err := c.validateLedgerReference(ctx, doc, aliasID, checkCtx, result); err != nil {
		return err
	}

	return c.validateAccountReference(ctx, doc, aliasID, checkCtx, result)
}

func (c *AliasChecker) validateLedgerReference(ctx context.Context, doc aliasDocument, aliasID string, checkCtx *aliasCheckContext, result *domain.CRMAliasCheckResult) error {
	if doc.LedgerID == "" {
		return nil
	}

	exists, err := c.existsInOnboarding(ctx, "ledger", doc.LedgerID, checkCtx.ledgerCache)
	if err != nil {
		return err
	}

	if !exists {
		result.MissingLedgerIDs++
		c.appendSample(result, aliasID, doc.LedgerID, "", "missing_ledger", checkCtx.maxResults)
	}

	return nil
}

func (c *AliasChecker) validateAccountReference(ctx context.Context, doc aliasDocument, aliasID string, checkCtx *aliasCheckContext, result *domain.CRMAliasCheckResult) error {
	if doc.AccountID == "" {
		return nil
	}

	exists, err := c.existsInOnboarding(ctx, "account", doc.AccountID, checkCtx.accountCache)
	if err != nil {
		return err
	}

	if !exists {
		result.MissingAccountIDs++
		c.appendSample(result, aliasID, "", doc.AccountID, "missing_account", checkCtx.maxResults)
	}

	return nil
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
		if errors.Is(err, sql.ErrNoRows) {
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
		return "", fmt.Errorf("%w: %s", ErrEntityNotAllowed, entity)
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
