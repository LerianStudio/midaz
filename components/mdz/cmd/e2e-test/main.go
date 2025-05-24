package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/LerianStudio/midaz/components/mdz/pkg/e2e"
)

func main() {
	var (
		mdzBinary     = flag.String("binary", "./mdz", "Path to MDZ binary")
		outputDir     = flag.String("output", "./e2e-results", "Output directory for results")
		timeout       = flag.Duration("timeout", 60*time.Second, "Timeout for each scenario")
		debug         = flag.Bool("debug", false, "Enable debug output")
		recordAll     = flag.Bool("record", true, "Record all sessions")
		analyzeFlow   = flag.Bool("analyze", true, "Analyze user flows for UX insights")
		scenario      = flag.String("scenario", "", "Run specific scenario (empty for all)")
		listScenarios = flag.Bool("list", false, "List available scenarios")
	)

	flag.Parse()

	// Verify MDZ binary exists
	if _, err := os.Stat(*mdzBinary); os.IsNotExist(err) {
		log.Fatalf("MDZ binary not found at %s", *mdzBinary)
	}

	// Get scenarios
	scenarios := e2e.GetDefaultScenarios()

	// List scenarios if requested
	if *listScenarios {
		fmt.Println("Available scenarios:")

		for _, s := range scenarios {
			fmt.Printf("  %-20s - %s\n", s.Name, s.Description)
		}

		return
	}

	// Filter scenarios if specific one requested
	if *scenario != "" {
		filtered := make([]*e2e.Scenario, 0)

		for _, s := range scenarios {
			if s.Name == *scenario {
				filtered = append(filtered, s)
				break
			}
		}

		if len(filtered) == 0 {
			log.Fatalf("Scenario '%s' not found", *scenario)
		}

		scenarios = filtered
	}

	// Create output directory
	absOutputDir, err := filepath.Abs(*outputDir)
	if err != nil {
		log.Fatalf("Failed to resolve output directory: %v", err)
	}

	if err := os.MkdirAll(absOutputDir, 0755); err != nil {
		log.Fatalf("Failed to create output directory: %v", err)
	}

	// Configure runner
	config := &e2e.RunnerConfig{
		MDZBinary:   *mdzBinary,
		OutputDir:   absOutputDir,
		Timeout:     *timeout,
		Debug:       *debug,
		RecordAll:   *recordAll,
		AnalyzeFlow: *analyzeFlow,
	}

	// Create and run scenarios
	runner := e2e.NewScenarioRunner(config)

	fmt.Printf("🚀 Starting MDZ CLI E2E Testing\n")
	fmt.Printf("Binary: %s\n", *mdzBinary)
	fmt.Printf("Output: %s\n", absOutputDir)
	fmt.Printf("Scenarios: %d\n", len(scenarios))
	fmt.Printf("Debug: %v\n", *debug)
	fmt.Printf("Analysis: %v\n", *analyzeFlow)
	fmt.Println()

	startTime := time.Now()

	results, err := runner.RunScenarios(scenarios)
	if err != nil {
		log.Fatalf("Failed to run scenarios: %v", err)
	}

	// Summary
	totalDuration := time.Since(startTime)
	passed := 0
	failed := 0

	for _, result := range results {
		if result.Success {
			passed++
		} else {
			failed++
		}
	}

	fmt.Printf("\n📊 Test Summary\n")
	fmt.Printf("================\n")
	fmt.Printf("Total scenarios: %d\n", len(results))
	fmt.Printf("Passed: %d ✓\n", passed)
	fmt.Printf("Failed: %d ✗\n", failed)
	fmt.Printf("Duration: %v\n", totalDuration)
	fmt.Printf("Success rate: %.1f%%\n", float64(passed)/float64(len(results))*100)

	if failed > 0 {
		fmt.Printf("\n❌ Failed scenarios:\n")

		for _, result := range results {
			if !result.Success {
				fmt.Printf("  - %s: %s\n", result.Scenario, result.Error)
			}
		}
	}

	// Analysis summary
	if *analyzeFlow {
		fmt.Printf("\n🔍 UX Analysis Summary\n")
		fmt.Printf("======================\n")

		totalIssues := 0
		highPriorityRecs := 0

		for _, result := range results {
			if result.Analysis != nil {
				totalIssues += len(result.Analysis.UXIssues)

				for _, rec := range result.Analysis.Recommendations {
					if rec.Priority == "High" {
						highPriorityRecs++
					}
				}

				fmt.Printf("%s:\n", result.Scenario)
				fmt.Printf("  Flow efficiency: %.1f%%\n", result.Analysis.FlowEfficiency*100)
				fmt.Printf("  UX issues: %d\n", len(result.Analysis.UXIssues))
				fmt.Printf("  Recommendations: %d\n", len(result.Analysis.Recommendations))
			}
		}

		fmt.Printf("\nOverall:\n")
		fmt.Printf("  Total UX issues: %d\n", totalIssues)
		fmt.Printf("  High priority recommendations: %d\n", highPriorityRecs)
	}

	fmt.Printf("\n📁 Results saved to: %s\n", absOutputDir)
	fmt.Printf("   - report.html (detailed report)\n")
	fmt.Printf("   - report.json (machine-readable)\n")
	fmt.Printf("   - *.json (individual recordings)\n")

	// Exit with error code if any tests failed
	if failed > 0 {
		os.Exit(1)
	}
}
