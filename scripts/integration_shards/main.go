// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"
)

const (
	modulePrefix = "github.com/LerianStudio/midaz/v4/"

	shardLedgerPostgres     = "ledger-postgres"
	shardLedgerMongoCRM     = "ledger-mongodb-crm"
	shardAsyncBroker        = "async-broker"
	shardTracer             = "tracer"
	shardLifecycleMigration = "lifecycle-migration"
	shardChaosCapability    = "chaos-capability"

	modeParallel = "parallel"
	modeSerial   = "serial"

	skipAllowlistVersion = 1

	capabilityChaosIntegration = "integration-chaos:CHAOS=1"
	capabilityStreamingBroker  = "required-streaming:STREAMING_BROKERS"

	tracerJourneyPackage = modulePrefix + "components/tracer/tests/integration"
)

var shardOrder = []string{
	shardLedgerPostgres,
	shardLedgerMongoCRM,
	shardAsyncBroker,
	shardTracer,
	shardLifecycleMigration,
}

type testRecord struct {
	Package string
	Test    string
}

type assignment struct {
	testRecord
	Shard string
	Mode  string
}

type skipAllowance struct {
	Package             string `json:"package"`
	Test                string `json:"test"`
	Reason              string `json:"reason"`
	AlternateCapability string `json:"alternate_capability"`
}

type skipAllowlistDocument struct {
	Version int             `json:"version"`
	Entries []skipAllowance `json:"entries"`
}

type testOutcome struct {
	Package             string
	Test                string
	Outcome             string
	Reason              string
	AlternateCapability string
}

type eventCoverage struct {
	Passed   int
	Skipped  int
	Failed   int
	Missing  int
	Outcomes []testOutcome
}

var skipLocationPattern = regexp.MustCompile(`^[^:[:space:]]+\.go:[0-9]+:\s*`)

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "integration shard contract: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdin io.Reader, stdout io.Writer) error {
	if len(args) > 0 && args[0] == "verify-events" {
		return runVerifyEvents(args[1:])
	}

	flags := flag.NewFlagSet("integration-shards", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	shard := flags.String("shard", "", "emit only one shard")
	mode := flags.String("mode", "", "emit only parallel or serial work")
	capability := flags.String("capability", "", "emit the exact supplemental capability plan")
	skipAllowlistPath := flags.String("skip-allowlist", "", "versioned integration skip allowlist")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %s", strings.Join(flags.Args(), " "))
	}
	if *shard != "" && !knownShard(*shard) {
		return fmt.Errorf("unknown shard %q", *shard)
	}
	if *mode != "" && *mode != modeParallel && *mode != modeSerial {
		return fmt.Errorf("unknown execution mode %q", *mode)
	}
	if *skipAllowlistPath == "" {
		return errors.New("--skip-allowlist is required")
	}

	inventory, err := readInventory(stdin)
	if err != nil {
		return err
	}

	assignments := make([]assignment, 0, len(inventory))
	for _, record := range inventory {
		item, classifyErr := classify(record)
		if classifyErr != nil {
			return classifyErr
		}
		assignments = append(assignments, item)
	}
	if err := verifyAssignments(inventory, assignments); err != nil {
		return err
	}
	allowances, err := readSkipAllowlistFile(*skipAllowlistPath)
	if err != nil {
		return err
	}
	if err := verifySkipAllowlist(inventory, assignments, allowances); err != nil {
		return err
	}
	if *capability != "" {
		assignments, err = buildCapabilityAssignments(*capability, allowances)
		if err != nil {
			return err
		}
	}

	sort.Slice(assignments, func(i, j int) bool {
		left, right := assignments[i], assignments[j]
		if shardRank(left.Shard) != shardRank(right.Shard) {
			return shardRank(left.Shard) < shardRank(right.Shard)
		}
		if left.Mode != right.Mode {
			return left.Mode < right.Mode
		}
		if left.Package != right.Package {
			return left.Package < right.Package
		}
		return left.Test < right.Test
	})

	writer := bufio.NewWriter(stdout)
	defer writer.Flush()
	for _, item := range assignments {
		if *shard != "" && item.Shard != *shard {
			continue
		}
		if *mode != "" && item.Mode != *mode {
			continue
		}
		if _, err := fmt.Fprintf(writer, "%s\t%s\t%s\t%s\n", item.Shard, item.Mode, item.Package, item.Test); err != nil {
			return fmt.Errorf("write shard plan: %w", err)
		}
	}

	return nil
}

