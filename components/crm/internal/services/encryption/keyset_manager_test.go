// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package encryption

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"

	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/crypto/kms/vault"
	"github.com/LerianStudio/midaz/v3/pkg/crypto/tink"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/tests/helpers"
)

// fakeKeysetRepo is a test double for mongoEncryption.KeysetRepository.
type fakeKeysetRepo struct {
	mu     sync.Mutex
	keyset *mmodel.OrganizationKeyset
	err    error
	calls  int
}

func (f *fakeKeysetRepo) Get(_ context.Context, _ string) (*mmodel.OrganizationKeyset, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.calls++

	return f.keyset, f.err
}

func (f *fakeKeysetRepo) GetActive(ctx context.Context, organizationID string) (*mmodel.OrganizationKeyset, error) {
	return f.Get(ctx, organizationID)
}

func (f *fakeKeysetRepo) GetByVersion(ctx context.Context, organizationID string, _ int) (*mmodel.OrganizationKeyset, error) {
	return f.Get(ctx, organizationID)
}

func (f *fakeKeysetRepo) Save(_ context.Context, _ *mmodel.OrganizationKeyset) error {
	return nil
}

func (f *fakeKeysetRepo) Update(_ context.Context, _ *mmodel.OrganizationKeyset, _ int64) error {
	return nil
}

func (f *fakeKeysetRepo) getCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.calls
}

// fakeKeysetUnwrapper is a test double for KeysetUnwrapper.
type fakeKeysetUnwrapper struct {
	mu         sync.Mutex
	aeadKeyset []byte
	prfKeyset  []byte
	err        error
	calls      int
	mountPaths []string // Records the mountPath received on each call.
}

func (f *fakeKeysetUnwrapper) UnwrapKeyset(_ context.Context, mountPath, _ string, wrappedKeyset string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.calls++
	f.mountPaths = append(f.mountPaths, mountPath)

	if f.err != nil {
		return nil, f.err
	}

	// Return AEAD keyset for AEAD wrapped data, PRF keyset for the compatibility slot.
	if wrappedKeyset == "wrapped-aead" {
		return f.aeadKeyset, nil
	}

	if wrappedKeyset == "wrapped-prf" {
		return f.prfKeyset, nil
	}

	return nil, errors.New("unknown wrapped keyset")
}

func (f *fakeKeysetUnwrapper) getCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.calls
}

func (f *fakeKeysetUnwrapper) getMountPaths() []string {
	f.mu.Lock()
	defer f.mu.Unlock()

	out := make([]string, len(f.mountPaths))
	copy(out, f.mountPaths)

	return out
}

// generateTestKeysets creates real Tink keysets for testing.
func generateTestKeysets(t *testing.T) ([]byte, []byte) {
	t.Helper()

	keysets := helpers.GenerateTinkKeysets(t)

	return keysets.AEADBytes, keysets.PRFBytes
}

func TestKeysetManager_GetPrimitives_CacheMiss_Success(t *testing.T) {
	t.Parallel()

	aeadBytes, prfBytes := generateTestKeysets(t)

	reader := &fakeKeysetRepo{
		keyset: &mmodel.OrganizationKeyset{
			OrganizationID:    "org-123",
			KEKPath:           "org-org-123",
			WrappedKeyset:     "wrapped-aead",
			WrappedHMACKeyset: "wrapped-prf",
		},
	}

	unwrapper := &fakeKeysetUnwrapper{
		aeadKeyset: aeadBytes,
		prfKeyset:  prfBytes,
	}

	manager := NewKeysetManager(reader, unwrapper, nil, DefaultKeysetManagerConfig(), NewProtectionMetrics(nil))

	prims, err := manager.GetActivePrimitives(context.Background(), "org-123")
	aead := prims.AEAD
	prf := prims.PRF
	if err != nil {
		t.Fatalf("GetPrimitives() error = %v", err)
	}

	if aead == nil {
		t.Error("GetPrimitives() AEAD is nil")
	}

	if prf == nil {
		t.Error("GetPrimitives() PRF is nil")
	}

	if reader.getCalls() != 1 {
		t.Errorf("GetPrimitives() reader calls = %d, want 1", reader.getCalls())
	}

	if unwrapper.getCalls() != 2 {
		t.Errorf("GetPrimitives() unwrapper calls = %d, want 2", unwrapper.getCalls())
	}
}

func TestKeysetManager_GetPrimitives_CacheHit_ReturnsCached(t *testing.T) {
	t.Parallel()

	aeadBytes, prfBytes := generateTestKeysets(t)

	reader := &fakeKeysetRepo{
		keyset: &mmodel.OrganizationKeyset{
			OrganizationID:    "org-456",
			KEKPath:           "org-org-456",
			WrappedKeyset:     "wrapped-aead",
			WrappedHMACKeyset: "wrapped-prf",
		},
	}

	unwrapper := &fakeKeysetUnwrapper{
		aeadKeyset: aeadBytes,
		prfKeyset:  prfBytes,
	}

	manager := NewKeysetManager(reader, unwrapper, nil, DefaultKeysetManagerConfig(), NewProtectionMetrics(nil))

	// First call - cache miss
	prims2, err := manager.GetActivePrimitives(context.Background(), "org-456")
	aead1 := prims2.AEAD
	prf1 := prims2.PRF
	if err != nil {
		t.Fatalf("GetPrimitives() first call error = %v", err)
	}

	// Second call - should be cache hit
	prims3, err := manager.GetActivePrimitives(context.Background(), "org-456")
	aead2 := prims3.AEAD
	prf2 := prims3.PRF
	if err != nil {
		t.Fatalf("GetPrimitives() second call error = %v", err)
	}

	// Should return the same primitives (cached)
	if aead1 != aead2 {
		t.Error("GetPrimitives() AEAD primitives should be the same instance")
	}

	if prf1 != prf2 {
		t.Error("GetPrimitives() PRF primitives should be the same instance")
	}

	// Reader should only be called once
	if reader.getCalls() != 1 {
		t.Errorf("GetPrimitives() reader calls = %d, want 1", reader.getCalls())
	}

	// Unwrapper should only be called twice (once for AEAD, once for PRF on first call)
	if unwrapper.getCalls() != 2 {
		t.Errorf("GetPrimitives() unwrapper calls = %d, want 2", unwrapper.getCalls())
	}
}

func TestKeysetManager_GetPrimitives_CacheExpired_Refetches(t *testing.T) {
	t.Parallel()

	aeadBytes, prfBytes := generateTestKeysets(t)

	reader := &fakeKeysetRepo{
		keyset: &mmodel.OrganizationKeyset{
			OrganizationID:    "org-789",
			KEKPath:           "org-org-789",
			WrappedKeyset:     "wrapped-aead",
			WrappedHMACKeyset: "wrapped-prf",
		},
	}

	unwrapper := &fakeKeysetUnwrapper{
		aeadKeyset: aeadBytes,
		prfKeyset:  prfBytes,
	}

	// Use a very short TTL for testing
	config := KeysetManagerConfig{
		CacheTTL: 10 * time.Millisecond,
	}
	manager := NewKeysetManager(reader, unwrapper, nil, config, NewProtectionMetrics(nil))

	// First call - cache miss
	_, err := manager.GetActivePrimitives(context.Background(), "org-789")
	if err != nil {
		t.Fatalf("GetPrimitives() first call error = %v", err)
	}

	// Wait for cache to expire
	time.Sleep(20 * time.Millisecond)

	// Second call - cache expired, should refetch
	_, err = manager.GetActivePrimitives(context.Background(), "org-789")
	if err != nil {
		t.Fatalf("GetPrimitives() second call error = %v", err)
	}

	// Reader should be called twice (once per cache miss)
	if reader.getCalls() != 2 {
		t.Errorf("GetPrimitives() reader calls = %d, want 2", reader.getCalls())
	}

	// Unwrapper should be called 4 times (2 per cache miss)
	if unwrapper.getCalls() != 4 {
		t.Errorf("GetPrimitives() unwrapper calls = %d, want 4", unwrapper.getCalls())
	}
}

func TestKeysetManager_GetPrimitives_KeysetNotFound_Error(t *testing.T) {
	t.Parallel()

	reader := &fakeKeysetRepo{
		keyset: nil,
		err:    mmodel.ErrKeysetNotFound,
	}

	unwrapper := &fakeKeysetUnwrapper{}

	manager := NewKeysetManager(reader, unwrapper, nil, DefaultKeysetManagerConfig(), NewProtectionMetrics(nil))

	_, err := manager.GetActivePrimitives(context.Background(), "org-not-found")
	if err == nil {
		t.Fatal("GetPrimitives() expected error, got nil")
	}

	if !errors.Is(err, mmodel.ErrKeysetNotFound) {
		t.Errorf("GetPrimitives() error = %v, want %v", err, mmodel.ErrKeysetNotFound)
	}

	// Unwrapper should not be called
	if unwrapper.getCalls() != 0 {
		t.Errorf("GetPrimitives() unwrapper calls = %d, want 0", unwrapper.getCalls())
	}
}

func TestKeysetManager_GetPrimitives_UnwrapError_Propagated(t *testing.T) {
	t.Parallel()

	reader := &fakeKeysetRepo{
		keyset: &mmodel.OrganizationKeyset{
			OrganizationID:    "org-unwrap-fail",
			KEKPath:           "org-org-unwrap-fail",
			WrappedKeyset:     "wrapped-aead",
			WrappedHMACKeyset: "wrapped-prf",
		},
	}

	unwrapErr := errors.New("KMS unwrap failed")
	unwrapper := &fakeKeysetUnwrapper{
		err: unwrapErr,
	}

	manager := NewKeysetManager(reader, unwrapper, nil, DefaultKeysetManagerConfig(), NewProtectionMetrics(nil))

	_, err := manager.GetActivePrimitives(context.Background(), "org-unwrap-fail")
	if err == nil {
		t.Fatal("GetPrimitives() expected error, got nil")
	}

	if !errors.Is(err, unwrapErr) {
		t.Errorf("GetPrimitives() error = %v, want %v", err, unwrapErr)
	}
}

func TestKeysetManager_GetPrimitives_ParseError_Propagated(t *testing.T) {
	t.Parallel()

	reader := &fakeKeysetRepo{
		keyset: &mmodel.OrganizationKeyset{
			OrganizationID:    "org-parse-fail",
			KEKPath:           "org-org-parse-fail",
			WrappedKeyset:     "wrapped-aead",
			WrappedHMACKeyset: "wrapped-prf",
		},
	}

	// Return invalid keyset bytes that cannot be parsed
	unwrapper := &fakeKeysetUnwrapper{
		aeadKeyset: []byte("invalid-keyset-data"),
		prfKeyset:  []byte("invalid-prf-keyset-data"),
	}

	manager := NewKeysetManager(reader, unwrapper, nil, DefaultKeysetManagerConfig(), NewProtectionMetrics(nil))

	_, err := manager.GetActivePrimitives(context.Background(), "org-parse-fail")
	if err == nil {
		t.Fatal("GetPrimitives() expected error for invalid keyset, got nil")
	}
}

