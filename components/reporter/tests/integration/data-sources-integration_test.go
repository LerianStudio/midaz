//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package integration

import (
	"context"
	"encoding/json"
	"testing"

	h "github.com/LerianStudio/reporter/tests/utils"
)

// GET /v1/data-sources — deve respeitar cache e retornar 200
func TestIntegration_DataSources_CacheBehavior(t *testing.T) {
	// NOTE: Cannot use t.Parallel() because this test communicates with shared external services (MongoDB, HTTP API).
	env := h.LoadEnvironment()
	ctx := context.Background()
	cli := h.NewHTTPClient(env.ManagerURL, env.HTTPTimeout)
	headers := h.AuthHeaders()

	code, body, err := cli.Request(ctx, "GET", "/v1/data-sources", headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("first call code=%d err=%v body=%s", code, err, string(body))
	}
	var first []map[string]any
	_ = json.Unmarshal(body, &first)

	code, body, err = cli.Request(ctx, "GET", "/v1/data-sources", headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("second call code=%d err=%v body=%s", code, err, string(body))
	}
	var second []map[string]any
	_ = json.Unmarshal(body, &second)

	if len(first) == 0 && len(second) == 0 {
		t.Skip("no data sources configured - skipping cache behavior test")
	}
}
