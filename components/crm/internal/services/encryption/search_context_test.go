// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package encryption

import "testing"

func TestSearchTokenContext_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		ctx         SearchTokenContext
		wantErr     bool
		errContains string
	}{
		{
			name: "valid context",
			ctx: SearchTokenContext{
				TenantID:       "tenant-123",
				OrganizationID: "org-456",
				FieldName:      "tax_id",
			},
			wantErr: false,
		},
		{
			name: "missing tenant_id",
			ctx: SearchTokenContext{
				TenantID:       "",
				OrganizationID: "org-456",
				FieldName:      "tax_id",
			},
			wantErr:     true,
			errContains: "tenant_id is required",
		},
		{
			name: "missing organization_id",
			ctx: SearchTokenContext{
				TenantID:       "tenant-123",
				OrganizationID: "",
				FieldName:      "tax_id",
			},
			wantErr:     true,
			errContains: "organization_id is required",
		},
		{
			name: "missing field_name",
			ctx: SearchTokenContext{
				TenantID:       "tenant-123",
				OrganizationID: "org-456",
				FieldName:      "",
			},
			wantErr:     true,
			errContains: "field_name is required",
		},
		{
			name: "whitespace only tenant_id",
			ctx: SearchTokenContext{
				TenantID:       "   ",
				OrganizationID: "org-456",
				FieldName:      "tax_id",
			},
			wantErr:     true,
			errContains: "tenant_id is required",
		},
		{
			name: "whitespace only organization_id",
			ctx: SearchTokenContext{
				TenantID:       "tenant-123",
				OrganizationID: "   ",
				FieldName:      "tax_id",
			},
			wantErr:     true,
			errContains: "organization_id is required",
		},
		{
			name: "whitespace only field_name",
			ctx: SearchTokenContext{
				TenantID:       "tenant-123",
				OrganizationID: "org-456",
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

func TestSearchTokenContext_CanonicalInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		ctx             SearchTokenContext
		normalizedValue string
		expected        string
	}{
		{
			name: "standard context with simple value",
			ctx: SearchTokenContext{
				TenantID:       "tenant-123",
				OrganizationID: "org-456",
				FieldName:      "tax_id",
			},
			normalizedValue: "123456789",
			expected:        "tenant:tenant-123:org:org-456:field:tax_id:123456789",
		},
		{
			name: "different field name",
			ctx: SearchTokenContext{
				TenantID:       "tenant-abc",
				OrganizationID: "org-def",
				FieldName:      "document_number",
			},
			normalizedValue: "ABC-12345",
			expected:        "tenant:tenant-abc:org:org-def:field:document_number:ABC-12345",
		},
		{
			name: "uuid style identifiers",
			ctx: SearchTokenContext{
				TenantID:       "550e8400-e29b-41d4-a716-446655440000",
				OrganizationID: "6ba7b810-9dad-11d1-80b4-00c04fd430c8",
				FieldName:      "social_security_number",
			},
			normalizedValue: "123-45-6789",
			expected:        "tenant:550e8400-e29b-41d4-a716-446655440000:org:6ba7b810-9dad-11d1-80b4-00c04fd430c8:field:social_security_number:123-45-6789",
		},
		{
			name: "empty normalized value",
			ctx: SearchTokenContext{
				TenantID:       "tenant-123",
				OrganizationID: "org-456",
				FieldName:      "tax_id",
			},
			normalizedValue: "",
			expected:        "tenant:tenant-123:org:org-456:field:tax_id:",
		},
		{
			name: "value with special characters",
			ctx: SearchTokenContext{
				TenantID:       "tenant-123",
				OrganizationID: "org-456",
				FieldName:      "email",
			},
			normalizedValue: "user@example.com",
			expected:        "tenant:tenant-123:org:org-456:field:email:user@example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.ctx.CanonicalInput(tt.normalizedValue)
			gotStr := string(got)

			if gotStr != tt.expected {
				t.Errorf("CanonicalInput(%q) = %q, want %q", tt.normalizedValue, gotStr, tt.expected)
			}
		})
	}
}

func TestSearchTokenContext_CanonicalInput_Determinism(t *testing.T) {
	t.Parallel()

	ctx := SearchTokenContext{
		TenantID:       "tenant-123",
		OrganizationID: "org-456",
		FieldName:      "tax_id",
	}

	normalizedValue := "123456789"

	// Call multiple times to verify deterministic output.
	first := ctx.CanonicalInput(normalizedValue)
	second := ctx.CanonicalInput(normalizedValue)
	third := ctx.CanonicalInput(normalizedValue)

	if string(first) != string(second) || string(second) != string(third) {
		t.Errorf("CanonicalInput() is not deterministic: %q != %q != %q",
			string(first), string(second), string(third))
	}
}

func TestSearchTokenContext_CanonicalInput_DifferentValues(t *testing.T) {
	t.Parallel()

	ctx := SearchTokenContext{
		TenantID:       "tenant-123",
		OrganizationID: "org-456",
		FieldName:      "tax_id",
	}

	// Same context, different values should produce different outputs.
	value1 := ctx.CanonicalInput("123456789")
	value2 := ctx.CanonicalInput("987654321")

	if string(value1) == string(value2) {
		t.Errorf("CanonicalInput() should produce different output for different values: %q == %q",
			string(value1), string(value2))
	}
}
