// Package environment provides environment configuration management for the MDZ CLI.
//
// This package manages CLI configuration including API endpoints, authentication
// credentials, and version information. Configuration can be set via environment
// variables or configuration files.
package environment

// CLIVersion is the prefix for the CLI version string.
const CLIVersion = "Mdz CLI "

// Package-level variables for environment configuration.
// These are typically set at build time via ldflags or loaded from config files.
var (
	ClientID     string // OAuth client ID for authentication
	ClientSecret string // OAuth client secret for authentication
	URLAPIAuth   string // Authentication API endpoint URL
	URLAPILedger string // Ledger API endpoint URL
	Version      string // CLI version number
)

// Env holds environment configuration for the CLI application.
//
// This struct encapsulates all environment-specific settings needed by the CLI,
// including API endpoints and authentication credentials.
type Env struct {
	ClientID     string // OAuth client ID
	ClientSecret string // OAuth client secret
	URLAPIAuth   string // Auth API URL
	URLAPILedger string // Ledger API URL
	Version      string // Full version string (CLIVersion + Version)
}

// New creates a new Env instance from package-level variables.
//
// This function initializes an Env struct with values from package-level variables,
// which are typically set at build time or loaded from configuration files.
//
// Returns:
//   - *Env: Initialized environment configuration
func New() *Env {
	return &Env{
		ClientID:     ClientID,
		ClientSecret: ClientSecret,
		URLAPIAuth:   URLAPIAuth,
		URLAPILedger: URLAPILedger,
		Version:      CLIVersion + Version,
	}
}