func TestKeysetManager_GetPrimitives_ConcurrentAccess_Safe(t *testing.T) {
	t.Parallel()

	aeadBytes, prfBytes := generateTestKeysets(t)

	reader := &fakeKeysetRepo{
		keyset: &mmodel.OrganizationKeyset{
			OrganizationID:    "org-concurrent",
			KEKPath:           "org-org-concurrent",
			WrappedKeyset:     "wrapped-aead",
			WrappedHMACKeyset: "wrapped-prf",
		},
	}

	unwrapper := &fakeKeysetUnwrapper{
		aeadKeyset: aeadBytes,
		prfKeyset:  prfBytes,
	}

	manager := NewKeysetManager(reader, unwrapper, nil, DefaultKeysetManagerConfig(), NewProtectionMetrics(nil))

	const goroutines = 10

	var wg sync.WaitGroup

	wg.Add(goroutines)

	errChan := make(chan error, goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()

			prims4, err := manager.GetActivePrimitives(context.Background(), "org-concurrent")
			aead := prims4.AEAD
			prf := prims4.PRF
			if err != nil {
				errChan <- err
				return
			}

			if aead == nil || prf == nil {
				errChan <- errors.New("nil primitives returned")
			}
		}()
	}

	wg.Wait()
	close(errChan)

	for err := range errChan {
		t.Errorf("GetPrimitives() concurrent error = %v", err)
	}
}

func TestKeysetManager_GetPrimitives_ContextCancelled(t *testing.T) {
	t.Parallel()

	reader := &fakeKeysetRepo{
		keyset: &mmodel.OrganizationKeyset{
			OrganizationID:    "org-ctx-cancel",
			KEKPath:           "org-org-ctx-cancel",
			WrappedKeyset:     "wrapped-aead",
			WrappedHMACKeyset: "wrapped-prf",
		},
	}

	unwrapper := &fakeKeysetUnwrapper{}

	manager := NewKeysetManager(reader, unwrapper, nil, DefaultKeysetManagerConfig(), NewProtectionMetrics(nil))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := manager.GetActivePrimitives(ctx, "org-ctx-cancel")
	if err == nil {
		t.Fatal("GetPrimitives() expected error for cancelled context, got nil")
	}

	if !errors.Is(err, context.Canceled) {
		t.Errorf("GetPrimitives() error = %v, want %v", err, context.Canceled)
	}
}

func TestKeysetManager_InvalidateCache_RemovesEntry(t *testing.T) {
	t.Parallel()

	aeadBytes, prfBytes := generateTestKeysets(t)

	reader := &fakeKeysetRepo{
		keyset: &mmodel.OrganizationKeyset{
			OrganizationID:    "org-invalidate",
			KEKPath:           "org-org-invalidate",
			WrappedKeyset:     "wrapped-aead",
			WrappedHMACKeyset: "wrapped-prf",
		},
	}

	unwrapper := &fakeKeysetUnwrapper{
		aeadKeyset: aeadBytes,
		prfKeyset:  prfBytes,
	}

	manager := NewKeysetManager(reader, unwrapper, nil, DefaultKeysetManagerConfig(), NewProtectionMetrics(nil))

	// First call - cache miss
	_, err := manager.GetActivePrimitives(context.Background(), "org-invalidate")
	if err != nil {
		t.Fatalf("GetPrimitives() first call error = %v", err)
	}

	// Invalidate the cache
	manager.InvalidateCache("org-invalidate")

	// Second call - should refetch after invalidation
	_, err = manager.GetActivePrimitives(context.Background(), "org-invalidate")
	if err != nil {
		t.Fatalf("GetPrimitives() second call error = %v", err)
	}

	// Reader should be called twice (once per cache miss)
	if reader.getCalls() != 2 {
		t.Errorf("GetPrimitives() reader calls = %d, want 2", reader.getCalls())
	}
}

func TestKeysetManager_ClearCache_RemovesAllEntries(t *testing.T) {
	t.Parallel()

	aeadBytes, prfBytes := generateTestKeysets(t)

	reader := &fakeKeysetRepo{
		keyset: &mmodel.OrganizationKeyset{
			OrganizationID:    "org-clear",
			KEKPath:           "org-org-clear",
			WrappedKeyset:     "wrapped-aead",
			WrappedHMACKeyset: "wrapped-prf",
		},
	}

	unwrapper := &fakeKeysetUnwrapper{
		aeadKeyset: aeadBytes,
		prfKeyset:  prfBytes,
	}

	manager := NewKeysetManager(reader, unwrapper, nil, DefaultKeysetManagerConfig(), NewProtectionMetrics(nil))

	// First call - cache miss
	_, err := manager.GetActivePrimitives(context.Background(), "org-clear")
	if err != nil {
		t.Fatalf("GetPrimitives() first call error = %v", err)
	}

	// Clear all cache
	manager.ClearCache()

	// Second call - should refetch after clear
	_, err = manager.GetActivePrimitives(context.Background(), "org-clear")
	if err != nil {
		t.Fatalf("GetPrimitives() second call error = %v", err)
	}

	// Reader should be called twice (once per cache miss)
	if reader.getCalls() != 2 {
		t.Errorf("GetPrimitives() reader calls = %d, want 2", reader.getCalls())
	}
}

func TestCachedPrimitives_IsExpired(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		expiresAt time.Time
		want      bool
	}{
		{
			name:      "future expiration returns false",
			expiresAt: time.Now().Add(1 * time.Hour),
			want:      false,
		},
		{
			name:      "past expiration returns true",
			expiresAt: time.Now().Add(-1 * time.Hour),
			want:      true,
		},
		{
			name:      "zero time is expired",
			expiresAt: time.Time{},
			want:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cp := &CachedPrimitives{
				ExpiresAt: tt.expiresAt,
			}

			if got := cp.IsExpired(); got != tt.want {
				t.Errorf("IsExpired() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewKeysetManager(t *testing.T) {
	t.Parallel()

	reader := &fakeKeysetRepo{}
	unwrapper := &fakeKeysetUnwrapper{}

	manager := NewKeysetManager(reader, unwrapper, nil, DefaultKeysetManagerConfig(), NewProtectionMetrics(nil))

	if manager == nil {
		t.Error("NewKeysetManager() returned nil")
	}
}

func TestDefaultKeysetManagerConfig(t *testing.T) {
	t.Parallel()

	config := DefaultKeysetManagerConfig()

	if config.CacheTTL != 5*time.Minute {
		t.Errorf("DefaultKeysetManagerConfig() CacheTTL = %v, want %v", config.CacheTTL, 5*time.Minute)
	}
}

func TestKeysetManager_GetPrimitives_MultipleOrganizations(t *testing.T) {
	t.Parallel()

	aeadBytes, prfBytes := generateTestKeysets(t)

	reader := &fakeKeysetRepo{}
	unwrapper := &fakeKeysetUnwrapper{
		aeadKeyset: aeadBytes,
		prfKeyset:  prfBytes,
	}

	manager := NewKeysetManager(reader, unwrapper, nil, DefaultKeysetManagerConfig(), NewProtectionMetrics(nil))

	// Test with org-1
	reader.keyset = &mmodel.OrganizationKeyset{
		OrganizationID:    "org-1",
		KEKPath:           "org-org-1",
		WrappedKeyset:     "wrapped-aead",
		WrappedHMACKeyset: "wrapped-prf",
	}

	prims5, err := manager.GetActivePrimitives(context.Background(), "org-1")
	aead1 := prims5.AEAD
	prf1 := prims5.PRF
	if err != nil {
		t.Fatalf("GetPrimitives(org-1) error = %v", err)
	}

	// Test with org-2
	reader.keyset = &mmodel.OrganizationKeyset{
		OrganizationID:    "org-2",
		KEKPath:           "org-org-2",
		WrappedKeyset:     "wrapped-aead",
		WrappedHMACKeyset: "wrapped-prf",
	}

	prims6, err := manager.GetActivePrimitives(context.Background(), "org-2")
	aead2 := prims6.AEAD
	prf2 := prims6.PRF
	if err != nil {
		t.Fatalf("GetPrimitives(org-2) error = %v", err)
	}

	// Should have different primitive instances for different orgs
	if aead1 == aead2 {
		t.Error("GetPrimitives() AEAD primitives should be different for different orgs")
	}

	if prf1 == prf2 {
		t.Error("GetPrimitives() PRF primitives should be different for different orgs")
	}

	// Reader should be called twice (once for each org)
	if reader.getCalls() != 2 {
		t.Errorf("GetPrimitives() reader calls = %d, want 2", reader.getCalls())
	}
}

// slowKeysetRepo is a test double that introduces a delay to simulate slow KMS operations.
// This allows testing per-org mutex deduplication by ensuring concurrent requests overlap.
type slowKeysetRepo struct {
	mu         sync.Mutex
	keyset     *mmodel.OrganizationKeyset
	err        error
	calls      int32 // Use atomic for concurrent access
	delay      time.Duration
	fetchStart chan struct{} // Signals when fetch starts
}

func (f *slowKeysetRepo) Get(_ context.Context, _ string) (*mmodel.OrganizationKeyset, error) {
	atomic.AddInt32(&f.calls, 1)

	// Signal that fetch has started
	if f.fetchStart != nil {
		select {
		case f.fetchStart <- struct{}{}:
		default:
		}
	}

	// Simulate slow KMS operation
	time.Sleep(f.delay)

	f.mu.Lock()
	defer f.mu.Unlock()

	return f.keyset, f.err
}

func (f *slowKeysetRepo) GetActive(ctx context.Context, organizationID string) (*mmodel.OrganizationKeyset, error) {
	return f.Get(ctx, organizationID)
}

func (f *slowKeysetRepo) GetByVersion(ctx context.Context, organizationID string, _ int) (*mmodel.OrganizationKeyset, error) {
	return f.Get(ctx, organizationID)
}

func (f *slowKeysetRepo) Save(_ context.Context, _ *mmodel.OrganizationKeyset) error {
	return nil
}

func (f *slowKeysetRepo) Update(_ context.Context, _ *mmodel.OrganizationKeyset, _ int64) error {
	return nil
}

func (f *slowKeysetRepo) getCalls() int {
	return int(atomic.LoadInt32(&f.calls))
}

func TestKeysetManager_GetPrimitives_PerOrgMutex_DeduplicatesConcurrentFetches(t *testing.T) {
	t.Parallel()

	aeadBytes, prfBytes := generateTestKeysets(t)

	// Use a slow reader to ensure concurrent requests overlap
	reader := &slowKeysetRepo{
		keyset: &mmodel.OrganizationKeyset{
			OrganizationID:    "org-dedup",
			KEKPath:           "org-org-dedup",
			WrappedKeyset:     "wrapped-aead",
			WrappedHMACKeyset: "wrapped-prf",
		},
		delay:      50 * time.Millisecond,
		fetchStart: make(chan struct{}, 1),
	}

	unwrapper := &fakeKeysetUnwrapper{
		aeadKeyset: aeadBytes,
		prfKeyset:  prfBytes,
	}

	manager := NewKeysetManager(reader, unwrapper, nil, DefaultKeysetManagerConfig(), NewProtectionMetrics(nil))

	const goroutines = 10

	var wg sync.WaitGroup

	wg.Add(goroutines)

	errChan := make(chan error, goroutines)
	resultChan := make(chan *tink.AEADPrimitive, goroutines)

	// Launch all goroutines simultaneously
	start := make(chan struct{})

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()

			// Wait for start signal to ensure concurrent execution
			<-start

			prims7, err := manager.GetActivePrimitives(context.Background(), "org-dedup")
			aead := prims7.AEAD
			prf := prims7.PRF
			if err != nil {
				errChan <- err
				return
			}

			if aead == nil || prf == nil {
				errChan <- errors.New("nil primitives returned")
				return
			}

			resultChan <- aead
		}()
	}

	// Start all goroutines at once
	close(start)

	wg.Wait()
	close(errChan)
	close(resultChan)

	// Check for errors
	for err := range errChan {
		t.Errorf("GetPrimitives() concurrent error = %v", err)
	}

	// Verify per-org mutex behavior: reader should only be called ONCE
	// despite multiple concurrent requests for the same organization
	readerCalls := reader.getCalls()
	if readerCalls != 1 {
		t.Errorf("GetPrimitives() reader calls = %d, want 1 (per-org mutex should deduplicate)", readerCalls)
	}

	// Unwrapper should only be called twice (once for AEAD, once for PRF)
	unwrapperCalls := unwrapper.getCalls()
	if unwrapperCalls != 2 {
		t.Errorf("GetPrimitives() unwrapper calls = %d, want 2", unwrapperCalls)
	}

	// All goroutines should receive the same AEAD primitive instance
	var firstAEAD *tink.AEADPrimitive

	for aead := range resultChan {
		if firstAEAD == nil {
			firstAEAD = aead
		} else if aead != firstAEAD {
			t.Error("GetPrimitives() all concurrent callers should receive the same primitive instance")
		}
	}
}

func TestKeysetManager_GetPrimitives_NilKeyset_ReturnsError(t *testing.T) {
	t.Parallel()

	// Reader returns nil without error (edge case)
	reader := &fakeKeysetRepo{
		keyset: nil,
		err:    nil,
	}

	unwrapper := &fakeKeysetUnwrapper{}

	manager := NewKeysetManager(reader, unwrapper, nil, DefaultKeysetManagerConfig(), NewProtectionMetrics(nil))

	_, err := manager.GetActivePrimitives(context.Background(), "org-nil-keyset")
	if err == nil {
		t.Fatal("GetPrimitives() expected error for nil keyset, got nil")
	}

	if !errors.Is(err, constant.ErrKeysetNotFound) {
		t.Errorf("GetPrimitives() error = %v, want %v", err, constant.ErrKeysetNotFound)
	}

	// Unwrapper should not be called when keyset is nil
	if unwrapper.getCalls() != 0 {
		t.Errorf("GetPrimitives() unwrapper calls = %d, want 0", unwrapper.getCalls())
	}
}

// fakeProvisioningService is a test double for ProvisioningService interface.
type fakeProvisioningService struct {
	mu              sync.Mutex
	provisionCalled int
	provisionErr    error
	provisionResult ProvisionResult
	lastRequest     ProvisionInput
	onProvision     func() // Optional callback after provisioning
	isProvisioned   bool
	isActive        bool
}

func (f *fakeProvisioningService) Provision(_ context.Context, req ProvisionInput) (ProvisionResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.provisionCalled++
	f.lastRequest = req

	if f.provisionErr != nil {
		return ProvisionResult{}, f.provisionErr
	}

	if f.onProvision != nil {
		f.onProvision()
	}

	return f.provisionResult, nil
}

func (f *fakeProvisioningService) GetProvisioningStatus(_ context.Context, _ string) (*mmodel.RegistryStatus, error) {
	return nil, nil
}

func (f *fakeProvisioningService) IsProvisioned(_ context.Context, _ string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.isProvisioned, nil
}

func (f *fakeProvisioningService) IsActive(_ context.Context, _ string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.isActive, nil
}

func (f *fakeProvisioningService) getCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.provisionCalled
}

