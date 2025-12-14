package panicguard_test

import (
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg/mlint/panicguard"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestNoRawGoroutineAnalyzer(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, panicguard.NoRawGoroutineAnalyzer, "goroutine")
}

func TestNoRecoverOutsideBoundaryAnalyzer(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, panicguard.NoRecoverOutsideBoundaryAnalyzer, "recover")
}

func TestNoPanicInProductionAnalyzer(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, panicguard.NoPanicInProductionAnalyzer, "panic")
}

func TestNoBareRecoverAnalyzer(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, panicguard.NoBareRecoverAnalyzer, "barerecover")
}
