# MDZ CLI E2E Testing Framework

A comprehensive end-to-end testing framework for CLI applications, inspired by Playwright but designed specifically for command-line interfaces. This framework provides automated testing, session recording, flow analysis, and UX insights for the MDZ CLI.

## Features

🎭 **Playwright-like API** - Familiar testing patterns adapted for CLI
📹 **Session Recording** - Capture every interaction for replay and analysis  
🔍 **Flow Analysis** - Automatic UX analysis with actionable recommendations
📊 **Rich Reporting** - HTML and JSON reports with screenshots and metrics
🚀 **Performance Metrics** - Track response times and identify bottlenecks
🎯 **Scenario-based** - Reusable test scenarios for common workflows
🔧 **Configurable** - Flexible configuration for different testing needs

## Quick Start

### 1. Build the E2E Test Runner

```bash
# From the MDZ root directory
cd components/mdz
go build -o e2e-test ./cmd/e2e-test
```

### 2. Build MDZ Binary (if not already built)

```bash
go build -o mdz .
```

### 3. Run All Default Scenarios

```bash
./e2e-test --binary=./mdz --debug
```

### 4. Run Specific Scenario

```bash
./e2e-test --binary=./mdz --scenario=login_flow --debug
```

### 5. List Available Scenarios

```bash
./e2e-test --list
```

## Usage Examples

### Basic Session Usage

```go
package main

import (
    "time"
    "github.com/LerianStudio/midaz/components/mdz/pkg/e2e"
)

func main() {
    // Create session config
    config := &e2e.SessionConfig{
        Command:     "./mdz",
        Args:        []string{},
        Timeout:     30 * time.Second,
        Debug:       true,
        RecordPath:  "./recording.json",
    }

    // Start session
    session, _ := e2e.NewCLISession(config)
    session.Start()
    defer session.Close()

    // Interact with CLI
    session.Type("help")
    session.Press("enter")
    session.WaitForOutput("Usage:", 5*time.Second)
    
    // Capture state
    screenshot := session.Screenshot()
    output := session.GetOutput()
}
```

### Creating Custom Scenarios

```go
scenario := &e2e.Scenario{
    Name:        "custom_flow",
    Description: "My custom test flow",
    Steps: []e2e.Step{
        {
            Type:        "wait_for_output",
            ExpectText:  "Welcome",
            Timeout:     5 * time.Second,
            Description: "Wait for welcome message",
        },
        {
            Type:        "type",
            Input:       "organization list",
            Description: "List organizations",
        },
        {
            Type:        "press",
            Key:         "enter",
            Description: "Execute command",
        },
        {
            Type:        "screenshot",
            Description: "Capture result",
        },
    },
    Expected: []string{"Available organizations"},
    Setup: &e2e.Setup{
        Environment: map[string]string{
            "MDZ_TEST_MODE": "true",
        },
    },
}

// Run scenario
config := &e2e.RunnerConfig{
    MDZBinary:   "./mdz",
    OutputDir:   "./results",
    Debug:       true,
    AnalyzeFlow: true,
}

runner := e2e.NewScenarioRunner(config)
result, err := runner.RunScenario(scenario)
```

## Available Step Types

| Type | Description | Parameters |
|------|-------------|------------|
| `type` | Send text input | `input`: text to type |
| `press` | Send special keys | `key`: enter, tab, ctrl+c, etc. |
| `wait` | Wait for duration | `wait`: time.Duration |
| `wait_for_output` | Wait for text in output | `expect_text`, `timeout` |
| `wait_for_prompt` | Wait for prompt pattern | `expect_text`, `timeout` |
| `screenshot` | Capture terminal state | - |

## Special Keys

- `enter`, `return` - Enter key
- `tab` - Tab key  
- `ctrl+c`, `sigint` - Interrupt signal
- `ctrl+d`, `eof` - End of file
- `escape`, `esc` - Escape key
- `backspace` - Backspace
- `delete` - Delete key

## Default Scenarios

The framework includes predefined scenarios for common MDZ workflows:

- **login_flow** - Authentication and login process
- **organization_list** - Organization listing and selection
- **ledger_management** - Ledger creation and management
- **account_creation** - Account creation flow
- **transaction_flow** - Transaction creation process
- **repl_interactive** - REPL mode testing
- **error_handling** - Error scenarios and recovery
- **help_system** - Help system discoverability

## Flow Analysis Features

The framework automatically analyzes user flows and provides:

### Performance Metrics
- Total duration and average step times
- Response time analysis
- Slowest operations identification
- Menu load time tracking

### UX Issues Detection
- Slow response times (>3 seconds)
- High error rates
- Excessive input corrections
- Poor menu navigation efficiency

### Recommendations
- Performance optimizations
- Error handling improvements
- Usability enhancements
- Navigation simplifications

### Efficiency Metrics
- Flow efficiency percentage
- Completion rate tracking
- Interaction pattern analysis

