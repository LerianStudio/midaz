// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build testhooks

package testhooks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	envPoint       = "MIDAZ_TESTHOOK_POINT"
	envRunID       = "MIDAZ_TESTHOOK_RUN_ID"
	envRunToken    = "MIDAZ_TESTHOOK_RUN_TOKEN"
	envSignalDir   = "MIDAZ_TESTHOOK_SIGNAL_DIR"
	envDeployMode  = "DEPLOYMENT_MODE"
	hookWaitLimit  = 30 * time.Second
	hookPollPeriod = 50 * time.Millisecond
)

// Point identifies a deterministic pause in the money path. The values are
// intentionally stable because the external E2E harness uses them as a
// process-boundary contract.
type Point string

const (
	PointAfterEconomicMutation   Point = "after_economic_mutation"
	PointAfterRevertClaimMutated Point = "after_revert_claim_mutated"
)

// IDs is the request and transaction identity written to a local marker when
// the testhook build is active. Empty origin/reverse IDs mean this is a normal
// transaction rather than a revert.
type IDs struct {
	RequestID     string
	TransactionID string
	OriginID      string
	ReverseID     string
}

type hookConfig struct {
	point     Point
	runID     string
	runToken  string
	signalDir string
}

type marker struct {
	Point         Point  `json:"point"`
	RunID         string `json:"run_id"`
	RequestID     string `json:"request_id"`
	TransactionID string `json:"transaction_id"`
	OriginID      string `json:"origin_id"`
	ReverseID     string `json:"reverse_id"`
}

// Pause activates only when MIDAZ_TESTHOOK_POINT is configured for this
// point. The testhook build is deliberately local-only: a configured hook in
// any other deployment mode fails closed, as do missing identities, stale
// marker files, wrong release tokens, context cancellation, and timeout.
//
// The protocol is filesystem-only and has no HTTP/admin surface:
//
//	marker: <signal-dir>/<run-id>.<point>.marker.json
//	release: <signal-dir>/<run-id>.<point>.release
//
// The release file must contain the exact MIDAZ_TESTHOOK_RUN_TOKEN bytes. A
// marker is atomically published before the release file is accepted.
func Pause(ctx context.Context, point Point, ids IDs) error {
	if ctx == nil {
		return errors.New("testhook context is nil")
	}

	cfg, active, err := loadConfig()
	if err != nil {
		return err
	}
	if !active || cfg.point != point {
		return nil
	}

	markerPath := filepath.Join(cfg.signalDir, cfg.runID+"."+string(point)+".marker.json")
	releasePath := filepath.Join(cfg.signalDir, cfg.runID+"."+string(point)+".release")

	if _, err := os.Lstat(releasePath); err == nil {
		return fmt.Errorf("testhook release signal already exists: %s", releasePath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect testhook release signal: %w", err)
	}

	payload, err := json.Marshal(marker{
		Point:         point,
		RunID:         cfg.runID,
		RequestID:     ids.RequestID,
		TransactionID: ids.TransactionID,
		OriginID:      ids.OriginID,
		ReverseID:     ids.ReverseID,
	})
	if err != nil {
		return fmt.Errorf("encode testhook marker: %w", err)
	}

	if err := writeMarker(markerPath, payload); err != nil {
		return fmt.Errorf("publish testhook marker: %w", err)
	}

	if err := waitForRelease(ctx, releasePath, cfg.runToken); err != nil {
		return fmt.Errorf("wait for testhook release at %s: %w", point, err)
	}

	return nil
}

func loadConfig() (hookConfig, bool, error) {
	pointValue := os.Getenv(envPoint)
	if pointValue == "" {
		return hookConfig{}, false, nil
	}

	point := Point(pointValue)
	if point != PointAfterEconomicMutation && point != PointAfterRevertClaimMutated {
		return hookConfig{}, true, fmt.Errorf("unsupported %s value %q", envPoint, pointValue)
	}

	deploymentMode := strings.ToLower(strings.TrimSpace(os.Getenv(envDeployMode)))
	if deploymentMode == "" {
		deploymentMode = "local"
	}
	if deploymentMode != "local" {
		return hookConfig{}, true, fmt.Errorf("testhooks require %s=local, got %q", envDeployMode, deploymentMode)
	}

	runID := os.Getenv(envRunID)
	runToken := os.Getenv(envRunToken)
	signalDir := os.Getenv(envSignalDir)
	if runID == "" || runToken == "" || signalDir == "" {
		return hookConfig{}, true, fmt.Errorf("%s, %s, and %s are required when %s is set", envRunID, envRunToken, envSignalDir, envPoint)
	}
	if !safeRunID(runID) {
		return hookConfig{}, true, fmt.Errorf("%s must contain only letters, digits, dot, dash, or underscore", envRunID)
	}
	if !filepath.IsAbs(signalDir) {
		return hookConfig{}, true, fmt.Errorf("%s must be an absolute local directory", envSignalDir)
	}

	info, err := os.Stat(signalDir)
	if err != nil {
		return hookConfig{}, true, fmt.Errorf("stat %s: %w", envSignalDir, err)
	}
	if !info.IsDir() {
		return hookConfig{}, true, fmt.Errorf("%s is not a directory: %s", envSignalDir, signalDir)
	}

	return hookConfig{point: point, runID: runID, runToken: runToken, signalDir: signalDir}, true, nil
}

func safeRunID(value string) bool {
	if len(value) == 0 || len(value) > 128 {
		return false
	}

	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '-' || r == '_' {
			continue
		}

		return false
	}

	return true
}

func writeMarker(path string, payload []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".testhook-marker-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(payload); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	// A hard-link publish creates the final name atomically without replacing a
	// marker from a prior run. Both files are in the caller-owned signal dir.
	if err := os.Link(tmpName, path); err != nil {
		return err
	}

	return nil
}

func waitForRelease(ctx context.Context, path, expectedToken string) error {
	timer := time.NewTimer(hookWaitLimit)
	defer timer.Stop()

	ticker := time.NewTicker(hookPollPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			return fmt.Errorf("release signal timed out after %s", hookWaitLimit)
		case <-ticker.C:
			signal, err := os.ReadFile(path)
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				return err
			}
			if string(signal) != expectedToken {
				return errors.New("release signal token mismatch")
			}

			return nil
		}
	}
}