func (f *fakeProvisioningService) getLastRequest() ProvisionInput {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.lastRequest
}

// fakeKeysetRepoWithProvision allows changing behavior after provisioning.
type fakeKeysetRepoWithProvision struct {
	mu               sync.Mutex
	keyset           *mmodel.OrganizationKeyset
	keysetAfterProv  *mmodel.OrganizationKeyset
	err              error
	errAfterProv     error
	calls            int
	provisioned      bool
	provisionTrigger chan struct{}
}

func (f *fakeKeysetRepoWithProvision) Get(_ context.Context, _ string) (*mmodel.OrganizationKeyset, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.calls++

	if f.provisioned {
		return f.keysetAfterProv, f.errAfterProv
	}

	return f.keyset, f.err
}

func (f *fakeKeysetRepoWithProvision) GetActive(ctx context.Context, organizationID string) (*mmodel.OrganizationKeyset, error) {
	return f.Get(ctx, organizationID)
}

func (f *fakeKeysetRepoWithProvision) GetByVersion(ctx context.Context, organizationID string, _ int) (*mmodel.OrganizationKeyset, error) {
	return f.Get(ctx, organizationID)
}

func (f *fakeKeysetRepoWithProvision) Save(_ context.Context, _ *mmodel.OrganizationKeyset) error {
	return nil
}

func (f *fakeKeysetRepoWithProvision) Update(_ context.Context, _ *mmodel.OrganizationKeyset, _ int64) error {
	return nil
}

func (f *fakeKeysetRepoWithProvision) getCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.calls
}

func (f *fakeKeysetRepoWithProvision) markProvisioned() {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.provisioned = true
}

func TestKeysetManager_GetPrimitives_AutoProvisionOnNotFound(t *testing.T) {
	t.Parallel()

	aeadBytes, prfBytes := generateTestKeysets(t)

	// Reader returns not found initially, then returns keyset after provisioning
	reader := &fakeKeysetRepoWithProvision{
		keyset: nil,
		err:    constant.ErrKeysetNotFound,
		keysetAfterProv: &mmodel.OrganizationKeyset{
			OrganizationID:    "org-auto-prov",
			KEKPath:           "org-org-auto-prov",
			WrappedKeyset:     "wrapped-aead",
			WrappedHMACKeyset: "wrapped-prf",
		},
		errAfterProv: nil,
	}

	unwrapper := &fakeKeysetUnwrapper{
		aeadKeyset: aeadBytes,
		prfKeyset:  prfBytes,
	}

	provisioner := &fakeProvisioningService{
		provisionResult: ProvisionResult{
			OrganizationID:   "org-auto-prov",
			KEKPath:          "org-org-auto-prov",
			AEADPrimaryKeyID: 12345,
			PRFPrimaryKeyID:  67890,
		},
		onProvision: func() {
			reader.markProvisioned()
		},
	}

	manager := NewKeysetManager(reader, unwrapper, provisioner, DefaultKeysetManagerConfig(), NewProtectionMetrics(nil))

	// Context with tenant ID (required for auto-provisioning)
	ctx := tmcore.ContextWithTenantID(context.Background(), "test-tenant")

	prims8, err := manager.GetActivePrimitives(ctx, "org-auto-prov")
	aead := prims8.AEAD
	prf := prims8.PRF
	if err != nil {
		t.Fatalf("GetPrimitives() error = %v", err)
	}

	if aead == nil {
		t.Error("GetPrimitives() AEAD is nil")
	}

	if prf == nil {
		t.Error("GetPrimitives() PRF is nil")
	}

	// Provisioner should be called once
	if provisioner.getCalls() != 1 {
		t.Errorf("GetPrimitives() provisioner calls = %d, want 1", provisioner.getCalls())
	}

	// Reader should be called twice (before and after provisioning)
	if reader.getCalls() != 2 {
		t.Errorf("GetPrimitives() reader calls = %d, want 2", reader.getCalls())
	}
}

func TestKeysetManager_GetPrimitives_NoAutoProvisionWhenKeysetExists(t *testing.T) {
	t.Parallel()

	aeadBytes, prfBytes := generateTestKeysets(t)

	reader := &fakeKeysetRepo{
		keyset: &mmodel.OrganizationKeyset{
			OrganizationID:    "org-existing",
			KEKPath:           "org-org-existing",
			WrappedKeyset:     "wrapped-aead",
			WrappedHMACKeyset: "wrapped-prf",
		},
	}

	unwrapper := &fakeKeysetUnwrapper{
		aeadKeyset: aeadBytes,
		prfKeyset:  prfBytes,
	}

	provisioner := &fakeProvisioningService{}

	manager := NewKeysetManager(reader, unwrapper, provisioner, DefaultKeysetManagerConfig(), NewProtectionMetrics(nil))

	prims9, err := manager.GetActivePrimitives(context.Background(), "org-existing")
	aead := prims9.AEAD
	prf := prims9.PRF
	if err != nil {
		t.Fatalf("GetPrimitives() error = %v", err)
	}

	if aead == nil {
		t.Error("GetPrimitives() AEAD is nil")
	}

	if prf == nil {
		t.Error("GetPrimitives() PRF is nil")
	}

	// Provisioner should NOT be called when keyset exists
	if provisioner.getCalls() != 0 {
		t.Errorf("GetPrimitives() provisioner calls = %d, want 0", provisioner.getCalls())
	}
}

func TestKeysetManager_GetPrimitives_AutoProvisionFails(t *testing.T) {
	t.Parallel()

	reader := &fakeKeysetRepo{
		keyset: nil,
		err:    constant.ErrKeysetNotFound,
	}

	unwrapper := &fakeKeysetUnwrapper{}

	provisionErr := errors.New("KMS provisioning failed")
	provisioner := &fakeProvisioningService{
		provisionErr: provisionErr,
	}

	manager := NewKeysetManager(reader, unwrapper, provisioner, DefaultKeysetManagerConfig(), NewProtectionMetrics(nil))

	// Context with tenant ID (required for auto-provisioning)
	ctx := tmcore.ContextWithTenantID(context.Background(), "test-tenant")

	_, err := manager.GetActivePrimitives(ctx, "org-prov-fail")
	if err == nil {
		t.Fatal("GetPrimitives() expected error, got nil")
	}

	// Should contain the provision error
	if !errors.Is(err, provisionErr) {
		// Check if error message contains the provision error
		if err.Error() == "" || !contains(err.Error(), "auto-provision failed") {
			t.Errorf("GetPrimitives() error = %v, want error containing 'auto-provision failed'", err)
		}
	}

	// Provisioner should be called once
	if provisioner.getCalls() != 1 {
		t.Errorf("GetPrimitives() provisioner calls = %d, want 1", provisioner.getCalls())
	}
}

