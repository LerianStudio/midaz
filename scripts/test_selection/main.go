// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"go/build"
	"go/build/constraint"
	"os"
	"regexp"
	"strings"
)

const scannerCapacity = 16 * 1024 * 1024

func main() {
	if len(os.Args) < 2 {
		exitUsage()
	}

	var err error
	switch os.Args[1] {
	case "filter-files":
		if len(os.Args) != 4 {
			exitUsage()
		}
		err = filterFiles(os.Args[2], os.Args[3])
	case "filter-tests":
		if len(os.Args) != 3 {
			exitUsage()
		}
		err = filterTests(os.Args[2])
	case "verify-events":
		if len(os.Args) != 3 {
			exitUsage()
		}
		err = verifyEvents(os.Args[2])
	default:
		exitUsage()
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "[error] %v\n", err)
		os.Exit(1)
	}
}

func exitUsage() {
	fmt.Fprintln(os.Stderr, "usage: test-selection <filter-files|filter-tests|verify-events> ...")
	os.Exit(2)
}

func filterFiles(requiredTag, buildTags string) error {
	enabledTags := configuredTags(buildTags)
	scanner := newScanner(os.Stdin)

	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.SplitN(line, "\t", 2)
		if len(fields) != 2 || fields[0] == "" || fields[1] == "" {
			return fmt.Errorf("invalid tagged-test file record %q", line)
		}

		expressions, err := readBuildConstraints(fields[1])
		if err != nil {
			return err
		}
		if len(expressions) == 0 {
			continue
		}

		withRequiredTag := evaluateAll(expressions, func(tag string) bool {
			if tag == requiredTag {
				return true
			}
			return enabledTags[tag]
		})
		withoutRequiredTag := evaluateAll(expressions, func(tag string) bool {
			if tag == requiredTag {
				return false
			}
			return enabledTags[tag]
		})

		if withRequiredTag && !withoutRequiredTag {
			fmt.Println(line)
		}
	}

	return scanner.Err()
}

func configuredTags(buildTags string) map[string]bool {
	tags := make(map[string]bool)
	for _, tag := range strings.FieldsFunc(buildTags, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t'
	}) {
		tags[tag] = true
	}

	context := build.Default
	tags[context.GOOS] = true
	tags[context.GOARCH] = true
	tags[context.Compiler] = true
	if context.CgoEnabled {
		tags["cgo"] = true
	}
	for _, tag := range context.BuildTags {
		tags[tag] = true
	}
	for _, tag := range context.ToolTags {
		tags[tag] = true
	}
	for _, tag := range context.ReleaseTags {
		tags[tag] = true
	}

	switch context.GOOS {
	case "aix", "android", "darwin", "dragonfly", "freebsd", "hurd", "illumos", "ios", "linux", "netbsd", "openbsd", "solaris":
		tags["unix"] = true
	}
	if context.GOOS == "android" {
		tags["linux"] = true
	}
	if context.GOOS == "illumos" {
		tags["solaris"] = true
	}
	if context.GOOS == "ios" {
		tags["darwin"] = true
	}

	return tags
}

func readBuildConstraints(path string) ([]constraint.Expr, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open build constraints from %s: %w", path, err)
	}
	defer file.Close()

	var goBuild constraint.Expr
	var plusBuild []constraint.Expr
	scanner := newScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "package ") {
			break
		}

		if constraint.IsGoBuild(line) {
			if goBuild != nil {
				return nil, fmt.Errorf("multiple //go:build lines in %s", path)
			}
			goBuild, err = constraint.Parse(line)
			if err != nil {
				return nil, fmt.Errorf("parse build constraint in %s: %w", path, err)
			}
			continue
		}
		if constraint.IsPlusBuild(line) {
			expression, parseErr := constraint.Parse(line)
			if parseErr != nil {
				return nil, fmt.Errorf("parse build constraint in %s: %w", path, parseErr)
			}
			plusBuild = append(plusBuild, expression)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read build constraints from %s: %w", path, err)
	}

	if goBuild != nil {
		return []constraint.Expr{goBuild}, nil
	}
	return plusBuild, nil
}

func evaluateAll(expressions []constraint.Expr, tagValue func(string) bool) bool {
	for _, expression := range expressions {
		if !expression.Eval(tagValue) {
			return false
		}
	}
	return true
}

func filterTests(runPattern string) error {
	pattern, err := regexp.Compile(runPattern)
	if err != nil {
		return fmt.Errorf("invalid RUN pattern %q: %w", runPattern, err)
	}

	selected := 0
	scanner := newScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 {
			return fmt.Errorf("invalid tagged-test inventory record %q", line)
		}
		if pattern.MatchString(fields[1]) {
			fmt.Println(line)
			selected++
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if selected == 0 {
		return fmt.Errorf("run pattern %q selected zero runnable tests", runPattern)
	}
	return nil
}

type testEvent struct {
	Action string `json:"Action"`
	Test   string `json:"Test"`
}

func verifyEvents(runPattern string) error {
	patterns, err := compileRunPattern(runPattern)
	if err != nil {
		return err
	}

	scanner := newScanner(os.Stdin)
	for scanner.Scan() {
		var event testEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return fmt.Errorf("decode go test JSON event: %w", err)
		}
		if event.Action == "run" && matchesRunPattern(patterns, event.Test) {
			return nil
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return fmt.Errorf("run pattern %q started zero tests matching the full name", runPattern)
}

func compileRunPattern(runPattern string) ([]*regexp.Regexp, error) {
	parts := splitRunPattern(runPattern)
	patterns := make([]*regexp.Regexp, 0, len(parts))
	for _, part := range parts {
		pattern, err := regexp.Compile(part)
		if err != nil {
			return nil, fmt.Errorf("invalid RUN pattern %q: %w", runPattern, err)
		}
		patterns = append(patterns, pattern)
	}
	return patterns, nil
}

func splitRunPattern(pattern string) []string {
	var parts []string
	start := 0
	bracketDepth := 0
	for index := 0; index < len(pattern); index++ {
		switch pattern[index] {
		case '\\':
			index++
		case '[':
			bracketDepth++
		case ']':
			if bracketDepth > 0 {
				bracketDepth--
			}
		case '/':
			if bracketDepth == 0 {
				parts = append(parts, pattern[start:index])
				start = index + 1
			}
		}
	}
	return append(parts, pattern[start:])
}

func matchesRunPattern(patterns []*regexp.Regexp, testName string) bool {
	nameParts := strings.Split(testName, "/")
	if len(nameParts) < len(patterns) {
		return false
	}
	for index, pattern := range patterns {
		if !pattern.MatchString(nameParts[index]) {
			return false
		}
	}
	return true
}

func newScanner(file *os.File) *bufio.Scanner {
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), scannerCapacity)
	return scanner
}
