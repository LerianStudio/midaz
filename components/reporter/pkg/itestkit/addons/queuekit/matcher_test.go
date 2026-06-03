package queuekit

import (
	"testing"
)

func TestMatchRoutingKey(t *testing.T) {
	msg := Message{RoutingKey: "job.completed"}

	tests := []struct {
		name     string
		matcher  Matcher
		expected bool
	}{
		{"exact match", MatchRoutingKey("job.completed"), true},
		{"no match", MatchRoutingKey("job.failed"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.matcher(msg); got != tt.expected {
				t.Errorf("got %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestMatchRoutingKeyPrefix(t *testing.T) {
	msg := Message{RoutingKey: "job.completed.success"}

	tests := []struct {
		name     string
		prefix   string
		expected bool
	}{
		{"full prefix", "job.completed", true},
		{"partial prefix", "job", true},
		{"no match", "task", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher := MatchRoutingKeyPrefix(tt.prefix)
			if got := matcher(msg); got != tt.expected {
				t.Errorf("got %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestMatchHeader(t *testing.T) {
	msg := Message{
		Headers: map[string]any{
			"x-type":    "notification",
			"x-version": 2,
		},
	}

	tests := []struct {
		name     string
		key      string
		value    any
		expected bool
	}{
		{"string match", "x-type", "notification", true},
		{"int match", "x-version", 2, true},
		{"wrong value", "x-type", "event", false},
		{"missing key", "x-missing", "value", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher := MatchHeader(tt.key, tt.value)
			if got := matcher(msg); got != tt.expected {
				t.Errorf("got %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestMatchJSONField(t *testing.T) {
	msg := Message{
		Body: []byte(`{"jobId": "123", "status": "completed", "user": {"id": 42, "name": "test"}}`),
	}

	tests := []struct {
		name     string
		path     string
		value    any
		expected bool
	}{
		{"top level string", "jobId", "123", true},
		{"top level string mismatch", "jobId", "456", false},
		{"nested field", "user.id", 42, true},
		{"nested string", "user.name", "test", true},
		{"missing field", "missing", "value", false},
		{"missing nested", "user.email", "test@test.com", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher := MatchJSONField(tt.path, tt.value)
			if got := matcher(msg); got != tt.expected {
				t.Errorf("got %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestMatchJSONFieldExists(t *testing.T) {
	msg := Message{
		Body: []byte(`{"jobId": "123", "user": {"id": 42}}`),
	}

	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{"top level exists", "jobId", true},
		{"nested exists", "user.id", true},
		{"top level missing", "status", false},
		{"nested missing", "user.email", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher := MatchJSONFieldExists(tt.path)
			if got := matcher(msg); got != tt.expected {
				t.Errorf("got %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestMatchAll(t *testing.T) {
	msg := Message{
		RoutingKey: "job.completed",
		Body:       []byte(`{"jobId": "123"}`),
	}

	tests := []struct {
		name     string
		matchers []Matcher
		expected bool
	}{
		{
			"all match",
			[]Matcher{
				MatchRoutingKey("job.completed"),
				MatchJSONField("jobId", "123"),
			},
			true,
		},
		{
			"one fails",
			[]Matcher{
				MatchRoutingKey("job.completed"),
				MatchJSONField("jobId", "456"),
			},
			false,
		},
		{
			"empty matchers",
			[]Matcher{},
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher := MatchAll(tt.matchers...)
			if got := matcher(msg); got != tt.expected {
				t.Errorf("got %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestMatchAny(t *testing.T) {
	msg := Message{
		RoutingKey: "job.completed",
	}

	tests := []struct {
		name     string
		matchers []Matcher
		expected bool
	}{
		{
			"first matches",
			[]Matcher{
				MatchRoutingKey("job.completed"),
				MatchRoutingKey("job.failed"),
			},
			true,
		},
		{
			"second matches",
			[]Matcher{
				MatchRoutingKey("job.failed"),
				MatchRoutingKey("job.completed"),
			},
			true,
		},
		{
			"none match",
			[]Matcher{
				MatchRoutingKey("job.failed"),
				MatchRoutingKey("job.pending"),
			},
			false,
		},
		{
			"empty matchers",
			[]Matcher{},
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher := MatchAny(tt.matchers...)
			if got := matcher(msg); got != tt.expected {
				t.Errorf("got %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestMatchNone(t *testing.T) {
	msg := Message{RoutingKey: "job.completed"}

	tests := []struct {
		name     string
		matcher  Matcher
		expected bool
	}{
		{"inverts true to false", MatchRoutingKey("job.completed"), false},
		{"inverts false to true", MatchRoutingKey("job.failed"), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher := MatchNone(tt.matcher)
			if got := matcher(msg); got != tt.expected {
				t.Errorf("got %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestMatchBodyContains(t *testing.T) {
	msg := Message{Body: []byte(`{"status": "completed", "error": null}`)}

	tests := []struct {
		name     string
		substr   string
		expected bool
	}{
		{"contains", "completed", true},
		{"not contains", "failed", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher := MatchBodyContains(tt.substr)
			if got := matcher(msg); got != tt.expected {
				t.Errorf("got %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestMatchBodyPattern(t *testing.T) {
	msg := Message{Body: []byte(`{"jobId": "job-123-abc"}`)}

	tests := []struct {
		name     string
		pattern  string
		expected bool
	}{
		{"matches pattern", `job-\d+-[a-z]+`, true},
		{"no match", `task-\d+`, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher := MatchBodyPattern(tt.pattern)
			if got := matcher(msg); got != tt.expected {
				t.Errorf("got %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestMatchCorrelationID(t *testing.T) {
	msg := Message{CorrelationID: "corr-123"}

	tests := []struct {
		name     string
		id       string
		expected bool
	}{
		{"matches", "corr-123", true},
		{"no match", "corr-456", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher := MatchCorrelationID(tt.id)
			if got := matcher(msg); got != tt.expected {
				t.Errorf("got %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestCompareValues(t *testing.T) {
	tests := []struct {
		name     string
		actual   any
		expected any
		want     bool
	}{
		{"nil equal", nil, nil, true},
		{"nil vs value", nil, "value", false},
		{"value vs nil", "value", nil, false},
		{"string equal", "test", "test", true},
		{"string not equal", "test", "other", false},
		{"int to float64", float64(42), 42, true},
		{"int64 to float64", float64(42), int64(42), true},
		{"float64 equal", float64(3.14), float64(3.14), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := compareValues(tt.actual, tt.expected); got != tt.want {
				t.Errorf("compareValues(%v, %v) = %v, want %v", tt.actual, tt.expected, got, tt.want)
			}
		})
	}
}
