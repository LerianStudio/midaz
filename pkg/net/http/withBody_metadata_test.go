package http

import (
	"errors"
	"strings"
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
)

func TestValidateStruct_MetadataValueMaxLength(t *testing.T) {
	t.Parallel()

	type payload struct {
		Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,omitempty,nonested,valuemax=2000"`
	}

	s := &payload{Metadata: map[string]any{"note": strings.Repeat("x", 2001)}}

	err := ValidateStruct(s)
	if err == nil {
		t.Fatalf("expected validation error")
	}

	var vErr pkg.ValidationError
	if !errors.As(err, &vErr) {
		t.Fatalf("expected pkg.ValidationError, got %T: %v", err, err)
	}

	if vErr.Code != constant.ErrMetadataValueLengthExceeded.Error() {
		t.Fatalf("expected code %s, got %s", constant.ErrMetadataValueLengthExceeded.Error(), vErr.Code)
	}
}

func TestValidateStruct_MetadataKeyMaxLength(t *testing.T) {
	t.Parallel()

	type payload struct {
		Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,omitempty,nonested,valuemax=2000"`
	}

	key := strings.Repeat("k", 101)
	s := &payload{Metadata: map[string]any{key: "ok"}}

	err := ValidateStruct(s)
	if err == nil {
		t.Fatalf("expected validation error")
	}

	var vErr pkg.ValidationError
	if !errors.As(err, &vErr) {
		t.Fatalf("expected pkg.ValidationError, got %T: %v", err, err)
	}

	if vErr.Code != constant.ErrMetadataKeyLengthExceeded.Error() {
		t.Fatalf("expected code %s, got %s", constant.ErrMetadataKeyLengthExceeded.Error(), vErr.Code)
	}
}
