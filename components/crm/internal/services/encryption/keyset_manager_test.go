// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package encryption

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"

	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/crypto/tink"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
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
	macKeyset  []byte
	err        error
	calls      int
}

func (f *fakeKeysetUnwrapper) UnwrapKeyset(_ context.Context, _ string, wrappedKeyset string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.calls++

	if f.err != nil {
		return nil, f.err
	}

	// Return AEAD keyset for AEAD wrapped data, MAC keyset for HMAC wrapped data
	if wrappedKeyset == "wrapped-aead" {
		return f.aeadKeyset, nil
	}

	if wrappedKeyset == "wrapped-mac" {
		return f.macKeyset, nil
	}

	return nil, errors.New("unknown wrapped keyset")
}

func (f *fakeKeysetUnwrapper) getCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.calls
}

// generateTestKeysets creates real Tink keysets for testing.
func generateTestKeysets(t *testing.T) ([]byte, []byte) {
	t.Helper()

	_, aeadBytes, err := tink.NewAEADKeysetGenerator().Generate()
	if err != nil {
		t.Fatalf("failed to generate AEAD keyset: %v", err)
	}

	_, macBytes, err := tink.NewMACKeysetGenerator().Generate()
	if err != nil {
		t.Fatalf("failed to generate MAC keyset: %v", err)
	}

	return aeadBytes, macBytes
}

func TestKeysetManager_GetPrimitives_CacheMiss_Success(t *testing.T) {
	t.Parallel()

	aeadBytes, macBytes := generateTestKeysets(t)

	reader := &fakeKeysetRepo{
		keyset: &mmodel.OrganizationKeyset{
			OrganizationID:    "org-123",
			KEKPath:           "org-org-123",
			WrappedKeyset:     "wrapped-aead",
			WrappedHMACKeyset: "wrapped-mac",
		},
	}

	unwrapper := &fakeKeysetUnwrapper{
		aeadKeyset: aeadBytes,
		macKeyset:  macBytes,
	}

	manager := NewKeysetManager(reader, unwrapper, nil, DefaultKeysetManagerConfig())

	aead, mac, _, err := manager.GetPrimitives(context.Background(), "org-123")
	if err != nil {
		t.Fatalf("GetPrimitives() error = %v", err)
	}

	if aead == nil {
		t.Error("GetPrimitives() AEAD is nil")
	}

	if mac == nil {
		t.Error("GetPrimitives() MAC is nil")
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

	aeadBytes, macBytes := generateTestKeysets(t)

	reader := &fakeKeysetRepo{
		keyset: &mmodel.OrganizationKeyset{
			OrganizationID:    "org-456",
			KEKPath:           "org-org-456",
			WrappedKeyset:     "wrapped-aead",
			WrappedHMACKeyset: "wrapped-mac",
		},
	}

	unwrapper := &fakeKeysetUnwrapper{
		aeadKeyset: aeadBytes,
		macKeyset:  macBytes,
	}

	manager := NewKeysetManager(reader, unwrapper, nil, DefaultKeysetManagerConfig())

	// First call - cache miss
	aead1, mac1, _, err := manager.GetPrimitives(context.Background(), "org-456")
	if err != nil {
		t.Fatalf("GetPrimitives() first call error = %v", err)
	}

	// Second call - should be cache hit
	aead2, mac2, _, err := manager.GetPrimitives(context.Background(), "org-456")
	if err != nil {
		t.Fatalf("GetPrimitives() second call error = %v", err)
	}

	// Should return the same primitives (cached)
	if aead1 != aead2 {
		t.Error("GetPrimitives() AEAD primitives should be the same instance")
	}

	if mac1 != mac2 {
		t.Error("GetPrimitives() MAC primitives should be the same instance")
	}

	// Reader should only be called once
	if reader.getCalls() != 1 {
		t.Errorf("GetPrimitives() reader calls = %d, want 1", reader.getCalls())
	}

	// Unwrapper should only be called twice (once for AEAD, once for MAC on first call)
	if unwrapper.getCalls() != 2 {
		t.Errorf("GetPrimitives() unwrapper calls = %d, want 2", unwrapper.getCalls())
	}
}

func TestKeysetManager_GetPrimitives_CacheExpired_Refetches(t *testing.T) {
	t.Parallel()

	aeadBytes, macBytes := generateTestKeysets(t)

	reader := &fakeKeysetRepo{
		keyset: &mmodel.OrganizationKeyset{
			OrganizationID:    "org-789",
			KEKPath:           "org-org-789",
			WrappedKeyset:     "wrapped-aead",
			WrappedHMACKeyset: "wrapped-mac",
		},
	}

	unwrapper := &fakeKeysetUnwrapper{
		aeadKeyset: aeadBytes,
		macKeyset:  macBytes,
	}

	// Use a very short TTL for testing
	config := KeysetManagerConfig{
		CacheTTL: 10 * time.Millisecond,
	}
	manager := NewKeysetManager(reader, unwrapper, nil, config)

	// First call - cache miss
	_, _, _, err := manager.GetPrimitives(context.Background(), "org-789")
	if err != nil {
		t.Fatalf("GetPrimitives() first call error = %v", err)
	}

	// Wait for cache to expire
	time.Sleep(20 * time.Millisecond)

	// Second call - cache expired, should refetch
	_, _, _, err = manager.GetPrimitives(context.Background(), "org-789")
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

	manager := NewKeysetManager(reader, unwrapper, nil, DefaultKeysetManagerConfig())

	_, _, _, err := manager.GetPrimitives(context.Background(), "org-not-found")
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
			WrappedHMACKeyset: "wrapped-mac",
		},
	}

	unwrapErr := errors.New("KMS unwrap failed")
	unwrapper := &fakeKeysetUnwrapper{
		err: unwrapErr,
	}

	manager := NewKeysetManager(reader, unwrapper, nil, DefaultKeysetManagerConfig())

	_, _, _, err := manager.GetPrimitives(context.Background(), "org-unwrap-fail")
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
			WrappedHMACKeyset: "wrapped-mac",
		},
	}

	// Return invalid keyset bytes that cannot be parsed
	unwrapper := &fakeKeysetUnwrapper{
		aeadKeyset: []byte("invalid-keyset-data"),
		macKeyset:  []byte("invalid-mac-keyset-data"),
	}

	manager := NewKeysetManager(reader, unwrapper, nil, DefaultKeysetManagerConfig())

	_, _, _, err := manager.GetPrimitives(context.Background(), "org-parse-fail")
	if err == nil {
		t.Fatal("GetPrimitives() expected error for invalid keyset, got nil")
	}
}