func TestKeysetManager_GetPrimitives_NoProvisionerConfigured(t *testing.T) {
	t.Parallel()

	reader := &fakeKeysetRepo{
		keyset: nil,
		err:    constant.ErrKeysetNotFound,
	}

	unwrapper := &fakeKeysetUnwrapper{}

	// No provisioner configured (nil)
	manager := NewKeysetManager(reader, unwrapper, nil, DefaultKeysetManagerConfig(), NewProtectionMetrics(nil))

	_, err := manager.GetActivePrimitives(context.Background(), "org-no-prov")
	if err == nil {
		t.Fatal("GetPrimitives() expected error, got nil")
	}

	// Should return the original not found error
	if !errors.Is(err, constant.ErrKeysetNotFound) {
		t.Errorf("GetPrimitives() error = %v, want %v", err, constant.ErrKeysetNotFound)
	}
}

func TestKeysetManager_GetPrimitives_NilKeysetAfterProvision(t *testing.T) {
	t.Parallel()

	// Reader always returns nil keyset even after provisioning
	reader := &fakeKeysetRepo{
		keyset: nil,
		err:    constant.ErrKeysetNotFound,
	}

	unwrapper := &fakeKeysetUnwrapper{}

	provisioner := &fakeProvisioningService{
		provisionResult: ProvisionResult{
			OrganizationID: "org-nil-after",
		},
	}

	manager := NewKeysetManager(reader, unwrapper, provisioner, DefaultKeysetManagerConfig(), NewProtectionMetrics(nil))

	// Context with tenant ID (required for auto-provisioning)
	ctx := tmcore.ContextWithTenantID(context.Background(), "test-tenant")

	_, err := manager.GetActivePrimitives(ctx, "org-nil-after")
	if err == nil {
		t.Fatal("GetPrimitives() expected error for nil keyset after provisioning, got nil")
	}
}

func TestKeysetManager_autoProvision_UsesTenantFromContext(t *testing.T) {
	t.Parallel()

	aeadBytes, prfBytes := generateTestKeysets(t)

	// Reader returns not found initially, then returns keyset after provisioning
	reader := &fakeKeysetRepoWithProvision{
		keyset: nil,
		err:    constant.ErrKeysetNotFound,
		keysetAfterProv: &mmodel.OrganizationKeyset{
			OrganizationID:    "org-tenant-ctx",
			KEKPath:           "org-org-tenant-ctx",
			WrappedKeyset:     "wrapped-aead",
			WrappedHMACKeyset: "wrapped-prf",
		},
		errAfterProv: nil,
	}

	unwrapper := &fakeKeysetUnwrapper{
		aeadKeyset: aeadBytes,
		prfKeyset:  prfBytes,
	}

	provisioner := &fakeProvisioningService{
		provisionResult: ProvisionResult{
			OrganizationID: "org-tenant-ctx",
		},
		onProvision: func() {
			reader.markProvisioned()
		},
	}

	manager := NewKeysetManager(reader, unwrapper, provisioner, DefaultKeysetManagerConfig(), NewProtectionMetrics(nil))

	// Create context with tenant ID using tmcore
	ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-from-context")

	_, err := manager.GetActivePrimitives(ctx, "org-tenant-ctx")
	if err != nil {
		t.Fatalf("GetPrimitives() error = %v", err)
	}

	// Verify the provisioner was called with tenant ID from context
	lastReq := provisioner.getLastRequest()
	if lastReq.TenantID != "tenant-from-context" {
		t.Errorf("autoProvision() tenant ID = %q, want %q", lastReq.TenantID, "tenant-from-context")
	}

	if lastReq.OrganizationID != "org-tenant-ctx" {
		t.Errorf("autoProvision() organization ID = %q, want %q", lastReq.OrganizationID, "org-tenant-ctx")
	}

	if lastReq.Actor != "system:auto-provision" {
		t.Errorf("autoProvision() actor = %q, want %q", lastReq.Actor, "system:auto-provision")
	}

	if lastReq.Reason != "Lazy migration: imported legacy key material on first encrypted field access" {
		t.Errorf("autoProvision() reason = %q, want %q", lastReq.Reason, "Lazy migration: imported legacy key material on first encrypted field access")
	}
}

func TestKeysetManager_autoProvision_RejectsReservedTenantID(t *testing.T) {
	t.Parallel()

	// Multi-tenant mode: the tenant middleware populated a real, non-empty tenant
	// id literally equal to "default" from the JWT. Resolving it to the flat-base
	// mount would break KEK isolation, so auto-provisioning MUST be refused. The
	// keyset is missing, so without the guard this would auto-provision under the
	// flat base.
	reader := &fakeKeysetRepo{
		keyset: nil,
		err:    constant.ErrKeysetNotFound,
	}

	unwrapper := &fakeKeysetUnwrapper{}

	provisioner := &fakeProvisioningService{}

	manager := NewKeysetManager(reader, unwrapper, provisioner, DefaultKeysetManagerConfig(), NewProtectionMetrics(nil))

	// Non-empty tenant id of "default" supplied via context (multi-tenant).
	ctx := tmcore.ContextWithTenantID(context.Background(), "default")

	_, err := manager.GetActivePrimitives(ctx, "org-reserved")
	if err == nil {
		t.Fatal("GetPrimitives() expected error for reserved tenant id, got nil")
	}

	// The reserved-tenant rejection surfaces as the ErrReservedTenantID business
	// error (code ENC-0013), %w-wrapped through fetchAndCache. The typed
	// UnprocessableOperationError has no Unwrap, so assert on its code string,
	// matching the handler test which checks the JSON "code" field.
	if !strings.Contains(err.Error(), constant.ErrReservedTenantID.Error()) {
		t.Errorf("GetPrimitives() error = %v, want error carrying code %q", err, constant.ErrReservedTenantID.Error())
	}

	// The provisioner MUST NOT be called: rejection happens before provisioning
	// (fail-closed, no keyset written under the flat base).
	if provisioner.getCalls() != 0 {
		t.Errorf("GetPrimitives() provisioner calls = %d, want 0 (no provisioning under flat base)", provisioner.getCalls())
	}

	// The unwrapper MUST NOT be called either (no keyset to unwrap).
	if unwrapper.getCalls() != 0 {
		t.Errorf("GetPrimitives() unwrapper calls = %d, want 0", unwrapper.getCalls())
	}
}

func TestKeysetManager_autoProvision_DefaultsTenantWhenMissing(t *testing.T) {
	t.Parallel()

	// Setup keyset that will be returned after provisioning
	aeadBytes, prfBytes := generateTestKeysets(t)

	provisionedKeyset := &mmodel.OrganizationKeyset{
		TenantID:          "default",
		OrganizationID:    "org-no-tenant",
		KEKPath:           "transit/keys/crm/org-no-tenant",
		WrappedKeyset:     "wrapped-aead",
		WrappedHMACKeyset: "wrapped-prf",
		KeysetInfo:        mmodel.KeysetInfo{PrimaryKeyID: 111},
		HMACKeysetInfo:    mmodel.KeysetInfo{PrimaryKeyID: 222},
	}

	// Reader returns not found first, then returns keyset after provisioning
	reader := &fakeKeysetRepo{
		keyset: nil,
		err:    constant.ErrKeysetNotFound,
	}

	unwrapper := &fakeKeysetUnwrapper{
		aeadKeyset: aeadBytes,
		prfKeyset:  prfBytes,
	}

	// Provisioner that simulates successful provisioning by updating the reader
	provisioner := &fakeProvisioningService{
		onProvision: func() {
			reader.mu.Lock()
			reader.keyset = provisionedKeyset
			reader.err = nil
			reader.mu.Unlock()
		},
	}

	manager := NewKeysetManager(reader, unwrapper, provisioner, DefaultKeysetManagerConfig(), NewProtectionMetrics(nil))

	// Context WITHOUT tenant ID - should default to "default"
	ctx := context.Background()

	_, err := manager.GetActivePrimitives(ctx, "org-no-tenant")
	if err != nil {
		t.Fatalf("GetPrimitives() unexpected error: %v", err)
	}

	// Verify provisioner was called with "default" tenant
	lastReq := provisioner.getLastRequest()
	if lastReq.TenantID != "default" {
		t.Errorf("autoProvision() tenant ID = %q, want %q", lastReq.TenantID, "default")
	}
}

// contains checks if a string contains a substring.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestKeysetManager_GetPrimitives_TenantIsolation_CacheKeysScopedByTenant verifies that
// the cache uses tenant-scoped keys to prevent cross-tenant cache collisions.
// Two different tenants with the same organization ID should get different cache entries.
func TestKeysetManager_GetPrimitives_TenantIsolation_CacheKeysScopedByTenant(t *testing.T) {
	t.Parallel()

	aeadBytes, prfBytes := generateTestKeysets(t)

	// Track which organization IDs were requested
	var readerMu sync.Mutex
	readerCalls := make(map[string]int)

	reader := &fakeKeysetRepo{
		keyset: &mmodel.OrganizationKeyset{
			OrganizationID:    "same-org-id",
			KEKPath:           "org-same-org-id",
			WrappedKeyset:     "wrapped-aead",
			WrappedHMACKeyset: "wrapped-prf",
		},
	}

	// Wrap the reader to track calls per context
	unwrapper := &fakeKeysetUnwrapper{
		aeadKeyset: aeadBytes,
		prfKeyset:  prfBytes,
	}

	manager := NewKeysetManager(reader, unwrapper, nil, DefaultKeysetManagerConfig(), NewProtectionMetrics(nil))

	// Create contexts for two different tenants
	ctxTenantA := tmcore.ContextWithTenantID(context.Background(), "tenant-alpha")
	ctxTenantB := tmcore.ContextWithTenantID(context.Background(), "tenant-beta")

	// First call from tenant A - should fetch from repo
	prims10, err := manager.GetActivePrimitives(ctxTenantA, "same-org-id")
	aeadA := prims10.AEAD
	prfA := prims10.PRF
	if err != nil {
		t.Fatalf("GetPrimitives(tenant-alpha) error = %v", err)
	}

	readerMu.Lock()
	readerCalls["tenant-alpha"] = reader.getCalls()
	readerMu.Unlock()

	// First call from tenant B - should fetch from repo (different cache key)
	prims11, err := manager.GetActivePrimitives(ctxTenantB, "same-org-id")
	aeadB := prims11.AEAD
	prfB := prims11.PRF
	if err != nil {
		t.Fatalf("GetPrimitives(tenant-beta) error = %v", err)
	}

	// Reader should have been called twice (once per tenant)
	if reader.getCalls() != 2 {
		t.Errorf("GetPrimitives() reader calls = %d, want 2 (one per tenant)", reader.getCalls())
	}

	// Second call from tenant A - should use cache
	prims12, err := manager.GetActivePrimitives(ctxTenantA, "same-org-id")
	aeadA2 := prims12.AEAD
	prfA2 := prims12.PRF
	if err != nil {
		t.Fatalf("GetPrimitives(tenant-alpha, second call) error = %v", err)
	}

	// Reader should still only have been called twice
	if reader.getCalls() != 2 {
		t.Errorf("GetPrimitives() reader calls = %d, want 2 (tenant A should use cache)", reader.getCalls())
	}

	// Tenant A should get same cached primitives
	if aeadA != aeadA2 || prfA != prfA2 {
		t.Error("GetPrimitives() tenant A should get same cached primitives on second call")
	}

	// Primitives for different tenants should be different instances
	// (even though they have same content, they're cached separately)
	if aeadA == aeadB {
		t.Error("GetPrimitives() different tenants should have separate cache entries")
	}

	if prfA == prfB {
		t.Error("GetPrimitives() different tenants should have separate PRF cache entries")
	}
}

