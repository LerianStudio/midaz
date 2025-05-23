package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ScenarioRunner executes predefined test scenarios
type ScenarioRunner struct {
	config   *RunnerConfig
	results  []ScenarioResult
	analyzer *FlowAnalyzer
}

// RunnerConfig configures the scenario runner
type RunnerConfig struct {
	MDZBinary    string
	OutputDir    string
	Timeout      time.Duration
	Debug        bool
	RecordAll    bool
	AnalyzeFlow  bool
}

// Scenario represents a test scenario to execute
type Scenario struct {
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Steps       []Step    `json:"steps"`
	Expected    []string  `json:"expected_outputs"`
	Setup       *Setup    `json:"setup,omitempty"`
	Cleanup     *Cleanup  `json:"cleanup,omitempty"`
}

// Step represents a single interaction step
type Step struct {
	Type        string        `json:"type"`
	Action      string        `json:"action"`
	Input       string        `json:"input,omitempty"`
	Key         string        `json:"key,omitempty"`
	Wait        time.Duration `json:"wait,omitempty"`
	Timeout     time.Duration `json:"timeout,omitempty"`
	ExpectText  string        `json:"expect_text,omitempty"`
	Description string        `json:"description,omitempty"`
}

// Setup configures the environment before running a scenario
type Setup struct {
	Environment map[string]string `json:"environment"`
	WorkingDir  string            `json:"working_dir"`
	Commands    []string          `json:"commands"`
}

// Cleanup defines cleanup actions after scenario completion
type Cleanup struct {
	Commands    []string          `json:"commands"`
	Environment []string          `json:"remove_env"`
}

// ScenarioResult contains the results of a scenario execution
type ScenarioResult struct {
	Scenario    string           `json:"scenario"`
	Success     bool             `json:"success"`
	Duration    time.Duration    `json:"duration"`
	Error       string           `json:"error,omitempty"`
	Steps       []StepResult     `json:"steps"`
	Recording   string           `json:"recording_path"`
	Screenshot  *TerminalSnapshot `json:"final_screenshot"`
	Analysis    *FlowAnalysis    `json:"analysis,omitempty"`
}

// StepResult contains the result of a single step
type StepResult struct {
	Step        Step          `json:"step"`
	Success     bool          `json:"success"`
	Duration    time.Duration `json:"duration"`
	Error       string        `json:"error,omitempty"`
	OutputMatch bool          `json:"output_match"`
	Screenshot  *TerminalSnapshot `json:"screenshot,omitempty"`
}

// NewScenarioRunner creates a new scenario runner
func NewScenarioRunner(config *RunnerConfig) *ScenarioRunner {
	if config.Timeout == 0 {
		config.Timeout = 60 * time.Second
	}
	
	if config.OutputDir == "" {
		config.OutputDir = "./e2e-results"
	}

	return &ScenarioRunner{
		config:   config,
		results:  make([]ScenarioResult, 0),
		analyzer: NewFlowAnalyzer(),
	}
}

// RunScenario executes a single scenario
func (sr *ScenarioRunner) RunScenario(scenario *Scenario) (*ScenarioResult, error) {
	startTime := time.Now()
	
	result := &ScenarioResult{
		Scenario: scenario.Name,
		Steps:    make([]StepResult, 0),
	}

	// Setup
	if scenario.Setup != nil {
		if err := sr.executeSetup(scenario.Setup); err != nil {
			result.Error = fmt.Sprintf("Setup failed: %v", err)
			result.Success = false
			result.Duration = time.Since(startTime)
			return result, err
		}
	}

	// Prepare session config
	sessionConfig := &SessionConfig{
		Command:    sr.config.MDZBinary,
		Args:       []string{}, // Can be customized per scenario
		Timeout:    sr.config.Timeout,
		Debug:      sr.config.Debug,
		RecordPath: filepath.Join(sr.config.OutputDir, fmt.Sprintf("%s.json", scenario.Name)),
	}

	if scenario.Setup != nil {
		sessionConfig.Env = scenario.Setup.Environment
		sessionConfig.WorkingDir = scenario.Setup.WorkingDir
	}

	// Create and start session
	session, err := NewCLISession(sessionConfig)
	if err != nil {
		result.Error = fmt.Sprintf("Failed to create session: %v", err)
		result.Success = false
		result.Duration = time.Since(startTime)
		return result, err
	}

	if err := session.Start(); err != nil {
		result.Error = fmt.Sprintf("Failed to start session: %v", err)
		result.Success = false
		result.Duration = time.Since(startTime)
		return result, err
	}

	defer session.Close()

	// Execute steps
	for i, step := range scenario.Steps {
		stepResult := sr.executeStep(session, &step, i+1)
		result.Steps = append(result.Steps, *stepResult)
		
		if !stepResult.Success {
			result.Success = false
			result.Error = fmt.Sprintf("Step %d failed: %s", i+1, stepResult.Error)
			break
		}
	}

	// If all steps passed, check expected outputs
	if result.Error == "" {
		output := session.GetOutput()
		for _, expected := range scenario.Expected {
			if !strings.Contains(output, expected) {
				result.Success = false
				result.Error = fmt.Sprintf("Expected output not found: %s", expected)
				break
			}
		}
		
		if result.Error == "" {
			result.Success = true
		}
	}

	// Take final screenshot
	result.Screenshot = session.Screenshot()
	result.Duration = time.Since(startTime)
	result.Recording = sessionConfig.RecordPath

	// Analyze flow if enabled
	if sr.config.AnalyzeFlow {
		analysis, err := sr.analyzer.AnalyzeSession(session)
		if err == nil {
			result.Analysis = analysis
		}
	}

	// Cleanup
	if scenario.Cleanup != nil {
		sr.executeCleanup(scenario.Cleanup)
	}

	sr.results = append(sr.results, *result)
	return result, nil
}

