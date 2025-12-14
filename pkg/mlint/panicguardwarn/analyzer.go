package panicguardwarn

import (
	"github.com/LerianStudio/midaz/v3/pkg/mlint/panicguard"
	"golang.org/x/tools/go/analysis"
)

// New is the entry point for golangci-lint module plugins.
// It returns only report-only WARNING analyzers (Semgrep WARNING parity).
func New(conf any) ([]*analysis.Analyzer, error) {
	return []*analysis.Analyzer{
		panicguard.NoPanicInProductionAnalyzer,
		panicguard.NoBareRecoverAnalyzer,
	}, nil
}
