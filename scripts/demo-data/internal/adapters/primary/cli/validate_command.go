package cli

import (
	"fmt"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"demo-data/internal/domain/entities"
)

// newValidateCommand creates the validate command
func (a *CLIAdapter) newValidateCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "Validate configuration and environment",
		Long: `Validate configuration settings, environment variables, and required dependencies.

This command checks:
- Configuration file syntax and values
- Environment variable presence and format
- Required authentication tokens
- API endpoint accessibility (if possible)
- Volume configuration settings

Use this command to troubleshoot configuration issues before running data generation.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runValidateCommand(cmd, args)
		},
	}
}

// runValidateCommand executes the validate command
func (a *CLIAdapter) runValidateCommand(cmd *cobra.Command, args []string) error {

	green := color.New(color.FgGreen)
	red := color.New(color.FgRed)
	yellow := color.New(color.FgYellow)
	cyan := color.New(color.FgCyan)

	fmt.Println("🔍 Validating configuration and environment...")
	fmt.Println()

	// Configuration is already loaded and validated in PersistentPreRunE
	config := a.container.GetConfiguration()
	if config == nil {
		red.Println("❌ Configuration not loaded")
		return fmt.Errorf("configuration not available")
	}

	green.Println("✅ Configuration loaded and validated successfully")
	fmt.Println()

	// Display configuration summary
	cyan.Println("📋 Configuration Summary:")
	fmt.Printf("  API URL:           %s\n", config.APIBaseURL)
	fmt.Printf("  Debug Mode:        %v\n", config.Debug)
	fmt.Printf("  Log Level:         %s\n", config.LogLevel)
	fmt.Printf("  Timeout:           %v\n", config.TimeoutDuration)

	// Check authentication token
	if config.AuthToken != "" {
		tokenLen := len(config.AuthToken)
		maskedToken := config.AuthToken[:min(4, tokenLen)] + strings.Repeat("*", max(0, tokenLen-8)) +
			config.AuthToken[max(4, tokenLen-4):]
		fmt.Printf("  Auth Token:        %s %s\n",
			green.Sprint("✅"),
			maskedToken)
	} else {
		fmt.Printf("  Auth Token:        %s %s\n",
			yellow.Sprint("⚠️"),
			yellow.Sprint("Not configured"))
	}

	fmt.Println()

	// Display volume configuration
	cyan.Println("📊 Volume Configuration:")
	fmt.Println("  Small Volume:")
	fmt.Printf("    Organizations:      %d\n", config.VolumeConfig.Small.Organizations)
	fmt.Printf("    Ledgers per Org:    %d\n", config.VolumeConfig.Small.LedgersPerOrg)
	fmt.Printf("    Accounts per Ledger: %d\n", config.VolumeConfig.Small.AccountsPerLedger)
	fmt.Printf("    Transactions per Account: %d\n", config.VolumeConfig.Small.TransactionsPerAccount)

	fmt.Println("  Medium Volume:")
	fmt.Printf("    Organizations:      %d\n", config.VolumeConfig.Medium.Organizations)
	fmt.Printf("    Ledgers per Org:    %d\n", config.VolumeConfig.Medium.LedgersPerOrg)
	fmt.Printf("    Accounts per Ledger: %d\n", config.VolumeConfig.Medium.AccountsPerLedger)
	fmt.Printf("    Transactions per Account: %d\n", config.VolumeConfig.Medium.TransactionsPerAccount)

	fmt.Println("  Large Volume:")
	fmt.Printf("    Organizations:      %d\n", config.VolumeConfig.Large.Organizations)
	fmt.Printf("    Ledgers per Org:    %d\n", config.VolumeConfig.Large.LedgersPerOrg)
	fmt.Printf("    Accounts per Ledger: %d\n", config.VolumeConfig.Large.AccountsPerLedger)
	fmt.Printf("    Transactions per Account: %d\n", config.VolumeConfig.Large.TransactionsPerAccount)

	fmt.Println()

	// Calculate estimated entity counts
	cyan.Println("🧮 Estimated Entity Counts:")
	a.displayEstimatedCounts("Small", config.VolumeConfig.Small)
	a.displayEstimatedCounts("Medium", config.VolumeConfig.Medium)
	a.displayEstimatedCounts("Large", config.VolumeConfig.Large)

	fmt.Println()
	green.Println("🎉 Configuration validation completed successfully!")

	return nil
}

// displayEstimatedCounts calculates and displays estimated entity counts for a volume configuration
func (a *CLIAdapter) displayEstimatedCounts(volumeName string, volume entities.VolumeMetrics) {
	totalLedgers := volume.Organizations * volume.LedgersPerOrg
	totalAssets := totalLedgers * volume.AssetsPerLedger
	totalPortfolios := totalLedgers * volume.PortfoliosPerLedger
	totalSegments := totalLedgers * volume.SegmentsPerLedger
	totalAccounts := totalLedgers * volume.AccountsPerLedger
	totalTransactions := totalAccounts * volume.TransactionsPerAccount

	fmt.Printf("  %s Volume Totals:\n", volumeName)
	fmt.Printf("    Ledgers:      %s\n", formatNumber(totalLedgers))
	fmt.Printf("    Assets:       %s\n", formatNumber(totalAssets))
	fmt.Printf("    Portfolios:   %s\n", formatNumber(totalPortfolios))
	fmt.Printf("    Segments:     %s\n", formatNumber(totalSegments))
	fmt.Printf("    Accounts:     %s\n", formatNumber(totalAccounts))
	fmt.Printf("    Transactions: %s\n", formatNumber(totalTransactions))
	fmt.Println()
}

// formatNumber formats a number with thousands separators
func formatNumber(n int) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	if n < 1000000 {
		return fmt.Sprintf("%.1fK", float64(n)/1000)
	}
	return fmt.Sprintf("%.1fM", float64(n)/1000000)
}

// Helper functions for string manipulation
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
