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

	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/crypto/tink"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
)

// fakeKeysetReader is a test double for KeysetReader.
type fakeKeysetReader struct {
	mu     sync.Mutex
	keyset *mmodel.OrganizationKeyset
	err    error
	calls  int
}

func (f *fakeKeysetReader) Get(_ context.Context, _ string) (*mmodel.OrganizationKeyset, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.calls++

	return f.keyset, f.err
}

func (f *fakeKeysetReader) getCalls() int {
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

	reader := &fakeKeysetReader{
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

	manager := NewKeysetManager(reader, unwrapper, DefaultKeysetManagerConfig())

	aead, mac, err := manager.GetPrimitives(context.Background(), "org-123")
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

	reader := &fakeKeysetReader{
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

	manager := NewKeysetManager(reader, unwrapper, DefaultKeysetManagerConfig())

	// First call - cache miss
	aead1, mac1, err := manager.GetPrimitives(context.Background(), "org-456")
	if err != nil {
		t.Fatalf("GetPrimitives() first call error = %v", err)
	}

	// Second call - should be cache hit
	aead2, mac2, err := manager.GetPrimitives(context.Background(), "org-456")
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

	reader := &fakeKeysetReader{
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
	manager := NewKeysetManager(reader, unwrapper, config)

	// First call - cache miss
	_, _, err := manager.GetPrimitives(context.Background(), "org-789")
	if err != nil {
		t.Fatalf("GetPrimitives() first call error = %v", err)
	}

	// Wait for cache to expire
	time.Sleep(20 * time.Millisecond)

	// Second call - cache expired, should refetch
	_, _, err = manager.GetPrimitives(context.Background(), "org-789")
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

	reader := &fakeKeysetReader{
		keyset: nil,
		err:    mmodel.ErrKeysetNotFound,
	}

	unwrapper := &fakeKeysetUnwrapper{}

	manager := NewKeysetManager(reader, unwrapper, DefaultKeysetManagerConfig())

	_, _, err := manager.GetPrimitives(context.Background(), "org-not-found")
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

	reader := &fakeKeysetReader{
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

	manager := NewKeysetManager(reader, unwrapper, DefaultKeysetManagerConfig())

	_, _, err := manager.GetPrimitives(context.Background(), "org-unwrap-fail")
	if err == nil {
		t.Fatal("GetPrimitives() expected error, got nil")
	}

	if !errors.Is(err, unwrapErr) {
		t.Errorf("GetPrimitives() error = %v, want %v", err, unwrapErr)
	}
}

func TestKeysetManager_GetPrimitives_ParseError_Propagated(t *testing.T) {
	t.Parallel()

	reader := &fakeKeysetReader{
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

	manager := NewKeysetManager(reader, unwrapper, DefaultKeysetManagerConfig())

	_, _, err := manager.GetPrimitives(context.Background(), "org-parse-fail")
	if err == nil {
		t.Fatal("GetPrimitives() expected error for invalid keyset, got nil")
	}
}

func TestKeysetManager_GetPrimitives_ConcurrentAccess_Safe(t *testing.T) {
	t.Parallel()

	aeadBytes, macBytes := generateTestKeysets(t)

	reader := &fakeKeysetReader{
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

	manager := NewKeysetManager(reader, unwrapper, DefaultKeysetManagerConfig())

	const goroutines = 10

	var wg sync.WaitGroup

	wg.Add(goroutines)

	errChan := make(chan error, goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()

			aead, mac, err := manager.GetPrimitives(context.Background(), "org-concurrent")
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

	reader := &fakeKeysetReader{
		keyset: &mmodel.OrganizationKeyset{
			OrganizationID:    "org-ctx-cancel",
			KEKPath:           "org-org-ctx-cancel",
			WrappedKeyset:     "wrapped-aead",
			WrappedHMACKeyset: "wrapped-mac",
		},
	}

	unwrapper := &fakeKeysetUnwrapper{}

	manager := NewKeysetManager(reader, unwrapper, DefaultKeysetManagerConfig())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := manager.GetPrimitives(ctx, "org-ctx-cancel")
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

	reader := &fakeKeysetReader{
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

	manager := NewKeysetManager(reader, unwrapper, DefaultKeysetManagerConfig())

	// First call - cache miss
	_, _, err := manager.GetPrimitives(context.Background(), "org-invalidate")
	if err != nil {
		t.Fatalf("GetPrimitives() first call error = %v", err)
	}

	// Invalidate the cache
	manager.InvalidateCache("org-invalidate")

	// Second call - should refetch after invalidation
	_, _, err = manager.GetPrimitives(context.Background(), "org-invalidate")
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

	reader := &fakeKeysetReader{
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

	manager := NewKeysetManager(reader, unwrapper, DefaultKeysetManagerConfig())

	// First call - cache miss
	_, _, err := manager.GetPrimitives(context.Background(), "org-clear")
	if err != nil {
		t.Fatalf("GetPrimitives() first call error = %v", err)
	}

	// Clear all cache
	manager.ClearCache()

	// Second call - should refetch after clear
	_, _, err = manager.GetPrimitives(context.Background(), "org-clear")
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

	reader := &fakeKeysetReader{}
	unwrapper := &fakeKeysetUnwrapper{}

	manager := NewKeysetManager(reader, unwrapper, DefaultKeysetManagerConfig())

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

	reader := &fakeKeysetReader{}
	unwrapper := &fakeKeysetUnwrapper{
		aeadKeyset: aeadBytes,
		macKeyset:  macBytes,
	}

	manager := NewKeysetManager(reader, unwrapper, DefaultKeysetManagerConfig())

	// Test with org-1
	reader.keyset = &mmodel.OrganizationKeyset{
		OrganizationID:    "org-1",
		KEKPath:           "org-org-1",
		WrappedKeyset:     "wrapped-aead",
		WrappedHMACKeyset: "wrapped-mac",
	}

	aead1, mac1, err := manager.GetPrimitives(context.Background(), "org-1")
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

	aead2, mac2, err := manager.GetPrimitives(context.Background(), "org-2")
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

// slowKeysetReader is a test double that introduces a delay to simulate slow KMS operations.
// This allows testing per-org mutex deduplication by ensuring concurrent requests overlap.
type slowKeysetReader struct {
	mu         sync.Mutex
	keyset     *mmodel.OrganizationKeyset
	err        error
	calls      int32 // Use atomic for concurrent access
	delay      time.Duration
	fetchStart chan struct{} // Signals when fetch starts
}

func (f *slowKeysetReader) Get(_ context.Context, _ string) (*mmodel.OrganizationKeyset, error) {
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

func (f *slowKeysetReader) getCalls() int {
	return int(atomic.LoadInt32(&f.calls))
}

func TestKeysetManager_GetPrimitives_PerOrgMutex_DeduplicatesConcurrentFetches(t *testing.T) {
	t.Parallel()

	aeadBytes, macBytes := generateTestKeysets(t)

	// Use a slow reader to ensure concurrent requests overlap
	reader := &slowKeysetReader{
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

	manager := NewKeysetManager(reader, unwrapper, DefaultKeysetManagerConfig())

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

			aead, mac, err := manager.GetPrimitives(context.Background(), "org-dedup")
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
	reader := &fakeKeysetReader{
		keyset: nil,
		err:    nil,
	}

	unwrapper := &fakeKeysetUnwrapper{}

	manager := NewKeysetManager(reader, unwrapper, DefaultKeysetManagerConfig())

	_, _, err := manager.GetPrimitives(context.Background(), "org-nil-keyset")
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
