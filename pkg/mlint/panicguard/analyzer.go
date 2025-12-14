package panicguard

import (
	"golang.org/x/tools/go/analysis"
)

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

// New is the entry point for golangci-lint module plugins.
// It returns only blocking analyzers (Semgrep ERROR parity).
func New(conf any) ([]*analysis.Analyzer, error) {
	return BlockingAnalyzers(), nil
}
