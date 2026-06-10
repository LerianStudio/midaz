// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package encryption

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// spyAuditWriter is a controllable AuditWriter test double. It records every
// event passed to Emit/EmitAsync so a test can assert exactly one event was
// emitted with the expected outcome. It can be configured to return an error
// (via the underlying repo) or to panic to prove provisioning is unaffected.
type spyAuditWriter struct {
	mu         sync.Mutex
	emitted    []*mmodel.ProtectionAuditEvent
	asyncCalls int
	syncCalls  int
}

func newSpyAuditWriter() *spyAuditWriter {
	return &spyAuditWriter{}
}

func (s *spyAuditWriter) Emit(_ context.Context, event *mmodel.ProtectionAuditEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.syncCalls++
	s.emitted = append(s.emitted, event)
}

// EmitAsync records synchronously (no goroutine) so tests observe the event
// deterministically without sleeping. The production writer detaches; this spy
// captures the call to verify the single-point emission contract.
func (s *spyAuditWriter) EmitAsync(_ context.Context, event *mmodel.ProtectionAuditEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.asyncCalls++
	s.emitted = append(s.emitted, event)
}

func (s *spyAuditWriter) events() []*mmodel.ProtectionAuditEvent {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]*mmodel.ProtectionAuditEvent, len(s.emitted))
	copy(out, s.emitted)

	return out
}

// requireSingleEvent asserts exactly one event was emitted and returns it.
func requireSingleEvent(t *testing.T, spy *spyAuditWriter) *mmodel.ProtectionAuditEvent {
	t.Helper()

	events := spy.events()
	require.Len(t, events, 1, "exactly one audit event must be emitted per terminal path")
	require.NotNil(t, events[0])

	return events[0]
}

func TestProvisioningService_Provision_EmitsSuccessAuditEvent(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	keysetRepo := newFakeKeysetRepoForProv()
	registryRepo := newFakeRegistryRepoForProv()
	keysetGenerator := newFakeKeysetGenerator()
	spy := newSpyAuditWriter()

	svc := NewProvisioningService(keysetRepo, registryRepo, keysetGenerator, DefaultProvisioningConfig(), spy)

	req := ProvisionInput{
		TenantID:       "tenant-123",
		OrganizationID: "org-456",
		Actor:          "admin@example.com",
		Reason:         "Initial provisioning",
	}

	result, err := svc.Provision(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, mmodel.RegistryStatusActive, result.RegistryStatus)

	event := requireSingleEvent(t, spy)
	assert.Equal(t, mmodel.AuditEventTypeProvisioning, event.EventType)
	assert.Equal(t, mmodel.AuditActionProvision, event.Action)
	assert.Equal(t, mmodel.AuditOutcomeSuccess, event.Outcome)
	assert.Equal(t, "org-456", event.OrganizationID)
	assert.Equal(t, "tenant-123", event.TenantID)
	assert.Equal(t, "admin@example.com", event.ActorID)
	assert.Equal(t, "service", event.ActorType)
	require.NotNil(t, event.Details)
	assert.Equal(t, "active", event.Details.NewStatus)
	assert.NotEmpty(t, event.Details.AffectedKeyIDs)
}

func newProvisionReq() ProvisionInput {
	return ProvisionInput{
		TenantID:       "tenant-123",
		OrganizationID: "org-456",
		Actor:          "admin@example.com",
		Reason:         "Initial provisioning",
	}
}

func TestProvisioningService_Provision_AlreadyProvisioned_EmitsAlreadyExists(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	keysetRepo := newFakeKeysetRepoForProv()
	registryRepo := newFakeRegistryRepoForProv()
	keysetGenerator := newFakeKeysetGenerator()
	spy := newSpyAuditWriter()

	keysetRepo.keysets["org-456"] = &mmodel.OrganizationKeyset{
		OrganizationID: "org-456",
		KEKPath:        "org-org-456",
		KeysetInfo:     mmodel.KeysetInfo{PrimaryKeyID: 12345},
		HMACKeysetInfo: mmodel.KeysetInfo{PrimaryKeyID: 67890},
	}
	registryRepo.records["org-456"] = &mmodel.OrganizationRegistryRecord{
		OrganizationID: "org-456",
		Status:         mmodel.RegistryStatusActive,
	}

	svc := NewProvisioningService(keysetRepo, registryRepo, keysetGenerator, DefaultProvisioningConfig(), spy)

	_, err := svc.Provision(ctx, newProvisionReq())
	require.NoError(t, err)

	event := requireSingleEvent(t, spy)
	assert.Equal(t, mmodel.AuditOutcomeAlreadyExists, event.Outcome)
	assert.Equal(t, mmodel.AuditEventTypeProvisioning, event.EventType)
	assert.Equal(t, mmodel.AuditActionProvision, event.Action)
}

