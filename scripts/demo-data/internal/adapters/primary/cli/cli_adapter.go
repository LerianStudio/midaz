package cli

import (
	"context"
	"fmt"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"demo-data/internal/domain/entities"
	"demo-data/internal/infrastructure/di"
)

// CLIAdapter implements the primary adapter for CLI interactions
type CLIAdapter struct {
	container *di.Container
	rootCmd   *cobra.Command
}

// NewCLIAdapter creates a new CLI adapter with dependency injection
func NewCLIAdapter(container *di.Container) *CLIAdapter {
	adapter := &CLIAdapter{
		container: container,
	}

	adapter.setupRootCommand()
	adapter.setupCommands()

	return adapter
}

// Execute runs the CLI application
func (a *CLIAdapter) Execute(ctx context.Context) error {
	return a.rootCmd.ExecuteContext(ctx)
}

// ExecuteWithArgs runs the CLI application with specific arguments (for testing)
func (a *CLIAdapter) ExecuteWithArgs(ctx context.Context, args []string) error {
	a.rootCmd.SetArgs(args)
	return a.rootCmd.ExecuteContext(ctx)
}

// setupRootCommand configures the root command with global flags
func (a *CLIAdapter) setupRootCommand() {
	a.rootCmd = &cobra.Command{
		Use:   "demo-data",
		Short: "Midaz demo data generator",
		Long: `A comprehensive demo data generator for the Midaz financial ledger system.
        
This tool generates realistic financial data hierarchies including organizations,
ledgers, assets, portfolios, segments, accounts, and transactions for testing
and demonstration purposes.

The tool follows Hexagonal Architecture principles and supports multiple
configuration sources including environment variables, configuration files,
and command-line flags.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return a.loadConfiguration(cmd)
		},
	}

	// Global flags
	a.rootCmd.PersistentFlags().BoolP("debug", "d", false, "Enable debug mode")
	a.rootCmd.PersistentFlags().StringP("config", "c", "", "Configuration file path")
	a.rootCmd.PersistentFlags().String("log-level", "info", "Log level (debug, info, warn, error)")
	a.rootCmd.PersistentFlags().String("api-url", "", "API base URL (overrides config)")
	a.rootCmd.PersistentFlags().String("auth-token", "", "Authentication token (overrides config)")
	a.rootCmd.PersistentFlags().Duration("timeout", 0, "Request timeout (overrides config)")
}

// setupCommands adds all subcommands to the root command
func (a *CLIAdapter) setupCommands() {
	a.rootCmd.AddCommand(a.newVersionCommand())
	a.rootCmd.AddCommand(a.newValidateCommand())
	a.rootCmd.AddCommand(a.newTestConnectionCommand())
}

// loadConfiguration loads and validates configuration before command execution
func (a *CLIAdapter) loadConfiguration(cmd *cobra.Command) error {
	ctx := cmd.Context()

	// Get configuration port
	configPort := a.container.GetConfigurationPort()
	if configPort == nil {
		return fmt.Errorf("configuration port not available")
	}

	// Load configuration
	config, err := configPort.Load(ctx)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Apply command-line flag overrides
	if err := a.applyFlagOverrides(cmd, config); err != nil {
		return fmt.Errorf("failed to apply flag overrides: %w", err)
	}

	// Validate configuration
	if err := configPort.Validate(ctx, config); err != nil {
		// For validation errors, show helpful message
		red := color.New(color.FgRed)
		red.Printf("❌ Configuration validation failed: %v\n", err)
		return err
	}

	// Store configuration in container
	a.container.SetConfiguration(config)

	// Recreate logger with proper configuration
	logger, err := a.container.CreateLoggerFromConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create logger from configuration: %w", err)
	}
	a.container.SetLogger(logger)

	// Log configuration loaded event
	if logger := a.container.GetLogger(); logger != nil {
		logger.Debug("Configuration loaded and validated",
			"source", "cli",
			"api_url", config.APIBaseURL,
			"debug", config.Debug,
			"log_level", config.LogLevel,
		)
	}

	return nil
}

// applyFlagOverrides applies command-line flag values to configuration
func (a *CLIAdapter) applyFlagOverrides(cmd *cobra.Command, config interface{}) error {
	// Type assert to the actual configuration type
	cfg, ok := config.(*entities.Configuration)
	if !ok {
		return fmt.Errorf("invalid configuration type")
	}

	// Debug flag
	if cmd.Flags().Changed("debug") {
		debug, _ := cmd.Flags().GetBool("debug")
		cfg.Debug = debug
	}

	// API URL flag
	if cmd.Flags().Changed("api-url") {
		apiURL, _ := cmd.Flags().GetString("api-url")
		if apiURL != "" {
			cfg.APIBaseURL = apiURL
		}
	}

	// Auth token flag
	if cmd.Flags().Changed("auth-token") {
		authToken, _ := cmd.Flags().GetString("auth-token")
		if authToken != "" {
			cfg.AuthToken = authToken
		}
	}

	// Log level flag
	if cmd.Flags().Changed("log-level") {
		logLevel, _ := cmd.Flags().GetString("log-level")
		if logLevel != "" {
			cfg.LogLevel = logLevel
		}
	}

	// Timeout flag
	if cmd.Flags().Changed("timeout") {
		timeout, _ := cmd.Flags().GetDuration("timeout")
		if timeout > 0 {
			cfg.TimeoutDuration = timeout
		}
	}

	return nil
}