func runVerifyEvents(args []string) error {
	flags := flag.NewFlagSet("verify-events", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	pkg := flags.String("package", "", "expected package import path")
	expectedPath := flags.String("expected", "", "file containing one selected test per line")
	eventsPath := flags.String("events", "", "go test JSON event stream")
	skipAllowlistPath := flags.String("skip-allowlist", "", "versioned integration skip allowlist")
	outcomesPath := flags.String("outcomes", "", "classified test outcome artifact")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || *pkg == "" || *expectedPath == "" || *eventsPath == "" ||
		*skipAllowlistPath == "" || *outcomesPath == "" {
		return errors.New("verify-events requires --package, --expected, --events, --skip-allowlist, and --outcomes")
	}

	expectedFile, err := os.Open(*expectedPath)
	if err != nil {
		return fmt.Errorf("open expected test list: %w", err)
	}
	expected, err := readExpectedTests(expectedFile)
	closeErr := expectedFile.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return fmt.Errorf("close expected test list: %w", closeErr)
	}
	allowances, err := readSkipAllowlistFile(*skipAllowlistPath)
	if err != nil {
		return err
	}

	eventsFile, err := os.Open(*eventsPath)
	if err != nil {
		return fmt.Errorf("open Go test events: %w", err)
	}
	coverage, verifyErr := verifyEventCoverage(*pkg, expected, allowances, eventsFile)
	closeErr = eventsFile.Close()
	if closeErr != nil {
		return fmt.Errorf("close Go test events: %w", closeErr)
	}
	if err := writeOutcomes(*outcomesPath, coverage.Outcomes); err != nil {
		return err
	}
	if verifyErr != nil {
		return verifyErr
	}

	return nil
}

func readInventory(reader io.Reader) ([]testRecord, error) {
	scanner := bufio.NewScanner(reader)
	// The Tracer journey currently has hundreds of tests. Keep the parser well
	// above the default token limit so future fully-qualified names cannot turn
	// selection into a partial pass.
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	records := make([]testRecord, 0)
	seen := make(map[testRecord]struct{})
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 {
			return nil, fmt.Errorf("inventory line %d: want package and test, got %q", lineNumber, line)
		}
		record := testRecord{Package: fields[0], Test: fields[1]}
		if !strings.HasPrefix(record.Test, "Test") {
			return nil, fmt.Errorf("inventory line %d: %q is not a top-level Go test", lineNumber, record.Test)
		}
		if _, exists := seen[record]; exists {
			return nil, fmt.Errorf("duplicate integration test %s %s", record.Package, record.Test)
		}
		seen[record] = struct{}{}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read integration inventory: %w", err)
	}
	if len(records) == 0 {
		return nil, errors.New("zero integration tests discovered")
	}

	return records, nil
}

func classify(record testRecord) (assignment, error) {
	if record.Package == "" || record.Test == "" {
		return assignment{}, errors.New("integration test package and name are required")
	}

	if record.Package == tracerJourneyPackage {
		return assignment{testRecord: record, Shard: shardTracer, Mode: modeSerial}, nil
	}
	if isSerialExclusion(record) {
		return assignment{testRecord: record, Shard: shardLifecycleMigration, Mode: modeSerial}, nil
	}

	path := strings.TrimPrefix(record.Package, modulePrefix)
	var shard string
	switch {
	case strings.HasPrefix(path, "components/ledger/internal/adapters/postgres/"),
		path == "tests/utils/postgres":
		shard = shardLedgerPostgres
	case strings.HasPrefix(path, "components/ledger/internal/adapters/mongodb/"),
		strings.HasPrefix(path, "components/ledger/internal/crm/"),
		path == "tests/utils/mongodb":
		shard = shardLedgerMongoCRM
	case path == "components/ledger/internal/adapters/http/in",
		path == "components/ledger/internal/bootstrap",
		strings.HasPrefix(path, "components/ledger/internal/adapters/rabbitmq"),
		strings.HasPrefix(path, "components/ledger/internal/adapters/redis/"),
		path == "components/ledger/internal/adapters/tracer",
		strings.HasPrefix(path, "components/ledger/internal/services/"),
		path == "tests/utils/rabbitmq",
		path == "tests/utils/redis":
		shard = shardAsyncBroker
	case strings.HasPrefix(path, "components/tracer/"):
		shard = shardTracer
	default:
		return assignment{}, fmt.Errorf("unclassified integration test %s %s", record.Package, record.Test)
	}

	return assignment{testRecord: record, Shard: shard, Mode: modeParallel}, nil
}