func TestProvisioningService_Provision_RegistryRecreatedFromExistingKeyset_EmitsSuccess(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	keysetRepo := newFakeKeysetRepoForProv()
	registryRepo := newFakeRegistryRepoForProv()
	keysetGenerator := newFakeKeysetGenerator()
	spy := newSpyAuditWriter()

	// Keyset exists but registry missing -> handleExistingKeyset recreates registry.
	keysetRepo.keysets["org-456"] = &mmodel.OrganizationKeyset{
		TenantID:       "tenant-123",
		OrganizationID: "org-456",
		KEKPath:        "org-org-456",
		KeysetInfo:     mmodel.KeysetInfo{PrimaryKeyID: 12345},
		HMACKeysetInfo: mmodel.KeysetInfo{PrimaryKeyID: 67890},
	}

	svc := NewProvisioningService(keysetRepo, registryRepo, keysetGenerator, DefaultProvisioningConfig(), spy)

	result, err := svc.Provision(ctx, newProvisionReq())
	require.NoError(t, err)
	assert.Equal(t, mmodel.RegistryStatusActive, result.RegistryStatus)

	event := requireSingleEvent(t, spy)
	assert.Equal(t, mmodel.AuditOutcomeSuccess, event.Outcome)
	require.NotNil(t, event.Details)
	assert.Equal(t, "active", event.Details.NewStatus)
	assert.Contains(t, event.Details.AffectedKeyIDs, uint32(12345))
}

func TestProvisioningService_Provision_RegistrySaveRace_EmitsAlreadyExists(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	keysetRepo := newFakeKeysetRepoForProv()
	keysetGenerator := newFakeKeysetGenerator()
	spy := newSpyAuditWriter()

	// Drive the createAndSaveRegistry race deterministically with a registry repo
	// that simulates a concurrent winner: IsProvisioned's first Get sees no
	// registry (not provisioned), the flow generates + saves a fresh keyset, then
	// Save returns ErrRegistryAlreadyExists; the recovery Get then returns the
	// concurrently-written registry so getExistingProvisionResult succeeds.
	registryRepo := &raceRegistryRepo{}

	svc := NewProvisioningService(keysetRepo, registryRepo, keysetGenerator, DefaultProvisioningConfig(), spy)

	_, err := svc.Provision(ctx, newProvisionReq())
	require.NoError(t, err)

	event := requireSingleEvent(t, spy)
	assert.Equal(t, mmodel.AuditOutcomeAlreadyExists, event.Outcome)
}

// raceRegistryRepo simulates a concurrent provisioning winner. The first Get
// (from IsProvisioned) reports not-provisioned; Save loses the race with
// ErrRegistryAlreadyExists; subsequent Gets return the concurrently-written
// active registry.
type raceRegistryRepo struct {
	getCalls int
}

func (r *raceRegistryRepo) Get(_ context.Context, organizationID string) (*mmodel.OrganizationRegistryRecord, error) {
	r.getCalls++

	if r.getCalls == 1 {
		return nil, mmodel.ErrRegistryNotFound
	}

	return &mmodel.OrganizationRegistryRecord{
		TenantID:       "tenant-123",
		OrganizationID: organizationID,
		Status:         mmodel.RegistryStatusActive,
	}, nil
}

func (r *raceRegistryRepo) Save(_ context.Context, _ *mmodel.OrganizationRegistryRecord) error {
	return mmodel.ErrRegistryAlreadyExists
}

func (r *raceRegistryRepo) Update(_ context.Context, _ *mmodel.OrganizationRegistryRecord, _ int64) error {
	return nil
}