// TestKeysetManager_InvalidateCacheForTenant_OnlyAffectsSpecificTenant verifies that
// cache invalidation for one tenant does not affect other tenants.
func TestKeysetManager_InvalidateCacheForTenant_OnlyAffectsSpecificTenant(t *testing.T) {
	t.Parallel()

	aeadBytes, prfBytes := generateTestKeysets(t)

	reader := &fakeKeysetRepo{
		keyset: &mmodel.OrganizationKeyset{
			OrganizationID:    "shared-org",
			KEKPath:           "org-shared-org",
			WrappedKeyset:     "wrapped-aead",
			WrappedHMACKeyset: "wrapped-prf",
		},
	}

	unwrapper := &fakeKeysetUnwrapper{
		aeadKeyset: aeadBytes,
		prfKeyset:  prfBytes,
	}

	manager := NewKeysetManager(reader, unwrapper, nil, DefaultKeysetManagerConfig(), NewProtectionMetrics(nil))

	ctxTenantA := tmcore.ContextWithTenantID(context.Background(), "tenant-one")
	ctxTenantB := tmcore.ContextWithTenantID(context.Background(), "tenant-two")

	// Populate cache for both tenants
	prims13, err := manager.GetActivePrimitives(ctxTenantA, "shared-org")
	aeadA1 := prims13.AEAD
	if err != nil {
		t.Fatalf("GetPrimitives(tenant-one) error = %v", err)
	}

	prims14, err := manager.GetActivePrimitives(ctxTenantB, "shared-org")
	aeadB1 := prims14.AEAD
	if err != nil {
		t.Fatalf("GetPrimitives(tenant-two) error = %v", err)
	}

	// Verify reader was called twice
	if reader.getCalls() != 2 {
		t.Errorf("setup: reader calls = %d, want 2", reader.getCalls())
	}

	// Invalidate cache for tenant-one only
	manager.InvalidateCacheForTenant("tenant-one", "shared-org")

	// Tenant A should refetch (cache was invalidated)
	prims15, err := manager.GetActivePrimitives(ctxTenantA, "shared-org")
	aeadA2 := prims15.AEAD
	if err != nil {
		t.Fatalf("GetPrimitives(tenant-one, after invalidation) error = %v", err)
	}

	// Reader should have been called again for tenant-one
	if reader.getCalls() != 3 {
		t.Errorf("GetPrimitives() reader calls = %d, want 3 (tenant-one should refetch)", reader.getCalls())
	}

	// Tenant A should get new primitives (different instance)
	if aeadA1 == aeadA2 {
		t.Error("GetPrimitives() tenant-one should get new primitives after invalidation")
	}

	// Tenant B should still use cache (not invalidated)
	prims16, err := manager.GetActivePrimitives(ctxTenantB, "shared-org")
	aeadB2 := prims16.AEAD
	if err != nil {
		t.Fatalf("GetPrimitives(tenant-two) error = %v", err)
	}

	// Reader should NOT have been called again for tenant-two
	if reader.getCalls() != 3 {
		t.Errorf("GetPrimitives() reader calls = %d, want 3 (tenant-two should use cache)", reader.getCalls())
	}

	// Tenant B should get same cached primitives
	if aeadB1 != aeadB2 {
		t.Error("GetPrimitives() tenant-two should still get same cached primitives")
	}
}

// TestKeysetManager_GetPrimitives_AutoProvision_RegistryProperties verifies that when
// auto-provisioning occurs, the provisioned registry has the expected properties:
// - Status: active
// - ProtectionModel: envelope
// - LegacyReadable: true
func TestKeysetManager_GetPrimitives_AutoProvision_RegistryProperties(t *testing.T) {
	t.Parallel()

	aeadBytes, prfBytes := generateTestKeysets(t)

	// Reader returns not found initially, then returns keyset after provisioning
	reader := &fakeKeysetRepoWithProvision{
		keyset: nil,
		err:    constant.ErrKeysetNotFound,
		keysetAfterProv: &mmodel.OrganizationKeyset{
			OrganizationID:    "org-registry-test",
			KEKPath:           "org-org-registry-test",
			WrappedKeyset:     "wrapped-aead",
			WrappedHMACKeyset: "wrapped-prf",
		},
		errAfterProv: nil,
	}

	unwrapper := &fakeKeysetUnwrapper{
		aeadKeyset: aeadBytes,
		prfKeyset:  prfBytes,
	}

	// Provisioner that returns result with expected registry properties
	provisioner := &fakeProvisioningService{
		provisionResult: ProvisionResult{
			OrganizationID:   "org-registry-test",
			KEKPath:          "org-org-registry-test",
			AEADPrimaryKeyID: 12345,
			PRFPrimaryKeyID:  67890,
			RegistryStatus:   mmodel.RegistryStatusActive,
		},
		onProvision: func() {
			reader.markProvisioned()
		},
	}

	manager := NewKeysetManager(reader, unwrapper, provisioner, DefaultKeysetManagerConfig(), NewProtectionMetrics(nil))

	// Context with tenant ID (required for auto-provisioning)
	ctx := tmcore.ContextWithTenantID(context.Background(), "test-tenant")

	prims17, err := manager.GetActivePrimitives(ctx, "org-registry-test")
	aead := prims17.AEAD
	prf := prims17.PRF
	if err != nil {
		t.Fatalf("GetPrimitives() error = %v", err)
	}

	if aead == nil {
		t.Error("GetPrimitives() AEAD is nil")
	}

	if prf == nil {
		t.Error("GetPrimitives() PRF is nil")
	}

	// Verify provisioner was called
	if provisioner.getCalls() != 1 {
		t.Errorf("GetPrimitives() provisioner calls = %d, want 1", provisioner.getCalls())
	}

	// Verify provisioner received correct request
	lastReq := provisioner.getLastRequest()
	if lastReq.OrganizationID != "org-registry-test" {
		t.Errorf("Provision request OrganizationID = %q, want %q", lastReq.OrganizationID, "org-registry-test")
	}

	if lastReq.TenantID != "test-tenant" {
		t.Errorf("Provision request TenantID = %q, want %q", lastReq.TenantID, "test-tenant")
	}

	// The provisioner result should have active registry status
	// (actual registry persistence is verified in provisioning_test.go)
	if provisioner.provisionResult.RegistryStatus != mmodel.RegistryStatusActive {
		t.Errorf("ProvisionResult RegistryStatus = %v, want %v",
			provisioner.provisionResult.RegistryStatus, mmodel.RegistryStatusActive)
	}
}

// TestBuildCacheKey verifies the cache key format.
func TestBuildCacheKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		tenantID       string
		organizationID string
		want           string
	}{
		{
			name:           "standard format",
			tenantID:       "tenant-123",
			organizationID: "org-456",
			want:           "tenant-123:org-456",
		},
		{
			name:           "default tenant",
			tenantID:       "default",
			organizationID: "org-789",
			want:           "default:org-789",
		},
		{
			name:           "empty tenant",
			tenantID:       "",
			organizationID: "org-abc",
			want:           ":org-abc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := buildCacheKey(tt.tenantID, tt.organizationID)
			if got != tt.want {
				t.Errorf("buildCacheKey(%q, %q) = %q, want %q", tt.tenantID, tt.organizationID, got, tt.want)
			}
		})
	}
}

// TestCachedPrimitives_MultiKeyPRF verifies that CachedPrimitives includes the MultiKeyPRF field.
func TestCachedPrimitives_MultiKeyPRF(t *testing.T) {
	t.Parallel()

	t.Run("struct has MultiKeyPRF field", func(t *testing.T) {
		t.Parallel()

		aeadBytes, prfBytes := generateTestKeysets(t)

		// Parse the PRF keyset to get a handle for creating the single and multi PRF primitives.
		prfHandle, err := tink.DeserializePRFKeyset(prfBytes)
		if err != nil {
			t.Fatalf("failed to deserialize PRF keyset: %v", err)
		}

		multiKeyPRF, err := tink.NewPRFMultiPrimitive(prfHandle)
		if err != nil {
			t.Fatalf("failed to create PRFMultiPrimitive: %v", err)
		}

		// Parse AEAD and PRF for the other fields.
		aeadPrimitive, err := tink.ParseAEADKeyset(aeadBytes)
		if err != nil {
			t.Fatalf("failed to parse AEAD keyset: %v", err)
		}

		prfPrimitive, err := tink.NewPRFPrimitive(prfHandle)
		if err != nil {
			t.Fatalf("failed to create PRF primitive: %v", err)
		}

		// Create CachedPrimitives with all fields including MultiKeyPRF.
		cached := &CachedPrimitives{
			AEAD:         aeadPrimitive,
			PRF:          prfPrimitive,
			MultiKeyPRF:  multiKeyPRF,
			PrimaryKeyID: 12345,
			ExpiresAt:    time.Now().Add(5 * time.Minute),
		}

		// Verify all fields are set correctly.
		if cached.AEAD == nil {
			t.Error("CachedPrimitives.AEAD is nil")
		}
		if cached.PRF == nil {
			t.Error("CachedPrimitives.PRF is nil")
		}
		if cached.MultiKeyPRF == nil {
			t.Error("CachedPrimitives.MultiKeyPRF is nil")
		}
		if cached.PrimaryKeyID != 12345 {
			t.Errorf("CachedPrimitives.PrimaryKeyID = %d, want 12345", cached.PrimaryKeyID)
		}
	})
}

