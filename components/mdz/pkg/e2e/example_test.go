package e2e

import (
	"testing"
	"time"
)

// TestCLIAutomationExample demonstrates how to use the CLI automation framework
func TestCLIAutomationExample(t *testing.T) {
	// Skip this test in normal runs - it's for demonstration
	t.Skip("Example test - run manually with real MDZ binary")

	// Create a simple scenario
	scenario := &Scenario{
		Name:        "example_test",
		Description: "Example demonstration of CLI automation",
		Steps: []Step{
			{
				Type:        "wait_for_output",
				ExpectText:  "mdz",
				Timeout:     5 * time.Second,
				Description: "Wait for CLI to start",
			},
			{
				Type:        "type",
				Input:       "help",
				Description: "Get help",
			},
			{
				Type:        "press",
				Key:         "enter",
				Description: "Execute help command",
			},
			{
				Type:        "wait_for_output",
				ExpectText:  "Usage:",
				Timeout:     5 * time.Second,
				Description: "Wait for help output",
			},
			{
				Type:        "screenshot",
				Description: "Take final screenshot",
			},
		},
		Expected: []string{"Usage:", "Available commands"},
		Setup: &Setup{
			Environment: map[string]string{
				"MDZ_TEST_MODE": "true",
			},
		},
	}

	// Create runner configuration
	config := &RunnerConfig{
		MDZBinary:   "./mdz", // Path to your MDZ binary
		OutputDir:   "./test-results",
		Timeout:     30 * time.Second,
		Debug:       true,
		RecordAll:   true,
		AnalyzeFlow: true,
	}

	// Create runner and execute scenario
	runner := NewScenarioRunner(config)
	result, err := runner.RunScenario(scenario)

	if err != nil {
		t.Fatalf("Scenario execution failed: %v", err)
	}

	if !result.Success {
		t.Errorf("Scenario failed: %s", result.Error)
	}

	// Check analysis results if available
	if result.Analysis != nil {
		t.Logf("Flow efficiency: %.1f%%", result.Analysis.FlowEfficiency*100)
		t.Logf("UX issues found: %d", len(result.Analysis.UXIssues))
		t.Logf("Recommendations: %d", len(result.Analysis.Recommendations))

		// Log high-priority recommendations
		for _, rec := range result.Analysis.Recommendations {
			if rec.Priority == "High" {
				t.Logf("HIGH PRIORITY: %s - %s", rec.Title, rec.Description)
			}
		}
	}
}

// TestSessionBasics demonstrates basic session usage
func TestSessionBasics(t *testing.T) {
	// Skip this test in normal runs
	t.Skip("Example test - requires manual setup")

	config := &SessionConfig{
		Command:     "echo",
		Args:        []string{"Hello, World!"},
		Timeout:     10 * time.Second,
		Debug:       true,
		Interactive: false,
	}

	session, err := NewCLISession(config)
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	if err := session.Start(); err != nil {
		t.Fatalf("Failed to start session: %v", err)
	}

	// Wait for output
	time.Sleep(1 * time.Second)

	output := session.GetOutput()
	if output == "" {
		t.Error("Expected output but got none")
	}

	t.Logf("Output: %s", output)

	session.Close()
}

// BenchmarkScenarioExecution benchmarks scenario execution
func BenchmarkScenarioExecution(b *testing.B) {
	// Skip benchmark in normal runs
	b.Skip("Benchmark requires real MDZ binary")

	scenario := &Scenario{
		Name:        "benchmark_test",
		Description: "Performance benchmark test",
		Steps: []Step{
			{
				Type:        "type",
				Input:       "version",
				Description: "Get version",
			},
			{
				Type:        "press",
				Key:         "enter",
				Description: "Execute version command",
			},
		},
		Expected: []string{"version"},
	}

	config := &RunnerConfig{
		MDZBinary:   "./mdz",
		OutputDir:   "/tmp/benchmark-results",
		Timeout:     10 * time.Second,
		Debug:       false,
		RecordAll:   false,
		AnalyzeFlow: false,
	}

	runner := NewScenarioRunner(config)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result, err := runner.RunScenario(scenario)
		if err != nil || !result.Success {
			b.Fatalf("Scenario failed: %v", err)
		}
	}
}