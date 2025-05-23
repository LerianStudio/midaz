package repl

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/LerianStudio/midaz/components/mdz/internal/rest"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/spf13/cobra"
)

// CommandInterceptor intercepts commands to provide context-aware functionality
type CommandInterceptor struct {
	repl     *REPL
	factory  *factory.Factory
	selector *Selector
}

// NewCommandInterceptor creates a new command interceptor
func NewCommandInterceptor(repl *REPL, f *factory.Factory) *CommandInterceptor {
	return &CommandInterceptor{
		repl:     repl,
		factory:  f,
		selector: NewSelector(f),
	}
}

// InterceptCommand intercepts a command and provides context if needed
func (ci *CommandInterceptor) InterceptCommand(ctx context.Context, cmd *cobra.Command, args []string) error {
	// Get the full command path
	cmdPath := cmd.CommandPath()

	// Skip interceptor for organization list in REPL mode - it handles context itself
	if strings.Contains(cmdPath, "organization list") && os.Getenv("MDZ_REPL_MODE") == "true" {
		return nil
	}

	// Check if this command needs context
	switch {
	case strings.Contains(cmdPath, "ledger") && !strings.Contains(cmdPath, "create"):
		return ci.ensureLedgerContext(ctx, cmd, args)
	case strings.Contains(cmdPath, "account"):
		return ci.ensureAccountContextRefactored(ctx, cmd, args)
	case strings.Contains(cmdPath, "portfolio") && !strings.Contains(cmdPath, "create"):
		return ci.ensurePortfolioContext(ctx, cmd, args)
	case strings.Contains(cmdPath, "transaction"):
		return ci.ensureTransactionContext(ctx, cmd, args)
	case strings.Contains(cmdPath, "balance"):
		return ci.ensureBalanceContext(ctx, cmd, args)
	case strings.Contains(cmdPath, "operation"):
		return ci.ensureOperationContext(ctx, cmd, args)
	}

	return nil
}

// ensureLedgerContext ensures organization context is available for ledger commands
func (ci *CommandInterceptor) ensureLedgerContext(ctx context.Context, cmd *cobra.Command, _ []string) error {
	// Check if organization-id flag is provided
	orgFlag := cmd.Flag("organization-id")
	if orgFlag != nil && orgFlag.Changed {
		// Flag is provided, no need to prompt
		return nil
	}

	// Refresh context from environment before checking
	ci.repl.context.loadFromEnvironment()

	// Check if we have organization context
	if ci.repl.context.OrganizationID == "" {
		// Need to select organization
		orgs, err := ci.fetchOrganizations(ctx)
		if err != nil {
			return fmt.Errorf("failed to fetch organizations: %w", err)
		}

		selected, err := ci.selector.SelectWithTUI(EntityOrganization, orgs)
		if err != nil {
			return err
		}

		ci.repl.context.SetOrganization(selected.ID, selected.Name)
	}

	// Set the flag value from context
	if orgFlag != nil && ci.repl.context.OrganizationID != "" {
		if err := orgFlag.Value.Set(ci.repl.context.OrganizationID); err != nil {
			return fmt.Errorf("failed to set organization-id flag: %w", err)
		}

		orgFlag.Changed = true
	}

	return nil
}

// ensurePortfolioContext ensures all necessary context for portfolio commands
func (ci *CommandInterceptor) ensurePortfolioContext(ctx context.Context, cmd *cobra.Command, args []string) error {
	// Similar to account context but without requiring portfolio selection
	return ci.ensureLedgerContext(ctx, cmd, args)
}

// ensureTransactionContext ensures all necessary context for transaction commands
func (ci *CommandInterceptor) ensureTransactionContext(ctx context.Context, cmd *cobra.Command, args []string) error {
	return ci.ensureAccountContextRefactored(ctx, cmd, args)
}

// ensureBalanceContext ensures all necessary context for balance commands
func (ci *CommandInterceptor) ensureBalanceContext(ctx context.Context, cmd *cobra.Command, args []string) error {
	return ci.ensureAccountContextRefactored(ctx, cmd, args)
}

// ensureOperationContext ensures all necessary context for operation commands
func (ci *CommandInterceptor) ensureOperationContext(ctx context.Context, cmd *cobra.Command, args []string) error {
	return ci.ensureAccountContextRefactored(ctx, cmd, args)
}

// fetchOrganizations fetches available organizations
func (ci *CommandInterceptor) fetchOrganizations(_ context.Context) ([]Entity, error) {
	// Create organization repository
	orgRepo := rest.NewOrganization(ci.factory)

	// Fetch organizations (page 1, limit 100)
	orgs, err := orgRepo.Get(100, 1, "", "", "")
	if err != nil {
		return nil, err
	}

	// Convert to Entity slice
	entities := make([]Entity, 0, len(orgs.Items))
	for _, org := range orgs.Items {
		entities = append(entities, Entity{
			ID:          org.ID,
			Name:        org.LegalName,
			Description: org.LegalDocument,
			Type:        EntityOrganization,
		})
	}

	return entities, nil
}

// fetchLedgers fetches available ledgers for an organization
func (ci *CommandInterceptor) fetchLedgers(_ context.Context, orgID string) ([]Entity, error) {
	// Create ledger repository
	ledgerRepo := rest.NewLedger(ci.factory)

	// Fetch ledgers for the organization
	ledgers, err := ledgerRepo.Get(orgID, 100, 1, "", "", "")
	if err != nil {
		return nil, err
	}

	// Convert to Entity slice
	entities := make([]Entity, 0, len(ledgers.Items))
	for _, ledger := range ledgers.Items {
		entities = append(entities, Entity{
			ID:          ledger.ID,
			Name:        ledger.Name,
			Description: "",
			Type:        EntityLedger,
		})
	}

	return entities, nil
}

// fetchPortfolios fetches available portfolios for a ledger
func (ci *CommandInterceptor) fetchPortfolios(_ context.Context, orgID, ledgerID string) ([]Entity, error) {
	// Create portfolio repository
	portfolioRepo := rest.NewPortfolio(ci.factory)

	// Fetch portfolios for the ledger
	portfolios, err := portfolioRepo.Get(orgID, ledgerID, 100, 1, "", "", "")
	if err != nil {
		return nil, err
	}

	// Convert to Entity slice
	entities := make([]Entity, 0, len(portfolios.Items))
	for _, portfolio := range portfolios.Items {
		entities = append(entities, Entity{
			ID:          portfolio.ID,
			Name:        portfolio.Name,
			Description: "",
			Type:        EntityPortfolio,
		})
	}

	return entities, nil
}

// fetchAccounts fetches available accounts for a portfolio
func (ci *CommandInterceptor) fetchAccounts(_ context.Context, orgID, ledgerID, portfolioID string) ([]Entity, error) {
	// Create account repository
	accountRepo := rest.NewAccount(ci.factory)

	// Fetch all accounts for the ledger
	// TODO: Add filtering by portfolio when API supports it
	accounts, err := accountRepo.Get(orgID, ledgerID, 100, 1, "", "", "")
	if err != nil {
		return nil, err
	}

	// Convert to Entity slice and filter by portfolio if provided
	entities := make([]Entity, 0)

	for _, account := range accounts.Items {
		// Filter by portfolio if specified
		if portfolioID != "" && account.PortfolioID != nil && *account.PortfolioID != portfolioID {
			continue
		}

		name := account.Name
		if account.Alias != nil && *account.Alias != "" {
			name = *account.Alias
		}

		entities = append(entities, Entity{
			ID:          account.ID,
			Name:        name,
			Description: "",
			Type:        EntityAccount,
		})
	}

	return entities, nil
}
