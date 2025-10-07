// Package factory provides dependency injection for the MDZ CLI application.
//
// This package implements the Factory pattern to create and manage dependencies
// for CLI commands, including HTTP clients, I/O streams, environment configuration,
// and authentication tokens.
package factory

import (
	"net/http"

	"github.com/LerianStudio/midaz/v3/components/mdz/pkg/environment"
	"github.com/LerianStudio/midaz/v3/components/mdz/pkg/iostreams"
)

// Factory provides centralized dependency injection for CLI commands.
//
// This struct holds all dependencies needed by CLI commands:
//   - Token: Authentication token for API requests
//   - HTTPClient: HTTP client for making API calls
//   - IOStreams: Input/output streams for user interaction
//   - Env: Environment configuration (API URLs, credentials)
//   - Flags: Global CLI flags
type Factory struct {
	Token      string
	HTTPClient *http.Client
	IOStreams  *iostreams.IOStreams
	Env        *environment.Env
	Flags
}

// Flags contains global CLI flags that affect command behavior.
type Flags struct {
	NoColor bool // Disable colored output
}

// NewFactory creates a new Factory instance with default dependencies.
//
// This function initializes:
//   - HTTP client with default configuration
//   - System I/O streams (stdin, stdout, stderr)
//   - Environment from provided configuration
//
// Parameters:
//   - env: Environment configuration
//
// Returns:
//   - *Factory: Initialized factory ready for use
func NewFactory(env *environment.Env) *Factory {
	return &Factory{
		HTTPClient: &http.Client{},
		IOStreams:  iostreams.System(),
		Env:        env,
	}
}
