// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package helpers

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// RunChaosTests authenticates, verifies readiness, and runs chaos tests.
// Chaos tests are opt-in and run only when CHAOS=1.
func RunChaosTests(m *testing.M) {
	os.Exit(runChaosTests(m))
}

func runChaosTests(m *testing.M) int {
	if os.Getenv("CHAOS") != "1" {
		log.Printf("chaos tests skipped (set CHAOS=1 to enable)")
		return 0
	}

	if err := AuthenticateFromEnv(); err != nil {
		log.Printf("chaos auth failed: %v", err)
		return 1
	}

	env := LoadEnvironment()

	if env.ManageStack {
		root, err := findRepoRoot()
		if err != nil {
			log.Printf("chaos setup failed: %v", err)
			return 1
		}

		if err := runMake(root, "up-backend"); err != nil {
			log.Printf("chaos setup failed to start backend stack: %v", err)
			return 1
		}

		defer func() {
			if err := runMake(root, "down-backend"); err != nil {
				log.Printf("chaos teardown failed to stop backend stack: %v", err)
			}
		}()
	}

	preflightTimeout := env.HTTPTimeout
	if preflightTimeout <= 0 {
		preflightTimeout = 20 * time.Second
	}

	if err := WaitForHTTP200(env.OnboardingURL+"/health", preflightTimeout); err != nil {
		log.Printf("chaos preflight failed: onboarding not healthy at %s: %v", env.OnboardingURL, err)
		return 1
	}

	if err := WaitForHTTP200(env.TransactionURL+"/health", preflightTimeout); err != nil {
		log.Printf("chaos preflight failed: transaction not healthy at %s: %v", env.TransactionURL, err)
		return 1
	}

	return m.Run()
}

func findRepoRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	current := wd
	for {
		if _, err := os.Stat(filepath.Join(current, "go.mod")); err == nil {
			return current, nil
		}

		next := filepath.Dir(current)
		if next == current {
			return "", fmt.Errorf("could not find repository root from %s", wd)
		}

		current = next
	}
}

func runMake(root, target string) error {
	cmd := exec.Command("make", "-C", root, target)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("make %s failed: %w", target, err)
	}

	return nil
}