func isSerialExclusion(record testRecord) bool {
	path := strings.TrimPrefix(record.Package, modulePrefix)
	if path == "tests/integration" || path == "tests/utils/chaos" ||
		path == "components/tracer/pkg/migration" {
		return true
	}

	name := strings.ToLower(record.Test)
	serialFragments := []string{
		"chaos",
		"lifecycle",
		"migration",
		"migrator",
		"upgradepath",
		"startsallservers",
		"initservers",
		"streamingsmoke",
	}
	for _, fragment := range serialFragments {
		if strings.Contains(name, fragment) {
			return true
		}
	}

	return false
}

func verifyAssignments(inventory []testRecord, assignments []assignment) error {
	if len(inventory) == 0 {
		return errors.New("cannot verify an empty integration inventory")
	}

	expected := make(map[testRecord]struct{}, len(inventory))
	for _, record := range inventory {
		expected[record] = struct{}{}
	}
	counts := make(map[testRecord]int, len(assignments))
	for _, item := range assignments {
		if !knownShard(item.Shard) {
			return fmt.Errorf("unknown shard %q for %s %s", item.Shard, item.Package, item.Test)
		}
		if item.Mode != modeParallel && item.Mode != modeSerial {
			return fmt.Errorf("unknown execution mode %q for %s %s", item.Mode, item.Package, item.Test)
		}
		if _, exists := expected[item.testRecord]; !exists {
			return fmt.Errorf("assignment contains unknown integration test %s %s", item.Package, item.Test)
		}
		counts[item.testRecord]++
	}

	for _, record := range inventory {
		switch counts[record] {
		case 0:
			return fmt.Errorf("integration test omitted from shards: %s %s", record.Package, record.Test)
		case 1:
			continue
		default:
			return fmt.Errorf("integration test %s %s assigned %d times", record.Package, record.Test, counts[record])
		}
	}

	return nil
}

func knownShard(shard string) bool {
	for _, candidate := range shardOrder {
		if shard == candidate {
			return true
		}
	}
	return false
}

func shardRank(shard string) int {
	for index, candidate := range shardOrder {
		if shard == candidate {
			return index
		}
	}
	return len(shardOrder)
}

func buildCapabilityAssignments(capability string, allowances map[testRecord]skipAllowance) ([]assignment, error) {
	if capability != capabilityChaosIntegration {
		return nil, fmt.Errorf("unknown alternate capability %q", capability)
	}

	assignments := make([]assignment, 0, len(allowances))
	for record, allowance := range allowances {
		if allowance.AlternateCapability != capability {
			continue
		}
		assignments = append(assignments, assignment{
			testRecord: record,
			Shard:      shardChaosCapability,
			Mode:       modeParallel,
		})
	}
	if len(assignments) == 0 {
		return nil, fmt.Errorf("alternate capability %q selected zero tests", capability)
	}
	sort.Slice(assignments, func(i, j int) bool {
		if assignments[i].Package != assignments[j].Package {
			return assignments[i].Package < assignments[j].Package
		}
		return assignments[i].Test < assignments[j].Test
	})

	return assignments, nil
}