// TestKeysetManager_GetPrimitives_PopulatesMultiKeyPRF verifies that fetchAndCache
// populates the MultiKeyPRF field in CachedPrimitives.
func TestKeysetManager_GetPrimitives_PopulatesMultiKeyPRF(t *testing.T) {
	t.Parallel()

	aeadBytes, prfBytes := generateTestKeysets(t)

	reader := &fakeKeysetRepo{
		keyset: &mmodel.OrganizationKeyset{
			OrganizationID:    "org-multikey",
			Version:           1,
			KEKPath:           "org-org-multikey",
			WrappedKeyset:     "wrapped-aead",
			WrappedHMACKeyset: "wrapped-prf",
			KeysetInfo:        mmodel.KeysetInfo{PrimaryKeyID: 111},
		},
	}

	unwrapper := &fakeKeysetUnwrapper{
		aeadKeyset: aeadBytes,
		prfKeyset:  prfBytes,
	}

	manager := NewKeysetManager(reader, unwrapper, nil, DefaultKeysetManagerConfig(), NewProtectionMetrics(nil))

	// Call GetPrimitives to trigger fetchAndCache
	prims18, err := manager.GetActivePrimitives(context.Background(), "org-multikey")
	aead := prims18.AEAD
	prf := prims18.PRF
	if err != nil {
		t.Fatalf("GetPrimitives() error = %v", err)
	}

	if aead == nil {
		t.Error("GetPrimitives() AEAD is nil")
	}
	if prf == nil {
		t.Error("GetPrimitives() PRF is nil")
	}

	// Verify MultiKeyPRF is populated in the cache. The active path caches under the
	// non-versioned key (the active keyset is looked up by tenant+org, not version).
	manager.mu.RLock()
	cacheKey := buildCacheKey("default", "org-multikey")
	cached, ok := manager.cache[cacheKey]
	manager.mu.RUnlock()

	if !ok {
		t.Fatal("cache entry not found after GetPrimitives")
	}

	if cached.MultiKeyPRF == nil {
		t.Error("CachedPrimitives.MultiKeyPRF should be populated after fetchAndCache")
	}

	// Verify MultiKeyPRF works correctly
	tokens, err := cached.MultiKeyPRF.ComputeSearchTokenCandidates([]byte("test@example.com"))
	if err != nil {
		t.Fatalf("MultiKeyPRF.ComputeSearchTokenCandidates() error = %v", err)
	}
	if len(tokens) == 0 {
		t.Error("MultiKeyPRF.ComputeSearchTokenCandidates() returned empty tokens")
	}
}

// TestKeysetManager_fetchAndCache_MultiKeyPRF_StrictErrorMode verifies that when
// the PRF keyset cannot be deserialized the entire fetchAndCache operation fails
// (strict error mode - no nil fallback). A completely invalid PRF keyset fails to
// deserialize before any PRF primitive is built, and the error propagates.
func TestKeysetManager_fetchAndCache_MultiKeyPRF_StrictErrorMode(t *testing.T) {
	t.Parallel()

	reader := &fakeKeysetRepo{
		keyset: &mmodel.OrganizationKeyset{
			OrganizationID:    "org-strict-error",
			KEKPath:           "org-org-strict-error",
			WrappedKeyset:     "wrapped-aead",
			WrappedHMACKeyset: "wrapped-prf",
			KeysetInfo:        mmodel.KeysetInfo{PrimaryKeyID: 111},
		},
	}

	// Generate valid AEAD keyset but invalid PRF keyset
	_, aeadBytes, err := tink.NewAEADKeysetGenerator().Generate()
	if err != nil {
		t.Fatalf("failed to generate AEAD keyset: %v", err)
	}

	unwrapper := &fakeKeysetUnwrapper{
		aeadKeyset: aeadBytes,
		prfKeyset:  []byte("invalid-prf-keyset-data"),
	}

	manager := NewKeysetManager(reader, unwrapper, nil, DefaultKeysetManagerConfig(), NewProtectionMetrics(nil))

	// GetPrimitives should fail due to invalid PRF keyset
	_, err = manager.GetActivePrimitives(context.Background(), "org-strict-error")
	if err == nil {
		t.Fatal("GetPrimitives() expected error for invalid PRF keyset, got nil")
	}

	// Verify no partial cache entry was created
	manager.mu.RLock()
	cacheKey := buildCacheKey("default", "org-strict-error")
	_, ok := manager.cache[cacheKey]
	manager.mu.RUnlock()

	if ok {
		t.Error("cache entry should NOT exist after fetchAndCache failure")
	}
}

// TestKeysetManager_GetPrimitives_UnwrapMount_DefaultTenant_Flat verifies that a stored
// keyset with TenantID "default" resolves to the flat base mount ("transit") for both the
// AEAD and PRF unwrap calls. The mount is derived from the STORED keyset.TenantID, not ctx.
func TestKeysetManager_GetPrimitives_UnwrapMount_DefaultTenant_Flat(t *testing.T) {
	t.Parallel()

	aeadBytes, prfBytes := generateTestKeysets(t)

	reader := &fakeKeysetRepo{
		keyset: &mmodel.OrganizationKeyset{
			TenantID:          "default",
			OrganizationID:    "org-flat",
			KEKPath:           "crm/org-flat",
			WrappedKeyset:     "wrapped-aead",
			WrappedHMACKeyset: "wrapped-prf",
		},
	}

	unwrapper := &fakeKeysetUnwrapper{
		aeadKeyset: aeadBytes,
		prfKeyset:  prfBytes,
	}

	config := KeysetManagerConfig{BaseMountPath: "transit"}
	manager := NewKeysetManager(reader, unwrapper, nil, config, NewProtectionMetrics(nil))

	_, err := manager.GetActivePrimitives(context.Background(), "org-flat")
	if err != nil {
		t.Fatalf("GetPrimitives() error = %v", err)
	}

	mounts := unwrapper.getMountPaths()
	if len(mounts) != 2 {
		t.Fatalf("unwrapper mountPaths len = %d, want 2", len(mounts))
	}

	for i, got := range mounts {
		if got != "transit" {
			t.Errorf("unwrap call %d mountPath = %q, want %q", i, got, "transit")
		}
	}
}

// TestKeysetManager_GetPrimitives_UnwrapMount_NonDefaultTenant_SharedEngine verifies
// that a stored keyset with no KEKMountPath and a non-default TenantID resolves to the
// shared engine "transit" verbatim for both AEAD and PRF unwrap calls (tenant scope is
// in the key name, not the mount).
func TestKeysetManager_GetPrimitives_UnwrapMount_NonDefaultTenant_SharedEngine(t *testing.T) {
	t.Parallel()

	aeadBytes, prfBytes := generateTestKeysets(t)

	const tenant = "11111111-2222-3333-4444-555555555555"

	reader := &fakeKeysetRepo{
		keyset: &mmodel.OrganizationKeyset{
			TenantID:          tenant,
			OrganizationID:    "org-sub",
			KEKPath:           "crm/org-sub",
			WrappedKeyset:     "wrapped-aead",
			WrappedHMACKeyset: "wrapped-prf",
		},
	}

	unwrapper := &fakeKeysetUnwrapper{
		aeadKeyset: aeadBytes,
		prfKeyset:  prfBytes,
	}

	config := KeysetManagerConfig{BaseMountPath: "transit", MultiTenant: true}
	manager := NewKeysetManager(reader, unwrapper, nil, config, NewProtectionMetrics(nil))

	// ctx carries a DIFFERENT tenant to prove the mount comes from the stored keyset, not ctx.
	ctx := tmcore.ContextWithTenantID(context.Background(), "ctx-tenant-should-be-ignored")

	_, err := manager.GetActivePrimitives(ctx, "org-sub")
	if err != nil {
		t.Fatalf("GetPrimitives() error = %v", err)
	}

	want := "transit"

	mounts := unwrapper.getMountPaths()
	if len(mounts) != 2 {
		t.Fatalf("unwrapper mountPaths len = %d, want 2", len(mounts))
	}

	for i, got := range mounts {
		if got != want {
			t.Errorf("unwrap call %d mountPath = %q, want %q", i, got, want)
		}
	}
}

// TestKeysetManager_GetPrimitives_MountNotFound_FailsClosed verifies that when the
// unwrapper returns vault.ErrMountNotFound, GetPrimitives propagates an error that is
// errors.Is(vault.ErrMountNotFound) — fail-closed, no fallback to the base mount.
func TestKeysetManager_GetPrimitives_MountNotFound_FailsClosed(t *testing.T) {
	t.Parallel()

	reader := &fakeKeysetRepo{
		keyset: &mmodel.OrganizationKeyset{
			TenantID:          "11111111-2222-3333-4444-555555555555",
			OrganizationID:    "org-no-mount",
			KEKPath:           "crm/org-no-mount",
			WrappedKeyset:     "wrapped-aead",
			WrappedHMACKeyset: "wrapped-prf",
		},
	}

	unwrapper := &fakeKeysetUnwrapper{
		err: vault.ErrMountNotFound,
	}

	config := KeysetManagerConfig{BaseMountPath: "transit"}
	manager := NewKeysetManager(reader, unwrapper, nil, config, NewProtectionMetrics(nil))

	_, err := manager.GetActivePrimitives(context.Background(), "org-no-mount")
	if err == nil {
		t.Fatal("GetPrimitives() expected error, got nil")
	}

	if !errors.Is(err, vault.ErrMountNotFound) {
		t.Errorf("GetPrimitives() error = %v, want errors.Is(vault.ErrMountNotFound)", err)
	}
}

// TestKeysetManager_GetPrimitives_StoredMountWins verifies that when the fetched
// keyset carries a stored KEKMountPath, unwrap uses that mount for BOTH AEAD and
// PRF, even when the manager's BaseMountPath would derive a different value.
func TestKeysetManager_GetPrimitives_StoredMountWins(t *testing.T) {
	t.Parallel()

	aeadBytes, prfBytes := generateTestKeysets(t)

	const storedMount = "transit/tenant-x"

	reader := &fakeKeysetRepo{
		keyset: &mmodel.OrganizationKeyset{
			TenantID:          "tenant-x",
			OrganizationID:    "org-stored",
			KEKPath:           "crm/org-stored",
			KEKMountPath:      storedMount,
			WrappedKeyset:     "wrapped-aead",
			WrappedHMACKeyset: "wrapped-prf",
		},
	}

	unwrapper := &fakeKeysetUnwrapper{
		aeadKeyset: aeadBytes,
		prfKeyset:  prfBytes,
	}

	// BaseMountPath deliberately differs from the stored mount's base.
	config := KeysetManagerConfig{BaseMountPath: "other-base"}
	manager := NewKeysetManager(reader, unwrapper, nil, config, NewProtectionMetrics(nil))

	_, err := manager.GetActivePrimitives(context.Background(), "org-stored")
	if err != nil {
		t.Fatalf("GetPrimitives() error = %v", err)
	}

	mounts := unwrapper.getMountPaths()
	if len(mounts) != 2 {
		t.Fatalf("unwrapper mountPaths len = %d, want 2", len(mounts))
	}

	for i, got := range mounts {
		if got != storedMount {
			t.Errorf("unwrap call %d mountPath = %q, want stored %q", i, got, storedMount)
		}
	}
}

