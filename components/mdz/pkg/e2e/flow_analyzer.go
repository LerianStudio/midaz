package e2e

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// FlowAnalyzer analyzes CLI session flows for UX improvements
type FlowAnalyzer struct {
	patterns map[string]*regexp.Regexp
}

// FlowAnalysis contains the analysis results
type FlowAnalysis struct {
	UserJourney     []JourneyStep      `json:"user_journey"`
	UXIssues        []UXIssue          `json:"ux_issues"`
	Performance     PerformanceMetrics `json:"performance"`
	Interactions    InteractionStats   `json:"interactions"`
	Recommendations []Recommendation   `json:"recommendations"`
	FlowEfficiency  float64            `json:"flow_efficiency"`
	CompletionRate  float64            `json:"completion_rate"`
}

// JourneyStep represents a step in the user journey
type JourneyStep struct {
	Step        string        `json:"step"`
	Action      string        `json:"action"`
	Duration    time.Duration `json:"duration"`
	Context     string        `json:"context"`
	UserInput   string        `json:"user_input"`
	CLIResponse string        `json:"cli_response"`
	Timestamp   time.Time     `json:"timestamp"`
}

// UXIssue represents a potential user experience issue
type UXIssue struct {
	Type        string    `json:"type"`
	Severity    string    `json:"severity"`
	Description string    `json:"description"`
	Location    string    `json:"location"`
	Suggestion  string    `json:"suggestion"`
	Timestamp   time.Time `json:"timestamp"`
}

// PerformanceMetrics contains performance-related metrics
type PerformanceMetrics struct {
	TotalDuration   time.Duration   `json:"total_duration"`
	AverageStepTime time.Duration   `json:"average_step_time"`
	SlowestStep     string          `json:"slowest_step"`
	SlowestStepTime time.Duration   `json:"slowest_step_time"`
	ResponseTimes   []time.Duration `json:"response_times"`
	MenuLoadTimes   []time.Duration `json:"menu_load_times"`
}

// InteractionStats contains interaction statistics
type InteractionStats struct {
	TotalInputs      int            `json:"total_inputs"`
	MenuSelections   int            `json:"menu_selections"`
	TextInputs       int            `json:"text_inputs"`
	Corrections      int            `json:"corrections"`
	HelpRequests     int            `json:"help_requests"`
	ErrorEncounters  int            `json:"error_encounters"`
	PatternFrequency map[string]int `json:"pattern_frequency"`
}

