// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// check-integration-profile compares rendered deployment resource settings.
// It never connects to services, loads credentials or changes environment state.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/seamidentity"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

func main() {
	ledger := flag.String("ledger-env", "", "rendered Ledger environment file")
	tracer := flag.String("tracer-env", "", "rendered Tracer environment file")

	flag.Parse()

	if err := checkProfiles(*ledger, *tracer); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Println("Ledger and Tracer activation profiles match; this does not certify remote readiness or SLOs.")
}

func checkProfiles(ledgerPath, tracerPath string) error {
	if ledgerPath == "" || tracerPath == "" {
		return fmt.Errorf("both --ledger-env and --tracer-env are required")
	}

	ledgerEnv, err := readEnvironment(ledgerPath)
	if err != nil {
		return err
	}

	tracerEnv, err := readEnvironment(tracerPath)
	if err != nil {
		return err
	}

	ledger, err := resourceProfile(ledgerEnv, true)
	if err != nil {
		return err
	}

	tracer, err := resourceProfile(tracerEnv, false)
	if err != nil {
		return err
	}

	if ledger != tracer {
		return fmt.Errorf("ledger and tracer resource profiles differ; align accounts, entries, text, integer/fraction digits, body bytes and reservation counts before activation")
	}

	return checkActivationProfile(ledgerEnv, tracerEnv)
}

func checkActivationProfile(ledgerEnv, tracerEnv map[string]string) error {
	if err := requireEnabled(ledgerEnv, "TRACER_CONTEXT_ENABLED"); err != nil {
		return err
	}

	if err := requireEnabled(tracerEnv, "CONTEXT_RESERVE_ENABLED"); err != nil {
		return err
	}

	bindings, err := producerBindings(tracerEnv["CONTEXT_PRODUCER_BINDINGS"])
	if err != nil {
		return err
	}

	if !hasReserveBinding(bindings) {
		return fmt.Errorf("CONTEXT_PRODUCER_BINDINGS has no identity with the reserve purpose")
	}

	return requireReaperEnabled(tracerEnv)
}

// requireReaperEnabled rejects a Tracer that admits decisions without the
// reaper: an operation whose completion never arrives only returns its
// capacity when the reaper expires it. Unset keeps the Tracer default (on).
func requireReaperEnabled(tracerEnv map[string]string) error {
	raw, present := tracerEnv["RESERVATION_REAPER_ENABLED"]
	if !present {
		return nil
	}

	enabled, err := strconv.ParseBool(strings.TrimSpace(raw))
	if err != nil || !enabled {
		return fmt.Errorf("RESERVATION_REAPER_ENABLED must stay true while CONTEXT_RESERVE_ENABLED is true; without it, capacity held by an operation that never completes is never returned")
	}

	return nil
}

func hasReserveBinding(bindings []seamidentity.Binding) bool {
	for _, binding := range bindings {
		for _, purpose := range binding.Purposes {
			if purpose == seamidentity.PurposeReserve {
				return true
			}
		}
	}

	return false
}

func requireEnabled(values map[string]string, key string) error {
	raw, present := values[key]
	if !present || strings.TrimSpace(raw) == "" {
		return fmt.Errorf("%s must be explicitly true before activation", key)
	}

	enabled, err := strconv.ParseBool(raw)
	if err != nil || !enabled {
		return fmt.Errorf("%s must be explicitly true before activation", key)
	}

	return nil
}

func producerBindings(raw string) ([]seamidentity.Binding, error) {
	if len(raw) == 0 || len(raw) > 65536 {
		return nil, fmt.Errorf("CONTEXT_PRODUCER_BINDINGS must contain 1 to 65536 bytes of JSON")
	}

	var bindings []seamidentity.Binding

	decoder := json.NewDecoder(bytes.NewBufferString(raw))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&bindings); err != nil {
		return nil, fmt.Errorf("invalid CONTEXT_PRODUCER_BINDINGS (contents suppressed)")
	}

	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, fmt.Errorf("CONTEXT_PRODUCER_BINDINGS must contain one JSON array")
	}

	if _, err := seamidentity.NewResolver(bindings); err != nil {
		return nil, fmt.Errorf("invalid CONTEXT_PRODUCER_BINDINGS")
	}

	return bindings, nil
}

func readEnvironment(path string) (map[string]string, error) {
	// Local operator CLI: the explicit input path is not supplied by a network
	// caller. Reading arbitrary deployment files is intentional; contents stay private.
	file, err := os.Open(path) // #nosec G304 -- operator-selected, read-only input; bounded below.
	if err != nil {
		return nil, fmt.Errorf("open deployment environment: %w", err)
	}
	defer func() { _ = file.Close() }()

	const maxBytes = 1 << 20

	raw, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil || len(raw) > maxBytes {
		return nil, fmt.Errorf("deployment environment cannot be read within the 1 MiB limit")
	}

	values, err := godotenv.Unmarshal(string(raw))
	if err != nil {
		return nil, fmt.Errorf("invalid deployment environment syntax (contents suppressed)")
	}

	return values, nil
}

func resourceProfile(values map[string]string, ledger bool) (tracercontract.ResourceProfile, error) {
	profile := tracercontract.DefaultResourceProfile()

	prefix, reserve := "CONTEXT_", "CONTEXT_RESERVE_"
	if ledger {
		prefix, reserve = "TRACER_CONTEXT_", "TRACER_CONTEXT_"
	}

	fields := []struct {
		key    string
		target *int
	}{
		{prefix + "MAX_ACCOUNTS", &profile.Facts.MaxAccounts},
		{prefix + "MAX_ENTRIES", &profile.Facts.MaxEntries},
		{prefix + "MAX_TEXT_BYTES", &profile.Facts.MaxTextBytes},
		{prefix + "MAX_INTEGER_DIGITS", &profile.Facts.MaxIntegerDigits},
		{prefix + "MAX_FRACTION_DIGITS", &profile.Facts.MaxFractionDigits},
		{reserve + "MAX_BODY_BYTES", &profile.MaxBodyBytes},
		{reserve + "MAX_RESERVATIONS", &profile.MaxReservations},
	}
	for _, field := range fields {
		if raw, present := values[field.key]; present {
			value, err := strconv.Atoi(raw)
			if err != nil {
				return profile, fmt.Errorf("invalid numeric resource setting %s", field.key)
			}

			*field.target = value
		}
	}

	return profile, profile.Validate()
}