## Reports

### HTML Report
Interactive HTML report with:
- Scenario results overview
- Step-by-step execution details
- Screenshots and terminal output
- Performance metrics
- UX analysis results

### JSON Report
Machine-readable JSON containing:
- Complete test results
- Session recordings
- Performance data
- Analysis results

### Session Recordings
Individual JSON files for each scenario containing:
- Complete event timeline
- User inputs and CLI responses
- Timing information
- Context data

## Configuration Options

### RunnerConfig
```go
type RunnerConfig struct {
    MDZBinary    string        // Path to MDZ binary
    OutputDir    string        // Results output directory
    Timeout      time.Duration // Scenario timeout
    Debug        bool          // Enable debug output
    RecordAll    bool          // Record all sessions
    AnalyzeFlow  bool          // Enable flow analysis
}
```

### SessionConfig
```go
type SessionConfig struct {
    Command     string            // Command to execute
    Args        []string          // Command arguments
    Env         map[string]string // Environment variables
    WorkingDir  string            // Working directory
    Timeout     time.Duration     // Session timeout
    RecordPath  string            // Recording file path
    Interactive bool              // Interactive mode
    Debug       bool              // Debug output
}
```

## Integration with CI/CD

### GitHub Actions Example

```yaml
name: E2E Tests

on: [push, pull_request]

jobs:
  e2e-tests:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      
      - name: Setup Go
        uses: actions/setup-go@v3
        with:
          go-version: '1.21'
          
      - name: Build MDZ
        run: |
          cd components/mdz
          go build -o mdz .
          
      - name: Build E2E Runner
        run: |
          cd components/mdz
          go build -o e2e-test ./cmd/e2e-test
          
      - name: Run E2E Tests
        run: |
          cd components/mdz
          ./e2e-test --binary=./mdz --output=./e2e-results
          
      - name: Upload Results
        uses: actions/upload-artifact@v3
        if: always()
        with:
          name: e2e-results
          path: components/mdz/e2e-results/
```

## Best Practices

### Writing Scenarios
1. **Be Specific** - Use precise expected text patterns
2. **Add Timeouts** - Set appropriate timeouts for operations
3. **Include Screenshots** - Capture key states for visual verification
4. **Use Descriptions** - Document each step for clarity
5. **Test Error Cases** - Include negative scenarios

### Performance Considerations
1. **Optimize Timeouts** - Use shorter timeouts for fast operations
2. **Parallel Execution** - Run independent scenarios in parallel
3. **Resource Cleanup** - Clean up test data between scenarios
4. **Mock External Services** - Use test mode when possible

### Debugging
1. **Enable Debug Mode** - Use `--debug` flag for detailed output
2. **Check Recordings** - Review JSON recordings for timing issues
3. **Use Screenshots** - Visual verification of CLI state
4. **Analyze Flow Reports** - Look for UX bottlenecks

## Advanced Usage

### Custom Flow Analysis

```go
analyzer := e2e.NewFlowAnalyzer()
analysis, err := analyzer.AnalyzeSession(session)

// Custom analysis logic
for _, issue := range analysis.UXIssues {
    if issue.Severity == "high" {
        // Handle critical UX issues
    }
}
```

### Session Replay

```go
// Load recorded session
recording := loadRecording("session.json")

// Replay with modifications
replaySession(recording, modifications)
```

### Performance Benchmarking

```go
func BenchmarkCLIOperation(b *testing.B) {
    for i := 0; i < b.N; i++ {
        // Run scenario and measure performance
        result := runScenario(scenario)
        recordMetrics(result.Duration)
    }
}
```

## Troubleshooting

### Common Issues

1. **Binary Not Found**
   ```bash
   # Ensure MDZ binary is built and path is correct
   go build -o mdz .
   ./e2e-test --binary=./mdz
   ```

2. **Timeout Errors**
   ```bash
   # Increase timeout for slow operations
   ./e2e-test --timeout=120s
   ```

3. **Permission Issues**
   ```bash
   # Ensure binary is executable
   chmod +x ./mdz
   ```

4. **Environment Variables**
   ```bash
   # Set test environment
   export MDZ_TEST_MODE=true
   ./e2e-test --binary=./mdz
   ```

### Debug Output

Enable debug mode to see detailed execution:

```bash
./e2e-test --binary=./mdz --debug --scenario=login_flow
```

This will show:
- Step-by-step execution
- CLI output in real-time
- Timing information
- Error details

## Contributing

To add new scenarios or improve the framework:

1. **Add Scenarios** - Create new scenarios in `scenarios.go`
2. **Extend Analysis** - Add new UX analysis rules in `flow_analyzer.go`
3. **Improve Reports** - Enhance HTML/JSON reporting
4. **Add Features** - Extend session capabilities

See the example tests in `example_test.go` for implementation patterns.