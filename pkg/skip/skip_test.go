// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package skip

import (
	"errors"
	"strings"
	"testing"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

func TestResolveSkipForTruthTable(t *testing.T) {
	tests := []struct {
		name        string
		requested   bool
		allowed     bool
		wantHonored bool
		wantErr     bool
	}{
		{name: "not requested, not allowed", requested: false, allowed: false, wantHonored: false, wantErr: false},
		{name: "not requested, allowed", requested: false, allowed: true, wantHonored: false, wantErr: false},
		{name: "requested, not allowed", requested: true, allowed: false, wantHonored: false, wantErr: true},
		{name: "requested, allowed", requested: true, allowed: true, wantHonored: true, wantErr: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			honored, err := ResolveSkipFor("tracer", tt.requested, tt.allowed)

			if honored != tt.wantHonored {
				t.Fatalf("ResolveSkipFor honored = %v, want %v", honored, tt.wantHonored)
			}

			if tt.wantErr {
				if err == nil {
					t.Fatalf("ResolveSkipFor err = nil, want a business error")
				}
			} else if err != nil {
				t.Fatalf("ResolveSkipFor err = %v, want nil", err)
			}
		})
	}
}

func TestResolveSkipForUnauthorizedIs422(t *testing.T) {
	honored, err := ResolveSkipFor("tracer", true, false)

	if honored {
		t.Fatalf("ResolveSkipFor honored = true, want false for unauthorized skip")
	}

	if err == nil {
		t.Fatal("ResolveSkipFor err = nil, want UnprocessableOperationError")
	}

	var unprocessable pkg.UnprocessableOperationError
	if !errors.As(err, &unprocessable) {
		t.Fatalf("ResolveSkipFor err = %T, want pkg.UnprocessableOperationError (HTTP 422)", err)
	}

	if unprocessable.Code != constant.ErrSkipNotPermitted.Error() {
		t.Fatalf("UnprocessableOperationError.Code = %q, want %q", unprocessable.Code, constant.ErrSkipNotPermitted.Error())
	}
}

func TestResolveSkipForControlLabelInMessage(t *testing.T) {
	_, err := ResolveSkipFor("fees", true, false)
	if err == nil {
		t.Fatal("ResolveSkipFor err = nil, want UnprocessableOperationError")
	}

	if !strings.Contains(err.Error(), "fees") {
		t.Fatalf("error message %q does not mention the control label %q", err.Error(), "fees")
	}
}