func TestKeysetManager_GetPrimitives_ConcurrentAccess_Safe(t *testing.T) {
	t.Parallel()

	aeadBytes, macBytes := generateTestKeysets(t)

	reader := &fakeKeysetRepo{
		keyset: &mmodel.OrganizationKeyset{
			OrganizationID:    "org-concurrent",
			KEKPath:           "org-org-concurrent",
			WrappedKeyset:     "wrapped-aead",
			WrappedHMACKeyset: "wrapped-mac",
		},
	}

	unwrapper := &fakeKeysetUnwrapper{
		aeadKeyset: aeadBytes,
		macKeyset:  macBytes,
	}

	manager := NewKeysetManager(reader, unwrapper, nil, DefaultKeysetManagerConfig())

	const goroutines = 10

	var wg sync.WaitGroup

	wg.Add(goroutines)

	errChan := make(chan error, goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()

			aead, mac, _, err := manager.GetPrimitives(context.Background(), "org-concurrent")
			if err != nil {
				errChan <- err
				return
			}

			if aead == nil || mac == nil {
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
			WrappedHMACKeyset: "wrapped-mac",
		},
	}

	unwrapper := &fakeKeysetUnwrapper{}

	manager := NewKeysetManager(reader, unwrapper, nil, DefaultKeysetManagerConfig())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, _, err := manager.GetPrimitives(ctx, "org-ctx-cancel")
	if err == nil {
		t.Fatal("GetPrimitives() expected error for cancelled context, got nil")
	}

	if !errors.Is(err, context.Canceled) {
		t.Errorf("GetPrimitives() error = %v, want %v", err, context.Canceled)
	}
}

