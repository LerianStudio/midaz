// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package encryption

import (
	"testing"
)

func TestFieldContext_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		ctx         FieldContext
		wantErr     bool
		errContains string
	}{
		{
			name: "valid context",
			ctx: FieldContext{
				TenantID:       "tenant-123",
				OrganizationID: "org-456",
				RecordID:       "record-789",
				FieldName:      "tax_id",
			},
			wantErr: false,
		},
		{
			name: "missing tenant_id",
			ctx: FieldContext{
				TenantID:       "",
				OrganizationID: "org-456",
				RecordID:       "record-789",
				FieldName:      "tax_id",
			},
			wantErr:     true,
			errContains: "tenant_id is required",
		},
		{
			name: "missing organization_id",
			ctx: FieldContext{
				TenantID:       "tenant-123",
				OrganizationID: "",
				RecordID:       "record-789",
				FieldName:      "tax_id",
			},
			wantErr:     true,
			errContains: "organization_id is required",
		},
		{
			name: "missing record_id",
			ctx: FieldContext{
				TenantID:       "tenant-123",
				OrganizationID: "org-456",
				RecordID:       "",
				FieldName:      "tax_id",
			},
			wantErr:     true,
			errContains: "record_id is required",
		},
		{
			name: "missing field_name",
			ctx: FieldContext{
				TenantID:       "tenant-123",
				OrganizationID: "org-456",
				RecordID:       "record-789",
				FieldName:      "",
			},
			wantErr:     true,
			errContains: "field_name is required",
		},
		{
			name: "whitespace only tenant_id",
			ctx: FieldContext{
				TenantID:       "   ",
				OrganizationID: "org-456",
				RecordID:       "record-789",
				FieldName:      "tax_id",
			},
			wantErr:     true,
			errContains: "tenant_id is required",
		},
		{
			name: "whitespace only organization_id",
			ctx: FieldContext{
				TenantID:       "tenant-123",
				OrganizationID: "   ",
				RecordID:       "record-789",
				FieldName:      "tax_id",
			},
			wantErr:     true,
			errContains: "organization_id is required",
		},
		{
			name: "whitespace only record_id",
			ctx: FieldContext{
				TenantID:       "tenant-123",
				OrganizationID: "org-456",
				RecordID:       "   ",
				FieldName:      "tax_id",
			},
			wantErr:     true,
			errContains: "record_id is required",
		},
		{
			name: "whitespace only field_name",
			ctx: FieldContext{
				TenantID:       "tenant-123",
				OrganizationID: "org-456",
				RecordID:       "record-789",
				FieldName:      "   ",
			},
			wantErr:     true,
			errContains: "field_name is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.ctx.Validate()

			if tt.wantErr {
				if err == nil {
					t.Errorf("Validate() expected error containing %q, got nil", tt.errContains)
					return
				}

				if !containsString(err.Error(), tt.errContains) {
					t.Errorf("Validate() error = %q, want error containing %q", err.Error(), tt.errContains)
				}

				return
			}

			if err != nil {
				t.Errorf("Validate() unexpected error = %v", err)
			}
		})
	}
}

func TestFieldContext_CanonicalAAD(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		ctx      FieldContext
		expected string
	}{
		{
			name: "standard context",
			ctx: FieldContext{
				TenantID:       "tenant-123",
				OrganizationID: "org-456",
				RecordID:       "record-789",
				FieldName:      "tax_id",
			},
			expected: "tenant:tenant-123:org:org-456:record:record-789:field:tax_id",
		},
		{
			name: "different field name",
			ctx: FieldContext{
				TenantID:       "tenant-abc",
				OrganizationID: "org-def",
				RecordID:       "record-ghi",
				FieldName:      "document_number",
			},
			expected: "tenant:tenant-abc:org:org-def:record:record-ghi:field:document_number",
		},
		{
			name: "uuid style identifiers",
			ctx: FieldContext{
				TenantID:       "550e8400-e29b-41d4-a716-446655440000",
				OrganizationID: "6ba7b810-9dad-11d1-80b4-00c04fd430c8",
				RecordID:       "f47ac10b-58cc-4372-a567-0e02b2c3d479",
				FieldName:      "social_security_number",
			},
			expected: "tenant:550e8400-e29b-41d4-a716-446655440000:org:6ba7b810-9dad-11d1-80b4-00c04fd430c8:record:f47ac10b-58cc-4372-a567-0e02b2c3d479:field:social_security_number",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.ctx.CanonicalAAD()
			gotStr := string(got)

			if gotStr != tt.expected {
				t.Errorf("CanonicalAAD() = %q, want %q", gotStr, tt.expected)
			}
		})
	}
}

func TestFieldContext_CanonicalAAD_Determinism(t *testing.T) {
	t.Parallel()

	ctx := FieldContext{
		TenantID:       "tenant-123",
		OrganizationID: "org-456",
		RecordID:       "record-789",
		FieldName:      "tax_id",
	}

	// Call multiple times to verify deterministic output
	first := ctx.CanonicalAAD()
	second := ctx.CanonicalAAD()
	third := ctx.CanonicalAAD()

	if string(first) != string(second) || string(second) != string(third) {
		t.Errorf("CanonicalAAD() is not deterministic: %q != %q != %q",
			string(first), string(second), string(third))
	}
}

// containsString checks if s contains substr.
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}

	return false
}
