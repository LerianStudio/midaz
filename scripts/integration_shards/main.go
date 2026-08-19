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

	modeParallel = "parallel"
	modeSerial   = "serial"

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
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || *pkg == "" || *expectedPath == "" || *eventsPath == "" {
		return errors.New("verify-events requires --package, --expected, and --events")
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

	eventsFile, err := os.Open(*eventsPath)
	if err != nil {
		return fmt.Errorf("open Go test events: %w", err)
	}
	err = verifyEventCoverage(*pkg, expected, eventsFile)
	closeErr = eventsFile.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return fmt.Errorf("close Go test events: %w", closeErr)
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

func verifyEventCoverage(pkg string, expected []string, reader io.Reader) error {
	if pkg == "" {
		return errors.New("event verification package is required")
	}
	if len(expected) == 0 {
		return errors.New("event verification expected zero tests")
	}

	type goTestEvent struct {
		Action  string `json:"Action"`
		Package string `json:"Package"`
		Test    string `json:"Test"`
	}

	expectedSet := make(map[string]struct{}, len(expected))
	for _, name := range expected {
		expectedSet[name] = struct{}{}
	}
	started := make(map[string]int, len(expected))
	decoder := json.NewDecoder(reader)
	eventCount := 0
	for {
		var event goTestEvent
		err := decoder.Decode(&event)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("decode Go test event %d: %w", eventCount+1, err)
		}
		eventCount++
		if event.Action != "run" || event.Package != pkg || event.Test == "" || strings.Contains(event.Test, "/") {
			continue
		}
		if _, exists := expectedSet[event.Test]; !exists {
			return fmt.Errorf("package %s started unselected test %s", pkg, event.Test)
		}
		started[event.Test]++
	}
	if eventCount == 0 {
		return errors.New("Go test event stream contained zero events")
	}

	for _, name := range expected {
		switch started[name] {
		case 0:
			return fmt.Errorf("package %s did not start selected test %s", pkg, name)
		case 1:
			continue
		default:
			return fmt.Errorf("package %s started selected test %s %d times", pkg, name, started[name])
		}
	}

	return nil
}