func readExpectedTests(reader io.Reader) ([]string, error) {
	scanner := bufio.NewScanner(reader)
	tests := make([]string, 0)
	seen := make(map[string]struct{})
	for scanner.Scan() {
		name := strings.TrimSpace(scanner.Text())
		if name == "" {
			continue
		}
		if !strings.HasPrefix(name, "Test") {
			return nil, fmt.Errorf("expected test list contains invalid name %q", name)
		}
		if _, exists := seen[name]; exists {
			return nil, fmt.Errorf("expected test list contains duplicate %s", name)
		}
		seen[name] = struct{}{}
		tests = append(tests, name)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read expected test list: %w", err)
	}
	if len(tests) == 0 {
		return nil, errors.New("expected test list is empty")
	}

	return tests, nil
}

func readSkipAllowlistFile(path string) (map[testRecord]skipAllowance, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open integration skip allowlist: %w", err)
	}
	allowances, readErr := readSkipAllowlist(file)
	closeErr := file.Close()
	if readErr != nil {
		return nil, readErr
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close integration skip allowlist: %w", closeErr)
	}
	return allowances, nil
}

func readSkipAllowlist(reader io.Reader) (map[testRecord]skipAllowance, error) {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	var document skipAllowlistDocument
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("decode integration skip allowlist: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("integration skip allowlist contains multiple JSON values")
		}
		return nil, fmt.Errorf("decode integration skip allowlist trailer: %w", err)
	}
	if document.Version != skipAllowlistVersion {
		return nil, fmt.Errorf("integration skip allowlist version %d is unsupported; want %d", document.Version, skipAllowlistVersion)
	}

	allowances := make(map[testRecord]skipAllowance, len(document.Entries))
	for index, allowance := range document.Entries {
		allowance.Package = strings.TrimSpace(allowance.Package)
		allowance.Test = strings.TrimSpace(allowance.Test)
		allowance.Reason = strings.TrimSpace(allowance.Reason)
		allowance.AlternateCapability = strings.TrimSpace(allowance.AlternateCapability)
		if allowance.Package == "" || allowance.Test == "" || allowance.Reason == "" || allowance.AlternateCapability == "" {
			return nil, fmt.Errorf("integration skip allowance %d requires package, test, reason, and alternate_capability", index+1)
		}
		if !strings.HasPrefix(allowance.Test, "Test") {
			return nil, fmt.Errorf("integration skip allowance %d has invalid top-level test %q", index+1, allowance.Test)
		}
		if strings.ContainsAny(allowance.Reason, "\r\n\t") {
			return nil, fmt.Errorf("integration skip allowance %s %s reason must be one line", allowance.Package, allowance.Test)
		}
		if err := validateAlternateCapabilityDefinition(allowance); err != nil {
			return nil, err
		}
		record := testRecord{Package: allowance.Package, Test: allowance.Test}
		if _, exists := allowances[record]; exists {
			return nil, fmt.Errorf("duplicate skip allowance for %s %s", record.Package, record.Test)
		}
		allowances[record] = allowance
	}

	return allowances, nil
}

func validateAlternateCapabilityDefinition(allowance skipAllowance) error {
	switch allowance.AlternateCapability {
	case capabilityChaosIntegration:
		if !strings.Contains(allowance.Reason, "CHAOS=1") {
			return fmt.Errorf("skip allowance %s %s chaos reason must name CHAOS=1", allowance.Package, allowance.Test)
		}
	case capabilityStreamingBroker:
		if !strings.Contains(allowance.Reason, "STREAMING_BROKERS") {
			return fmt.Errorf("skip allowance %s %s streaming reason must name STREAMING_BROKERS", allowance.Package, allowance.Test)
		}
	default:
		return fmt.Errorf("skip allowance %s %s has unknown alternate capability %q", allowance.Package, allowance.Test, allowance.AlternateCapability)
	}
	return nil
}

