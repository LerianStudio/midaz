// Package panicguardwarn provides golangci-lint plugin for panic guard warnings.
package panicguardwarn

import (
	"github.com/LerianStudio/midaz/v3/pkg/mlint/panicguard"
	"github.com/golangci/plugin-module-register/register"
	"golang.org/x/tools/go/analysis"
)

func init() {
	register.Plugin("panicguardwarn", New)
}

// Plugin implements the golangci-lint module plugin interface.
type Plugin struct{}

// New is the entry point for golangci-lint module plugins.
// It returns only report-only WARNING analyzers (Semgrep WARNING parity).
func New(settings any) (register.LinterPlugin, error) {
	return &Plugin{}, nil
}

// BuildAnalyzers returns the analyzers to run.
func (p *Plugin) BuildAnalyzers() ([]*analysis.Analyzer, error) {
	return []*analysis.Analyzer{
		panicguard.NoPanicInProductionAnalyzer,
		panicguard.NoBareRecoverAnalyzer,
	}, nil
}

// GetLoadMode returns the load mode for the analyzers.
func (p *Plugin) GetLoadMode() string {
	// We use types information for proper AST inspection
	return register.LoadModeTypesInfo
}