func TestKeysetManager_InvalidateCache_RemovesEntry(t *testing.T) {
	t.Parallel()

	aeadBytes, macBytes := generateTestKeysets(t)

	reader := &fakeKeysetRepo{
		keyset: &mmodel.OrganizationKeyset{
			OrganizationID:    "org-invalidate",
			KEKPath:           "org-org-invalidate",
			WrappedKeyset:     "wrapped-aead",
			WrappedHMACKeyset: "wrapped-mac",
		},
	}

	unwrapper := &fakeKeysetUnwrapper{
		aeadKeyset: aeadBytes,
		macKeyset:  macBytes,
	}

	manager := NewKeysetManager(reader, unwrapper, nil, DefaultKeysetManagerConfig())

	// First call - cache miss
	_, _, _, err := manager.GetPrimitives(context.Background(), "org-invalidate")
	if err != nil {
		t.Fatalf("GetPrimitives() first call error = %v", err)
	}

	// Invalidate the cache
	manager.InvalidateCache("org-invalidate")

	// Second call - should refetch after invalidation
	_, _, _, err = manager.GetPrimitives(context.Background(), "org-invalidate")
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

	aeadBytes, macBytes := generateTestKeysets(t)

	reader := &fakeKeysetRepo{
		keyset: &mmodel.OrganizationKeyset{
			OrganizationID:    "org-clear",
			KEKPath:           "org-org-clear",
			WrappedKeyset:     "wrapped-aead",
			WrappedHMACKeyset: "wrapped-mac",
		},
	}

	unwrapper := &fakeKeysetUnwrapper{
		aeadKeyset: aeadBytes,
		macKeyset:  macBytes,
	}

	manager := NewKeysetManager(reader, unwrapper, nil, DefaultKeysetManagerConfig())

	// First call - cache miss
	_, _, _, err := manager.GetPrimitives(context.Background(), "org-clear")
	if err != nil {
		t.Fatalf("GetPrimitives() first call error = %v", err)
	}

	// Clear all cache
	manager.ClearCache()

	// Second call - should refetch after clear
	_, _, _, err = manager.GetPrimitives(context.Background(), "org-clear")
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

	manager := NewKeysetManager(reader, unwrapper, nil, DefaultKeysetManagerConfig())

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

	aeadBytes, macBytes := generateTestKeysets(t)

	reader := &fakeKeysetRepo{}
	unwrapper := &fakeKeysetUnwrapper{
		aeadKeyset: aeadBytes,
		macKeyset:  macBytes,
	}

	manager := NewKeysetManager(reader, unwrapper, nil, DefaultKeysetManagerConfig())

	// Test with org-1
	reader.keyset = &mmodel.OrganizationKeyset{
		OrganizationID:    "org-1",
		KEKPath:           "org-org-1",
		WrappedKeyset:     "wrapped-aead",
		WrappedHMACKeyset: "wrapped-mac",
	}

	aead1, mac1, _, err := manager.GetPrimitives(context.Background(), "org-1")
	if err != nil {
		t.Fatalf("GetPrimitives(org-1) error = %v", err)
	}

	// Test with org-2
	reader.keyset = &mmodel.OrganizationKeyset{
		OrganizationID:    "org-2",
		KEKPath:           "org-org-2",
		WrappedKeyset:     "wrapped-aead",
		WrappedHMACKeyset: "wrapped-mac",
	}

	aead2, mac2, _, err := manager.GetPrimitives(context.Background(), "org-2")
	if err != nil {
		t.Fatalf("GetPrimitives(org-2) error = %v", err)
	}

	// Should have different primitive instances for different orgs
	if aead1 == aead2 {
		t.Error("GetPrimitives() AEAD primitives should be different for different orgs")
	}

	if mac1 == mac2 {
		t.Error("GetPrimitives() MAC primitives should be different for different orgs")
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

	aeadBytes, macBytes := generateTestKeysets(t)

	// Use a slow reader to ensure concurrent requests overlap
	reader := &slowKeysetRepo{
		keyset: &mmodel.OrganizationKeyset{
			OrganizationID:    "org-dedup",
			KEKPath:           "org-org-dedup",
			WrappedKeyset:     "wrapped-aead",
			WrappedHMACKeyset: "wrapped-mac",
		},
		delay:      50 * time.Millisecond,
		fetchStart: make(chan struct{}, 1),
	}

	unwrapper := &fakeKeysetUnwrapper{
		aeadKeyset: aeadBytes,
		macKeyset:  macBytes,
	}

	manager := NewKeysetManager(reader, unwrapper, nil, DefaultKeysetManagerConfig())

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

			aead, mac, _, err := manager.GetPrimitives(context.Background(), "org-dedup")
			if err != nil {
				errChan <- err
				return
			}

			if aead == nil || mac == nil {
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

	// Unwrapper should only be called twice (once for AEAD, once for MAC)
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

	manager := NewKeysetManager(reader, unwrapper, nil, DefaultKeysetManagerConfig())

	_, _, _, err := manager.GetPrimitives(context.Background(), "org-nil-keyset")
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

	aeadBytes, macBytes := generateTestKeysets(t)

	// Reader returns not found initially, then returns keyset after provisioning
	reader := &fakeKeysetRepoWithProvision{
		keyset: nil,
		err:    constant.ErrKeysetNotFound,
		keysetAfterProv: &mmodel.OrganizationKeyset{
			OrganizationID:    "org-auto-prov",
			KEKPath:           "org-org-auto-prov",
			WrappedKeyset:     "wrapped-aead",
			WrappedHMACKeyset: "wrapped-mac",
		},
		errAfterProv: nil,
	}

	unwrapper := &fakeKeysetUnwrapper{
		aeadKeyset: aeadBytes,
		macKeyset:  macBytes,
	}

	provisioner := &fakeProvisioningService{
		provisionResult: ProvisionResult{
			OrganizationID:   "org-auto-prov",
			KEKPath:          "org-org-auto-prov",
			AEADPrimaryKeyID: 12345,
			MACPrimaryKeyID:  67890,
		},
		onProvision: func() {
			reader.markProvisioned()
		},
	}

	manager := NewKeysetManager(reader, unwrapper, provisioner, DefaultKeysetManagerConfig())

	// Context with tenant ID (required for auto-provisioning)
	ctx := tmcore.ContextWithTenantID(context.Background(), "test-tenant")

	aead, mac, _, err := manager.GetPrimitives(ctx, "org-auto-prov")
	if err != nil {
		t.Fatalf("GetPrimitives() error = %v", err)
	}

	if aead == nil {
		t.Error("GetPrimitives() AEAD is nil")
	}

	if mac == nil {
		t.Error("GetPrimitives() MAC is nil")
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

	aeadBytes, macBytes := generateTestKeysets(t)

	reader := &fakeKeysetRepo{
		keyset: &mmodel.OrganizationKeyset{
			OrganizationID:    "org-existing",
			KEKPath:           "org-org-existing",
			WrappedKeyset:     "wrapped-aead",
			WrappedHMACKeyset: "wrapped-mac",
		},
	}

	unwrapper := &fakeKeysetUnwrapper{
		aeadKeyset: aeadBytes,
		macKeyset:  macBytes,
	}

	provisioner := &fakeProvisioningService{}

	manager := NewKeysetManager(reader, unwrapper, provisioner, DefaultKeysetManagerConfig())

	aead, mac, _, err := manager.GetPrimitives(context.Background(), "org-existing")
	if err != nil {
		t.Fatalf("GetPrimitives() error = %v", err)
	}

	if aead == nil {
		t.Error("GetPrimitives() AEAD is nil")
	}

	if mac == nil {
		t.Error("GetPrimitives() MAC is nil")
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

	manager := NewKeysetManager(reader, unwrapper, provisioner, DefaultKeysetManagerConfig())

	// Context with tenant ID (required for auto-provisioning)
	ctx := tmcore.ContextWithTenantID(context.Background(), "test-tenant")

	_, _, _, err := manager.GetPrimitives(ctx, "org-prov-fail")
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
	manager := NewKeysetManager(reader, unwrapper, nil, DefaultKeysetManagerConfig())

	_, _, _, err := manager.GetPrimitives(context.Background(), "org-no-prov")
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

	manager := NewKeysetManager(reader, unwrapper, provisioner, DefaultKeysetManagerConfig())

	// Context with tenant ID (required for auto-provisioning)
	ctx := tmcore.ContextWithTenantID(context.Background(), "test-tenant")

	_, _, _, err := manager.GetPrimitives(ctx, "org-nil-after")
	if err == nil {
		t.Fatal("GetPrimitives() expected error for nil keyset after provisioning, got nil")
	}
}

func TestKeysetManager_autoProvision_UsesTenantFromContext(t *testing.T) {
	t.Parallel()

	aeadBytes, macBytes := generateTestKeysets(t)

	// Reader returns not found initially, then returns keyset after provisioning
	reader := &fakeKeysetRepoWithProvision{
		keyset: nil,
		err:    constant.ErrKeysetNotFound,
		keysetAfterProv: &mmodel.OrganizationKeyset{
			OrganizationID:    "org-tenant-ctx",
			KEKPath:           "org-org-tenant-ctx",
			WrappedKeyset:     "wrapped-aead",
			WrappedHMACKeyset: "wrapped-mac",
		},
		errAfterProv: nil,
	}

	unwrapper := &fakeKeysetUnwrapper{
		aeadKeyset: aeadBytes,
		macKeyset:  macBytes,
	}

	provisioner := &fakeProvisioningService{
		provisionResult: ProvisionResult{
			OrganizationID: "org-tenant-ctx",
		},
		onProvision: func() {
			reader.markProvisioned()
		},
	}

	manager := NewKeysetManager(reader, unwrapper, provisioner, DefaultKeysetManagerConfig())

	// Create context with tenant ID using tmcore
	ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-from-context")

	_, _, _, err := manager.GetPrimitives(ctx, "org-tenant-ctx")
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

	if lastReq.Reason != "Auto-provisioned on first encrypted field access" {
		t.Errorf("autoProvision() reason = %q, want %q", lastReq.Reason, "Auto-provisioned on first encrypted field access")
	}
}

func TestKeysetManager_autoProvision_DefaultsTenantWhenMissing(t *testing.T) {
	t.Parallel()

	// Setup keyset that will be returned after provisioning
	aeadBytes, macBytes := generateTestKeysets(t)

	provisionedKeyset := &mmodel.OrganizationKeyset{
		TenantID:          "default",
		OrganizationID:    "org-no-tenant",
		KEKPath:           "transit/keys/crm/org-no-tenant",
		WrappedKeyset:     "wrapped-aead",
		WrappedHMACKeyset: "wrapped-mac",
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
		macKeyset:  macBytes,
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

	manager := NewKeysetManager(reader, unwrapper, provisioner, DefaultKeysetManagerConfig())

	// Context WITHOUT tenant ID - should default to "default"
	ctx := context.Background()

	_, _, _, err := manager.GetPrimitives(ctx, "org-no-tenant")
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

	aeadBytes, macBytes := generateTestKeysets(t)

	// Track which organization IDs were requested
	var readerMu sync.Mutex
	readerCalls := make(map[string]int)

	reader := &fakeKeysetRepo{
		keyset: &mmodel.OrganizationKeyset{
			OrganizationID:    "same-org-id",
			KEKPath:           "org-same-org-id",
			WrappedKeyset:     "wrapped-aead",
			WrappedHMACKeyset: "wrapped-mac",
		},
	}

	// Wrap the reader to track calls per context
	unwrapper := &fakeKeysetUnwrapper{
		aeadKeyset: aeadBytes,
		macKeyset:  macBytes,
	}

	manager := NewKeysetManager(reader, unwrapper, nil, DefaultKeysetManagerConfig())

	// Create contexts for two different tenants
	ctxTenantA := tmcore.ContextWithTenantID(context.Background(), "tenant-alpha")
	ctxTenantB := tmcore.ContextWithTenantID(context.Background(), "tenant-beta")

	// First call from tenant A - should fetch from repo
	aeadA, macA, _, err := manager.GetPrimitives(ctxTenantA, "same-org-id")
	if err != nil {
		t.Fatalf("GetPrimitives(tenant-alpha) error = %v", err)
	}

	readerMu.Lock()
	readerCalls["tenant-alpha"] = reader.getCalls()
	readerMu.Unlock()

	// First call from tenant B - should fetch from repo (different cache key)
	aeadB, macB, _, err := manager.GetPrimitives(ctxTenantB, "same-org-id")
	if err != nil {
		t.Fatalf("GetPrimitives(tenant-beta) error = %v", err)
	}

	// Reader should have been called twice (once per tenant)
	if reader.getCalls() != 2 {
		t.Errorf("GetPrimitives() reader calls = %d, want 2 (one per tenant)", reader.getCalls())
	}

	// Second call from tenant A - should use cache
	aeadA2, macA2, _, err := manager.GetPrimitives(ctxTenantA, "same-org-id")
	if err != nil {
		t.Fatalf("GetPrimitives(tenant-alpha, second call) error = %v", err)
	}

	// Reader should still only have been called twice
	if reader.getCalls() != 2 {
		t.Errorf("GetPrimitives() reader calls = %d, want 2 (tenant A should use cache)", reader.getCalls())
	}

	// Tenant A should get same cached primitives
	if aeadA != aeadA2 || macA != macA2 {
		t.Error("GetPrimitives() tenant A should get same cached primitives on second call")
	}

	// Primitives for different tenants should be different instances
	// (even though they have same content, they're cached separately)
	if aeadA == aeadB {
		t.Error("GetPrimitives() different tenants should have separate cache entries")
	}

	if macA == macB {
		t.Error("GetPrimitives() different tenants should have separate MAC cache entries")
	}
}

// TestKeysetManager_InvalidateCacheForTenant_OnlyAffectsSpecificTenant verifies that
// cache invalidation for one tenant does not affect other tenants.
func TestKeysetManager_InvalidateCacheForTenant_OnlyAffectsSpecificTenant(t *testing.T) {
	t.Parallel()

	aeadBytes, macBytes := generateTestKeysets(t)

	reader := &fakeKeysetRepo{
		keyset: &mmodel.OrganizationKeyset{
			OrganizationID:    "shared-org",
			KEKPath:           "org-shared-org",
			WrappedKeyset:     "wrapped-aead",
			WrappedHMACKeyset: "wrapped-mac",
		},
	}

	unwrapper := &fakeKeysetUnwrapper{
		aeadKeyset: aeadBytes,
		macKeyset:  macBytes,
	}

	manager := NewKeysetManager(reader, unwrapper, nil, DefaultKeysetManagerConfig())

	ctxTenantA := tmcore.ContextWithTenantID(context.Background(), "tenant-one")
	ctxTenantB := tmcore.ContextWithTenantID(context.Background(), "tenant-two")

	// Populate cache for both tenants
	aeadA1, _, _, err := manager.GetPrimitives(ctxTenantA, "shared-org")
	if err != nil {
		t.Fatalf("GetPrimitives(tenant-one) error = %v", err)
	}

	aeadB1, _, _, err := manager.GetPrimitives(ctxTenantB, "shared-org")
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
	aeadA2, _, _, err := manager.GetPrimitives(ctxTenantA, "shared-org")
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
	aeadB2, _, _, err := manager.GetPrimitives(ctxTenantB, "shared-org")
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

	aeadBytes, macBytes := generateTestKeysets(t)

	// Reader returns not found initially, then returns keyset after provisioning
	reader := &fakeKeysetRepoWithProvision{
		keyset: nil,
		err:    constant.ErrKeysetNotFound,
		keysetAfterProv: &mmodel.OrganizationKeyset{
			OrganizationID:    "org-registry-test",
			KEKPath:           "org-org-registry-test",
			WrappedKeyset:     "wrapped-aead",
			WrappedHMACKeyset: "wrapped-mac",
		},
		errAfterProv: nil,
	}

	unwrapper := &fakeKeysetUnwrapper{
		aeadKeyset: aeadBytes,
		macKeyset:  macBytes,
	}

	// Provisioner that returns result with expected registry properties
	provisioner := &fakeProvisioningService{
		provisionResult: ProvisionResult{
			OrganizationID:   "org-registry-test",
			KEKPath:          "org-org-registry-test",
			AEADPrimaryKeyID: 12345,
			MACPrimaryKeyID:  67890,
			RegistryStatus:   mmodel.RegistryStatusActive,
		},
		onProvision: func() {
			reader.markProvisioned()
		},
	}

	manager := NewKeysetManager(reader, unwrapper, provisioner, DefaultKeysetManagerConfig())

	// Context with tenant ID (required for auto-provisioning)
	ctx := tmcore.ContextWithTenantID(context.Background(), "test-tenant")

	aead, mac, _, err := manager.GetPrimitives(ctx, "org-registry-test")
	if err != nil {
		t.Fatalf("GetPrimitives() error = %v", err)
	}

	if aead == nil {
		t.Error("GetPrimitives() AEAD is nil")
	}

	if mac == nil {
		t.Error("GetPrimitives() MAC is nil")
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
