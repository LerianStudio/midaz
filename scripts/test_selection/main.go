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
	"strconv"
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
	case "prepare-run":
		if len(os.Args) != 4 {
			exitUsage()
		}
		err = prepareRun(os.Args[2], os.Args[3])
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
	fmt.Fprintln(os.Stderr, "usage: test-selection <filter-files|prepare-run|verify-events> ...")
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

type testEvent struct {
	Action string `json:"Action"`
	Test   string `json:"Test"`
}

func verifyEvents(runPattern string) error {
	filter, err := parseRunPattern(runPattern)
	if err != nil {
		return err
	}

	scanner := newScanner(os.Stdin)
	for scanner.Scan() {
		var event testEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return fmt.Errorf("decode go test JSON event: %w", err)
		}
		if event.Action == "run" {
			if filter.matchesAnyCompleteAlternative(strings.Split(event.Test, "/")) {
				return nil
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return fmt.Errorf("run pattern %q started zero tests matching the full name", runPattern)
}

type taggedTest struct {
	packagePath string
	name        string
	line        string
}

type packageTests struct {
	packagePath string
	tests       []taggedTest
}

func prepareRun(runPattern, runtimePatternsPath string) error {
	filter, err := parseRunPattern(runPattern)
	if err != nil {
		return err
	}

	packages := make([]packageTests, 0)
	packageIndexes := make(map[string]int)
	selected := make([]taggedTest, 0)
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

		matched, _ := filter.matches([]string{fields[1]})
		if !matched {
			continue
		}

		test := taggedTest{packagePath: fields[0], name: fields[1], line: line}
		selected = append(selected, test)
		packageIndex, exists := packageIndexes[test.packagePath]
		if !exists {
			packageIndex = len(packages)
			packageIndexes[test.packagePath] = packageIndex
			packages = append(packages, packageTests{packagePath: test.packagePath})
		}
		packages[packageIndex].tests = append(packages[packageIndex].tests, test)
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if len(selected) == 0 {
		return fmt.Errorf("run pattern %q selected zero runnable tests", runPattern)
	}

	patternsFile, err := os.OpenFile(runtimePatternsPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("create runtime RUN patterns: %w", err)
	}
	patternsWriter := bufio.NewWriter(patternsFile)
	for _, packageSelection := range packages {
		runtimePattern, patternErr := filter.runtimePattern(packageSelection.tests)
		if patternErr != nil {
			_ = patternsFile.Close()
			return patternErr
		}
		if _, err := fmt.Fprintf(patternsWriter, "%s\t%s\n", packageSelection.packagePath, runtimePattern); err != nil {
			_ = patternsFile.Close()
			return fmt.Errorf("write runtime RUN patterns: %w", err)
		}
	}
	if err := patternsWriter.Flush(); err != nil {
		_ = patternsFile.Close()
		return fmt.Errorf("write runtime RUN patterns: %w", err)
	}
	if err := patternsFile.Close(); err != nil {
		return fmt.Errorf("close runtime RUN patterns: %w", err)
	}

	for _, test := range selected {
		fmt.Println(test.line)
	}
	return nil
}

type runSegment struct {
	raw     string
	pattern *regexp.Regexp
}

type runAlternative []runSegment

type runFilter []runAlternative

func parseRunPattern(runPattern string) (runFilter, error) {
	rawAlternatives := splitRegexp(runPattern)
	filter := make(runFilter, 0, len(rawAlternatives))
	for alternativeIndex, rawSegments := range rawAlternatives {
		alternative := make(runAlternative, 0, len(rawSegments))
		for segmentIndex, rawSegment := range rawSegments {
			pattern, err := regexp.Compile(rewrite(rawSegment))
			if err != nil {
				return nil, fmt.Errorf(
					"invalid RUN pattern %q at alternative %d segment %d: %w",
					runPattern,
					alternativeIndex,
					segmentIndex,
					err,
				)
			}
			alternative = append(alternative, runSegment{raw: rawSegment, pattern: pattern})
		}
		filter = append(filter, alternative)
	}
	return filter, nil
}

// splitRegexp mirrors testing.splitRegexp. Slashes and pipes split only at the
// top level; escaped bytes, character classes, and parenthesized groups remain
// part of their current regular-expression segment.
func splitRegexp(runPattern string) [][]string {
	segments := make([]string, 0, strings.Count(runPattern, "/"))
	alternatives := make([][]string, 0, strings.Count(runPattern, "|"))
	classDepth := 0
	parenDepth := 0
	for index := 0; index < len(runPattern); {
		switch runPattern[index] {
		case '[':
			classDepth++
		case ']':
			classDepth--
			if classDepth < 0 {
				classDepth = 0
			}
		case '(':
			if classDepth == 0 {
				parenDepth++
			}
		case ')':
			if classDepth == 0 {
				parenDepth--
			}
		case '\\':
			index++
		case '/':
			if classDepth == 0 && parenDepth == 0 {
				segments = append(segments, runPattern[:index])
				runPattern = runPattern[index+1:]
				index = 0
				continue
			}
		case '|':
			if classDepth == 0 && parenDepth == 0 {
				segments = append(segments, runPattern[:index])
				runPattern = runPattern[index+1:]
				index = 0
				alternatives = append(alternatives, segments)
				segments = make([]string, 0, len(segments))
				continue
			}
		}
		index++
	}

	segments = append(segments, runPattern)
	return append(alternatives, segments)
}

func (filter runFilter) matches(name []string) (bool, bool) {
	for _, alternative := range filter {
		matched, partial := alternative.matches(name)
		if matched {
			return matched, partial
		}
	}
	return false, false
}

func (filter runFilter) matchesAnyCompleteAlternative(name []string) bool {
	for _, alternative := range filter {
		matched, partial := alternative.matches(name)
		if matched && !partial {
			return true
		}
	}
	return false
}

func (alternative runAlternative) matches(name []string) (bool, bool) {
	for index, namePart := range name {
		if index >= len(alternative) {
			break
		}
		if !alternative[index].pattern.MatchString(namePart) {
			return false, false
		}
	}
	return true, len(name) < len(alternative)
}

func (filter runFilter) runtimePattern(tests []taggedTest) (string, error) {
	runtimeAlternatives := make([]string, 0, len(filter))
	for _, alternative := range filter {
		matchingNames := make([]string, 0, len(tests))
		seenNames := make(map[string]bool)
		for _, test := range tests {
			matched, _ := alternative.matches([]string{test.name})
			if matched && !seenNames[test.name] {
				matchingNames = append(matchingNames, regexp.QuoteMeta(test.name))
				seenNames[test.name] = true
			}
		}
		if len(matchingNames) == 0 {
			continue
		}

		segments := make([]string, 0, len(alternative))
		segments = append(segments, "^("+strings.Join(matchingNames, "|")+")$")
		for _, segment := range alternative[1:] {
			segments = append(segments, segment.raw)
		}
		runtimeAlternatives = append(runtimeAlternatives, strings.Join(segments, "/"))
	}
	if len(runtimeAlternatives) == 0 {
		return "", fmt.Errorf("selected package has no runtime RUN alternatives")
	}
	return strings.Join(runtimeAlternatives, "|"), nil
}

// rewrite mirrors the normalization testing applies to each split -run
// segment before compiling its regular expression.
func rewrite(value string) string {
	var rewritten []byte
	for _, character := range value {
		switch {
		case isTestingSpace(character):
			rewritten = append(rewritten, '_')
		case !strconv.IsPrint(character):
			quoted := strconv.QuoteRune(character)
			rewritten = append(rewritten, quoted[1:len(quoted)-1]...)
		default:
			rewritten = append(rewritten, string(character)...)
		}
	}
	return string(rewritten)
}

func isTestingSpace(character rune) bool {
	if character < 0x2000 {
		switch character {
		case '\t', '\n', '\v', '\f', '\r', ' ', 0x85, 0xA0, 0x1680:
			return true
		}
		return false
	}
	if character <= 0x200A {
		return true
	}
	switch character {
	case 0x2028, 0x2029, 0x202F, 0x205F, 0x3000:
		return true
	}
	return false
}

func newScanner(file *os.File) *bufio.Scanner {
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), scannerCapacity)
	return scanner
}
