// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package systemfacts

import (
	"context"
	"database/sql"
	"io/fs"
	"slices"
	"sync"
	"testing"

	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libSystemFacts "github.com/LerianStudio/lib-system-facts"
	"github.com/LerianStudio/lib-system-facts/emitter"
	"github.com/LerianStudio/lib-system-facts/manifest"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger"
)

const embeddedContractsDir = "contracts/system-facts"

func parseUUID(t *testing.T, value string) uuid.UUID {
	t.Helper()

	id, err := uuid.Parse(value)
	require.NoError(t, err)

	return id
}

// The literal pins the namespace: a changed namespace or name recipe makes a
// retried completion record a second fact, so this value must never move.
func TestTransactionCommittedFactID_IsDeterministicAndPinned(t *testing.T) {
	t.Parallel()

	transactionID := parseUUID(t, fixedTransactionID)

	first := TransactionCommittedFactID(transactionID)
	second := TransactionCommittedFactID(transactionID)
	other := TransactionCommittedFactID(parseUUID(t, fixedParentID))

	assert.Equal(t, first, second, "the same transaction derives the same fact id")
	assert.NotEqual(t, first, other, "different transactions derive different fact ids")
	assert.Equal(t, uuid.Version(5), first.Version())
	assert.Equal(t, parseUUID(t, "3fd36a71-6dfb-562e-a7ba-432fdb70efb4"), first)
}

// capturingKitRecorder is the kit's Recorder port: it keeps every envelope
// the validating emitter accepted, in place of the outbox.
type capturingKitRecorder struct {
	mu        sync.Mutex
	envelopes []emitter.Envelope
}

func (c *capturingKitRecorder) Record(_ context.Context, _ *sql.Tx, env emitter.Envelope, _ []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.envelopes = append(c.envelopes, env)

	return nil
}

func (c *capturingKitRecorder) captured() []emitter.Envelope {
	c.mu.Lock()
	defer c.mu.Unlock()

	return slices.Clone(c.envelopes)
}

// embeddedCatalog compiles the embedded contracts the way the producer does
// at boot, so the emitter validates against what ships.
func embeddedCatalog(t *testing.T) *manifest.Catalog {
	t.Helper()

	dir, err := fs.Sub(ledger.SystemFactsContracts, embeddedContractsDir)
	require.NoError(t, err)

	m, err := manifest.Load(dir, ".")
	require.NoError(t, err, "embedded manifest must load")

	catalog, err := m.Compile()
	require.NoError(t, err, "embedded manifest must compile")

	return catalog
}

// Every fact scenario the builder produces is run through the kit's
// validating emitter over the embedded catalog: a payload the schema would
// refuse is a lost fact, so the proof lives beside the builder.
func TestNewTransactionCommitted_ValidatesAgainstEmbeddedContract(t *testing.T) {
	t.Parallel()

	catalog := embeddedCatalog(t)
	transactionID := parseUUID(t, fixedTransactionID)

	for _, tc := range payloadCases() {
		if !tc.wantFact {
			continue
		}

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			kitRecorder := &capturingKitRecorder{}

			facts, err := libSystemFacts.New(libSystemFacts.Config{Enabled: true, Catalog: catalog, Recorder: kitRecorder})
			require.NoError(t, err)

			recorder, err := NewRecorder(facts, libLog.NewNop())
			require.NoError(t, err)

			require.NoError(t, recorder.RecordTransactionCommitted(context.Background(), nil, tc.record))

			envelopes := kitRecorder.captured()
			require.Len(t, envelopes, 1, "exactly one envelope per fact")

			env := envelopes[0]
			assert.Equal(t, catalog.Producer(), env.Producer)
			assert.Equal(t, catalog.Namespace(), env.Namespace)
			assert.Equal(t, transactionCommittedFactType, env.Type)
			assert.Equal(t, transactionCommittedSchemaVersion, env.SchemaVersion)
			assert.Equal(t, TransactionCommittedFactID(transactionID), env.FactID)
			assert.True(t, env.OccurredAt.Equal(fixedUpdatedAt))
			assert.Equal(t, emitter.Subject{Type: transactionSubjectType, ID: fixedTransactionID}, env.Subject)

			require.Len(t, env.References, 1)
			assert.Equal(t, transactionIDReferenceType, env.References[0].Type)
			assert.Equal(t, fixedTransactionID, env.References[0].Value)
			assert.Equal(t, map[string]string{
				scopeKeyOrganizationID: fixedOrganizationID,
				scopeKeyLedgerID:       fixedLedgerID,
			}, env.References[0].Scope)
		})
	}
}