func verifySkipAllowlist(inventory []testRecord, assignments []assignment, allowances map[testRecord]skipAllowance) error {
	selected := make(map[testRecord]int, len(assignments))
	for _, item := range assignments {
		selected[item.testRecord]++
	}
	inventorySet := make(map[testRecord]struct{}, len(inventory))
	for _, record := range inventory {
		inventorySet[record] = struct{}{}
	}
	for record := range allowances {
		if _, exists := inventorySet[record]; !exists || selected[record] != 1 {
			return fmt.Errorf("integration skip allowance references unknown or stale selected test %s %s", record.Package, record.Test)
		}
	}
	return nil
}

func verifyEventCoverage(pkg string, expected []string, allowances map[testRecord]skipAllowance, reader io.Reader) (eventCoverage, error) {
	coverage := eventCoverage{Outcomes: make([]testOutcome, 0, len(expected))}
	if pkg == "" {
		return coverage, errors.New("event verification package is required")
	}
	if len(expected) == 0 {
		return coverage, errors.New("event verification expected zero tests")
	}

	type goTestEvent struct {
		Action  string `json:"Action"`
		Package string `json:"Package"`
		Test    string `json:"Test"`
		Output  string `json:"Output"`
	}

	expectedSet := make(map[string]struct{}, len(expected))
	for _, name := range expected {
		expectedSet[name] = struct{}{}
	}
	started := make(map[string]int, len(expected))
	terminals := make(map[string][]string, len(expected))
	outputs := make(map[string][]string, len(expected))
	decoder := json.NewDecoder(reader)
	eventCount := 0
	verificationErrors := make([]error, 0)
	for {
		var event goTestEvent
		err := decoder.Decode(&event)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			verificationErrors = append(verificationErrors, fmt.Errorf("decode Go test event %d: %w", eventCount+1, err))
			break
		}
		eventCount++
		if event.Package != pkg || event.Test == "" || strings.Contains(event.Test, "/") {
			continue
		}
		if event.Action != "run" && event.Action != "pass" && event.Action != "skip" &&
			event.Action != "fail" && event.Action != "output" {
			continue
		}
		if _, exists := expectedSet[event.Test]; !exists {
			if event.Action == "run" {
				verificationErrors = append(verificationErrors, fmt.Errorf("package %s started unselected test %s", pkg, event.Test))
			}
			continue
		}
		switch event.Action {
		case "run":
			started[event.Test]++
		case "pass", "skip", "fail":
			terminals[event.Test] = append(terminals[event.Test], event.Action)
		case "output":
			outputs[event.Test] = append(outputs[event.Test], event.Output)
		}
	}
	if eventCount == 0 {
		verificationErrors = append(verificationErrors, errors.New("Go test event stream contained zero events"))
	}

	for _, name := range expected {
		outcome := testOutcome{Package: pkg, Test: name}
		if started[name] == 0 {
			outcome.Outcome = "missing"
			coverage.Missing++
			coverage.Outcomes = append(coverage.Outcomes, outcome)
			verificationErrors = append(verificationErrors, fmt.Errorf("package %s did not start selected test %s", pkg, name))
			continue
		}
		if started[name] != 1 {
			outcome.Outcome = "failed"
			coverage.Failed++
			coverage.Outcomes = append(coverage.Outcomes, outcome)
			verificationErrors = append(verificationErrors, fmt.Errorf("package %s started selected test %s %d times", pkg, name, started[name]))
			continue
		}
		if len(terminals[name]) == 0 {
			outcome.Outcome = "missing"
			coverage.Missing++
			coverage.Outcomes = append(coverage.Outcomes, outcome)
			verificationErrors = append(verificationErrors, fmt.Errorf("package %s selected test %s produced no terminal event", pkg, name))
			continue
		}
		if len(terminals[name]) != 1 {
			outcome.Outcome = "failed"
			coverage.Failed++
			coverage.Outcomes = append(coverage.Outcomes, outcome)
			verificationErrors = append(verificationErrors, fmt.Errorf("package %s selected test %s produced %d terminal events", pkg, name, len(terminals[name])))
			continue
		}

		switch terminals[name][0] {
		case "pass":
			if allowance, exists := allowances[testRecord{Package: pkg, Test: name}]; exists && alternateCapabilityInactive(allowance.AlternateCapability) {
				outcome.Outcome = "failed"
				outcome.AlternateCapability = allowance.AlternateCapability
				coverage.Failed++
				verificationErrors = append(verificationErrors, fmt.Errorf("package %s selected test %s passed but still has a stale skip allowance", pkg, name))
			} else {
				outcome.Outcome = "passed"
				coverage.Passed++
			}
		case "fail":
			outcome.Outcome = "failed"
			coverage.Failed++
			verificationErrors = append(verificationErrors, fmt.Errorf("package %s selected test %s terminated with fail", pkg, name))
		case "skip":
			reason, reasonErr := extractSkipReason(outputs[name])
			outcome.Reason = reason
			allowance, allowed := allowances[testRecord{Package: pkg, Test: name}]
			switch {
			case reasonErr != nil:
				outcome.Outcome = "failed"
				coverage.Failed++
				verificationErrors = append(verificationErrors, fmt.Errorf("package %s selected test %s skip reason: %w", pkg, name, reasonErr))
			case !allowed:
				outcome.Outcome = "failed"
				coverage.Failed++
				verificationErrors = append(verificationErrors, fmt.Errorf("package %s selected test %s produced unallowlisted skip with reason %q", pkg, name, reason))
			case allowance.Reason != reason:
				outcome.Outcome = "failed"
				coverage.Failed++
				outcome.AlternateCapability = allowance.AlternateCapability
				verificationErrors = append(verificationErrors, fmt.Errorf("package %s selected test %s skip reason changed: got %q, want %q", pkg, name, reason, allowance.Reason))
			case !alternateCapabilityInactive(allowance.AlternateCapability):
				outcome.Outcome = "failed"
				coverage.Failed++
				outcome.AlternateCapability = allowance.AlternateCapability
				verificationErrors = append(verificationErrors, fmt.Errorf("package %s selected test %s skipped while alternate capability %s was active", pkg, name, allowance.AlternateCapability))
			default:
				outcome.Outcome = "skipped"
				outcome.AlternateCapability = allowance.AlternateCapability
				coverage.Skipped++
			}
		}
		coverage.Outcomes = append(coverage.Outcomes, outcome)
	}

	return coverage, errors.Join(verificationErrors...)
}

