package mmodel

import (
	"testing"

	"github.com/LerianStudio/midaz/components/mdz/pkg/ptr"
)

// \1 performs an operation
func TestStatus_IsEmpty(t *testing.T) {
	type fields struct {
		Code        string
		Description *string
	}
	tests := []struct {
		name   string
		fields fields
		want   bool
	}{
		{
			name: "case 01",
			fields: fields{
				Code:        "",
				Description: nil,
			},
			want: true,
		},
		{
			name: "case 02",
			fields: fields{
				Code:        "1",
				Description: nil,
			},
			want: false,
		},
		{
			name: "case 03",
			fields: fields{
				Code:        "1",
				Description: ptr.StringPtr("a"),
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := Status{
				Code:        tt.fields.Code,
				Description: tt.fields.Description,
			}

			if got := s.IsEmpty(); got != tt.want {
				t.Errorf("Status.IsEmpty() = %v, want %v", got, tt.want)
			}
		})
	}
}
