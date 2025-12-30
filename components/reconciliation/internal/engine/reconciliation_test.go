package engine

import (
	"sync"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/reconciliation/internal/domain"
	"github.com/stretchr/testify/assert"
)

// mockLogger implements the Logger interface for testing
type mockLogger struct {
	mu       sync.Mutex
	infos    []string
	errors   []string
	warnings []string
}

func newMockLogger() *mockLogger {
	return &mockLogger{
		infos:    make([]string, 0),
		errors:   make([]string, 0),
		warnings: make([]string, 0),
	}
}

func (l *mockLogger) Info(args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.infos = append(l.infos, "info")
}

func (l *mockLogger) Infof(format string, args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.infos = append(l.infos, format)
}

func (l *mockLogger) Error(args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.errors = append(l.errors, "error")
}

func (l *mockLogger) Errorf(format string, args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.errors = append(l.errors, format)
}

func (l *mockLogger) Warnf(format string, args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.warnings = append(l.warnings, format)
}

func TestReconciliationEngine_GetLastReport_Nil(t *testing.T) {
	t.Parallel()

	engine := &ReconciliationEngine{
		logger: newMockLogger(),
	}

	report := engine.GetLastReport()
	assert.Nil(t, report)
}

func TestReconciliationEngine_GetLastReport_NotNil(t *testing.T) {
	t.Parallel()

	engine := &ReconciliationEngine{
		logger: newMockLogger(),
		lastReport: &domain.ReconciliationReport{
			Status: domain.StatusHealthy,
		},
	}

	report := engine.GetLastReport()
	assert.NotNil(t, report)
	assert.Equal(t, domain.StatusHealthy, report.Status)
}

func TestReconciliationEngine_IsHealthy_NoReport(t *testing.T) {
	t.Parallel()

	engine := &ReconciliationEngine{
		logger: newMockLogger(),
	}

	assert.False(t, engine.IsHealthy())
}

func TestReconciliationEngine_IsHealthy_Healthy(t *testing.T) {
	t.Parallel()

	engine := &ReconciliationEngine{
		logger: newMockLogger(),
		lastReport: &domain.ReconciliationReport{
			Status: domain.StatusHealthy,
		},
	}

	assert.True(t, engine.IsHealthy())
}

func TestReconciliationEngine_IsHealthy_Warning(t *testing.T) {
	t.Parallel()

	engine := &ReconciliationEngine{
		logger: newMockLogger(),
		lastReport: &domain.ReconciliationReport{
			Status: domain.StatusWarning,
		},
	}

	// WARNING is not CRITICAL, so considered healthy
	assert.True(t, engine.IsHealthy())
}

func TestReconciliationEngine_IsHealthy_Critical(t *testing.T) {
	t.Parallel()

	engine := &ReconciliationEngine{
		logger: newMockLogger(),
		lastReport: &domain.ReconciliationReport{
			Status: domain.StatusCritical,
		},
	}

	assert.False(t, engine.IsHealthy())
}

func TestReconciliationEngine_GetLastReport_ThreadSafe(t *testing.T) {
	t.Parallel()

	engine := &ReconciliationEngine{
		logger: newMockLogger(),
		lastReport: &domain.ReconciliationReport{
			Status: domain.StatusHealthy,
		},
	}

	// Run multiple concurrent reads
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			report := engine.GetLastReport()
			assert.NotNil(t, report)
		}()
	}
	wg.Wait()
}

func TestReconciliationEngine_IsHealthy_ThreadSafe(t *testing.T) {
	t.Parallel()

	engine := &ReconciliationEngine{
		logger: newMockLogger(),
		lastReport: &domain.ReconciliationReport{
			Status: domain.StatusHealthy,
		},
	}

	// Run multiple concurrent reads
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = engine.IsHealthy()
		}()
	}
	wg.Wait()
}

func TestReconciliationEngine_LastReport_ConcurrentReadWrite(t *testing.T) {
	t.Parallel()

	engine := &ReconciliationEngine{
		logger: newMockLogger(),
		// Intentionally start with nil lastReport: readers may observe nil or non-nil depending on timing.
	}

	const (
		readerGoroutines = 128
		readerIterations = 2000
		writerIterations = 20000
	)

	startCh := make(chan struct{})
	panicCh := make(chan any, readerGoroutines+1)

	var readyWG sync.WaitGroup
	readyWG.Add(readerGoroutines + 1) // readers + writer

	var readersWG sync.WaitGroup
	readersWG.Add(readerGoroutines)

	// Writer: repeatedly assigns engine.lastReport while readers are concurrently calling getters.
	go func() {
		defer func() {
			if r := recover(); r != nil {
				panicCh <- r
			}
		}()

		readyWG.Done()
		<-startCh

		for i := 0; i < writerIterations; i++ {
			engine.mu.Lock()
			engine.lastReport = &domain.ReconciliationReport{
				Status: domain.StatusHealthy,
			}
			engine.mu.Unlock()
		}
	}()

	// Readers: concurrently call GetLastReport and IsHealthy in tight loops.
	for i := 0; i < readerGoroutines; i++ {
		go func() {
			defer func() {
				if r := recover(); r != nil {
					panicCh <- r
				}
				readersWG.Done()
			}()

			readyWG.Done()
			<-startCh

			for j := 0; j < readerIterations; j++ {
				_ = engine.IsHealthy()
				_ = engine.GetLastReport() // may be nil or non-nil; both are acceptable
			}
		}()
	}

	// Coordinate a simultaneous start, then wait for all readers to finish.
	readyWG.Wait()
	close(startCh)
	readersWG.Wait()
	close(panicCh)

	for p := range panicCh {
		t.Fatalf("unexpected panic in concurrent reader/writer goroutine: %v", p)
	}
}