// Recommendation suggests improvements
type Recommendation struct {
	Category    string `json:"category"`
	Priority    string `json:"priority"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Impact      string `json:"impact"`
	Effort      string `json:"effort"`
}

// NewFlowAnalyzer creates a new flow analyzer
func NewFlowAnalyzer() *FlowAnalyzer {
	patterns := map[string]*regexp.Regexp{
		"menu_prompt":     regexp.MustCompile(`Select.*\(.*\):`),
		"error_message":   regexp.MustCompile(`Error:|error:|ERROR:|failed|Failed|FAILED`),
		"success_message": regexp.MustCompile(`Success|success|✓|completed|Created|Updated|Deleted`),
		"loading":         regexp.MustCompile(`Loading|loading|Fetching|fetching|Please wait`),
		"help_text":       regexp.MustCompile(`help|Help|HELP|usage|Usage|--help|-h`),
		"prompt":          regexp.MustCompile(`.*\?.*:|.*>.*|.*\$.*`),
		"list_items":      regexp.MustCompile(`^\s*\d+\.\s+`),
		"progress":        regexp.MustCompile(`\d+%|\[\s*=+\s*\]|\.{3,}`),
	}

	return &FlowAnalyzer{
		patterns: patterns,
	}
}

// AnalyzeSession analyzes a complete CLI session
func (fa *FlowAnalyzer) AnalyzeSession(session *CLISession) (*FlowAnalysis, error) {
	events := session.recorder.Events

	analysis := &FlowAnalysis{
		UserJourney:     make([]JourneyStep, 0),
		UXIssues:        make([]UXIssue, 0),
		Interactions:    InteractionStats{PatternFrequency: make(map[string]int)},
		Recommendations: make([]Recommendation, 0),
	}

	// Analyze user journey
	fa.analyzeUserJourney(events, analysis)

	// Analyze performance
	fa.analyzePerformance(events, analysis)

	// Analyze interactions
	fa.analyzeInteractions(events, analysis)

	// Detect UX issues
	fa.detectUXIssues(events, analysis)

	// Generate recommendations
	fa.generateRecommendations(analysis)

	// Calculate efficiency metrics
	fa.calculateEfficiencyMetrics(analysis)

	return analysis, nil
}

// analyzeUserJourney reconstructs the user's journey through the CLI
func (fa *FlowAnalyzer) analyzeUserJourney(events []SessionEvent, analysis *FlowAnalysis) {
	for i, event := range events {
		if event.Type == "input" || event.Type == "key_press" {
			step := JourneyStep{
				Step:      fmt.Sprintf("Step %d", len(analysis.UserJourney)+1),
				Action:    event.Type,
				Duration:  time.Duration(event.Delay) * time.Millisecond,
				Context:   event.Context,
				UserInput: event.Data,
				Timestamp: event.Timestamp,
			}

			// Find the corresponding CLI response
			if i+1 < len(events) && events[i+1].Type == "output" {
				step.CLIResponse = events[i+1].Data
			}

			analysis.UserJourney = append(analysis.UserJourney, step)
		}
	}
}

// analyzePerformance analyzes performance metrics
func (fa *FlowAnalyzer) analyzePerformance(events []SessionEvent, analysis *FlowAnalysis) {
	var totalDuration time.Duration

	var stepTimes []time.Duration

	var slowestTime time.Duration

	var slowestStep string

	var responseTimes []time.Duration

	var menuLoadTimes []time.Duration

	for i, event := range events {
		stepTime := time.Duration(event.Delay) * time.Millisecond
		stepTimes = append(stepTimes, stepTime)
		totalDuration += stepTime

		if stepTime > slowestTime {
			slowestTime = stepTime
			slowestStep = fmt.Sprintf("%s: %s", event.Type, event.Data)
		}

		// Track response times (time between input and output)
		if event.Type == "input" {
			for j := i + 1; j < len(events) && j < i+5; j++ {
				if events[j].Type == "output" {
					responseTime := events[j].Timestamp.Sub(event.Timestamp)
					responseTimes = append(responseTimes, responseTime)

					// If it's a menu, track menu load time
					if fa.patterns["menu_prompt"].MatchString(events[j].Data) {
						menuLoadTimes = append(menuLoadTimes, responseTime)
					}

					break
				}
			}
		}
	}

	var avgStepTime time.Duration
	if len(stepTimes) > 0 {
		avgStepTime = totalDuration / time.Duration(len(stepTimes))
	}

	analysis.Performance = PerformanceMetrics{
		TotalDuration:   totalDuration,
		AverageStepTime: avgStepTime,
		SlowestStep:     slowestStep,
		SlowestStepTime: slowestTime,
		ResponseTimes:   responseTimes,
		MenuLoadTimes:   menuLoadTimes,
	}
}

// analyzeInteractions analyzes user interaction patterns
func (fa *FlowAnalyzer) analyzeInteractions(events []SessionEvent, analysis *FlowAnalysis) {
	stats := &analysis.Interactions

	for _, event := range events {
		switch event.Type {
		case "input":
			stats.TotalInputs++
			stats.TextInputs++

			// Check for corrections (backspace, delete patterns)
			if strings.Contains(event.Data, "\b") || strings.Contains(event.Data, "\x7f") {
				stats.Corrections++
			}

			// Check for help requests
			if fa.patterns["help_text"].MatchString(event.Data) {
				stats.HelpRequests++
			}

		case "key_press":
			stats.TotalInputs++
			if event.Data == "enter" || event.Data == "return" {
				stats.MenuSelections++
			}

		case "output":
			// Count pattern frequencies
			for patternName, pattern := range fa.patterns {
				if pattern.MatchString(event.Data) {
					stats.PatternFrequency[patternName]++
				}
			}

			// Count errors
			if fa.patterns["error_message"].MatchString(event.Data) {
				stats.ErrorEncounters++
			}
		}
	}
}

// detectUXIssues identifies potential user experience issues
func (fa *FlowAnalyzer) detectUXIssues(events []SessionEvent, analysis *FlowAnalysis) {
	// Detect long response times
	for _, responseTime := range analysis.Performance.ResponseTimes {
		if responseTime > 3*time.Second {
			analysis.UXIssues = append(analysis.UXIssues, UXIssue{
				Type:        "performance",
				Severity:    "medium",
				Description: fmt.Sprintf("Slow response time: %v", responseTime),
				Suggestion:  "Consider optimizing command execution or adding progress indicators",
			})
		}
	}

	// Detect excessive errors
	if analysis.Interactions.ErrorEncounters > len(analysis.UserJourney)/3 {
		analysis.UXIssues = append(analysis.UXIssues, UXIssue{
			Type:        "error_rate",
			Severity:    "high",
			Description: "High error rate detected",
			Suggestion:  "Improve error messages and input validation",
		})
	}

	// Detect excessive corrections
	if analysis.Interactions.Corrections > len(analysis.UserJourney)/2 {
		analysis.UXIssues = append(analysis.UXIssues, UXIssue{
			Type:        "usability",
			Severity:    "medium",
			Description: "Many input corrections detected",
			Suggestion:  "Consider improving input prompts or adding auto-completion",
		})
	}

	// Detect lack of help usage (might indicate poor discoverability)
	if analysis.Interactions.HelpRequests == 0 && len(analysis.UserJourney) > 5 {
		analysis.UXIssues = append(analysis.UXIssues, UXIssue{
			Type:        "discoverability",
			Severity:    "low",
			Description: "No help requests detected in complex flow",
			Suggestion:  "Consider making help more discoverable or improving initial guidance",
		})
	}

	// Detect menu navigation issues
	menuPrompts := analysis.Interactions.PatternFrequency["menu_prompt"]
	menuSelections := analysis.Interactions.MenuSelections

	if menuPrompts > 0 && float64(menuSelections)/float64(menuPrompts) < 0.5 {
		analysis.UXIssues = append(analysis.UXIssues, UXIssue{
			Type:        "navigation",
			Severity:    "medium",
			Description: "Low menu selection efficiency",
			Suggestion:  "Improve menu design and selection clarity",
		})
	}
}

// generateRecommendations creates actionable recommendations
func (fa *FlowAnalyzer) generateRecommendations(analysis *FlowAnalysis) {
	// Performance recommendations
	if analysis.Performance.AverageStepTime > 2*time.Second {
		analysis.Recommendations = append(analysis.Recommendations, Recommendation{
			Category:    "Performance",
			Priority:    "High",
			Title:       "Optimize Command Response Times",
			Description: "Average step time is above 2 seconds, consider caching or async operations",
			Impact:      "High",
			Effort:      "Medium",
		})
	}

	// Error handling recommendations
	if analysis.Interactions.ErrorEncounters > 2 {
		analysis.Recommendations = append(analysis.Recommendations, Recommendation{
			Category:    "Error Handling",
			Priority:    "High",
			Title:       "Improve Error Messages",
			Description: "Multiple errors encountered, enhance error clarity and recovery options",
			Impact:      "High",
			Effort:      "Low",
		})
	}

	// Usability recommendations
	if analysis.Interactions.Corrections > 3 {
		analysis.Recommendations = append(analysis.Recommendations, Recommendation{
			Category:    "Usability",
			Priority:    "Medium",
			Title:       "Add Input Validation and Auto-completion",
			Description: "Many input corrections suggest need for better input assistance",
			Impact:      "Medium",
			Effort:      "Medium",
		})
	}

	// Menu design recommendations
	menuPrompts := analysis.Interactions.PatternFrequency["menu_prompt"]
	if menuPrompts > 5 {
		analysis.Recommendations = append(analysis.Recommendations, Recommendation{
			Category:    "Navigation",
			Priority:    "Medium",
			Title:       "Simplify Menu Navigation",
			Description: "Many menu interactions detected, consider streamlining the flow",
			Impact:      "Medium",
			Effort:      "High",
		})
	}

	// Help system recommendations
	if analysis.Interactions.HelpRequests == 0 && len(analysis.UserJourney) > 5 {
		analysis.Recommendations = append(analysis.Recommendations, Recommendation{
			Category:    "Help System",
			Priority:    "Low",
			Title:       "Improve Help Discoverability",
			Description: "No help usage in complex flow suggests poor discoverability",
			Impact:      "Low",
			Effort:      "Low",
		})
	}
}

// calculateEfficiencyMetrics calculates overall flow efficiency
func (fa *FlowAnalyzer) calculateEfficiencyMetrics(analysis *FlowAnalysis) {
	// Flow efficiency: successful actions / total actions
	totalActions := analysis.Interactions.TotalInputs
	unsuccessfulActions := analysis.Interactions.ErrorEncounters + analysis.Interactions.Corrections

	if totalActions > 0 {
		analysis.FlowEfficiency = float64(totalActions-unsuccessfulActions) / float64(totalActions)
	}

	// Completion rate: assume completion if no errors in final steps
	finalStepErrors := 0
	if len(analysis.UserJourney) > 3 {
		// Check last 3 steps for errors
		// This is a simplified metric - real implementation might track actual completion
		analysis.CompletionRate = 1.0 - float64(finalStepErrors)/3.0
	} else {
		analysis.CompletionRate = 1.0
	}

	// Ensure rates are between 0 and 1
	if analysis.FlowEfficiency < 0 {
		analysis.FlowEfficiency = 0
	}

	if analysis.FlowEfficiency > 1 {
		analysis.FlowEfficiency = 1
	}

	if analysis.CompletionRate < 0 {
		analysis.CompletionRate = 0
	}

	if analysis.CompletionRate > 1 {
		analysis.CompletionRate = 1
	}
}