// TestKeysetManager_GetPrimitives_ConfigBaseChanged_UsesStoredMount is the #3
// regression: a keyset provisioned under "transit" must still unwrap under
// "transit" even after the configured BaseMountPath is changed to a new value.
// Before the read-side fix, unwrap derived the mount from the live config and
// would have targeted the CHANGED base, stranding existing keysets.
func TestKeysetManager_GetPrimitives_ConfigBaseChanged_UsesStoredMount(t *testing.T) {
	t.Parallel()

	aeadBytes, prfBytes := generateTestKeysets(t)

	const provisionedMount = "transit"

	reader := &fakeKeysetRepo{
		keyset: &mmodel.OrganizationKeyset{
			TenantID:          "default",
			OrganizationID:    "org-config-changed",
			KEKPath:           "crm/org-config-changed",
			KEKMountPath:      provisionedMount,
			WrappedKeyset:     "wrapped-aead",
			WrappedHMACKeyset: "wrapped-prf",
		},
	}

	unwrapper := &fakeKeysetUnwrapper{
		aeadKeyset: aeadBytes,
		prfKeyset:  prfBytes,
	}

	// Live config base changed after provisioning.
	config := KeysetManagerConfig{BaseMountPath: "CHANGED-BASE"}
	manager := NewKeysetManager(reader, unwrapper, nil, config, NewProtectionMetrics(nil))

	_, err := manager.GetActivePrimitives(context.Background(), "org-config-changed")
	if err != nil {
		t.Fatalf("GetPrimitives() error = %v", err)
	}

	mounts := unwrapper.getMountPaths()
	if len(mounts) != 2 {
		t.Fatalf("unwrapper mountPaths len = %d, want 2", len(mounts))
	}

	for i, got := range mounts {
		if got != provisionedMount {
			t.Errorf("unwrap call %d mountPath = %q, want stored %q (NOT derived from CHANGED-BASE)", i, got, provisionedMount)
		}
	}
}

// TestKeysetManager_GetPrimitives_LegacyEmptyMount_FallsBackToDerived verifies the
// back-compat path: a keyset with no stored KEKMountPath derives the mount from the
// manager BaseMountPath. The mount is the shared engine in both modes; "default"/""
// tenant fails closed in multi-tenant mode (tenant scope is in the key name).
func TestKeysetManager_GetPrimitives_LegacyEmptyMount_FallsBackToDerived(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		tenantID    string
		multiTenant bool
		want        string
		wantErr     bool
	}{
		// Single-tenant: the legacy fallback derives the shared engine.
		{name: "single-tenant default tenant -> shared engine", tenantID: "default", multiTenant: false, want: "transit"},
		{name: "single-tenant real tenant -> shared engine", tenantID: "t1", multiTenant: false, want: "transit"},
		// Multi-tenant: a real tenant derives the shared engine verbatim; "default" fails closed.
		{name: "multi-tenant real tenant -> shared engine", tenantID: "t1", multiTenant: true, want: "transit"},
		{name: "multi-tenant default tenant -> fail closed", tenantID: "default", multiTenant: true, wantErr: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			aeadBytes, prfBytes := generateTestKeysets(t)

			reader := &fakeKeysetRepo{
				keyset: &mmodel.OrganizationKeyset{
					TenantID:          tt.tenantID,
					OrganizationID:    "org-legacy",
					KEKPath:           "crm/org-legacy",
					KEKMountPath:      "", // legacy record predating the stored mount
					WrappedKeyset:     "wrapped-aead",
					WrappedHMACKeyset: "wrapped-prf",
				},
			}

			unwrapper := &fakeKeysetUnwrapper{
				aeadKeyset: aeadBytes,
				prfKeyset:  prfBytes,
			}

			config := KeysetManagerConfig{BaseMountPath: "transit", MultiTenant: tt.multiTenant}
			manager := NewKeysetManager(reader, unwrapper, nil, config, NewProtectionMetrics(nil))

			_, err := manager.GetActivePrimitives(context.Background(), "org-legacy")
			if tt.wantErr {
				if err == nil {
					t.Fatalf("GetPrimitives() expected error for multi-tenant default tenant, got nil")
				}

				return
			}

			if err != nil {
				t.Fatalf("GetPrimitives() error = %v", err)
			}

			mounts := unwrapper.getMountPaths()
			if len(mounts) != 2 {
				t.Fatalf("unwrapper mountPaths len = %d, want 2", len(mounts))
			}

			for i, got := range mounts {
				if got != tt.want {
					t.Errorf("unwrap call %d mountPath = %q, want derived %q", i, got, tt.want)
				}
			}
		})
	}
}

// TestKeysetManager_autoProvision_SetsImportLegacy verifies that lazy
// auto-provisioning takes the migration path by setting the internal
// ProvisionInput.importLegacy marker to true, without relying on audit fields
// (Actor/Reason). Lazy provisioning migrates an existing organization by
// importing its legacy key material (envelope PRIMARY + legacy ENABLED).
func TestKeysetManager_autoProvision_SetsImportLegacy(t *testing.T) {
	t.Parallel()

	provisioner := &fakeProvisioningService{}

	manager := NewKeysetManager(
		&fakeKeysetRepoWithProvision{},
		&fakeKeysetUnwrapper{},
		provisioner,
		DefaultKeysetManagerConfig(),
		NewProtectionMetrics(nil),
	)

	ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-envelope")

	if err := manager.autoProvision(ctx, "org-migrate"); err != nil {
		t.Fatalf("autoProvision() error = %v", err)
	}

	got := provisioner.getLastRequest()
	if !got.importLegacy {
		t.Errorf("autoProvision() importLegacy = %v, want true", got.importLegacy)
	}
}

// identityKMS is a test KMSClient that performs no real encryption: it base64-
// encodes on Encrypt and decodes on Decrypt. It lets tests build REAL Tink mixed
// keysets via tink.KeysetFactory without a live Vault, while the KeysetManager
// itself is driven through the separate fakeKeysetUnwrapper.
type identityKMS struct{}

func (identityKMS) Encrypt(_ context.Context, _, _ string, plaintext []byte) (string, error) {
	return "id:" + base64.StdEncoding.EncodeToString(plaintext), nil
}

func (identityKMS) Decrypt(_ context.Context, _, _ string, ciphertext string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(strings.TrimPrefix(ciphertext, "id:"))
}

// convertTestKeysetInfo maps a tink.KeysetInfo to the mmodel form the repository
// stores. Mirrors the production conversion so test fixtures reflect what a
// migrated org actually persists.
func convertTestKeysetInfo(info tink.KeysetInfo) mmodel.KeysetInfo {
	keys := make([]mmodel.KeyInfo, len(info.Keys))
	for i, k := range info.Keys {
		keys[i] = mmodel.KeyInfo{
			KeyID:     k.KeyID,
			Status:    string(k.Status),
			Type:      string(k.Type),
			IsPrimary: k.IsPrimary,
		}
	}

	return mmodel.KeysetInfo{PrimaryKeyID: info.PrimaryKeyID, Keys: keys}
}

// findNonPrimaryKey returns the single ENABLED non-primary entry (the imported
// legacy key), or nil. The composite keyset assigns the legacy key a fixed
// reserved ID, while the fresh primary uses a random Tink ID, so the legacy key
// is located by its non-primary flag rather than a hardcoded ID.
func findNonPrimaryKey(info mmodel.KeysetInfo) *mmodel.KeyInfo {
	for i := range info.Keys {
		if !info.Keys[i].IsPrimary {
			return &info.Keys[i]
		}
	}

	return nil
}

// TestKeysetManager_GetPrimitives_MixedKeyset_UnwrapsToWorkingPrimitives proves a
// PERSISTED MIXED keyset (fresh envelope PRIMARY + imported legacy ENABLED
// non-primary, on BOTH the AEAD and PRF sides) unwraps into working primitives via
// GetPrimitives. A migrated org runs GetPrimitives on its next encrypt/decrypt, so
// a keyset that cannot unwrap would break the org immediately.
func TestKeysetManager_GetPrimitives_MixedKeyset_UnwrapsToWorkingPrimitives(t *testing.T) {
	t.Parallel()

	factory := tink.NewKeysetFactory(identityKMS{})

	const legacyHexKey = "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"

	const legacySecret = "legacy-hash-secret"

	aeadBundle, err := factory.GenerateMixedAEADKeyset(context.Background(), "transit", "crm-org-mixed", legacyHexKey)
	if err != nil {
		t.Fatalf("GenerateMixedAEADKeyset() error = %v", err)
	}

	prfBundle, err := factory.GenerateMixedPRFKeyset(context.Background(), "transit", "crm-org-mixed", legacySecret)
	if err != nil {
		t.Fatalf("GenerateMixedPRFKeyset() error = %v", err)
	}

	aeadInfo := convertTestKeysetInfo(aeadBundle.Wrapped.Info)
	prfInfo := convertTestKeysetInfo(prfBundle.Wrapped.Info)

	// Persisted keyset fixture: stored metadata is the composite (two-key) info,
	// wrapped slots map to the fake unwrapper's known tokens.
	reader := &fakeKeysetRepo{
		keyset: &mmodel.OrganizationKeyset{
			OrganizationID:    "org-mixed",
			KEKPath:           "crm-org-mixed",
			KEKMountPath:      "transit",
			WrappedKeyset:     "wrapped-aead",
			WrappedHMACKeyset: "wrapped-prf",
			KeysetInfo:        aeadInfo,
			HMACKeysetInfo:    prfInfo,
		},
	}

	// The fake unwrapper returns the REAL composite raw keyset bytes for each slot.
	unwrapper := &fakeKeysetUnwrapper{
		aeadKeyset: aeadBundle.RawKeyset,
		prfKeyset:  prfBundle.RawKeyset,
	}

	manager := NewKeysetManager(reader, unwrapper, nil, DefaultKeysetManagerConfig(), NewProtectionMetrics(nil))

	prims, err := manager.GetActivePrimitives(context.Background(), "org-mixed")
	if err != nil {
		t.Fatalf("GetPrimitives() error = %v", err)
	}

	// All three primitives must be present.
	if prims.AEAD == nil {
		t.Fatal("GetPrimitives() AEAD primitive is nil")
	}

	if prims.PRF == nil {
		t.Fatal("GetPrimitives() PRF primitive is nil")
	}

	if prims.MultiKeyPRF == nil {
		t.Fatal("GetPrimitives() MultiKeyPRF primitive is nil")
	}

	// The envelope (fresh) key stays primary for encrypt on both sides.
	if prims.PrimaryKeyID != aeadInfo.PrimaryKeyID {
		t.Errorf("AEAD PrimaryKeyID = %d, want %d (fresh envelope key)", prims.PrimaryKeyID, aeadInfo.PrimaryKeyID)
	}

	if prims.PRFPrimaryKeyID != prfInfo.PrimaryKeyID {
		t.Errorf("PRFPrimaryKeyID = %d, want %d (fresh envelope key)", prims.PRFPrimaryKeyID, prfInfo.PrimaryKeyID)
	}

	// The AEAD primitive actually works: encrypt with the fresh primary, decrypt back.
	plaintext := []byte("migrated-org-secret-value")
	aad := []byte("aad-context")

	ciphertext, err := prims.AEAD.Encrypt(plaintext, aad)
	if err != nil {
		t.Fatalf("AEAD.Encrypt() error = %v", err)
	}

	decrypted, err := prims.AEAD.Decrypt(ciphertext, aad)
	if err != nil {
		t.Fatalf("AEAD.Decrypt() error = %v", err)
	}

	if string(decrypted) != string(plaintext) {
		t.Errorf("AEAD round-trip = %q, want %q", decrypted, plaintext)
	}

	// The PRF primitive produces a deterministic, non-empty search token.
	token, err := prims.PRF.ComputeSearchToken([]byte("searchable@example.com"))
	if err != nil {
		t.Fatalf("PRF.ComputeSearchToken() error = %v", err)
	}

	if token == "" {
		t.Error("PRF.ComputeSearchToken() returned empty token")
	}

	// MultiKeyPRF returns one candidate per key (>= 2: fresh primary + legacy).
	candidates, err := prims.MultiKeyPRF.ComputeSearchTokenCandidates([]byte("searchable@example.com"))
	if err != nil {
		t.Fatalf("MultiKeyPRF.ComputeSearchTokenCandidates() error = %v", err)
	}

	if len(candidates) < 2 {
		t.Errorf("MultiKeyPRF candidates = %d, want >= 2 (fresh + legacy)", len(candidates))
	}

	// The primary-key token must appear among the multi-key candidates.
	found := false

	for _, c := range candidates {
		if c == token {
			found = true

			break
		}
	}

	if !found {
		t.Error("primary PRF token not found among multi-key candidates")
	}

	// Stored metadata: legacy key present, ENABLED, non-primary on both sides.
	// Each composite keyset holds exactly two keys (fresh primary + legacy).
	if len(aeadInfo.Keys) != 2 {
		t.Errorf("AEAD stored keyset keys = %d, want 2 (fresh + legacy)", len(aeadInfo.Keys))
	}

	if len(prfInfo.Keys) != 2 {
		t.Errorf("PRF stored keyset keys = %d, want 2 (fresh + legacy)", len(prfInfo.Keys))
	}

	aeadLegacy := findNonPrimaryKey(aeadInfo)
	if aeadLegacy == nil {
		t.Fatal("AEAD legacy (non-primary) key missing from stored keyset")
	}

	if aeadLegacy.Type != string(tink.KeyTypeLegacyAESGCM) {
		t.Errorf("AEAD legacy key type = %q, want %q", aeadLegacy.Type, tink.KeyTypeLegacyAESGCM)
	}

	if aeadLegacy.Status != "ENABLED" {
		t.Errorf("AEAD legacy key status = %q, want ENABLED", aeadLegacy.Status)
	}

	prfLegacy := findNonPrimaryKey(prfInfo)
	if prfLegacy == nil {
		t.Fatal("PRF legacy (non-primary) key missing from stored keyset")
	}

	if prfLegacy.Type != string(tink.KeyTypeLegacyHMACSHA256) {
		t.Errorf("PRF legacy key type = %q, want %q", prfLegacy.Type, tink.KeyTypeLegacyHMACSHA256)
	}

	if prfLegacy.Status != "ENABLED" {
		t.Errorf("PRF legacy key status = %q, want ENABLED", prfLegacy.Status)
	}
}

