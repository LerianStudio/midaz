//go:build fuzz

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package fuzzy

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"time"

	h "github.com/LerianStudio/reporter/tests/utils"
)

// FuzzCreateReportInput — Fuzz test for create report input
func FuzzCreateReportInput(f *testing.F) {
	f.Add("00000000-0000-0000-0000-000000000000")
	f.Add("")
	f.Add("not-a-uuid")
	f.Add("<script>alert('xss')</script>")
	f.Add("' OR 1=1 --")
	f.Add(strings.Repeat("a", 1024))
	f.Add("\u540d\u524d\u30c6\u30b9\u30c8")
	f.Add("ffffffff-ffff-ffff-ffff-ffffffffffff")

	env := h.LoadEnvironment()
	ctx := context.Background()
	cli := h.NewHTTPClient(env.ManagerURL, env.HTTPTimeout)
	headers := h.AuthHeaders()
	reCtl := regexp.MustCompile(`[\x00-\x1F\x7F]`)

	f.Fuzz(func(t *testing.T, tplID string) {
		if len(tplID) > 128 {
			tplID = tplID[:128]
		}
		// Remove control characters and trim spaces
		tplID = reCtl.ReplaceAllString(tplID, "")
		tplID = strings.TrimSpace(tplID)

		payload := map[string]any{
			"templateId": tplID,
			"filters":    map[string]any{"any": map[string]any{"eq": []any{"x"}}},
		}
		code, body, err := cli.Request(ctx, "POST", "/v1/reports", headers, payload)
		if err != nil {
			t.Fatalf("request error: %v", err)
		}
		if code >= 500 {
			t.Fatalf("server 5xx on fuzz: %d body=%s", code, string(body))
		}
		if code == 201 {
			var rep struct {
				ID string `json:"id"`
			}
			_ = json.Unmarshal(body, &rep)
			if rep.ID == "" {
				t.Fatalf("accepted without ID: %s", string(body))
			}
			time.Sleep(10 * time.Millisecond)
		}
	})
}
