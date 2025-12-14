package panicguard

import (
	"testing"
)

func TestPathMatcher_ShouldExclude(t *testing.T) {
	tests := []struct {
		name     string
		patterns []string
		path     string
		want     bool
	}{
		{
			name:     "test file suffix",
			patterns: []string{"*_test.go"},
			path:     "/home/user/project/pkg/foo/bar_test.go",
			want:     true,
		},
		{
			name:     "non-test file",
			patterns: []string{"*_test.go"},
			path:     "/home/user/project/pkg/foo/bar.go",
			want:     false,
		},
		{
			name:     "pb.go generated file",
			patterns: []string{"*.pb.go"},
			path:     "/home/user/project/pkg/grpc/service.pb.go",
			want:     true,
		},
		{
			name:     "mock file suffix",
			patterns: []string{"*_mock.go"},
			path:     "/home/user/project/pkg/repo/repo_mock.go",
			want:     true,
		},
		{
			name:     "mock file prefix",
			patterns: []string{"mock_*.go"},
			path:     "/home/user/project/pkg/repo/mock_repo.go",
			want:     true,
		},
		{
			name:     "mocks directory",
			patterns: []string{"mocks/"},
			path:     "/home/user/project/pkg/mocks/repo.go",
			want:     true,
		},
		{
			name:     "mruntime package",
			patterns: []string{"/pkg/mruntime/"},
			path:     "/home/user/project/pkg/mruntime/goroutine.go",
			want:     true,
		},
		{
			name:     "http adapter boundary",
			patterns: []string{"/internal/adapters/http/"},
			path:     "/home/user/project/components/ledger/internal/adapters/http/handler.go",
			want:     true,
		},
		{
			name:     "grpc adapter boundary",
			patterns: []string{"/internal/adapters/grpc/"},
			path:     "/home/user/project/components/ledger/internal/adapters/grpc/server.go",
			want:     true,
		},
		{
			name:     "rabbitmq adapter boundary",
			patterns: []string{"/internal/adapters/rabbitmq/"},
			path:     "/home/user/project/components/ledger/internal/adapters/rabbitmq/consumer.go",
			want:     true,
		},
		{
			name:     "bootstrap boundary",
			patterns: []string{"/internal/bootstrap/"},
			path:     "/home/user/project/components/ledger/internal/bootstrap/worker.go",
			want:     true,
		},
		{
			name:     "non-boundary package",
			patterns: BoundaryPackageExclusions,
			path:     "/home/user/project/components/ledger/internal/usecase/service.go",
			want:     false,
		},
		{
			name:     "assert package",
			patterns: []string{"/pkg/assert/"},
			path:     "/home/user/project/pkg/assert/assert.go",
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher := NewPathMatcher(tt.patterns)
			got := matcher.ShouldExclude(tt.path)
			if got != tt.want {
				t.Errorf("ShouldExclude(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}
