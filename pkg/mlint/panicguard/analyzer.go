package panicguard

import (
	"github.com/golangci/plugin-module-register/register"
	"golang.org/x/tools/go/analysis"
)

func init() {
	register.Plugin("panicguard", New)
}

// BlockingAnalyzers returns analyzers that should block CI (ERROR severity).
// These correspond to Semgrep ERROR rules.
func BlockingAnalyzers() []*analysis.Analyzer {
	return []*analysis.Analyzer{
		NoRawGoroutineAnalyzer,
		NoRecoverOutsideBoundaryAnalyzer,
	}
}

// Analyzers returns all panic hardening analyzers.
// Use this when integrating with golangci-lint or running all checks together.
func Analyzers() []*analysis.Analyzer {
	return append(BlockingAnalyzers(),
		NoPanicInProductionAnalyzer,
		NoBareRecoverAnalyzer,
	)
}

// Plugin implements the golangci-lint module plugin interface.
type Plugin struct{}

// New is the entry point for golangci-lint module plugins.
// It returns only blocking analyzers (Semgrep ERROR parity).
func New(settings any) (register.LinterPlugin, error) {
	return &Plugin{}, nil
}

// BuildAnalyzers returns the analyzers to run.
func (p *Plugin) BuildAnalyzers() ([]*analysis.Analyzer, error) {
	return BlockingAnalyzers(), nil
}

// GetLoadMode returns the load mode for the analyzers.
func (p *Plugin) GetLoadMode() string {
	// We use types information for proper AST inspection
	return register.LoadModeTypesInfo
}
