//go:build property

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package property

import (
	"testing"
	"testing/quick"
	"time"

	"github.com/LerianStudio/lib-commons/v5/commons"
	"github.com/google/uuid"
)

func generateUUIDv7(tb testing.TB) uuid.UUID {
	tb.Helper()

	id, err := commons.GenerateUUIDv7()
	if err != nil {
		tb.Fatalf("failed to generate UUIDv7: %v", err)
	}

	return id
}

func uuidV7UnixMillis(id uuid.UUID) uint64 {
	return uint64(id[0])<<40 |
		uint64(id[1])<<32 |
		uint64(id[2])<<24 |
		uint64(id[3])<<16 |
		uint64(id[4])<<8 |
		uint64(id[5])
}

// Property 1: UUIDs v7 devem ser monotonicamente crescentes (ordenados por tempo)
func TestProperty_UUID_MonotonicallyIncreasing(t *testing.T) {
	t.Parallel()

	property := func(iterations uint8) bool {
		if iterations == 0 || iterations > 50 {
			return true
		}

		var previousUUID uuid.UUID

		for i := uint8(0); i < iterations; i++ {
			currentUUID := generateUUIDv7(t)

			// First iteration
			if i == 0 {
				previousUUID = currentUUID
				continue
			}

			// UUIDs should be in ascending order (time-based)
			// Compare as strings (lexicographic order matches time order for UUIDv7)
			if currentUUID.String() <= previousUUID.String() {
				// Allow same UUID in rare cases (same timestamp)
				if currentUUID.String() != previousUUID.String() {
					t.Logf("UUID not monotonically increasing: %s <= %s", currentUUID, previousUUID)
					return false
				}
			}

			previousUUID = currentUUID

			// Small delay to ensure different timestamps
			time.Sleep(1 * time.Microsecond)
		}

		return true
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 20}); err != nil {
		t.Errorf("Property violated: UUIDs not monotonically increasing: %v", err)
	}
}

// Property 2: UUIDs devem ser únicos
func TestProperty_UUID_Uniqueness(t *testing.T) {
	t.Parallel()

	property := func(count uint8) bool {
		if count == 0 || count > 100 {
			return true
		}

		seen := make(map[string]bool)

		for i := uint8(0); i < count; i++ {
			id := generateUUIDv7(t)
			idStr := id.String()

			if seen[idStr] {
				t.Logf("Duplicate UUID generated: %s", idStr)
				return false
			}

			seen[idStr] = true
		}

		return true
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 20}); err != nil {
		t.Errorf("Property violated: UUIDs not unique: %v", err)
	}
}

// Property 3: UUIDs devem ser parseáveis
func TestProperty_UUID_Parseable(t *testing.T) {
	t.Parallel()

	property := func(seed uint32) bool {
		generatedUUID := generateUUIDv7(t)
		uuidStr := generatedUUID.String()

		// Should be parseable back
		parsed, err := uuid.Parse(uuidStr)
		if err != nil {
			t.Logf("UUID not parseable: %s, error: %v", uuidStr, err)
			return false
		}

		// Parsed UUID should match original
		return parsed.String() == uuidStr
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 100}); err != nil {
		t.Errorf("Property violated: UUID not parseable: %v", err)
	}
}

// Property 4: UUID v7 deve ter version bits corretos
func TestProperty_UUID_VersionBits(t *testing.T) {
	t.Parallel()

	property := func(seed uint32) bool {
		generatedUUID := generateUUIDv7(t)

		// UUID v7 should have version bits set to 0111 (7)
		version := generatedUUID.Version()
		return version == 7
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 100}); err != nil {
		t.Errorf("Property violated: UUID version not 7: %v", err)
	}
}

// Property 5: UUIDs gerados em sequência devem ter timestamps crescentes
func TestProperty_UUID_TimestampIncreasing(t *testing.T) {
	t.Parallel()

	property := func(count uint8) bool {
		if count == 0 || count > 20 {
			return true
		}

		var previousTime uint64

		for i := uint8(0); i < count; i++ {
			id := generateUUIDv7(t)
			currentTime := uuidV7UnixMillis(id)

			if i > 0 && currentTime < previousTime {
				t.Logf("Timestamp not increasing: current=%d < previous=%d", currentTime, previousTime)
				return false
			}

			previousTime = currentTime

			time.Sleep(1 * time.Millisecond) // Ensure different timestamps
		}

		return true
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 20}); err != nil {
		t.Errorf("Property violated: timestamps not increasing: %v", err)
	}
}

// Property 6: UUID string format deve seguir padrão 8-4-4-4-12
func TestProperty_UUID_StringFormat(t *testing.T) {
	t.Parallel()

	property := func(seed uint32) bool {
		id := generateUUIDv7(t)
		str := id.String()

		// Format: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
		// Length should be 36 (32 hex + 4 dashes)
		if len(str) != 36 {
			return false
		}

		// Check dash positions
		if str[8] != '-' || str[13] != '-' || str[18] != '-' || str[23] != '-' {
			return false
		}

		return true
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 100}); err != nil {
		t.Errorf("Property violated: UUID string format: %v", err)
	}
}

// Property 7: NIL UUID deve ser diferente de qualquer UUID gerado
func TestProperty_UUID_NeverNil(t *testing.T) {
	t.Parallel()

	property := func(seed uint32) bool {
		id := generateUUIDv7(t)
		nilUUID := uuid.Nil

		return id != nilUUID
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 100}); err != nil {
		t.Errorf("Property violated: generated NIL UUID: %v", err)
	}
}

// Property 8: Conversion UUID → String → UUID deve ser lossless
func TestProperty_UUID_StringRoundTrip(t *testing.T) {
	t.Parallel()

	property := func(seed uint32) bool {
		original := generateUUIDv7(t)
		str := original.String()

		parsed, err := uuid.Parse(str)
		if err != nil {
			return false
		}

		return parsed == original
	}

	if err := quick.Check(property, &quick.Config{MaxCount: 100}); err != nil {
		t.Errorf("Property violated: UUID string round-trip: %v", err)
	}
}

// Benchmark: Performance de geração de UUID
func BenchmarkUUIDGeneration(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = generateUUIDv7(b)
	}
}

// Benchmark: Performance de parsing de UUID
func BenchmarkUUIDParsing(b *testing.B) {
	id := generateUUIDv7(b)
	str := id.String()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = uuid.Parse(str)
	}
}