// executeStep executes a single scenario step
func (sr *ScenarioRunner) executeStep(session *CLISession, step *Step, stepNumber int) *StepResult {
	startTime := time.Now()
	
	result := &StepResult{
		Step: *step,
	}

	if sr.config.Debug {
		fmt.Printf("Step %d: %s - %s\n", stepNumber, step.Type, step.Description)
	}

	var err error
	
	switch step.Type {
	case "type":
		err = session.Type(step.Input)
	case "press":
		err = session.Press(step.Key)
	case "wait":
		session.Wait(step.Wait)
	case "wait_for_output":
		timeout := step.Timeout
		if timeout == 0 {
			timeout = 5 * time.Second
		}
		err = session.WaitForOutput(step.ExpectText, timeout)
		result.OutputMatch = err == nil
	case "wait_for_prompt":
		timeout := step.Timeout
		if timeout == 0 {
			timeout = 5 * time.Second
		}
		err = session.WaitForPrompt(step.ExpectText, timeout)
		result.OutputMatch = err == nil
	case "screenshot":
		result.Screenshot = session.Screenshot()
	default:
		err = fmt.Errorf("unknown step type: %s", step.Type)
	}

	result.Success = err == nil
	if err != nil {
		result.Error = err.Error()
	}
	
	result.Duration = time.Since(startTime)
	return result
}

// executeSetup executes scenario setup
func (sr *ScenarioRunner) executeSetup(setup *Setup) error {
	// Set environment variables
	for k, v := range setup.Environment {
		os.Setenv(k, v)
	}
	
	// Execute setup commands
	for _, cmd := range setup.Commands {
		if sr.config.Debug {
			fmt.Printf("Setup: %s\n", cmd)
		}
		// Could execute setup commands here if needed
	}
	
	return nil
}

// executeCleanup executes scenario cleanup
func (sr *ScenarioRunner) executeCleanup(cleanup *Cleanup) {
	// Remove environment variables
	for _, env := range cleanup.Environment {
		os.Unsetenv(env)
	}
	
	// Execute cleanup commands
	for _, cmd := range cleanup.Commands {
		if sr.config.Debug {
			fmt.Printf("Cleanup: %s\n", cmd)
		}
		// Could execute cleanup commands here if needed
	}
}