func extractSkipReason(outputs []string) (string, error) {
	candidates := make([]string, 0, 1)
	for _, output := range outputs {
		for line := range strings.SplitSeq(output, "\n") {
			line = strings.TrimSpace(line)
			if !skipLocationPattern.MatchString(line) {
				continue
			}
			reason := strings.TrimSpace(skipLocationPattern.ReplaceAllString(line, ""))
			if reason != "" {
				candidates = append(candidates, reason)
			}
		}
	}
	if len(candidates) != 1 {
		return "", fmt.Errorf("expected exactly one stable reason line, got %d", len(candidates))
	}
	return candidates[0], nil
}

func alternateCapabilityInactive(capability string) bool {
	switch capability {
	case capabilityChaosIntegration:
		return os.Getenv("CHAOS") != "1"
	case capabilityStreamingBroker:
		return strings.TrimSpace(os.Getenv("STREAMING_BROKERS")) == ""
	default:
		return false
	}
}

func writeOutcomes(path string, outcomes []testOutcome) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create classified test outcome artifact: %w", err)
	}
	writer := bufio.NewWriter(file)
	if _, err := fmt.Fprintln(writer, "package\ttest\toutcome\treason\talternate_capability"); err != nil {
		_ = file.Close()
		return fmt.Errorf("write classified test outcome header: %w", err)
	}
	for _, outcome := range outcomes {
		reason := strings.NewReplacer("\t", " ", "\r", " ", "\n", " ").Replace(outcome.Reason)
		if _, err := fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\n", outcome.Package, outcome.Test, outcome.Outcome, reason, outcome.AlternateCapability); err != nil {
			_ = file.Close()
			return fmt.Errorf("write classified test outcome: %w", err)
		}
	}
	if err := writer.Flush(); err != nil {
		_ = file.Close()
		return fmt.Errorf("flush classified test outcome artifact: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close classified test outcome artifact: %w", err)
	}
	return nil
}
