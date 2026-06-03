// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pkg

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSafeDataSources_NewSafeDataSources(t *testing.T) {
	t.Parallel()

	initial := map[string]DataSource{
		"ds1": {DatabaseType: PostgreSQLType, Initialized: true},
		"ds2": {DatabaseType: MongoDBType, Initialized: false},
	}

	sds := NewSafeDataSources(initial)
	require.NotNil(t, sds)

	ds, exists := sds.Get("ds1")
	assert.True(t, exists)
	assert.Equal(t, PostgreSQLType, ds.DatabaseType)
	assert.True(t, ds.Initialized)
}

func TestSafeDataSources_Get_NotFound(t *testing.T) {
	t.Parallel()

	sds := NewSafeDataSources(map[string]DataSource{})

	ds, exists := sds.Get("nonexistent")
	assert.False(t, exists)
	assert.Equal(t, DataSource{}, ds)
}

func TestSafeDataSources_Set(t *testing.T) {
	t.Parallel()

	sds := NewSafeDataSources(map[string]DataSource{})

	ds := DataSource{
		DatabaseType: PostgreSQLType,
		Initialized:  true,
		Status:       "available",
	}
	sds.Set("ds1", ds)

	got, exists := sds.Get("ds1")
	assert.True(t, exists)
	assert.Equal(t, PostgreSQLType, got.DatabaseType)
	assert.True(t, got.Initialized)
}

func TestSafeDataSources_GetAll_ReturnsShallowCopy(t *testing.T) {
	t.Parallel()

	initial := map[string]DataSource{
		"ds1": {DatabaseType: PostgreSQLType},
		"ds2": {DatabaseType: MongoDBType},
	}

	sds := NewSafeDataSources(initial)

	snapshot := sds.GetAll()
	assert.Len(t, snapshot, 2)

	// Modifying the snapshot should NOT affect the internal map
	snapshot["ds3"] = DataSource{DatabaseType: "fake"}
	_, exists := sds.Get("ds3")
	assert.False(t, exists, "modifying snapshot must not affect SafeDataSources internal map")
}

func TestSafeDataSources_Len(t *testing.T) {
	t.Parallel()

	sds := NewSafeDataSources(map[string]DataSource{
		"ds1": {},
		"ds2": {},
	})

	assert.Equal(t, 2, sds.Len())
}

func TestSafeDataSources_ConcurrentAccess_NoPanic(t *testing.T) {
	t.Parallel()

	sds := NewSafeDataSources(map[string]DataSource{
		"ds1": {DatabaseType: PostgreSQLType, Initialized: true},
		"ds2": {DatabaseType: MongoDBType, Initialized: false},
	})

	const goroutines = 100
	const iterations = 1000

	var wg sync.WaitGroup
	wg.Add(goroutines * 3) // readers, writers, iterators

	// Concurrent readers
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				_, _ = sds.Get("ds1")
				_, _ = sds.Get("ds2")
				_, _ = sds.Get("nonexistent")
			}
		}()
	}

	// Concurrent writers
	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				sds.Set("ds1", DataSource{
					DatabaseType: PostgreSQLType,
					Initialized:  j%2 == 0,
					Status:       "available",
				})
			}
		}(i)
	}

	// Concurrent iterators (GetAll)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				_ = sds.GetAll()
			}
		}()
	}

	// If this completes without panic or data race, the test passes.
	wg.Wait()
}

func TestSafeDataSources_ConcurrentReadWrite_RaceDetector(t *testing.T) {
	// This test is specifically designed to trigger the race detector (-race flag).
	// If ExternalDataSources is a plain map, this WILL fail under -race.
	t.Parallel()

	sds := NewSafeDataSources(map[string]DataSource{
		"ds1": {DatabaseType: PostgreSQLType},
	})

	const goroutines = 50

	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

	// Half goroutines read
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < 500; j++ {
				ds, _ := sds.Get("ds1")
				_ = ds.DatabaseType
			}
		}()
	}

	// Half goroutines write
	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 500; j++ {
				sds.Set("ds1", DataSource{
					DatabaseType: MongoDBType,
					Initialized:  true,
				})
			}
		}(i)
	}

	wg.Wait()
}

