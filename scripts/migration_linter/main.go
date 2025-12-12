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
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		trimmedLine := strings.TrimSpace(line)
		if strings.HasPrefix(trimmedLine, "--") || strings.HasPrefix(trimmedLine, "/*") {
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

func main() {
	if len(os.Args) < 2 {
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
		os.Exit(1)
	}

	migrationPath := os.Args[1]
	strictMode := false

	for _, arg := range os.Args[2:] {
		if arg == "--strict" {
			strictMode = true
		}
	}

	if _, err := os.Stat(migrationPath); os.IsNotExist(err) {
		fmt.Printf("Directory not found: %s\n", migrationPath)
		os.Exit(1)
	}

	files, err := filepath.Glob(filepath.Join(migrationPath, "*.up.sql"))
	if err != nil {
		fmt.Printf("Error searching files: %v\n", err)
		os.Exit(1)
	}

	if len(files) == 0 {
		fmt.Printf("No migration files found in: %s\n", migrationPath)
		os.Exit(0)
	}

	fmt.Printf("Analyzing %d migrations in %s\n\n", len(files), migrationPath)

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

	if len(allIssues) > 0 {
		printIssuesByFile(allIssues)
	}

	errorCount := 0
	warningCount := 0
	for _, issue := range allIssues {
		switch issue.Severity {
		case "ERROR":
			errorCount++
		case "WARNING":
			warningCount++
		}
	}

	if len(allIssues) == 0 {
		fmt.Println("All migrations passed validation!")
		os.Exit(0)
	}

	fmt.Printf("Summary: %d error(s), %d warning(s)\n", errorCount, warningCount)
	fmt.Println("")
	fmt.Println("For more information, see:")
	fmt.Println("  Guidelines: scripts/migration_linter/docs/MIGRATION_GUIDELINES.md")
	fmt.Println("  Template:   scripts/migration_linter/docs/MIGRATION_TEMPLATE.md")

	if hasErrors || (strictMode && hasWarnings) {
		fmt.Println("\nValidation failed!")
		os.Exit(1)
	}

	fmt.Println("\nValidation passed with warnings.")
	os.Exit(0)
}
