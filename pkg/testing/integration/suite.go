package integration

import (
	"context"
	"testing"
	"time"
	
	"github.com/LerianStudio/midaz/pkg/testing/testutil"
)

// TestSuite provides common setup for integration tests
type TestSuite struct {
	T        *testing.T
	DB       *testutil.TestDB
	Timeout  time.Duration
	Context  context.Context
	Cancel   context.CancelFunc
}

// NewTestSuite creates a new integration test suite
func NewTestSuite(t *testing.T) *TestSuite {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	
	return &TestSuite{
		T:       t,
		Timeout: 30 * time.Second,
		Context: ctx,
		Cancel:  cancel,
	}
}

// SetupDatabase initializes the test database
func (s *TestSuite) SetupDatabase() {
	s.DB = testutil.SetupTestDB(s.T)
	
	if err := s.DB.RunMigrations(""); err != nil {
		s.T.Fatalf("Failed to run migrations: %v", err)
	}
}

// Cleanup tears down the test suite
func (s *TestSuite) Cleanup() {
	if s.Cancel != nil {
		s.Cancel()
	}
	
	if s.DB != nil {
		s.DB.Cleanup()
	}
}

// RunParallel executes test functions in parallel
func (s *TestSuite) RunParallel(name string, fn func(*testing.T)) {
	s.T.Run(name, func(t *testing.T) {
		t.Parallel()
		fn(t)
	})
}

// WaitForCondition polls until a condition is met or timeout
func (s *TestSuite) WaitForCondition(condition func() bool, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			if condition() {
				return true
			}
			if time.Now().After(deadline) {
				return false
			}
		case <-s.Context.Done():
			return false
		}
	}
}