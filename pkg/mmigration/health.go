package mmigration

// HealthStatus represents minimal migration health in health check responses.
// Intentionally minimal to avoid exposing internal architecture details.
type HealthStatus struct {
	// Healthy indicates if migrations are in a good state.
	Healthy bool `json:"healthy"`
}

// GetHealthStatus returns the current health status for health endpoints.
// Returns a minimal response with only healthy/unhealthy status.
// Detailed status information is available via logs and metrics.
func (w *MigrationWrapper) GetHealthStatus() HealthStatus {
	w.mu.RLock()
	defer w.mu.RUnlock()

	return HealthStatus{
		Healthy: w.status.IsHealthy(),
	}
}

// IsHealthy returns true if migrations are in a healthy state.
// Convenience method for simple health checks.
func (w *MigrationWrapper) IsHealthy() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()

	return w.status.IsHealthy()
}

// HealthChecker is an interface for migration health checking.
// Components can use this to integrate with their health endpoints.
type HealthChecker interface {
	GetHealthStatus() HealthStatus
	IsHealthy() bool
}

// Ensure MigrationWrapper implements HealthChecker.
var _ HealthChecker = (*MigrationWrapper)(nil)
