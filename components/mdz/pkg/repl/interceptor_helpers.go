package repl

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// ensureAccountContextRefactored ensures all necessary context for account commands
func (ci *CommandInterceptor) ensureAccountContextRefactored(ctx context.Context, cmd *cobra.Command, args []string) error {
	// First ensure organization and ledger context
	if err := ci.ensureLedgerContext(ctx, cmd, args); err != nil {
		return err
	}

	if err := ci.ensureLedgerContextForAccount(ctx, cmd); err != nil {
		return err
	}

	// Skip portfolio context for list commands - they should show all accounts
	cmdPath := cmd.CommandPath()
	if !strings.Contains(cmdPath, "list") {
		if err := ci.ensurePortfolioContextIfNeeded(ctx, cmd); err != nil {
			return err
		}
	}

	return ci.ensureAccountContextIfNeeded(ctx, cmd)
}

// ensureLedgerContextForAccount handles ledger context setup for account commands
func (ci *CommandInterceptor) ensureLedgerContextForAccount(ctx context.Context, cmd *cobra.Command) error {
	ledgerFlag := cmd.Flag("ledger-id")
	if ledgerFlag != nil && ledgerFlag.Changed {
		return nil
	}

	// Refresh context from environment before checking
	ci.repl.context.loadFromEnvironment()

	if ci.repl.context.LedgerID == "" {
		ledgers, err := ci.fetchLedgers(ctx, ci.repl.context.OrganizationID)
		if err != nil {
			return fmt.Errorf("failed to fetch ledgers: %w", err)
		}

		selected, err := ci.selector.SelectWithTUI(EntityLedger, ledgers)
		if err != nil {
			return err
		}

		ci.repl.context.SetLedger(selected.ID, selected.Name)
	}

	return ci.setLedgerFlag(ledgerFlag)
}

// ensurePortfolioContextIfNeeded handles portfolio context setup when needed
func (ci *CommandInterceptor) ensurePortfolioContextIfNeeded(ctx context.Context, cmd *cobra.Command) error {
	portfolioFlag := cmd.Flag("portfolio-id")
	if portfolioFlag == nil || portfolioFlag.Changed || ci.repl.context.PortfolioID != "" {
		return nil
	}

	portfolios, err := ci.fetchPortfolios(ctx, ci.repl.context.OrganizationID, ci.repl.context.LedgerID)
	if err != nil {
		return fmt.Errorf("failed to fetch portfolios: %w", err)
	}

	if len(portfolios) == 0 {
		return nil
	}

	selected, err := ci.selector.SelectWithTUI(EntityPortfolio, portfolios)
	if err != nil {
		return err
	}

	ci.repl.context.SetPortfolio(selected.ID, selected.Name)

	return ci.setPortfolioFlag(portfolioFlag)
}

// ensureAccountContextIfNeeded handles account context setup for specific commands
func (ci *CommandInterceptor) ensureAccountContextIfNeeded(ctx context.Context, cmd *cobra.Command) error {
	accountFlag := cmd.Flag("account-id")
	if accountFlag == nil || accountFlag.Changed || ci.repl.context.AccountID != "" {
		return nil
	}

	if !ci.needsAccountSelection(cmd) {
		return nil
	}

	accounts, err := ci.fetchAccounts(ctx, ci.repl.context.OrganizationID, ci.repl.context.LedgerID, ci.repl.context.PortfolioID)
	if err != nil {
		return fmt.Errorf("failed to fetch accounts: %w", err)
	}

	if len(accounts) == 0 {
		return nil
	}

	selected, err := ci.selector.SelectWithTUI(EntityAccount, accounts)
	if err != nil {
		return err
	}

	ci.repl.context.SetAccount(selected.ID, selected.Name)

	return ci.setAccountFlag(accountFlag)
}

// Helper methods for flag setting
func (ci *CommandInterceptor) setLedgerFlag(ledgerFlag *pflag.Flag) error {
	if ledgerFlag != nil && ci.repl.context.LedgerID != "" {
		if err := ledgerFlag.Value.Set(ci.repl.context.LedgerID); err != nil {
			return fmt.Errorf("failed to set ledger-id flag: %w", err)
		}

		ledgerFlag.Changed = true
	}

	return nil
}

func (ci *CommandInterceptor) setPortfolioFlag(portfolioFlag *pflag.Flag) error {
	if err := portfolioFlag.Value.Set(ci.repl.context.PortfolioID); err != nil {
		return fmt.Errorf("failed to set portfolio-id flag: %w", err)
	}

	portfolioFlag.Changed = true

	return nil
}

func (ci *CommandInterceptor) setAccountFlag(accountFlag *pflag.Flag) error {
	if err := accountFlag.Value.Set(ci.repl.context.AccountID); err != nil {
		return fmt.Errorf("failed to set account-id flag: %w", err)
	}

	accountFlag.Changed = true

	return nil
}

func (ci *CommandInterceptor) needsAccountSelection(cmd *cobra.Command) bool {
	cmdPath := cmd.CommandPath()

	return strings.Contains(cmdPath, "describe") ||
		strings.Contains(cmdPath, "delete") ||
		strings.Contains(cmdPath, "update")
}