// TestProvisioningService_Provision_HandleExistingKeyset_BothExist_EmitsAlreadyExists
// drives the handleExistingKeyset "both keyset and registry exist" branch: the
// org is not provisioned at the IsProvisioned check, generation succeeds, the
// keyset Save loses a race with ErrKeysetAlreadyExists, and a concurrently
// written registry is then found -> already_exists.
func TestProvisioningService_Provision_HandleExistingKeyset_BothExist_EmitsAlreadyExists(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	keysetGenerator := newFakeKeysetGenerator()
	spy := newSpyAuditWriter()

	keysetRepo := &raceKeysetRepo{}
	registryRepo := &raceRegistryRepoBothExist{}

	svc := NewProvisioningService(keysetRepo, registryRepo, keysetGenerator, DefaultProvisioningConfig(), spy)

	_, err := svc.Provision(ctx, newProvisionReq())
	require.NoError(t, err)

	event := requireSingleEvent(t, spy)
	assert.Equal(t, mmodel.AuditOutcomeAlreadyExists, event.Outcome)
}

// raceKeysetRepo loses the keyset Save race (ErrKeysetAlreadyExists) but returns
// a stored keyset on Get, simulating a concurrent winner.
type raceKeysetRepo struct{}

func (r *raceKeysetRepo) Save(_ context.Context, _ *mmodel.OrganizationKeyset) error {
	return mmodel.ErrKeysetAlreadyExists
}

func (r *raceKeysetRepo) Get(_ context.Context, organizationID string) (*mmodel.OrganizationKeyset, error) {
	return &mmodel.OrganizationKeyset{
		TenantID:       "tenant-123",
		OrganizationID: organizationID,
		KEKPath:        "org-org-456",
		KeysetInfo:     mmodel.KeysetInfo{PrimaryKeyID: 12345},
		HMACKeysetInfo: mmodel.KeysetInfo{PrimaryKeyID: 67890},
	}, nil
}

func (r *raceKeysetRepo) Update(_ context.Context, _ *mmodel.OrganizationKeyset, _ int64) error {
	return nil
}

// raceRegistryRepoBothExist reports not-provisioned on the first Get (so the
// create path runs) and an active registry afterward (so handleExistingKeyset
// sees an existing registry).
type raceRegistryRepoBothExist struct {
	getCalls int
}

func (r *raceRegistryRepoBothExist) Get(_ context.Context, organizationID string) (*mmodel.OrganizationRegistryRecord, error) {
	r.getCalls++

	if r.getCalls == 1 {
		return nil, mmodel.ErrRegistryNotFound
	}

	return &mmodel.OrganizationRegistryRecord{
		TenantID:       "tenant-123",
		OrganizationID: organizationID,
		Status:         mmodel.RegistryStatusActive,
	}, nil
}

func (r *raceRegistryRepoBothExist) Save(_ context.Context, _ *mmodel.OrganizationRegistryRecord) error {
	return nil
}

func (r *raceRegistryRepoBothExist) Update(_ context.Context, _ *mmodel.OrganizationRegistryRecord, _ int64) error {
	return nil
}

func TestProvisioningService_Provision_Failures_EmitFailure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mutate  func(k *fakeKeysetRepoForProv, r *fakeRegistryRepoForProv, g *fakeKeysetGenerator)
		wantErr bool
	}{
		{
			name: "aead generation failed",
			mutate: func(_ *fakeKeysetRepoForProv, _ *fakeRegistryRepoForProv, g *fakeKeysetGenerator) {
				g.aeadErr = errors.New("KMS unavailable")
			},
		},
		{
			name: "mac generation failed",
			mutate: func(_ *fakeKeysetRepoForProv, _ *fakeRegistryRepoForProv, g *fakeKeysetGenerator) {
				g.macErr = errors.New("KMS unavailable")
			},
		},
		{
			name: "keyset save failed",
			mutate: func(k *fakeKeysetRepoForProv, _ *fakeRegistryRepoForProv, _ *fakeKeysetGenerator) {
				k.saveErr = errors.New("database error")
			},
		},
		{
			name: "registry save failed",
			mutate: func(_ *fakeKeysetRepoForProv, r *fakeRegistryRepoForProv, _ *fakeKeysetGenerator) {
				r.saveErr = errors.New("database error")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			keysetRepo := newFakeKeysetRepoForProv()
			registryRepo := newFakeRegistryRepoForProv()
			keysetGenerator := newFakeKeysetGenerator()
			spy := newSpyAuditWriter()

			tt.mutate(keysetRepo, registryRepo, keysetGenerator)

			svc := NewProvisioningService(keysetRepo, registryRepo, keysetGenerator, DefaultProvisioningConfig(), spy)

			_, err := svc.Provision(ctx, newProvisionReq())
			require.Error(t, err)

			event := requireSingleEvent(t, spy)
			assert.Equal(t, mmodel.AuditOutcomeFailure, event.Outcome)
			require.NotNil(t, event.Details)
			assert.Equal(t, constant.ErrProvisioningFailed.Error(), event.Details.ErrorCode)
		})
	}
}