// TestKeysetManager_GetPrimitives_MigratedKeyset_ExposesLegacyHexTokenPRF is the
// hard gate for T-2.2.1: a MIGRATED composite PRF keyset (fresh HMAC-PRF primary +
// imported legacy HMAC-SHA256) must cause GetPrimitives to populate a non-nil
// CachedPrimitives.LegacyHexTokenPRF whose ComputeLegacyHexToken(value) is
// byte-identical to the indexed legacy token produced by
// tink.NewLegacyPRFPrimitiveFromSecret(secret) for the same secret.
func TestKeysetManager_GetPrimitives_MigratedKeyset_ExposesLegacyHexTokenPRF(t *testing.T) {
	t.Parallel()

	factory := tink.NewKeysetFactory(identityKMS{})

	const legacyHexKey = "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"

	const legacySecret = "legacy-hash-secret"

	aeadBundle, err := factory.GenerateMixedAEADKeyset(context.Background(), "transit", "crm-org-legacy-prf", legacyHexKey)
	if err != nil {
		t.Fatalf("GenerateMixedAEADKeyset() error = %v", err)
	}

	prfBundle, err := factory.GenerateMixedPRFKeyset(context.Background(), "transit", "crm-org-legacy-prf", legacySecret)
	if err != nil {
		t.Fatalf("GenerateMixedPRFKeyset() error = %v", err)
	}

	reader := &fakeKeysetRepo{
		keyset: &mmodel.OrganizationKeyset{
			OrganizationID:    "org-legacy-prf",
			KEKPath:           "crm-org-legacy-prf",
			KEKMountPath:      "transit",
			WrappedKeyset:     "wrapped-aead",
			WrappedHMACKeyset: "wrapped-prf",
			KeysetInfo:        convertTestKeysetInfo(aeadBundle.Wrapped.Info),
			HMACKeysetInfo:    convertTestKeysetInfo(prfBundle.Wrapped.Info),
		},
	}

	unwrapper := &fakeKeysetUnwrapper{
		aeadKeyset: aeadBundle.RawKeyset,
		prfKeyset:  prfBundle.RawKeyset,
	}

	manager := NewKeysetManager(reader, unwrapper, nil, DefaultKeysetManagerConfig(), NewProtectionMetrics(nil))

	prims, err := manager.GetActivePrimitives(context.Background(), "org-legacy-prf")
	if err != nil {
		t.Fatalf("GetPrimitives() error = %v", err)
	}

	if prims.LegacyHexTokenPRF == nil {
		t.Fatal("GetPrimitives() LegacyHexTokenPRF is nil for a migrated keyset, want non-nil")
	}

	fromSecret, err := tink.NewLegacyPRFPrimitiveFromSecret(legacySecret)
	if err != nil {
		t.Fatalf("NewLegacyPRFPrimitiveFromSecret() error = %v", err)
	}

	for _, value := range []string{"abc123", "", "a@b.com"} {
		want, err := fromSecret.ComputeLegacyHexToken([]byte(value))
		if err != nil {
			t.Fatalf("fromSecret.ComputeLegacyHexToken(%q) error = %v", value, err)
		}

		got, err := prims.LegacyHexTokenPRF.ComputeLegacyHexToken([]byte(value))
		if err != nil {
			t.Fatalf("LegacyHexTokenPRF.ComputeLegacyHexToken(%q) error = %v", value, err)
		}

		if got != want {
			t.Errorf("LegacyHexTokenPRF token for %q = %q, want indexed legacy token %q", value, got, want)
		}
	}
}

// TestKeysetManager_GetPrimitives_EnvelopeOnlyKeyset_LegacyHexTokenPRFNil verifies
// that an ENVELOPE-ONLY PRF keyset (no imported legacy key) leaves
// CachedPrimitives.LegacyHexTokenPRF nil.
func TestKeysetManager_GetPrimitives_EnvelopeOnlyKeyset_LegacyHexTokenPRFNil(t *testing.T) {
	t.Parallel()

	aeadBytes, prfBytes := generateTestKeysets(t)

	reader := &fakeKeysetRepo{
		keyset: &mmodel.OrganizationKeyset{
			OrganizationID:    "org-envelope-only",
			KEKPath:           "org-org-envelope-only",
			WrappedKeyset:     "wrapped-aead",
			WrappedHMACKeyset: "wrapped-prf",
			KeysetInfo:        mmodel.KeysetInfo{PrimaryKeyID: 111},
			HMACKeysetInfo:    mmodel.KeysetInfo{PrimaryKeyID: 222},
		},
	}

	unwrapper := &fakeKeysetUnwrapper{
		aeadKeyset: aeadBytes,
		prfKeyset:  prfBytes,
	}

	manager := NewKeysetManager(reader, unwrapper, nil, DefaultKeysetManagerConfig(), NewProtectionMetrics(nil))

	prims, err := manager.GetActivePrimitives(context.Background(), "org-envelope-only")
	if err != nil {
		t.Fatalf("GetPrimitives() error = %v", err)
	}

	if prims.LegacyHexTokenPRF != nil {
		t.Error("GetPrimitives() LegacyHexTokenPRF should be nil for an envelope-only keyset")
	}
}

// TestKeysetManager_GetPrimitives_LegacyMetadataButNoHandleKey_FailsClosed verifies
// the fail-closed path: stored PRF metadata flags an imported legacy HMAC key, but
// the unwrapped PRF handle is envelope-only (no legacy entry). This mismatch signals
// a provisioning/decoding bug, so GetPrimitives MUST error rather than silently drop
// the legacy search token.
func TestKeysetManager_GetPrimitives_LegacyMetadataButNoHandleKey_FailsClosed(t *testing.T) {
	t.Parallel()

	aeadBytes, prfBytes := generateTestKeysets(t)

	// Metadata claims a legacy HMAC key, but prfBytes is a fresh envelope-only keyset.
	reader := &fakeKeysetRepo{
		keyset: &mmodel.OrganizationKeyset{
			OrganizationID:    "org-legacy-mismatch",
			KEKPath:           "org-org-legacy-mismatch",
			WrappedKeyset:     "wrapped-aead",
			WrappedHMACKeyset: "wrapped-prf",
			KeysetInfo:        mmodel.KeysetInfo{PrimaryKeyID: 111},
			HMACKeysetInfo: mmodel.KeysetInfo{
				PrimaryKeyID: 222,
				Keys: []mmodel.KeyInfo{
					{KeyID: 222, Status: "ENABLED", Type: string(tink.KeyTypeHMACPRF), IsPrimary: true},
					{KeyID: 333, Status: "ENABLED", Type: string(tink.KeyTypeLegacyHMACSHA256), IsPrimary: false},
				},
			},
		},
	}

	unwrapper := &fakeKeysetUnwrapper{
		aeadKeyset: aeadBytes,
		prfKeyset:  prfBytes,
	}

	manager := NewKeysetManager(reader, unwrapper, nil, DefaultKeysetManagerConfig(), NewProtectionMetrics(nil))

	_, err := manager.GetActivePrimitives(context.Background(), "org-legacy-mismatch")
	if err == nil {
		t.Fatal("GetPrimitives() expected fail-closed error for legacy-metadata/handle mismatch, got nil")
	}

	if !errors.Is(err, tink.ErrNoLegacyPRFKey) {
		t.Errorf("GetPrimitives() error = %v, want errors.Is(tink.ErrNoLegacyPRFKey)", err)
	}

	// No partial cache entry should remain after the failure.
	manager.mu.RLock()
	_, ok := manager.cache[buildCacheKey("default", "org-legacy-mismatch")]
	manager.mu.RUnlock()

	if ok {
		t.Error("cache entry should NOT exist after fail-closed legacy mismatch")
	}
}
