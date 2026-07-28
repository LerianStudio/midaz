// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package readrouting

import (
	"context"
	"testing"
)

func TestPrimaryReadIntent(t *testing.T) {
	tests := []struct {
		name string
		ctx  func() context.Context
		want bool
	}{
		{
			name: "marked context returns true",
			ctx:  func() context.Context { return WithPrimaryRead(context.Background()) },
			want: true,
		},
		{
			name: "background context returns false",
			ctx:  func() context.Context { return context.Background() },
			want: false,
		},
		{
			name: "todo context returns false",
			ctx:  func() context.Context { return context.TODO() },
			want: false,
		},
		{
			name: "child of marked context stays true",
			ctx: func() context.Context {
				return context.WithValue(WithPrimaryRead(context.Background()), struct{}{}, "x")
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsPrimaryRead(tt.ctx()); got != tt.want {
				t.Fatalf("IsPrimaryRead() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestPrimaryReadIntent_ChildDoesNotLeakToParent verifies that marking a child
// context does not mutate the unmarked parent (isolation).
func TestPrimaryReadIntent_ChildDoesNotLeakToParent(t *testing.T) {
	parent := context.Background()
	child := WithPrimaryRead(parent)

	if !IsPrimaryRead(child) {
		t.Fatal("expected child context to be marked")
	}
	if IsPrimaryRead(parent) {
		t.Fatal("parent context must not inherit the child's mark")
	}
}

//nolint:staticcheck // intentionally passing a nil context to assert no panic
func TestPrimaryReadIntent_NilContextSafe(t *testing.T) {
	var ctx context.Context
	if IsPrimaryRead(ctx) {
		t.Fatal("nil context must return false")
	}
}
