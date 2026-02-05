// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func lintMigration(path string) []Issue {
	var issues []Issue

	file, err := os.Open(path)
	if err != nil {
		return []Issue{{
			File:     filepath.Base(path),
			Line:     0,
			Severity: "ERROR",
			Message:  fmt.Sprintf("Could not open file: %v", err),
		}}
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)

	fileIgnored := false

	var lines []string

	for scanner.Scan() {
		line := scanner.Text()

		lines = append(lines, line)

		trimmedLine := strings.TrimSpace(line)

		if strings.Contains(trimmedLine, "-- lint:ignore-file") {
			fileIgnored = true
		}
	}

	if fileIgnored {
		return issues
	}

	for i, line := range lines {
		lineNum := i + 1

		trimmedLine := strings.TrimSpace(line)

		if strings.HasPrefix(trimmedLine, "--") || strings.HasPrefix(trimmedLine, "/*") {
			continue
		}

		if isLineIgnored(lines, i) {
			continue
		}

		for _, pattern := range dangerousPatterns {
			if pattern.Regex.MatchString(line) {
				if pattern.ExcludeIf != nil && pattern.ExcludeIf.MatchString(line) {
					continue
				}

				issues = append(issues, Issue{
					File:       filepath.Base(path),
					Line:       lineNum,
					Severity:   pattern.Severity,
					Message:    pattern.Message,
					Suggestion: pattern.Suggestion,
				})
			}
		}
	}

	if err := scanner.Err(); err != nil {
		issues = append(issues, Issue{
			File:     filepath.Base(path),
			Line:     0,
			Severity: "ERROR",
			Message:  fmt.Sprintf("Error reading file: %v", err),
		})
	}

	return issues
}

func isLineIgnored(lines []string, currentIndex int) bool {
	if currentIndex == 0 {
		return false
	}

	prevLine := strings.TrimSpace(lines[currentIndex-1])

	return strings.Contains(prevLine, "-- lint:ignore") && !strings.Contains(prevLine, "-- lint:ignore-file")
}

func printIssuesByFile(issues []Issue) {
	issuesByFile := make(map[string][]Issue)

	var fileOrder []string

	for _, issue := range issues {
		if _, exists := issuesByFile[issue.File]; !exists {
			fileOrder = append(fileOrder, issue.File)
		}

		issuesByFile[issue.File] = append(issuesByFile[issue.File], issue)
	}

	for _, file := range fileOrder {
		fileIssues := issuesByFile[file]

		fmt.Printf("---> %s\n", file)

		suggestionIndexes := make(map[string][]int)

		var suggestionOrder []string

		for i, issue := range fileIssues {
			idx := i + 1

			fmt.Printf("  %d. [%s] Line %d: %s\n", idx, issue.Severity, issue.Line, issue.Message)

			if issue.Suggestion != "" {
				if _, exists := suggestionIndexes[issue.Suggestion]; !exists {
					suggestionOrder = append(suggestionOrder, issue.Suggestion)
				}

				suggestionIndexes[issue.Suggestion] = append(suggestionIndexes[issue.Suggestion], idx)
			}
		}

		if len(suggestionOrder) > 0 {
			fmt.Println()

			fmt.Println("  Suggestions:")

			for _, suggestion := range suggestionOrder {
				indexes := suggestionIndexes[suggestion]

				indexStrs := make([]string, len(indexes))

				for i, idx := range indexes {
					indexStrs[i] = fmt.Sprintf("#%d", idx)
				}

				fmt.Printf("    [%s]\n", strings.Join(indexStrs, ", "))

				for _, line := range strings.Split(suggestion, "\n") {
					fmt.Printf("      %s\n", line)
				}

				fmt.Println()
			}
		} else {
			fmt.Println()
		}
	}
}

func printUsage() {
	fmt.Println("Usage: migration-lint <path-to-migrations> [--strict]")
	fmt.Println("")
	fmt.Println("Options:")
	fmt.Println("  --strict    Treat WARNINGs as ERRORs")
	fmt.Println("")
	fmt.Println("Example:")
	fmt.Println("  migration-lint ./components/onboarding/migrations")
	fmt.Println("  migration-lint ./components/transaction/migrations --strict")
	fmt.Println("")
	fmt.Println("Documentation:")
	fmt.Println("  Guidelines: scripts/migration_linter/docs/MIGRATION_GUIDELINES.md")
	fmt.Println("  Template:   scripts/migration_linter/docs/MIGRATION_TEMPLATE.md")
}

func parseArgs() (string, bool) {
	migrationPath := os.Args[1]
	strictMode := false

	for _, arg := range os.Args[2:] {
		if arg == "--strict" {
			strictMode = true
		}
	}

	return migrationPath, strictMode
}

func getMigrationFiles(migrationPath string) ([]string, error) {
	if _, err := os.Stat(migrationPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("directory not found: %s", migrationPath)
	}

	return filepath.Glob(filepath.Join(migrationPath, "*.up.sql"))
}

func analyzeFiles(files []string) ([]Issue, bool, bool) {
	var allIssues []Issue

	hasErrors := false

	hasWarnings := false

	for _, file := range files {
		issues := lintMigration(file)

		allIssues = append(allIssues, issues...)

		for _, issue := range issues {
			if issue.Severity == "ERROR" {
				hasErrors = true
			}

			if issue.Severity == "WARNING" {
				hasWarnings = true
			}
		}
	}

	return allIssues, hasErrors, hasWarnings
}

func countIssues(issues []Issue) (int, int) {
	errorCount := 0
	warningCount := 0

	for _, issue := range issues {
		switch issue.Severity {
		case "ERROR":
			errorCount++
		case "WARNING":
			warningCount++
		}
	}

	return errorCount, warningCount
}

func printSummary(errorCount, warningCount int) {
	fmt.Printf("Summary: %d error(s), %d warning(s)\n", errorCount, warningCount)
	fmt.Println("")
	fmt.Println("For more information, see:")
	fmt.Println("  Guidelines: scripts/migration_linter/docs/MIGRATION_GUIDELINES.md")
	fmt.Println("  Template:   scripts/migration_linter/docs/MIGRATION_TEMPLATE.md")
}

func main() {
	if len(os.Args) < 2 {
		printUsage()

		os.Exit(1)
	}

	migrationPath, strictMode := parseArgs()

	files, err := getMigrationFiles(migrationPath)
	if err != nil {
		fmt.Println(err)

		os.Exit(1)
	}

	if len(files) == 0 {
		fmt.Printf("No migration files found in: %s\n", migrationPath)

		os.Exit(0)
	}

	fmt.Printf("Analyzing %d migrations in %s\n\n", len(files), migrationPath)

	allIssues, hasErrors, hasWarnings := analyzeFiles(files)

	if len(allIssues) > 0 {
		printIssuesByFile(allIssues)
	}

	if len(allIssues) == 0 {
		fmt.Println("All migrations passed validation!")
		os.Exit(0)
	}

	errorCount, warningCount := countIssues(allIssues)

	printSummary(errorCount, warningCount)

	if hasErrors || (strictMode && hasWarnings) {
		fmt.Println("\nValidation failed!")

		os.Exit(1)
	}

	fmt.Println("\nValidation passed with warnings.")

	os.Exit(0)
}
