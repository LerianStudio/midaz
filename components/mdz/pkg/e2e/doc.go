// Package e2e provides a comprehensive end-to-end testing framework for CLI applications,
// inspired by Playwright but designed specifically for command-line interfaces.
//
// The framework offers automated testing, session recording, flow analysis, and UX insights
// for the MDZ CLI and other command-line tools.
//
// # Key Features
//
//   - Playwright-like API for familiar testing patterns
//   - Session recording for replay and analysis
//   - Automatic flow analysis with UX recommendations
//   - Performance metrics and bottleneck identification
//   - Rich HTML and JSON reporting
//   - Scenario-based testing for reusable workflows
//
// # Quick Start
//
// Create a CLI session and interact with it:
//
//	config := &e2e.SessionConfig{
//		Command: "./mdz",
//		Timeout: 30 * time.Second,
//		Debug:   true,
//	}
//
//	session, _ := e2e.NewCLISession(config)
//	session.Start()
//	defer session.Close()
//
//	session.Type("help")
//	session.Press("enter")
//	session.WaitForOutput("Usage:", 5*time.Second)
//
// # Running Scenarios
//
// Execute predefined test scenarios:
//
//	scenarios := e2e.GetDefaultScenarios()
//	config := &e2e.RunnerConfig{
//		MDZBinary:   "./mdz",
//		OutputDir:   "./results",
//		AnalyzeFlow: true,
//	}
//
//	runner := e2e.NewScenarioRunner(config)
//	results, _ := runner.RunScenarios(scenarios)
//
// # Flow Analysis
//
// Automatically analyze user flows for UX improvements:
//
//	analyzer := e2e.NewFlowAnalyzer()
//	analysis, _ := analyzer.AnalyzeSession(session)
//
//	fmt.Printf("Flow efficiency: %.1f%%\n", analysis.FlowEfficiency*100)
//	fmt.Printf("UX issues: %d\n", len(analysis.UXIssues))
//
// # Command Line Tool
//
// Build and use the standalone e2e test runner:
//
//	go build -o e2e-test ./cmd/e2e-test
//	./e2e-test --binary=./mdz --debug
//
// See the README.md file for comprehensive documentation and examples.
package e2e