func TestSafeDataSources_NilReceiver(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		fn   func(t *testing.T)
	}{
		{
			name: "Get on nil receiver returns zero value and false",
			fn: func(t *testing.T) {
				t.Parallel()

				var sds *SafeDataSources

				ds, ok := sds.Get("any")
				assert.False(t, ok)
				assert.Equal(t, DataSource{}, ds)
			},
		},
		{
			name: "Set on nil receiver is no-op",
			fn: func(t *testing.T) {
				t.Parallel()

				var sds *SafeDataSources

				// Should not panic
				assert.NotPanics(t, func() {
					sds.Set("key", DataSource{DatabaseType: PostgreSQLType})
				})
			},
		},
		{
			name: "GetAll on nil receiver returns empty map",
			fn: func(t *testing.T) {
				t.Parallel()

				var sds *SafeDataSources

				result := sds.GetAll()
				assert.NotNil(t, result)
				assert.Empty(t, result)
			},
		},
		{
			name: "Len on nil receiver returns 0",
			fn: func(t *testing.T) {
				t.Parallel()

				var sds *SafeDataSources

				assert.Equal(t, 0, sds.Len())
			},
		},
		{
			name: "ConnectDataSource on nil receiver returns error",
			fn: func(t *testing.T) {
				t.Parallel()

				var sds *SafeDataSources

				ds := &DataSource{DatabaseType: PostgreSQLType}
				err := sds.ConnectDataSource(context.Background(), "test", ds, nil)

				require.Error(t, err)
				assert.Contains(t, err.Error(), "SafeDataSources is nil")
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			tt.fn(t)
		})
	}
}

func TestSafeDataSources_NewSafeDataSources_NilInitial(t *testing.T) {
	t.Parallel()

	sds := NewSafeDataSources(nil)
	require.NotNil(t, sds)
	assert.Equal(t, 0, sds.Len())

	// Should still be usable after nil initialization
	sds.Set("ds1", DataSource{DatabaseType: PostgreSQLType})

	ds, exists := sds.Get("ds1")
	assert.True(t, exists)
	assert.Equal(t, PostgreSQLType, ds.DatabaseType)
}

func TestSafeDataSources_Set_OverwriteExisting(t *testing.T) {
	t.Parallel()

	sds := NewSafeDataSources(map[string]DataSource{
		"ds1": {DatabaseType: PostgreSQLType, Initialized: false},
	})

	// Overwrite with a new value
	sds.Set("ds1", DataSource{DatabaseType: MongoDBType, Initialized: true})

	ds, exists := sds.Get("ds1")
	assert.True(t, exists)
	assert.Equal(t, MongoDBType, ds.DatabaseType)
	assert.True(t, ds.Initialized)
}

func TestSafeDataSources_Len_AfterMutations(t *testing.T) {
	t.Parallel()

	sds := NewSafeDataSources(nil)
	assert.Equal(t, 0, sds.Len())

	sds.Set("ds1", DataSource{})
	assert.Equal(t, 1, sds.Len())

	sds.Set("ds2", DataSource{})
	assert.Equal(t, 2, sds.Len())

	// Overwrite does not increase length
	sds.Set("ds1", DataSource{Initialized: true})
	assert.Equal(t, 2, sds.Len())
}

func TestSafeDataSources_GetAll_EmptyMap(t *testing.T) {
	t.Parallel()

	sds := NewSafeDataSources(nil)

	result := sds.GetAll()
	assert.NotNil(t, result)
	assert.Empty(t, result)
}

func TestSafeDataSources_NewSafeDataSources_DoesNotModifyOriginal(t *testing.T) {
	t.Parallel()

	original := map[string]DataSource{
		"ds1": {DatabaseType: PostgreSQLType},
	}

	sds := NewSafeDataSources(original)

	// Modify through SafeDataSources
	sds.Set("ds2", DataSource{DatabaseType: MongoDBType})

	// Original map should not be affected
	_, exists := original["ds2"]
	assert.False(t, exists, "original map should not be modified by SafeDataSources.Set")
}
