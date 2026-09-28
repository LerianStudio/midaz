// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// A 400 that names a parameter the API does not accept is worse than a 400 that
// says nothing: Huma silently DROPS an unregistered query param, so a caller who
// obeys the message gets HTTP 200 for the default window and reads a number for
// the wrong period. Measured on the shipped build before this test existed:
// ?startDate=…&endDate=… over a two-day window answered 200 with 82,095
// transactions, the whole 30-day default, instead of the 2,757 in the range.
//
// So the parameter names in the message are asserted against the names the Huma
// operations actually register, read off the generated spec rather than restated
// here. A second hard-coded copy would drift exactly the way the first one did.
func TestDashboardWindowErrorMessageNamesTheRegisteredParams(t *testing.T) {
	registered := dashboardQueryParamNames(t)
	require.NotEmpty(t, registered, "no dashboard query parameters found in the spec; the reader below is broken, not the message")

	message := pkg.ValidateBusinessError(constant.ErrInvalidDashboardWindow, "Dashboard").Error()

	for _, name := range registered {
		require.Containsf(t, message, name,
			"the 400 for an invalid dashboard window must name %q, the parameter the API registers; message was %q",
			name, message)

		// The failure this test exists to prevent is the message naming a
		// plausible-looking alias instead. For every registered snake_case name,
		// its camelCase spelling must be absent.
		if alias := camelAlias(name); alias != name {
			require.NotContainsf(t, message, alias,
				"the 400 names %q, which the API does not accept — Huma drops it and answers 200 for the default window; use %q",
				alias, name)
		}
	}
}

// dashboardQueryParamNames reads every query parameter registered on a
// /dashboard/* operation off the generated OpenAPI document — the same document
// openapi.ServeSpec publishes, so this is what a caller is told to send.
func dashboardQueryParamNames(t *testing.T) []string {
	t.Helper()

	doc := buildTracerHumaAPI().OpenAPI()

	seen := map[string]bool{}

	var names []string

	for path, item := range doc.Paths {
		if !strings.Contains(path, "/dashboard/") || item.Get == nil {
			continue
		}

		for _, param := range item.Get.Parameters {
			if param.In != "query" || seen[param.Name] {
				continue
			}

			seen[param.Name] = true
			names = append(names, param.Name)
		}
	}

	return names
}

// camelAlias renders a snake_case parameter name the way a Go or JSON author
// reflexively would. "start_date" -> "startDate"; a name with no underscore is
// returned unchanged and the caller skips it.
func camelAlias(name string) string {
	parts := strings.Split(name, "_")
	if len(parts) == 1 {
		return name
	}

	out := parts[0]
	for _, p := range parts[1:] {
		if p == "" {
			continue
		}

		out += strings.ToUpper(p[:1]) + p[1:]
	}

	return out
}