// TestProvisioningService_Provision_ResultUnchangedVsWriter proves the audit
// writer never alters the provisioning return value or error path. It uses the
// real repository-backed writer (EmitAsync, panic-safe per T-1.3.1) with a
// repository that errors and one that panics; both must yield the same result
// as the no-op writer.
func TestProvisioningService_Provision_ResultUnchangedVsWriter(t *testing.T) {
	t.Parallel()

	run := func(t *testing.T, w AuditWriter) (ProvisionResult, error) {
		t.Helper()

		ctx := context.Background()
		svc := NewProvisioningService(newFakeKeysetRepoForProv(), newFakeRegistryRepoForProv(), newFakeKeysetGenerator(), DefaultProvisioningConfig(), w)

		return svc.Provision(ctx, newProvisionReq())
	}

	// Baseline: recording spy writer that never mutates the result.
	baseResult, baseErr := run(t, newSpyAuditWriter())
	require.NoError(t, baseErr)

	// Repository-backed writer whose repo errors: best-effort contract swallows it.
	errRepo := newSpyRepo()
	errRepo.err = errors.New("audit store down")
	errWriter := NewAuditWriter(errRepo, testLogger())

	require.NotPanics(t, func() {
		result, err := run(t, errWriter)
		require.NoError(t, err)
		assert.Equal(t, baseResult.OrganizationID, result.OrganizationID)
		assert.Equal(t, baseResult.RegistryStatus, result.RegistryStatus)
	})

	// Wait for the async write so its (recovered) goroutine finishes before the
	// goleak check in TestMain runs.
	<-errRepo.called

	// Repository-backed writer whose repo panics: EmitAsync recovers on its
	// detached goroutine, so Provision is unaffected.
	panicRepo := newSpyRepo()
	panicRepo.panicMsg = "audit store panic"
	panicWriter := NewAuditWriter(panicRepo, testLogger())

	require.NotPanics(t, func() {
		result, err := run(t, panicWriter)
		require.NoError(t, err)
		assert.Equal(t, baseResult.OrganizationID, result.OrganizationID)
	})

	<-panicRepo.called
}

// TestProvisioningService_Provision_InvalidRequest_EmitsFailure verifies a
// validation failure still produces exactly one failure audit event.
func TestProvisioningService_Provision_InvalidRequest_EmitsFailure(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	spy := newSpyAuditWriter()
	svc := NewProvisioningService(nil, nil, nil, DefaultProvisioningConfig(), spy)

	req := newProvisionReq()
	req.TenantID = ""

	_, err := svc.Provision(ctx, req)
	require.Error(t, err)

	// Validation failure: org id is present so the event can be built.
	event := requireSingleEvent(t, spy)
	assert.Equal(t, mmodel.AuditOutcomeFailure, event.Outcome)
}

// TestProvisioningService_Provision_EventBuildFails_SkipsEmission verifies that
// when the audit event cannot be built (empty OrganizationID), emission is
// silently skipped and provisioning still returns its validation error.
func TestProvisioningService_Provision_EventBuildFails_SkipsEmission(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	spy := newSpyAuditWriter()
	svc := NewProvisioningService(nil, nil, nil, DefaultProvisioningConfig(), spy)

	req := newProvisionReq()
	req.OrganizationID = "" // fails validation AND event build (OrganizationID required)

	_, err := svc.Provision(ctx, req)
	require.Error(t, err)

	// Event build fails -> emission skipped -> no events recorded.
	assert.Empty(t, spy.events(), "emission must be skipped when the event cannot be built")
}
