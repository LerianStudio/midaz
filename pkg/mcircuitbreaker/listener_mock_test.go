package mcircuitbreaker

// mockListener implements StateListener for testing
type mockListener struct {
	calls []StateChangeEvent
}

func (m *mockListener) OnCircuitBreakerStateChange(event StateChangeEvent) {
	m.calls = append(m.calls, event)
}
