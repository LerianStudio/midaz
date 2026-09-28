// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const validProducerBindings = `[{"uri":"spiffe://example.test/ledger","integrationId":"ledger","purposes":["reserve"]}]`

func TestLoadContextProducerIdentity(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		bindings string
		valid    bool
		reason   string
	}{
		{name: "valid", bindings: validProducerBindings, valid: true},
		{name: "rotated identities share an integration", bindings: `[{"uri":"spiffe://example.test/ledger","integrationId":"ledger","purposes":["reserve"]},{"uri":"spiffe://example.test/ledger-next","integrationId":"ledger","purposes":["reserve"]}]`, valid: true},
		{name: "empty", bindings: ""},
		{name: "oversized", bindings: "[" + strings.Repeat(" ", 65536) + "]"},
		{name: "no identity", bindings: "[]"},
		{name: "not an array", bindings: `{"uri":"spiffe://example.test/ledger"}`},
		{name: "trailing document", bindings: validProducerBindings + `[]`},
		{name: "binding with unsupported field assetNamespace", bindings: `[{"uri":"spiffe://example.test/ledger","integrationId":"ledger","assetNamespace":"ledger","purposes":["reserve"]}]`, reason: `unknown field "assetNamespace"`},
		{name: "unknown field", bindings: `[{"uri":"spiffe://example.test/ledger","integrationId":"ledger","purposes":["reserve"],"extra":true}]`},
		{name: "unknown purpose", bindings: `[{"uri":"spiffe://example.test/ledger","integrationId":"ledger","purposes":["admin"]}]`},
		{name: "missing purposes", bindings: `[{"uri":"spiffe://example.test/ledger","integrationId":"ledger"}]`},
		{name: "duplicate URI", bindings: `[{"uri":"spiffe://example.test/ledger","integrationId":"ledger","purposes":["reserve"]},{"uri":"spiffe://example.test/ledger","integrationId":"other","purposes":["reserve"]}]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			resolver, err := loadContextProducerIdentity(&Config{ContextProducerBindings: tc.bindings})
			if tc.valid {
				require.NoError(t, err)
				require.NotNil(t, resolver)

				return
			}

			require.Error(t, err)
			require.Nil(t, resolver)

			if tc.reason != "" {
				require.ErrorContains(t, err, tc.reason)
			}
		})
	}
}