// RunScenarios executes multiple scenarios
func (sr *ScenarioRunner) RunScenarios(scenarios []*Scenario) ([]*ScenarioResult, error) {
	// Ensure output directory exists
	if err := os.MkdirAll(sr.config.OutputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	results := make([]*ScenarioResult, 0, len(scenarios))
	
	for _, scenario := range scenarios {
		fmt.Printf("Running scenario: %s\n", scenario.Name)
		
		result, err := sr.RunScenario(scenario)
		if err != nil {
			fmt.Printf("Scenario %s failed: %v\n", scenario.Name, err)
		} else if result.Success {
			fmt.Printf("Scenario %s passed ✓\n", scenario.Name)
		} else {
			fmt.Printf("Scenario %s failed: %s\n", scenario.Name, result.Error)
		}
		
		results = append(results, result)
	}

	// Generate summary report
	if err := sr.GenerateReport(results); err != nil {
		fmt.Printf("Warning: Failed to generate report: %v\n", err)
	}

	return results, nil
}

// GenerateReport creates a comprehensive test report
func (sr *ScenarioRunner) GenerateReport(results []*ScenarioResult) error {
	report := TestReport{
		Timestamp:    time.Now(),
		TotalScenarios: len(results),
		Results:      results,
	}

	// Calculate summary statistics
	for _, result := range results {
		if result.Success {
			report.PassedScenarios++
		} else {
			report.FailedScenarios++
		}
		report.TotalDuration += result.Duration
	}

	// Save JSON report
	jsonPath := filepath.Join(sr.config.OutputDir, "report.json")
	jsonData, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal report: %w", err)
	}
	
	if err := os.WriteFile(jsonPath, jsonData, 0644); err != nil {
		return fmt.Errorf("failed to write JSON report: %w", err)
	}

	// Generate HTML report
	htmlPath := filepath.Join(sr.config.OutputDir, "report.html")
	if err := sr.generateHTMLReport(&report, htmlPath); err != nil {
		return fmt.Errorf("failed to generate HTML report: %w", err)
	}

	fmt.Printf("Reports generated:\n")
	fmt.Printf("  JSON: %s\n", jsonPath)
	fmt.Printf("  HTML: %s\n", htmlPath)

	return nil
}

// TestReport contains the complete test execution report
type TestReport struct {
	Timestamp      time.Time        `json:"timestamp"`
	TotalScenarios int              `json:"total_scenarios"`
	PassedScenarios int             `json:"passed_scenarios"`
	FailedScenarios int             `json:"failed_scenarios"`
	TotalDuration  time.Duration    `json:"total_duration"`
	Results        []*ScenarioResult `json:"results"`
}

// generateHTMLReport creates an HTML report
func (sr *ScenarioRunner) generateHTMLReport(report *TestReport, path string) error {
	html := `<!DOCTYPE html>
<html>
<head>
    <title>MDZ CLI E2E Test Report</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 20px; }
        .header { background: #f5f5f5; padding: 20px; border-radius: 5px; }
        .summary { display: flex; gap: 20px; margin: 20px 0; }
        .metric { background: white; padding: 15px; border: 1px solid #ddd; border-radius: 5px; }
        .passed { color: green; }
        .failed { color: red; }
        .scenario { margin: 20px 0; padding: 15px; border: 1px solid #ddd; border-radius: 5px; }
        .steps { margin-left: 20px; }
        .step { margin: 5px 0; padding: 5px; background: #f9f9f9; }
        .screenshot { margin: 10px 0; }
        .output { background: #f0f0f0; padding: 10px; font-family: monospace; white-space: pre; }
    </style>
</head>
<body>
    <div class="header">
        <h1>MDZ CLI E2E Test Report</h1>
        <p>Generated: ` + report.Timestamp.Format("2006-01-02 15:04:05") + `</p>
    </div>
    
    <div class="summary">
        <div class="metric">
            <h3>Total Scenarios</h3>
            <p>` + fmt.Sprintf("%d", report.TotalScenarios) + `</p>
        </div>
        <div class="metric">
            <h3 class="passed">Passed</h3>
            <p>` + fmt.Sprintf("%d", report.PassedScenarios) + `</p>
        </div>
        <div class="metric">
            <h3 class="failed">Failed</h3>
            <p>` + fmt.Sprintf("%d", report.FailedScenarios) + `</p>
        </div>
        <div class="metric">
            <h3>Duration</h3>
            <p>` + report.TotalDuration.String() + `</p>
        </div>
    </div>`

	for _, result := range report.Results {
		status := "passed"
		if !result.Success {
			status = "failed"
		}
		
		html += fmt.Sprintf(`
    <div class="scenario %s">
        <h3>%s (%s)</h3>
        <p>Duration: %s</p>`, status, result.Scenario, status, result.Duration.String())
		
		if result.Error != "" {
			html += fmt.Sprintf(`<p class="failed">Error: %s</p>`, result.Error)
		}
		
		if result.Screenshot != nil {
			html += `<div class="screenshot"><h4>Final Screenshot:</h4><div class="output">`
			for _, line := range result.Screenshot.Lines {
				html += line + "\n"
			}
			html += `</div></div>`
		}
		
		html += `</div>`
	}

	html += `</body></html>`

	return os.WriteFile(path, []byte(html), 0644)
}